//go:build !windows

package main

import (
	"strings"
	"testing"
)

func TestDialogs(t *testing.T) {
	b := []string{"Prendre celle du serveur", "Garder celle du PC", "Ne pas jouer"}
	tcase := []struct {
		tool, output string
		code         int
		expected     string
	}{
		{"zenity", "", 0, "Prendre celle du serveur"},
		{"zenity", "Garder celle du PC\n", 1, "Garder celle du PC"},
		{"zenity", "", 1, "Ne pas jouer"}, // Annuler ou fenêtre fermée : le plus prudent
		{"zenity", "", 255, ""},           // erreur de zenity : aucune fenêtre (Q16)
		{"kdialog", "", 1, "Garder celle du PC"},
		{"kdialog", "", 2, "Ne pas jouer"},
		{"kdialog", "", 254, ""},
		{"osascript", "Garder celle du PC\n", 0, "Garder celle du PC"},
		{"osascript", "", 1, ""},
	}
	for _, c := range tcase {
		if r := dialogResponse(c.tool, b, c.code, c.output); r != c.expected {
			t.Errorf("%s code %d sortie %q : %q, attendu %q", c.tool, c.code, c.output, r, c.expected)
		}
	}
	z := strings.Join(dialogArgs("zenity", "<texte>", b), "|")
	if z != "--question|--no-markup|--title=3dssavesync|--text=<texte>|--ok-label=Prendre celle du serveur|--cancel-label=Ne pas jouer|--extra-button=Garder celle du PC" {
		t.Errorf("zenity : %s", z)
	}
	if k := dialogArgs("kdialog", "t", b[1:]); strings.Join(k[2:], "|") != "--yesno|t|--yes-label|Garder celle du PC|--no-label|Ne pas jouer" {
		t.Errorf("kdialog : %q", k)
	}
	if o := dialogArgs("osascript", "t", b); strings.Join(o[len(o)-4:], "|") != "t|Prendre celle du serveur|Garder celle du PC|Ne pas jouer" {
		t.Errorf("osascript : texte et boutons en arguments attendus : %q", o)
	}
}
