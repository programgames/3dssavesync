package main

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll") // loaded under the activation context: version 6
)

// utf16z returns s as a NUL-terminated UTF-16 string for the Windows API. A NUL inside s would end the text early, so
// it is dropped (syscall.UTF16FromString refuses it; the deprecated StringToUTF16 panicked).
func utf16z(s string) []uint16 {
	u, _ := syscall.UTF16FromString(strings.ReplaceAll(s, "\x00", ""))
	return u
}

// A Go binary has no manifest: without one, comctl32 v5 has no TaskDialog.
const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
<dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls"
 version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency>
</assembly>`

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func ptr(p *uint16) uint64 { return uint64(uintptr(unsafe.Pointer(p))) }

// Window in the foreground: the frontend is often full screen.
var dialogCallback = syscall.NewCallback(func(hwnd, msg, wparam, lparam, data uintptr) uintptr {
	if msg == 0 { // TDN_CREATED
		user32.NewProc("SetWindowPos").Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x3) // HWND_TOPMOST, SWP_NOSIZE|SWP_NOMOVE
		user32.NewProc("SetForegroundWindow").Call(hwnd)
	}
	return 0
})

// askSystem shows a TaskDialog with named buttons. Closing the window (Esc, close box) = last button,
// the safest; it is also the default button. "" if the window cannot show.
func askSystem(text string, buttons []string) string {
	runtime.LockOSThread() // the activation context applies to this thread
	defer runtime.UnlockOSThread()
	path := filepath.Join(os.TempDir(), "3dssavesync-comctl6.manifest")
	if os.WriteFile(path, []byte(manifest), 0o644) != nil {
		return ""
	}
	// ACTCTXW: cbSize, dwFlags, lpSource, wProcessorArchitecture, wLangId, then 4 pointers (56 bytes).
	var actx [56]byte
	binary.LittleEndian.PutUint32(actx[0:], 56)
	source := u16(path)
	binary.LittleEndian.PutUint64(actx[8:], ptr(source))
	h, _, _ := kernel32.NewProc("CreateActCtxW").Call(uintptr(unsafe.Pointer(&actx[0])))
	runtime.KeepAlive(source)
	if h == ^uintptr(0) {
		return ""
	}
	defer kernel32.NewProc("ReleaseActCtx").Call(h)
	var cookie uintptr
	if r, _, _ := kernel32.NewProc("ActivateActCtx").Call(h, uintptr(unsafe.Pointer(&cookie))); r == 0 {
		return ""
	}
	defer kernel32.NewProc("DeactivateActCtx").Call(0, cookie)
	proc := comctl32.NewProc("TaskDialogIndirect")
	if proc.Find() != nil {
		return ""
	}

	// First line (before an empty line) = bold title of the dialog.
	instruction, content, _ := strings.Cut(text, "\n\n")
	title, pInstr, pContent := u16("3dssavesync"), u16(instruction), u16(content)
	// TASKDIALOG_BUTTON (pack 1): int nButtonID, PCWSTR pszButtonText.
	texts := make([]*uint16, len(buttons))
	tab := make([]byte, 12*len(buttons))
	for i, b := range buttons {
		texts[i] = u16(b)
		binary.LittleEndian.PutUint32(tab[12*i:], uint32(100+i))
		binary.LittleEndian.PutUint64(tab[12*i+4:], ptr(texts[i]))
	}
	// TASKDIALOGCONFIG (pack 1, 160 bytes on 64-bit).
	var c [160]byte
	le := binary.LittleEndian
	le.PutUint32(c[0:], 160)
	le.PutUint32(c[20:], 0x8) // TDF_ALLOW_DIALOG_CANCELLATION
	le.PutUint64(c[28:], ptr(title))
	le.PutUint64(c[44:], ptr(pInstr))
	if content != "" {
		le.PutUint64(c[52:], ptr(pContent))
	}
	le.PutUint32(c[60:], uint32(len(buttons)))
	le.PutUint64(c[64:], uint64(uintptr(unsafe.Pointer(&tab[0]))))
	le.PutUint32(c[72:], uint32(100+len(buttons)-1))
	le.PutUint64(c[140:], uint64(dialogCallback))
	var chosen int32
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&c[0])), uintptr(unsafe.Pointer(&chosen)), 0, 0)
	runtime.KeepAlive(texts)
	runtime.KeepAlive(tab)
	runtime.KeepAlive([]*uint16{title, pInstr, pContent})
	if r != 0 {
		return ""
	}
	if i := int(chosen) - 100; i >= 0 && i < len(buttons) {
		return buttons[i]
	}
	return buttons[len(buttons)-1] // IDCANCEL
}

// NOTIFYICONDATAW (976 bytes on 64-bit).
type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeout         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

var activeNotifs sync.WaitGroup

// systemNotify shows a Windows notification, nothing to confirm, without delaying the game. Windows removes it with
// the icon: main waits for the notifications to end (notifySeconds each) before the program ends.
func systemNotify(text string) {
	activeNotifs.Add(1)
	go func() {
		defer activeNotifs.Done()
		runtime.LockOSThread() // the icon window belongs to this thread
		notification(text)
	}()
}

func waitNotifications() { activeNotifs.Wait() }

func notification(text string) {
	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(u16("STATIC"))),
		uintptr(unsafe.Pointer(u16("3dssavesync"))), 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		return
	}
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	icon, _, _ := user32.NewProc("LoadIconW").Call(0, 32516)                                       // IDI_INFORMATION
	n := notifyIconData{HWnd: hwnd, UID: 1, UFlags: 0x2 | 0x4 | 0x10, HIcon: icon, DwInfoFlags: 1} // NIF_ICON|NIF_TIP|NIF_INFO, NIIF_INFO
	n.CbSize = uint32(unsafe.Sizeof(n))
	copy(n.SzTip[:127], utf16z("3dssavesync"))
	copy(n.SzInfoTitle[:63], utf16z("3dssavesync"))
	info := utf16z(text)
	if len(info) > 256 { // 255 characters at most: cut with "…", never in the middle of a UTF-16 pair
		n := 254
		if info[n-1] >= 0xD800 && info[n-1] < 0xDC00 {
			n--
		}
		info = append(info[:n], '…', 0)
	}
	copy(n.SzInfo[:], info)
	notif := shell32.NewProc("Shell_NotifyIconW")
	if r, _, _ := notif.Call(0, uintptr(unsafe.Pointer(&n))); r == 0 { // NIM_ADD
		return
	}
	time.Sleep(time.Duration(notifySeconds()) * time.Second)
	notif.Call(2, uintptr(unsafe.Pointer(&n))) // NIM_DELETE
}

// notifySeconds is how long the icon (and so its notification) stays: the Windows setting "Dismiss notifications
// after this amount of time" (5 s by default) plus 1 s, at least 8 s.
func notifySeconds() uint32 {
	var s uint32
	user32.NewProc("SystemParametersInfoW").Call(0x2016, 0, uintptr(unsafe.Pointer(&s)), 0) // SPI_GETMESSAGEDURATION
	return max(s+1, 8)
}

// openConsole opens a console for the setup: the exe has none (-H windowsgui).
// Go writes and reads the console in UTF-16: accented letters work.
func openConsole() {
	kernel32.NewProc("AllocConsole").Call()
	if in, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		os.Stdin = in
	}
	if out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout, os.Stderr = out, out
	}
}

// The console window closes with the program: let the user read the end.
const cleanConsole = true

// noEcho turns off echo for input (token): removes ENABLE_ECHO_INPUT, returns the function that restores the mode.
func noEcho() func() {
	h := syscall.Handle(os.Stdin.Fd())
	var mode uint32
	if syscall.GetConsoleMode(h, &mode) != nil {
		return func() {}
	}
	kernel32.NewProc("SetConsoleMode").Call(uintptr(h), uintptr(mode&^0x4))
	return func() { kernel32.NewProc("SetConsoleMode").Call(uintptr(h), uintptr(mode)) }
}

// systemLanguage returns the Windows UI language: "fr" if the primary language is French (LANG_FRENCH = 0x0C).
func systemLanguage() string {
	id, _, _ := kernel32.NewProc("GetUserDefaultUILanguage").Call()
	if id&0x3FF == 0x0C {
		return "fr"
	}
	return "en"
}

// countProcesses returns the number of processes with this name (without .exe).
func countProcesses(name string) (n int) {
	s, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(s)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(s, &e); err == nil; err = syscall.Process32Next(s, &e) {
		if strings.EqualFold(strings.TrimSuffix(strings.ToLower(syscall.UTF16ToString(e.ExeFile[:])), ".exe"), name) {
			n++
		}
	}
	return n
}

// processAlive reports that the process pid still runs.
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(0x1000, false, uint32(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	return syscall.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}

// startDetached starts a process independent of the setup console (the exe is a GUI app: no inherited console).
func startDetached(exe string, args []string) error {
	c := exec.Command(exe, args...)
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// removableRoots returns the removable drives (SD card, USB stick): E:\, F:\…
func removableRoots() []string {
	var l []string
	mask, _, _ := kernel32.NewProc("GetLogicalDrives").Call()
	for i := 0; i < 26; i++ {
		r := string(rune('A'+i)) + `:\`
		if mask&(1<<i) == 0 {
			continue
		}
		if t, _, _ := kernel32.NewProc("GetDriveTypeW").Call(uintptr(unsafe.Pointer(u16(r)))); t == 2 { // DRIVE_REMOVABLE
			l = append(l, r)
		}
	}
	return l
}

// systemFreeSpace returns the free space for the user on the disk of dir (GetDiskFreeSpaceExW).
func systemFreeSpace(dir string) (uint64, error) {
	var free uint64
	if r, _, err := kernel32.NewProc("GetDiskFreeSpaceExW").Call(uintptr(unsafe.Pointer(u16(dir))), uintptr(unsafe.Pointer(&free)), 0, 0); r == 0 {
		return 0, err
	}
	return free, nil
}
