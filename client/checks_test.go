package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// LaunchBox ready: the "(3dssavesync)" emulator is the default on a platform of the entry; an emulator missing from
// LaunchBox is not a to-do.
func TestMissingLaunchBox(t *testing.T) {
	lb, launcher := filepath.Join("L", "LaunchBox"), filepath.Join("P", "3dssavesync.exe")
	entries := []LBEntry{
		{"melonds", filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), []System{"DS"}},
		{"mgba", filepath.Join("x", "mGBA.exe"), []System{"GB", "GBA"}},
	}
	if m := missingLaunchBox(emulatorsXML, launcher, entries); !slices.Equal(m, []string{"melonds"}) {
		t.Errorf("avant la réparation : %v", m)
	}
	newText, _ := launchbox(emulatorsXML, lb, launcher, entries)
	if m := missingLaunchBox(newText, launcher, entries); m != nil {
		t.Errorf("après la réparation : %v", m)
	}
	// The user sets the original emulator back as default in LaunchBox: to redo.
	i := strings.LastIndex(newText, "<Default>true</Default>")
	if m := missingLaunchBox(newText[:i]+"<Default>false</Default>"+newText[i+len("<Default>true</Default>"):], launcher, entries); !slices.Equal(m, []string{"melonds"}) {
		t.Errorf("plus par défaut : %v", m)
	}
}

// The "all ready" list: server, save folders, frontends, SD card, never guessing.
func TestChecks(t *testing.T) {
	m := fakeHome(t)
	server := httptest.NewServer(&Server{data: t.TempDir(), token: "j"})
	defer server.Close()
	config := filepath.Join(t.TempDir(), "config.json")
	texts := func(l []Check) (ok, todo string) {
		for _, v := range l {
			if v.Ok {
				ok += v.Text + "|"
			} else {
				todo += v.Page + ":" + v.Text + "|"
			}
		}
		return
	}
	if l, update := checks(config); len(l) != 1 || l[0].Ok || !strings.Contains(l[0].Text, "pas encore configuré") || update != nil {
		t.Errorf("sans config.json : %+v", l)
	}

	saves := filepath.Join(m, "saves")
	os.MkdirAll(saves, 0o755)
	esde := filepath.Join(m, "ES-DE")
	cfg := Config{URL: server.URL, Token: "faux", Emulators: map[string]*Emulator{
		"melonds": {Exe: filepath.Join(m, "melonDS.exe"), Saves: map[System]string{"DS": saves}},
		"mgba": {Exe: filepath.Join(m, "mGBA.exe"), Saves: map[System]string{"GB": filepath.Join(m, "absent"), "GBA": saves},
			Roms: map[System]string{"GBA": filepath.Join(m, "absent-roms")}},
	}, Frontends: map[string]string{"esde": esde}}
	writeJSON(config, &cfg, true)
	l, _ := checks(config)
	ok, todo := texts(l)
	for _, v := range l { // the emulator's row on the page reuses these lines (WebEmu.Problems)
		if strings.HasPrefix(v.Text, "mgba ") != (v.Emulator == "mgba") || strings.HasPrefix(v.Text, "melonds ") != (v.Emulator == "melonds") {
			t.Errorf("émulateur de la ligne %q : %q", v.Text, v.Emulator)
		}
	}
	for _, expected := range []string{"serveur:Serveur injoignable : Le serveur refuse ce jeton", "emulateurs:mgba GB\u00a0: dossier des sauvegardes introuvable (" + filepath.Join(m, "absent"),
		"frontends:ES-DE n'est pas encore configuré", "carte:La carte SD de la 3DS n'a pas encore été préparée",
		"dossier de jeux introuvable (" + filepath.Join(m, "absent-roms")} {
		if !strings.Contains(todo, expected) {
			t.Errorf("à faire : %q absent de\n%s", expected, todo)
		}
	}
	if !strings.Contains(ok, "melonds DS : dossier des sauvegardes trouvé") || !strings.Contains(ok, "mgba GBA : dossier des sauvegardes trouvé") ||
		!strings.Contains(ok, "Version de développement") || !strings.Contains(ok, "pas de dossier de jeux. Les jeux lancés depuis votre frontend") {
		t.Errorf("bons : %s", ok)
	}

	cfg.Token, cfg.PreparedCard = "j", "2026-11-03 14:05"
	cfg.Emulators["mgba"].Saves["GB"] = saves
	cfg.Emulators["mgba"].Roms["GBA"] = saves
	writeJSON(config, &cfg, true)
	repair(&cfg, launcherOf(&cfg))
	ok, todo = texts(func() []Check { l, _ := checks(config); return l }())
	for _, expected := range []string{"Serveur joignable, jeton accepté.", "ES-DE configuré.", "Carte SD de la 3DS préparée le 2026-11-03 14:05."} {
		if !strings.Contains(ok, expected) {
			t.Errorf("bon : %q absent de\n%s", expected, ok)
		}
	}
	if trayAvailable && !strings.Contains(todo, "frontends:L'icône près de l'horloge n'est pas lancée") || !trayAvailable && todo != "" {
		t.Errorf("seule l'icône (pas lancée en test) reste à faire : %s", todo)
	}
}
