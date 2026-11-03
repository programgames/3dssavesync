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
