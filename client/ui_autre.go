//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
)

// Fenêtres hors Windows : osascript (macOS), zenity ou kdialog (Linux).
// Le dernier bouton est le plus prudent : bouton par défaut, et choisi quand on ferme la fenêtre.
// "" = aucune fenêtre possible (pas d'outil, pas d'écran, erreur de l'outil) : un Conflit ne remplace rien (Q16).
// ponytail: affichage sous Gamescope (Steam Deck, mode Jeu) non vérifié.
func demanderSysteme(texte string, boutons []string) string {
	fmt.Fprintln(os.Stderr, texte)
	outil := outilDialogue()
	if outil == "" {
		return ""
	}
	sortie, err := exec.Command(outil, argsDialogue(outil, texte, boutons)...).Output()
	code := 0
	var fin *exec.ExitError
	if errors.As(err, &fin) {
		code = fin.ExitCode()
	} else if err != nil {
		return ""
	}
	return reponseDialogue(outil, boutons, code, string(sortie))
}

func outilDialogue() string {
	if runtime.GOOS == "darwin" {
		return "osascript"
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return ""
	}
	ordre := []string{"zenity", "kdialog"}
	if strings.Contains(os.Getenv("XDG_CURRENT_DESKTOP"), "KDE") {
		ordre = []string{"kdialog", "zenity"}
	}
	for _, o := range ordre {
		if _, err := exec.LookPath(o); err == nil {
			return o
		}
	}
	return ""
}

// Arguments de l'outil. Au plus 3 boutons (limite d'AppleScript et de kdialog).
func argsDialogue(outil, texte string, b []string) []string {
	switch outil {
	case "osascript": // texte et boutons en arguments : aucun échappement AppleScript
		return append([]string{"-e", "on run argv", "-e", "set b to items 2 thru -1 of argv", "-e",
			`button returned of (display dialog (item 1 of argv) with title "3dssavesync" buttons b default button (count of b))`,
			"-e", "end run", texte}, b...)
	case "kdialog": // codes de sortie : 0, 1, 2 = boutons 1, 2, 3
		a := []string{"--title", "3dssavesync"}
		switch len(b) {
		case 1:
			return append(a, "--msgbox", texte)
		case 2:
			return append(a, "--yesno", texte, "--yes-label", b[0], "--no-label", b[1])
		}
		return append(a, "--yesnocancel", texte, "--yes-label", b[0], "--no-label", b[1], "--cancel-label", b[2])
	}
	// zenity : OK = premier bouton, Annuler (et fermeture) = dernier, boutons du milieu en --extra-button.
	if len(b) == 1 {
		return []string{"--info", "--no-markup", "--title=3dssavesync", "--text=" + texte, "--ok-label=" + b[0]}
	}
	a := []string{"--question", "--no-markup", "--title=3dssavesync", "--text=" + texte, "--ok-label=" + b[0], "--cancel-label=" + b[len(b)-1]}
	for _, x := range b[1 : len(b)-1] {
		a = append(a, "--extra-button="+x)
	}
	return a
}

// Bouton choisi d'après le code de sortie et la sortie de l'outil ; "" si l'outil a échoué.
func reponseDialogue(outil string, b []string, code int, sortie string) string {
	sortie = strings.TrimSpace(sortie)
	switch outil {
	case "osascript":
		if code == 0 && slices.Contains(b, sortie) {
			return sortie
		}
	case "kdialog":
		if code >= 0 && code <= 2 {
			return b[min(code, len(b)-1)]
		}
	default: // zenity : 0 = OK ; 1 = Annuler, fermeture ou bouton du milieu (libellé sur la sortie)
		switch {
		case code == 0:
			return b[0]
		case code == 1 && slices.Contains(b, sortie):
			return sortie
		case code == 1:
			return b[len(b)-1]
		}
	}
	return ""
}

func notifierSysteme(texte string) {
	fmt.Fprintln(os.Stderr, texte)
	if runtime.GOOS == "darwin" {
		exec.Command("osascript", "-e", "on run argv", "-e", `display notification (item 1 of argv) with title "3dssavesync"`, "-e", "end run", texte).Run()
	} else if p, err := exec.LookPath("notify-send"); err == nil {
		exec.Command(p, "3dssavesync", texte).Run()
	}
}

func attendreNotifications() {} // notify-send et osascript rendent la main une fois la notification affichée

// L'assistant se lance dans un terminal : rien à ouvrir.
func ouvrirConsole() {}

const consolePropre = false

// Saisie sans écho (jeton) par stty ; sans terminal, rien à faire.
func sansEcho() func() {
	c := exec.Command("stty", "-echo")
	c.Stdin = os.Stdin
	if c.Run() != nil {
		return func() {}
	}
	return func() {
		c := exec.Command("stty", "echo")
		c.Stdin = os.Stdin
		c.Run()
	}
}

// Langue : variables de locale, puis la préférence de macOS (une application lancée par le Finder n'a pas LANG).
func langueSysteme() string {
	for _, v := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := os.Getenv(v); l != "" && l != "C" && l != "POSIX" {
			return l
		}
	}
	if runtime.GOOS == "darwin" {
		if o, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
			return string(o)
		}
	}
	return ""
}

// Nombre de processus de ce nom. Linux : /proc/<pid>/comm (15 caractères au plus) ; macOS : ps.
func compterProcessus(nom string) (n int) {
	nom = strings.ToLower(nom)
	if len(nom) > 15 {
		nom = nom[:15]
	}
	if comms, err := filepath.Glob("/proc/[0-9]*/comm"); err == nil && len(comms) > 0 {
		for _, c := range comms {
			if o, err := os.ReadFile(c); err == nil && strings.ToLower(strings.TrimSpace(string(o))) == nom {
				n++
			}
		}
		return n
	}
	o, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(o), "\n") {
		if strings.ToLower(filepath.Base(strings.TrimSpace(l))) == nom {
			n++
		}
	}
	return n
}

// Le processus pid tourne encore (signal 0 : vérifie sans rien envoyer ; EPERM = il existe, à un autre utilisateur).
func processusVivant(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// Processus qui survit à la fermeture du terminal de l'assistant (nouvelle session, sans SIGHUP).
func demarrerDetache(exe string, args []string) error {
	c := exec.Command(exe, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// Volumes montés (carte SD, clé USB) : /Volumes (macOS), /media/$USER et /run/media/$USER (Linux).
func racinesAmovibles() []string {
	motifs := []string{"/Volumes/*"}
	if runtime.GOOS != "darwin" {
		motifs = []string{filepath.Join("/media", os.Getenv("USER"), "*"), filepath.Join("/run/media", os.Getenv("USER"), "*")}
	}
	var l []string
	for _, m := range motifs {
		f, _ := filepath.Glob(m)
		l = append(l, f...)
	}
	return l
}

// Place libre pour l'utilisateur sur le disque du dossier.
func espaceLibreSysteme(dossier string) (uint64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(dossier, &s); err != nil {
		return 0, err
	}
	return uint64(s.Bavail) * uint64(s.Bsize), nil
}
