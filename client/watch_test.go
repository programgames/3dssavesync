package main

import (
	"strings"
	"testing"
)

// The Sync without the Launcher starts when an emulator's program closes, never while it runs nor at the first check.
func TestClosedEmulators(t *testing.T) {
	before := instances
	t.Cleanup(func() { instances = before })
	open := map[string]bool{}
	instances = func(name string) int {
		if open[name] {
			return 1
		}
		return 0
	}
	cfg := &Config{Emulators: map[string]*Emulator{"melonds": {Exe: "melonDS.exe"}, "mgba": {Exe: "mGBA.exe"}}}
	running := map[string]bool{}
	open["melonDS"] = true
	if c := closedEmulators(cfg, running); len(c) != 0 {
		t.Errorf("premier passage, melonDS ouvert : %v", c)
	}
	if c := closedEmulators(cfg, running); len(c) != 0 {
		t.Errorf("melonDS toujours ouvert : %v", c)
	}
	open["melonDS"] = false
	if c := closedEmulators(cfg, running); len(c) != 1 || c[0] != "melonds" {
		t.Errorf("melonDS fermé : %v", c)
	}
	if c := closedEmulators(cfg, running); len(c) != 0 {
		t.Errorf("une seule synchro par fermeture : %v", c)
	}
}

// A problem is notified once: not again at the next full pass, nor at the close of an emulator (a part of the
// same problems); again only after a full pass without it.
func TestNewProblem(t *testing.T) {
	melonds, mgba := "melonds : dossier de jeux DS inconnu.", "mgba : dossier de jeux GB inconnu."
	n := map[string]bool{}
	for i, c := range []struct {
		problems []string
		full     bool
		want     bool
	}{
		{[]string{melonds, mgba}, true, true},
		{[]string{melonds, mgba}, true, false},
		{[]string{melonds}, false, false}, // melonDS closed: its part only
		{[]string{mgba}, true, false},     // melonDS fixed
		{[]string{melonds}, false, true},  // and back
		{nil, false, false},
	} {
		if got := newProblem(n, c.problems, c.full); got != c.want {
			t.Errorf("passe %d : %v, attendu %v", i+1, got, c.want)
		}
	}
}

// A pass where nothing moved and nothing went wrong stays silent; a problem comes before the counts.
func TestAutoSyncText(t *testing.T) {
	if s := autoSyncText(syncReport{syncCount: syncCount{inSync: 40}, open: []string{"mGBA"}}); s != "" {
		t.Errorf("rien à signaler : %q", s)
	}
	s := autoSyncText(syncReport{syncCount: syncCount{uploaded: 1, inSync: 3}, problems: []string{"Conflit sur Pokemon Or"}})
	if s != "Synchronisation automatique\nConflit sur Pokemon Or\nEnvoyées : 1 · Récupérées : 0 · Déjà à jour : 3" {
		t.Errorf("conflit et envoi : %q", s)
	}
	if s := autoSyncText(syncReport{problems: []string{"Synchronisation impossible."}}); !strings.HasSuffix(s, "impossible.") {
		t.Errorf("échec seul, sans compteurs : %q", s)
	}
}
