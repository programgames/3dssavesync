package main

// Tray icon on Windows: Shell_NotifyIconW, a hidden window for its messages, TrackPopupMenu.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var trayAvailable = true

// trayMutexName names the tray icon mutex: one per settings folder (portable version, tests), for the Windows session.
func trayMutexName(dir string) *uint16 {
	if a, err := filepath.Abs(dir); err == nil {
		dir = a
	}
	h := sha256.Sum256([]byte(strings.ToLower(dir)))
	return u16(`Local\3dssavesync-tray-` + hex.EncodeToString(h[:8]))
}

// trayLock returns nil if a tray icon already runs for this folder; the returned function releases the lock. The
// system also releases it when the program ends.
func trayLock(dir string) func() {
	h, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(trayMutexName(dir))))
	if h == 0 { // no mutex: the tray icon starts anyway, better than none
		return func() {}
	}
	if err == syscall.ERROR_ALREADY_EXISTS {
		syscall.CloseHandle(syscall.Handle(h))
		return nil
	}
	return func() { syscall.CloseHandle(syscall.Handle(h)) }
}

func trayActive(dir string) bool {
	h, _, _ := kernel32.NewProc("OpenMutexW").Call(0x00100000, 0, uintptr(unsafe.Pointer(trayMutexName(dir)))) // SYNCHRONIZE
	if h != 0 {
		syscall.CloseHandle(syscall.Handle(h))
	}
	return h != 0
}

const wmIcon = 0x8000 + 1 // WM_APP + 1: clicks on the tray icon

var (
	trayMenu      func() []MenuItem
	trayChoose    func(string)
	trayData      notifyIconData
	trayRecreated uintptr // TaskbarCreated message: Explorer restarted, the tray icon must be added again
)

var trayCallback = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch {
	case msg == wmIcon && lparam&0xFFFF == 0x203: // WM_LBUTTONDBLCLK
		go trayChoose("reglages")
		return 0
	case msg == wmIcon && lparam&0xFFFF == 0x205: // WM_RBUTTONUP
		iconMenu(hwnd)
		return 0
	case msg == trayRecreated && trayRecreated != 0:
		shell32.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&trayData))) // NIM_ADD
		return 0
	case msg == 0x2: // WM_DESTROY
		user32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	r, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, msg, wparam, lparam)
	return r
})

// iconMenu shows the right-click menu, built each time from the state. The choice runs in a goroutine: the window
// stays served.
func iconMenu(hwnd uintptr) {
	l := trayMenu()
	m, _, _ := user32.NewProc("CreatePopupMenu").Call()
	defer user32.NewProc("DestroyMenu").Call(m)
	for i, e := range l {
		var flags uintptr // MF_STRING
		switch {
		case e.Text == "":
			flags = 0x800 // MF_SEPARATOR
		case e.Action == "":
			flags = 0x1 // MF_GRAYED
		}
		user32.NewProc("AppendMenuW").Call(m, flags, uintptr(i+1), uintptr(unsafe.Pointer(u16(e.Text))))
	}
	var p struct{ X, Y int32 }
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	user32.NewProc("SetForegroundWindow").Call(hwnd) // without it, the menu stays open after a click elsewhere
	// TPM_RETURNCMD|TPM_NONOTIFY|TPM_RIGHTBUTTON: returns the chosen id. Then WM_NULL, so the menu closes correctly.
	id, _, _ := user32.NewProc("TrackPopupMenu").Call(m, 0x100|0x80|0x2, uintptr(p.X), uintptr(p.Y), 0, hwnd, 0)
	user32.NewProc("PostMessageW").Call(hwnd, 0, 0, 0)
	if id > 0 && int(id) <= len(l) {
		go trayChoose(l[id-1].Action)
	}
}

// WNDCLASSEXW (80 bytes on 64-bit).
type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSm                             uintptr
}

// showIcon shows the tray icon and runs its message loop, until end is closed.
func showIcon(menu func() []MenuItem, choose func(string), end <-chan struct{}) error {
	runtime.LockOSThread() // the window and its messages belong to this thread
	defer runtime.UnlockOSThread()
	trayMenu, trayChoose = menu, choose
	fail := func(what string, err error) error {
		return fmt.Errorf("%s\n%s%s : %v", tr("Impossible d'afficher l'icône près de l'horloge. Ouvrez les réglages avec le raccourci « 3dssavesync réglages ».",
			"Can't show the icon near the clock. Open the settings with the \"3dssavesync settings\" shortcut."), tr("Détail : ", "Detail: "), what, err)
	}
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	class := u16("3dssavesync-tray")
	wc := wndClassEx{WndProc: trayCallback, Instance: instance, ClassName: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fail("RegisterClassExW", err)
	}
	// Window never shown, but not "message-only": such a window would not get TaskbarCreated.
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return fail("CreateWindowExW", err)
	}
	trayRecreated, _, _ = user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	size, _, _ := user32.NewProc("GetSystemMetrics").Call(49) // SM_CXSMICON
	var icon uintptr
	if img := imageIco(iconICO, int(size)); len(img) > 0 {
		icon, _, _ = user32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&img[0])), uintptr(len(img)), 1, 0x00030000, size, size, 0)
	}
	if icon == 0 {
		icon, _, _ = user32.NewProc("LoadIconW").Call(0, 32512) // IDI_APPLICATION
	}
	trayData = notifyIconData{HWnd: hwnd, UID: 1, UFlags: 0x1 | 0x2 | 0x4, UCallbackMessage: wmIcon, HIcon: icon} // NIF_MESSAGE|NIF_ICON|NIF_TIP
	trayData.CbSize = uint32(unsafe.Sizeof(trayData))
	copy(trayData.SzTip[:127], syscall.StringToUTF16("3dssavesync"))
	notif := shell32.NewProc("Shell_NotifyIconW")
	// At logon, the taskbar is not always ready: try again for one minute.
	for i := 0; ; i++ {
		r, _, err := notif.Call(0, uintptr(unsafe.Pointer(&trayData))) // NIM_ADD
		if r != 0 {
			break
		}
		if i == 30 {
			user32.NewProc("DestroyWindow").Call(hwnd)
			return fail("Shell_NotifyIconW", err)
		}
		select {
		case <-end:
			user32.NewProc("DestroyWindow").Call(hwnd)
			return nil
		case <-time.After(2 * time.Second):
		}
	}
	defer notif.Call(2, uintptr(unsafe.Pointer(&trayData))) // NIM_DELETE
	go func() {
		<-end
		user32.NewProc("PostMessageW").Call(hwnd, 0x10, 0, 0) // WM_CLOSE: DefWindowProc destroys the window, then WM_DESTROY
	}()
	var m struct {
		HWnd           uintptr
		Message        uint32
		WParam, LParam uintptr
		Time           uint32
		X, Y           int32
		Private        uint32
	}
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			return nil
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
}
