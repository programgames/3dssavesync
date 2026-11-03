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
func TestTrayMenu(t *testing.T) {
	texts := func(m []MenuItem) string {
		var l []string
		for _, e := range m {
			l = append(l, e.Text+"="+e.Action)
		}
		return strings.Join(l, "|")
	}
	if m := texts(trayMenuItems(0, "", "")); m != "Ouvrir les réglages=reglages|3dssavesync dev=|=|Quitter=quitter" {
		t.Errorf("sans attente ni serveur local : %s", m)
	}
	if m := texts(trayMenuItems(2, "Serveur local : en marche (x)", "3.8.0")); m != "Ouvrir les réglages=reglages|Mettre à jour vers la version 3.8.0=reglages|"+
		"Envoyer maintenant (2 en attente)=envoyer|Serveur local : en marche (x)=|3dssavesync dev=|=|Quitter=quitter" {
		t.Errorf("2 en attente, serveur local, mise à jour : %s", m)
	}

	if l := serverLine(&Config{}, nil); l != "" {
		t.Errorf("sans serveur local : pas de ligne, %q", l)
	}
	local := &Config{LocalServer: &LocalServer{Port: 8765}}
	if l := serverLine(local, nil); !strings.HasPrefix(l, "Serveur local : en marche (") || !strings.Contains(l, "8765)") {
		t.Errorf("serveur en marche : %q", l)
	}
	err := errors.New("le port 8765 est déjà pris par un autre programme : le serveur local ne démarre pas. Choisissez un autre port.")
	if l := serverLine(local, err); l != "Serveur local : arrêté (le port 8765 est déjà pris par un autre programme : le serveur local ne démarre pas)" {
		t.Errorf("serveur arrêté : %q", l)
	}

	d := t.TempDir()
	if n := pendingCount(d); n != 0 {
		t.Errorf("sans en-attente.json : %d", n)
	}
	write(t, filepath.Join(d, "en-attente.json"), []byte(`[{"profil":"melonds","Cle":"IRBF"},{"profil":"mgba","Cle":"BPEF"}]`))
	if n := pendingCount(d); n != 2 {
		t.Errorf("2 Envois en attente attendus : %d", n)
	}
}

// Démarrage automatique : l'icône (qui fait tourner le serveur local), sinon serve, sinon rien. Réglage absent = icône.
func TestStartupChoice(t *testing.T) {
	before := trayAvailable
	t.Cleanup(func() { trayAvailable = before })
	trayAvailable = true
	no := false
	config := filepath.Join(t.TempDir(), "config.json")
	for _, c := range []struct {
		cfg          Config
		expectations string
	}{
		{Config{}, "tray --config " + config},
		{Config{LocalServer: &LocalServer{Port: 8765}}, "tray --config " + config},
		{Config{LocalServer: &LocalServer{Port: 8765}, TrayIcon: &no}, "serve --config " + config},
		{Config{TrayIcon: &no}, ""},
	} {
		if a := strings.Join(startupChoice(&c.cfg, config), " "); a != c.expectations {
			t.Errorf("%+v : %q, attendu %q", c.cfg, a, c.expectations)
		}
	}
	if a := startupChoice(&Config{}, defaultConfig()); strings.Join(a, " ") != "tray" {
		t.Errorf("config.json par défaut : sans --config, %q", a)
	}
	trayAvailable = false // macOS
	if a := startupChoice(&Config{LocalServer: &LocalServer{Port: 8765}}, defaultConfig()); strings.Join(a, " ") != "serve" {
		t.Errorf("sans icône possible : serve, %q", a)
	}

	var cfg Config
	if o, _ := json.Marshal(&Config{}); strings.Contains(string(o), "icone_horloge") {
		t.Errorf("icône affichée : rien dans config.json, %s", o)
	}
	if json.Unmarshal([]byte(`{"icone_horloge": false}`), &cfg); cfg.trayIcon() {
		t.Error(`"icone_horloge": false : icône retirée`)
	}
}

// Page des réglages, case « Afficher l'icône près de l'horloge » : entrée « 3dssavesync » vers tray, lancée tout de
// suite, anciennes entrées retirées ; case décochée : "icone_horloge": false, plus aucune entrée (pas de serveur local).
func TestIconSetting(t *testing.T) {
	fakeHome(t)
	before, detectBefore := trayAvailable, detect
	t.Cleanup(func() { trayAvailable, detect = before, detectBefore })
	trayAvailable, detect = true, func(string) []Found { return nil }
	server := httptest.NewServer(&Server{data: t.TempDir(), token: "j"})
	defer server.Close()
	config := filepath.Join(t.TempDir(), "config.json")
	a := &WebSetup{config: config}
	var auto, launched []string
	var keep []bool
	autostart = func(name, _ string, args []string) (string, error) {
		auto = append([]string{name}, args...)
		return "test", nil
	}
	launchServer = func(_ string, args []string) error { launched = args; return nil }
	removeAutostart = func(g bool) error { keep = append(keep, g); return nil }

	if _, err := a.apply(WebChoices{URL: server.URL, Token: "j", Icon: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(auto, " ") != "3dssavesync tray --config "+config || strings.Join(launched, " ") != "tray --config "+config || len(keep) != 1 || !keep[0] {
		t.Errorf("icône : entrée %q, lancée %q, anciennes retirées %v", auto, launched, keep)
	}
	if o, _ := os.ReadFile(config); strings.Contains(string(o), "icone_horloge") {
		t.Errorf("icône affichée : rien dans config.json, %s", o)
	}
	auto, launched, keep = nil, nil, nil
	if _, err := a.apply(WebChoices{URL: server.URL, Token: "j"}); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	readJSON(config, &cfg)
	if cfg.trayIcon() || auto != nil || launched != nil || len(keep) != 1 || keep[0] {
		t.Errorf("case décochée : %+v, entrée %q, lancée %q, retirées %v", cfg.TrayIcon, auto, launched, keep)
	}
	if !a.state().IconAvailable || a.state().Icon {
		t.Error("la page relit la case décochée")
	}
}

// Une seule icône par dossier de réglages ; demande d'arrêt par le fichier arret-tray, vue dans les 2 s.
func TestTrayStop(t *testing.T) {
	d := t.TempDir()
	if trayActive(d) {
		t.Fatal("aucune icône ne tourne encore")
	}
	release := trayLock(d)
	if release == nil || !trayActive(d) {
		t.Fatal("première icône : verrou attendu")
	}
	if trayLock(d) != nil {
		t.Fatal("deuxième icône : elle doit s'arrêter")
	}
	end := make(chan struct{})
	go watchStop(d, func() { release(); close(end) }, end)
	if err := stopTray(d); err != nil {
		t.Fatal(err)
	}
	if trayActive(d) || exists(filepath.Join(d, stopFile)) {
		t.Error("icône arrêtée, fichier d'arrêt retiré attendus")
	}
	if err := stopTray(d); err != nil || exists(filepath.Join(d, stopFile)) {
		t.Errorf("icône arrêtée : rien à faire, aucun fichier laissé (%v)", err)
	}
	l := trayLock(d)
	if l == nil {
		t.Fatal("après l'arrêt, une nouvelle icône démarre")
	}
	l()
}

// Image de l'icône : la plus petite d'au moins la taille demandée, sinon la plus grande (largeur lue dans l'en-tête PNG).
func TestImageIco(t *testing.T) {
	for size, expectedValue := range map[int]uint32{16: 16, 20: 24, 64: 64, 300: 256} {
		img := imageIco(iconICO, size)
		if len(img) < 24 || string(img[1:4]) != "PNG" || binary.BigEndian.Uint32(img[16:]) != expectedValue {
			t.Errorf("taille %d : image de %d pixels attendue", size, expectedValue)
		}
	}
	if imageIco([]byte{0, 0, 1, 0, 1, 0}, 16) != nil {
		t.Error(".ico tronqué : aucune image")
	}
}

// Copie vers l'emplacement installé : l'icône, qui tourne souvent depuis l'exe installé, s'arrête avant la copie
// (Windows refuse de remplacer un exe en marche). Exe installé encore pris : le message dit quoi faire, puis le détail.
func TestInstallExeStopsIcon(t *testing.T) {
	fakeHome(t)
	exe := filepath.Join(t.TempDir(), "3dssavesync.exe")
	write(t, exe, []byte("nouveau"))
	ea, ra := executable, rename
	t.Cleanup(func() { executable, rename = ea, ra })
	executable = func() string { return exe }
	target := installedLocation()
	write(t, target, []byte("ancien"))
	d := t.TempDir() // dossier des réglages
	release := trayLock(d)
	stopped, end := make(chan string, 1), make(chan struct{})
	t.Cleanup(func() { close(end) })
	go watchStop(d, func() { stopped <- string(read(target)); release() }, end)
	if l := installExe(&Config{}, d); !strings.HasPrefix(l, "Programme copié") || string(read(target)) != "nouveau" {
		t.Fatalf("copie : %q, %q", l, read(target))
	}
	select {
	case before := <-stopped:
		if before != "ancien" {
			t.Errorf("icône arrêtée après la copie : exe %q", before)
		}
	default:
		t.Fatal("icône près de l'horloge non arrêtée avant la copie")
	}

	rename = func(a, b string) error {
		return &os.LinkError{Op: "rename", Old: a, New: b, Err: errors.New("Accès refusé.")}
	}
	write(t, exe, []byte("encore"))
	l := installExe(&Config{}, d)
	if !strings.HasPrefix(l, "Impossible de copier le programme\u00a0: la version installée est encore ouverte. Quittez l'icône près de l'horloge, fermez les autres fenêtres de 3dssavesync et enregistrez de nouveau.\nDétail : rename ") ||
		string(read(target)) != "nouveau" || exists(target+".3dssavesync-tmp") {
		t.Errorf("exe installé pris : %q, exe %q", l, read(target))
	}
}
