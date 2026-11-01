//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ponytail: pas encore de fenêtre hors Windows (phase 3 : macOS, zenity, kdialog).
// "" = aucune fenêtre : un Conflit ne remplace rien (Q16).
func demanderSysteme(texte string, boutons []string) string {
	fmt.Fprintln(os.Stderr, texte)
	return ""
}

func notifierSysteme(texte string) { fmt.Fprintln(os.Stderr, texte) }

// Linux : /proc/<pid>/comm (15 caractères au plus) ; macOS : ps.
func processusActif(nom string) bool {
	nom = strings.ToLower(nom)
	if len(nom) > 15 {
		nom = nom[:15]
	}
	if comms, err := filepath.Glob("/proc/[0-9]*/comm"); err == nil && len(comms) > 0 {
		for _, c := range comms {
			if o, err := os.ReadFile(c); err == nil && strings.ToLower(strings.TrimSpace(string(o))) == nom {
				return true
			}
		}
		return false
	}
	o, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(o), "\n") {
		if strings.ToLower(filepath.Base(strings.TrimSpace(l))) == nom {
			return true
		}
	}
	return false
}
