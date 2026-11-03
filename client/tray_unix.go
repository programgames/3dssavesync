//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// trayLock returns nil if a tray icon already runs for this folder (flock lock on tray.verrou, which the system
// releases when the program ends); the returned function releases the lock.
func trayLock(dir string) func() {
	os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "tray.verrou"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil { // no lock possible: the tray icon starts anyway, better than none
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
