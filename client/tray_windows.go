package main

// Icône près de l'horloge sous Windows : Shell_NotifyIconW, une fenêtre cachée pour ses messages, TrackPopupMenu.

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

var trayDisponible = true

// Mutex de l'icône : un par dossier de réglages (version portable, tests), pour la session Windows.
func nomMutexTray(dossier string) *uint16 {
	if a, err := filepath.Abs(dossier); err == nil {
		dossier = a
	}
	h := sha256.Sum256([]byte(strings.ToLower(dossier)))
	return u16(`Local\3dssavesync-tray-` + hex.EncodeToString(h[:8]))
}

// verrouTray : nil si une icône tourne déjà pour ce dossier ; la fonction rendue libère le verrou. Le système le libère
// aussi à la fin du programme.
func verrouTray(dossier string) func() {
	h, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(nomMutexTray(dossier))))
	if h == 0 { // pas de mutex : l'icône démarre quand même, mieux qu'aucune
		return func() {}
	}
	if err == syscall.ERROR_ALREADY_EXISTS {
		syscall.CloseHandle(syscall.Handle(h))
		return nil
	}
	return func() { syscall.CloseHandle(syscall.Handle(h)) }
}

func trayActif(dossier string) bool {
	h, _, _ := kernel32.NewProc("OpenMutexW").Call(0x00100000, 0, uintptr(unsafe.Pointer(nomMutexTray(dossier)))) // SYNCHRONIZE
	if h != 0 {
		syscall.CloseHandle(syscall.Handle(h))
	}
	return h != 0
}

const wmIcone = 0x8000 + 1 // WM_APP + 1 : clics sur l'icône

var (
	trayMenu    func() []ElementMenu
	trayChoisir func(string)
	trayDonnees notifyIconData
	trayRecree  uintptr // message TaskbarCreated : Explorer a redémarré, l'icône est à remettre
)

var rappelTray = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch {
	case msg == wmIcone && lparam&0xFFFF == 0x203: // WM_LBUTTONDBLCLK
		go trayChoisir("reglages")
		return 0
	case msg == wmIcone && lparam&0xFFFF == 0x205: // WM_RBUTTONUP
		menuIcone(hwnd)
		return 0
	case msg == trayRecree && trayRecree != 0:
		shell32.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&trayDonnees))) // NIM_ADD
		return 0
	case msg == 0x2: // WM_DESTROY
		user32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	r, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, msg, wparam, lparam)
	return r
})

// Menu au clic droit, construit à chaque fois d'après l'état. Le choix part dans un goroutine : la fenêtre reste servie.
func menuIcone(hwnd uintptr) {
	l := trayMenu()
	m, _, _ := user32.NewProc("CreatePopupMenu").Call()
	defer user32.NewProc("DestroyMenu").Call(m)
	for i, e := range l {
		var drapeaux uintptr // MF_STRING
		switch {
		case e.Texte == "":
			drapeaux = 0x800 // MF_SEPARATOR
		case e.Action == "":
			drapeaux = 0x1 // MF_GRAYED
		}
		user32.NewProc("AppendMenuW").Call(m, drapeaux, uintptr(i+1), uintptr(unsafe.Pointer(u16(e.Texte))))
	}
	var p struct{ X, Y int32 }
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	user32.NewProc("SetForegroundWindow").Call(hwnd) // sans elle, le menu reste ouvert après un clic ailleurs
	// TPM_RETURNCMD|TPM_NONOTIFY|TPM_RIGHTBUTTON : rend l'id choisi. Puis WM_NULL, pour que le menu se ferme bien.
	id, _, _ := user32.NewProc("TrackPopupMenu").Call(m, 0x100|0x80|0x2, uintptr(p.X), uintptr(p.Y), 0, hwnd, 0)
	user32.NewProc("PostMessageW").Call(hwnd, 0, 0, 0)
	if id > 0 && int(id) <= len(l) {
		go trayChoisir(l[id-1].Action)
	}
}

// WNDCLASSEXW (80 octets en 64 bits).
type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSm                             uintptr
}

// afficherIcone : l'icône et sa boucle de messages, jusqu'à la fermeture de fin.
func afficherIcone(menu func() []ElementMenu, choisir func(string), fin <-chan struct{}) error {
	runtime.LockOSThread() // la fenêtre et ses messages appartiennent à ce thread
	defer runtime.UnlockOSThread()
	trayMenu, trayChoisir = menu, choisir
	echec := func(quoi string, err error) error {
		return fmt.Errorf("%s\n%s%s : %v", tr("Impossible d'afficher l'icône près de l'horloge. Ouvrez les réglages avec le raccourci « 3dssavesync réglages ».",
			"Can't show the icon near the clock. Open the settings with the \"3dssavesync settings\" shortcut."), tr("Détail : ", "Detail: "), quoi, err)
	}
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	classe := u16("3dssavesync-tray")
	wc := wndClassEx{WndProc: rappelTray, Instance: instance, ClassName: classe}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return echec("RegisterClassExW", err)
	}
	// Fenêtre jamais affichée, mais pas « message-only » : celle-ci ne recevrait pas TaskbarCreated.
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(classe)), uintptr(unsafe.Pointer(classe)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return echec("CreateWindowExW", err)
	}
	trayRecree, _, _ = user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	taille, _, _ := user32.NewProc("GetSystemMetrics").Call(49) // SM_CXSMICON
	var icone uintptr
	if img := imageIco(iconeIco, int(taille)); len(img) > 0 {
		icone, _, _ = user32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&img[0])), uintptr(len(img)), 1, 0x00030000, taille, taille, 0)
	}
	if icone == 0 {
		icone, _, _ = user32.NewProc("LoadIconW").Call(0, 32512) // IDI_APPLICATION
	}
	trayDonnees = notifyIconData{HWnd: hwnd, UID: 1, UFlags: 0x1 | 0x2 | 0x4, UCallbackMessage: wmIcone, HIcon: icone} // NIF_MESSAGE|NIF_ICON|NIF_TIP
	trayDonnees.CbSize = uint32(unsafe.Sizeof(trayDonnees))
	copy(trayDonnees.SzTip[:127], syscall.StringToUTF16("3dssavesync"))
	notif := shell32.NewProc("Shell_NotifyIconW")
	// À l'ouverture de session, la barre des tâches n'est pas toujours prête : nouvel essai pendant une minute.
	for i := 0; ; i++ {
		r, _, err := notif.Call(0, uintptr(unsafe.Pointer(&trayDonnees))) // NIM_ADD
		if r != 0 {
			break
		}
		if i == 30 {
			user32.NewProc("DestroyWindow").Call(hwnd)
			return echec("Shell_NotifyIconW", err)
		}
		select {
		case <-fin:
			user32.NewProc("DestroyWindow").Call(hwnd)
			return nil
		case <-time.After(2 * time.Second):
		}
	}
	defer notif.Call(2, uintptr(unsafe.Pointer(&trayDonnees))) // NIM_DELETE
	go func() {
		<-fin
		user32.NewProc("PostMessageW").Call(hwnd, 0x10, 0, 0) // WM_CLOSE : DefWindowProc détruit la fenêtre, puis WM_DESTROY
	}()
	var m struct {
		HWnd           uintptr
		Message        uint32
		WParam, LParam uintptr
		Time           uint32
		X, Y           int32
		Prive          uint32
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
