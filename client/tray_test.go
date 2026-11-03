package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Menu de l'icône d'après l'état : Envois en attente, serveur local.
func TestMenuTray(t *testing.T) {
	textes := func(m []ElementMenu) string {
		var l []string
		for _, e := range m {
			l = append(l, e.Texte+"="+e.Action)
		}
		return strings.Join(l, "|")
	}
	if m := textes(menuTray(0, "", "")); m != "Ouvrir les réglages=reglages|3dssavesync dev=|=|Quitter=quitter" {
		t.Errorf("sans attente ni serveur local : %s", m)
	}
	if m := textes(menuTray(2, "Serveur local : en marche (x)", "3.8.0")); m != "Ouvrir les réglages=reglages|Version 3.8.0 disponible : mettre à jour=reglages|"+
		"Envoyer maintenant (2 en attente)=envoyer|Serveur local : en marche (x)=|3dssavesync dev=|=|Quitter=quitter" {
		t.Errorf("2 en attente, serveur local, mise à jour : %s", m)
	}

	if l := ligneServeur(&Config{}, nil); l != "" {
		t.Errorf("sans serveur local : pas de ligne, %q", l)
	}
	local := &Config{ServeurLocal: &ServeurLocal{Port: 8765}}
	if l := ligneServeur(local, nil); !strings.HasPrefix(l, "Serveur local : en marche (") || !strings.Contains(l, "8765)") {
		t.Errorf("serveur en marche : %q", l)
	}
	err := errors.New("le port 8765 est déjà pris par un autre programme : le serveur local ne démarre pas. Choisissez un autre port.")
	if l := ligneServeur(local, err); l != "Serveur local : arrêté (le port 8765 est déjà pris par un autre programme : le serveur local ne démarre pas)" {
		t.Errorf("serveur arrêté : %q", l)
	}

	d := t.TempDir()
	if n := nombreEnAttente(d); n != 0 {
		t.Errorf("sans en-attente.json : %d", n)
	}
	ecrire(t, filepath.Join(d, "en-attente.json"), []byte(`[{"profil":"melonds","Cle":"IRBF"},{"profil":"mgba","Cle":"BPEF"}]`))
	if n := nombreEnAttente(d); n != 2 {
		t.Errorf("2 Envois en attente attendus : %d", n)
	}
}

// Démarrage automatique : l'icône (qui fait tourner le serveur local), sinon serve, sinon rien. Réglage absent = icône.
func TestChoixDemarrage(t *testing.T) {
	avant := trayDisponible
	t.Cleanup(func() { trayDisponible = avant })
	trayDisponible = true
	non := false
	config := filepath.Join(t.TempDir(), "config.json")
	for _, c := range []struct {
		cfg      Config
		attendus string
	}{
		{Config{}, "tray --config " + config},
		{Config{ServeurLocal: &ServeurLocal{Port: 8765}}, "tray --config " + config},
		{Config{ServeurLocal: &ServeurLocal{Port: 8765}, IconeHorloge: &non}, "serve --config " + config},
		{Config{IconeHorloge: &non}, ""},
	} {
		if a := strings.Join(choixDemarrage(&c.cfg, config), " "); a != c.attendus {
			t.Errorf("%+v : %q, attendu %q", c.cfg, a, c.attendus)
		}
	}
	if a := choixDemarrage(&Config{}, configDefaut()); strings.Join(a, " ") != "tray" {
		t.Errorf("config.json par défaut : sans --config, %q", a)
	}
	trayDisponible = false // macOS
	if a := choixDemarrage(&Config{ServeurLocal: &ServeurLocal{Port: 8765}}, configDefaut()); strings.Join(a, " ") != "serve" {
		t.Errorf("sans icône possible : serve, %q", a)
	}

	var cfg Config
	if o, _ := json.Marshal(&Config{}); strings.Contains(string(o), "icone_horloge") {
		t.Errorf("icône affichée : rien dans config.json, %s", o)
	}
	if json.Unmarshal([]byte(`{"icone_horloge": false}`), &cfg); cfg.iconeHorloge() {
		t.Error(`"icone_horloge": false : icône retirée`)
	}
}

// Page des réglages, case « Afficher l'icône près de l'horloge » : entrée « 3dssavesync » vers tray, lancée tout de
// suite, anciennes entrées retirées ; case décochée : "icone_horloge": false, plus aucune entrée (pas de serveur local).
func TestReglageIcone(t *testing.T) {
	maisonFactice(t)
	avant, detecterAvant := trayDisponible, detecter
	t.Cleanup(func() { trayDisponible, detecter = avant, detecterAvant })
	trayDisponible, detecter = true, func(string) []Trouve { return nil }
	serveur := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "j"})
	defer serveur.Close()
	config := filepath.Join(t.TempDir(), "config.json")
	a := &AssistantWeb{config: config}
	var auto, lance []string
	var garder []bool
	autostart = func(nom, _ string, args []string) (string, error) {
		auto = append([]string{nom}, args...)
		return "test", nil
	}
	lancerServeur = func(_ string, args []string) error { lance = args; return nil }
	retirerAutostart = func(g bool) error { garder = append(garder, g); return nil }

	if _, err := a.appliquer(ChoixWeb{URL: serveur.URL, Jeton: "j", Icone: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(auto, " ") != "3dssavesync tray --config "+config || strings.Join(lance, " ") != "tray --config "+config || len(garder) != 1 || !garder[0] {
		t.Errorf("icône : entrée %q, lancée %q, anciennes retirées %v", auto, lance, garder)
	}
	if o, _ := os.ReadFile(config); strings.Contains(string(o), "icone_horloge") {
		t.Errorf("icône affichée : rien dans config.json, %s", o)
	}
	auto, lance, garder = nil, nil, nil
	if _, err := a.appliquer(ChoixWeb{URL: serveur.URL, Jeton: "j"}); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	lireJSON(config, &cfg)
	if cfg.iconeHorloge() || auto != nil || lance != nil || len(garder) != 1 || garder[0] {
		t.Errorf("case décochée : %+v, entrée %q, lancée %q, retirées %v", cfg.IconeHorloge, auto, lance, garder)
	}
	if !a.etat().IconeDisponible || a.etat().Icone {
		t.Error("la page relit la case décochée")
	}
}

// Une seule icône par dossier de réglages ; demande d'arrêt par le fichier arret-tray, vue dans les 2 s.
func TestArretTray(t *testing.T) {
	d := t.TempDir()
	if trayActif(d) {
		t.Fatal("aucune icône ne tourne encore")
	}
	liberer := verrouTray(d)
	if liberer == nil || !trayActif(d) {
		t.Fatal("première icône : verrou attendu")
	}
	if verrouTray(d) != nil {
		t.Fatal("deuxième icône : elle doit s'arrêter")
	}
	fin := make(chan struct{})
	go surveillerArret(d, func() { liberer(); close(fin) }, fin)
	if err := arreterTray(d); err != nil {
		t.Fatal(err)
	}
	if trayActif(d) || existe(filepath.Join(d, fichierArret)) {
		t.Error("icône arrêtée, fichier d'arrêt retiré attendus")
	}
	if err := arreterTray(d); err != nil || existe(filepath.Join(d, fichierArret)) {
		t.Errorf("icône arrêtée : rien à faire, aucun fichier laissé (%v)", err)
	}
	l := verrouTray(d)
	if l == nil {
		t.Fatal("après l'arrêt, une nouvelle icône démarre")
	}
	l()
}

// Image de l'icône : la plus petite d'au moins la taille demandée, sinon la plus grande (largeur lue dans l'en-tête PNG).
func TestImageIco(t *testing.T) {
	for taille, attendue := range map[int]uint32{16: 16, 20: 24, 64: 64, 300: 256} {
		img := imageIco(iconeIco, taille)
		if len(img) < 24 || string(img[1:4]) != "PNG" || binary.BigEndian.Uint32(img[16:]) != attendue {
			t.Errorf("taille %d : image de %d pixels attendue", taille, attendue)
		}
	}
	if imageIco([]byte{0, 0, 1, 0, 1, 0}, 16) != nil {
		t.Error(".ico tronqué : aucune image")
	}
}
