//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// verrouTray : nil si une icône tourne déjà pour ce dossier (verrou flock sur tray.verrou, que le système libère à la
// fin du programme) ; la fonction rendue libère le verrou.
func trayLock(dir string) func() {
	os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "tray.verrou"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil { // pas de verrou possible : l'icône démarre quand même, mieux qu'aucune
		return func() {}
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil
	}
	return func() { f.Close() }
}

func trayActive(dir string) bool {
	f, err := os.Open(filepath.Join(dir, "tray.verrou"))
	if err != nil {
		return false
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return true
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}
