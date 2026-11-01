package main

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll") // chargée sous le contexte d'activation : version 6
)

// Un binaire Go n'a pas de manifeste : sans lui, comctl32 v5 n'a pas TaskDialog.
const manifeste = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
<dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls"
 version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency>
</assembly>`

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func ptr(p *uint16) uint64 { return uint64(uintptr(unsafe.Pointer(p))) }

// Fenêtre au premier plan : le frontend est souvent en plein écran.
var rappelDialogue = syscall.NewCallback(func(hwnd, msg, wparam, lparam, data uintptr) uintptr {
	if msg == 0 { // TDN_CREATED
		user32.NewProc("SetWindowPos").Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x3) // HWND_TOPMOST, SWP_NOSIZE|SWP_NOMOVE
		user32.NewProc("SetForegroundWindow").Call(hwnd)
	}
	return 0
})

// demanderSysteme : TaskDialog à boutons nommés. Fermer la fenêtre (Échap, croix) = dernier bouton,
// le plus prudent ; c'est aussi le bouton par défaut. "" si la fenêtre ne peut pas s'afficher.
func demanderSysteme(texte string, boutons []string) string {
	runtime.LockOSThread() // le contexte d'activation vaut pour ce thread
	defer runtime.UnlockOSThread()
	chemin := filepath.Join(os.TempDir(), "3dssavesync-comctl6.manifest")
	if os.WriteFile(chemin, []byte(manifeste), 0o644) != nil {
		return ""
	}
	// ACTCTXW : cbSize, dwFlags, lpSource, wProcessorArchitecture, wLangId, puis 4 pointeurs (56 octets).
	var actx [56]byte
	binary.LittleEndian.PutUint32(actx[0:], 56)
	source := u16(chemin)
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

	// Première ligne (avant une ligne vide) = titre en gras du dialogue.
	instruction, contenu, _ := strings.Cut(texte, "\n\n")
	titre, pInstr, pContenu := u16("3dssavesync"), u16(instruction), u16(contenu)
	// TASKDIALOG_BUTTON (pack 1) : int nButtonID, PCWSTR pszButtonText.
	textes := make([]*uint16, len(boutons))
	tab := make([]byte, 12*len(boutons))
	for i, b := range boutons {
		textes[i] = u16(b)
		binary.LittleEndian.PutUint32(tab[12*i:], uint32(100+i))
		binary.LittleEndian.PutUint64(tab[12*i+4:], ptr(textes[i]))
	}
	// TASKDIALOGCONFIG (pack 1, 160 octets en 64 bits).
	var c [160]byte
	le := binary.LittleEndian
	le.PutUint32(c[0:], 160)
	le.PutUint32(c[20:], 0x8) // TDF_ALLOW_DIALOG_CANCELLATION
	le.PutUint64(c[28:], ptr(titre))
	le.PutUint64(c[44:], ptr(pInstr))
	if contenu != "" {
		le.PutUint64(c[52:], ptr(pContenu))
	}
	le.PutUint32(c[60:], uint32(len(boutons)))
	le.PutUint64(c[64:], uint64(uintptr(unsafe.Pointer(&tab[0]))))
	le.PutUint32(c[72:], uint32(100+len(boutons)-1))
	le.PutUint64(c[140:], uint64(rappelDialogue))
	var choisi int32
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&c[0])), uintptr(unsafe.Pointer(&choisi)), 0, 0)
	runtime.KeepAlive(textes)
	runtime.KeepAlive(tab)
	runtime.KeepAlive([]*uint16{titre, pInstr, pContenu})
	if r != 0 {
		return ""
	}
	if i := int(choisi) - 100; i >= 0 && i < len(boutons) {
		return boutons[i]
	}
	return boutons[len(boutons)-1] // IDCANCEL
}

// NOTIFYICONDATAW (976 octets en 64 bits).
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

// notifierSysteme : notification Windows, rien à valider. Le programme se termine juste après :
// on attend que la notification s'affiche, sinon Windows la retire avec l'icône.
func notifierSysteme(texte string) {
	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(u16("STATIC"))),
		uintptr(unsafe.Pointer(u16("3dssavesync"))), 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		return
	}
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	icone, _, _ := user32.NewProc("LoadIconW").Call(0, 32516)                                       // IDI_INFORMATION
	n := notifyIconData{HWnd: hwnd, UID: 1, UFlags: 0x2 | 0x4 | 0x10, HIcon: icone, DwInfoFlags: 1} // NIF_ICON|NIF_TIP|NIF_INFO, NIIF_INFO
	n.CbSize = uint32(unsafe.Sizeof(n))
	copy(n.SzTip[:127], syscall.StringToUTF16("3dssavesync"))
	copy(n.SzInfoTitle[:63], syscall.StringToUTF16("3dssavesync"))
	info := syscall.StringToUTF16(texte)
	if len(info) > 255 {
		info = append(info[:254], 0)
	}
	copy(n.SzInfo[:], info)
	notif := shell32.NewProc("Shell_NotifyIconW")
	if r, _, _ := notif.Call(0, uintptr(unsafe.Pointer(&n))); r == 0 { // NIM_ADD
		return
	}
	time.Sleep(8 * time.Second)
	notif.Call(2, uintptr(unsafe.Pointer(&n))) // NIM_DELETE
}

// Console de l'assistant : l'exe n'en a pas (-H windowsgui), on en ouvre une.
// Go écrit et lit la console en UTF-16 : les accents passent.
func ouvrirConsole() {
	kernel32.NewProc("AllocConsole").Call()
	if in, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		os.Stdin = in
	}
	if out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout, os.Stderr = out, out
	}
}

// La fenêtre de console se ferme avec le programme : on laisse lire la fin.
const consolePropre = true

// Saisie sans écho (jeton) : retire ENABLE_ECHO_INPUT, rend la fonction qui remet le mode.
func sansEcho() func() {
	h := syscall.Handle(os.Stdin.Fd())
	var mode uint32
	if syscall.GetConsoleMode(h, &mode) != nil {
		return func() {}
	}
	kernel32.NewProc("SetConsoleMode").Call(uintptr(h), uintptr(mode&^0x4))
	return func() { kernel32.NewProc("SetConsoleMode").Call(uintptr(h), uintptr(mode)) }
}

// Langue de l'interface de Windows : "fr" si la langue principale est le français (LANG_FRENCH = 0x0C).
func langueSysteme() string {
	id, _, _ := kernel32.NewProc("GetUserDefaultUILanguage").Call()
	if id&0x3FF == 0x0C {
		return "fr"
	}
	return "en"
}

func processusActif(nom string) bool {
	s, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(s)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(s, &e); err == nil; err = syscall.Process32Next(s, &e) {
		if strings.EqualFold(strings.TrimSuffix(strings.ToLower(syscall.UTF16ToString(e.ExeFile[:])), ".exe"), nom) {
			return true
		}
	}
	return false
}

// Processus indépendant de la console de l'assistant (l'exe est une application GUI : aucune console héritée).
func demarrerDetache(exe string, args []string) error {
	c := exec.Command(exe, args...)
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
