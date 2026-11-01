//go:build !windows

package main

import (
	"strings"
	"testing"
)

func TestDialogues(t *testing.T) {
	b := []string{"Prendre le serveur", "Garder le PC", "Ne pas jouer"}
	cas := []struct {
		outil, sortie string
		code          int
		attendu       string
	}{
		{"zenity", "", 0, "Prendre le serveur"},
		{"zenity", "Garder le PC\n", 1, "Garder le PC"},
		{"zenity", "", 1, "Ne pas jouer"}, // Annuler ou fenêtre fermée : le plus prudent
		{"zenity", "", 255, ""},           // erreur de zenity : aucune fenêtre (Q16)
		{"kdialog", "", 1, "Garder le PC"},
		{"kdialog", "", 2, "Ne pas jouer"},
		{"kdialog", "", 254, ""},
		{"osascript", "Garder le PC\n", 0, "Garder le PC"},
		{"osascript", "", 1, ""},
	}
	for _, c := range cas {
		if r := reponseDialogue(c.outil, b, c.code, c.sortie); r != c.attendu {
			t.Errorf("%s code %d sortie %q : %q, attendu %q", c.outil, c.code, c.sortie, r, c.attendu)
		}
	}
	z := strings.Join(argsDialogue("zenity", "<texte>", b), "|")
	if z != "--question|--no-markup|--title=3dssavesync|--text=<texte>|--ok-label=Prendre le serveur|--cancel-label=Ne pas jouer|--extra-button=Garder le PC" {
		t.Errorf("zenity : %s", z)
	}
	if k := argsDialogue("kdialog", "t", b[1:]); strings.Join(k[2:], "|") != "--yesno|t|--yes-label|Garder le PC|--no-label|Ne pas jouer" {
		t.Errorf("kdialog : %q", k)
	}
	if o := argsDialogue("osascript", "t", b); strings.Join(o[len(o)-4:], "|") != "t|Prendre le serveur|Garder le PC|Ne pas jouer" {
		t.Errorf("osascript : texte et boutons en arguments attendus : %q", o)
	}
}
