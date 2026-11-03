package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// LaunchBox prêt : l'émulateur « (3dssavesync) » par défaut sur une plateforme de l'entrée ; un émulateur absent de
// LaunchBox n'est pas à faire.
func TestManquantsLaunchBox(t *testing.T) {
	lb, lanceur := filepath.Join("L", "LaunchBox"), filepath.Join("P", "3dssavesync.exe")
	entrees := []EntreeLB{
		{"melonds", filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), []string{"DS"}},
		{"mgba", filepath.Join("x", "mGBA.exe"), []string{"GB", "GBA"}},
	}
	if m := manquantsLaunchBox(emulatorsXML, lanceur, entrees); !slices.Equal(m, []string{"melonds"}) {
		t.Errorf("avant la réparation : %v", m)
	}
	nouveau, _ := launchbox(emulatorsXML, lb, lanceur, entrees)
	if m := manquantsLaunchBox(nouveau, lanceur, entrees); m != nil {
		t.Errorf("après la réparation : %v", m)
	}
	// L'utilisateur remet l'émulateur d'origine par défaut dans LaunchBox : à refaire.
	i := strings.LastIndex(nouveau, "<Default>true</Default>")
	if m := manquantsLaunchBox(nouveau[:i]+"<Default>false</Default>"+nouveau[i+len("<Default>true</Default>"):], lanceur, entrees); !slices.Equal(m, []string{"melonds"}) {
		t.Errorf("plus par défaut : %v", m)
	}
}

// Liste « Tout est prêt » : serveur, dossiers des sauvegardes, frontends, carte SD, sans jamais deviner.
func TestVerifications(t *testing.T) {
	m := maisonFactice(t)
	serveur := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "j"})
	defer serveur.Close()
	config := filepath.Join(t.TempDir(), "config.json")
	textes := func(l []Verif) (ok, afaire string) {
		for _, v := range l {
			if v.Ok {
				ok += v.Texte + "|"
			} else {
				afaire += v.Page + ":" + v.Texte + "|"
			}
		}
		return
	}
	if l, maj := verifications(config); len(l) != 1 || l[0].Ok || !strings.Contains(l[0].Texte, "pas encore configuré") || maj != nil {
		t.Errorf("sans config.json : %+v", l)
	}

	saves := filepath.Join(m, "saves")
	os.MkdirAll(saves, 0o755)
	esde := filepath.Join(m, "ES-DE")
	cfg := Config{URL: serveur.URL, Jeton: "faux", Emulateurs: map[string]*Emulateur{
		"melonds": {Exe: filepath.Join(m, "melonDS.exe"), Sauvegardes: map[string]string{"DS": saves}},
		"mgba":    {Exe: filepath.Join(m, "mGBA.exe"), Sauvegardes: map[string]string{"GB": filepath.Join(m, "absent"), "GBA": saves}},
	}, Frontends: map[string]string{"esde": esde}}
	ecrireJSON(config, &cfg, true)
	ok, afaire := textes(func() []Verif { l, _ := verifications(config); return l }())
	for _, attendu := range []string{"serveur:Serveur injoignable : Le serveur répond, mais refuse ce jeton", "emulateurs:mgba GB : dossier des sauvegardes introuvable (" + filepath.Join(m, "absent"),
		"frontends:ES-DE : pas encore configuré", "carte:Carte SD de la 3DS pas encore préparée"} {
		if !strings.Contains(afaire, attendu) {
			t.Errorf("à faire : %q absent de\n%s", attendu, afaire)
		}
	}
	if !strings.Contains(ok, "melonds DS : dossier des sauvegardes trouvé") || !strings.Contains(ok, "mgba GBA : dossier des sauvegardes trouvé") ||
		!strings.Contains(ok, "Version de développement") {
		t.Errorf("bons : %s", ok)
	}

	cfg.Jeton, cfg.CartePreparee = "j", "2026-11-03 14:05"
	cfg.Emulateurs["mgba"].Sauvegardes["GB"] = saves
	ecrireJSON(config, &cfg, true)
	reparer(&cfg, lanceurDe(&cfg))
	ok, afaire = textes(func() []Verif { l, _ := verifications(config); return l }())
	for _, attendu := range []string{"Serveur joignable, jeton accepté.", "ES-DE configuré.", "Carte SD de la 3DS préparée le 2026-11-03 14:05."} {
		if !strings.Contains(ok, attendu) {
			t.Errorf("bon : %q absent de\n%s", attendu, ok)
		}
	}
	if trayDisponible && !strings.Contains(afaire, "frontends:L'icône près de l'horloge ne tourne pas") || !trayDisponible && afaire != "" {
		t.Errorf("seule l'icône (pas lancée en test) reste à faire : %s", afaire)
	}
}
