package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func init() {
	detectCards = func() []string { return nil } // jamais une vraie carte SD
}

// config.ini de la 3DS : url et jeton remplacés, les autres réglages gardés, copie de l'ancien, rien d'autre de touché.
func TestPrepareCard(t *testing.T) {
	if got := configIni("roms=sdmc:/roms/nds\r\nurl=https://ancien\r\njeton=vieux\r\nsaves=\r\n", "http://192.0.2.5:8765", "abc"); got !=
		"url=http://192.0.2.5:8765\njeton=abc\nroms=sdmc:/roms/nds\nsaves=\n" {
		t.Errorf("configIni : %q", got)
	}

	sd := t.TempDir()
	if _, err := prepareCard(sd, "http://192.0.2.5:8765", "abc", nil); err == nil || !strings.Contains(err.Error(), "Nintendo 3DS") {
		t.Errorf("sans dossier Nintendo 3DS : erreur attendue, %v", err)
	}
	if _, err := prepareCard("relatif", "http://192.0.2.5:8765", "abc", nil); err == nil {
		t.Error("chemin relatif : erreur attendue")
	}
	if exists(filepath.Join(sd, "3ds")) {
		t.Fatal("rien ne doit être écrit sur une carte refusée")
	}

	os.Mkdir(filepath.Join(sd, "Nintendo 3DS"), 0o755)
	f, err := prepareCard(`"`+sd+`" `, "http://192.0.2.5:8765", "abc", nil)
	if err != nil || f != filepath.Join(sd, "3ds", "3dssavesync", "config.ini") || string(read(f)) != "url=http://192.0.2.5:8765\njeton=abc\n" {
		t.Fatalf("carte neuve : %s %v", f, err)
	}

	write(t, f, []byte("url=https://ancien\njeton=vieux\nroms=sdmc:/jeux\nsaves_gba=sdmc:/gba\n"))
	write(t, filepath.Join(sd, "3ds", "3dssavesync", "base.ini"), []byte("ADAF=1234\n"))
	if _, err := prepareCard(sd, "http://192.0.2.5:8765", "abc", nil); err != nil {
		t.Fatal(err)
	}
	if string(read(f)) != "url=http://192.0.2.5:8765\njeton=abc\nroms=sdmc:/jeux\nsaves_gba=sdmc:/gba\n" {
		t.Errorf("config.ini gardé : %q", string(read(f)))
	}
	copies, _ := filepath.Glob(f + ".avant-3dssavesync-*")
	if len(copies) != 1 || string(read(copies[0])) != "url=https://ancien\njeton=vieux\nroms=sdmc:/jeux\nsaves_gba=sdmc:/gba\n" {
		t.Errorf("copie de l'ancien config.ini : %v", copies)
	}
	if string(read(filepath.Join(sd, "3ds", "3dssavesync", "base.ini"))) != "ADAF=1234\n" {
		t.Error("base.ini ne doit pas changer")
	}
	entries, _ := os.ReadDir(filepath.Dir(f))
	if len(entries) != 3 { // config.ini, sa copie, base.ini : pas de fichier temporaire laissé
		t.Errorf("fichiers du dossier : %v", entries)
	}

	// Dossier d'avant la v3 sans le nouveau : écrit là, le homebrew le reprend avec base.ini.
	sd2 := t.TempDir()
	os.Mkdir(filepath.Join(sd2, "Nintendo 3DS"), 0o755)
	old := filepath.Join(sd2, "3ds", "nds-save-sync", "config.ini")
	write(t, old, []byte("url=https://ancien\nroms=sdmc:/x\n"))
	if f, err := prepareCard(sd2, "https://saves.example.com", "j", nil); err != nil || f != old || string(read(old)) != "url=https://saves.example.com\njeton=j\nroms=sdmc:/x\n" ||
		exists(filepath.Join(sd2, "3ds", "3dssavesync")) {
		t.Errorf("ancien dossier : %s %v", f, err)
	}

	if got := cards3DS([]string{sd, sd2, t.TempDir()}); !slices.Equal(got, []string{sd, sd2}) {
		t.Errorf("cartes3DS : %v", got)
	}

	// Console : la carte choisie par son numéro.
	sd3 := t.TempDir()
	os.Mkdir(filepath.Join(sd3, "Nintendo 3DS"), 0o755)
	before := detectCards
	t.Cleanup(func() { detectCards = before })
	detectCards = func() []string { return []string{sd3} }
	var output bytes.Buffer
	config := filepath.Join(t.TempDir(), "config.json")
	writeJSON(config, &Config{URL: "https://saves.example.com", Token: "j"}, false)
	sdCardStep(&Questions{in: bufio.NewReader(strings.NewReader("1\n")), out: &output}, &Config{URL: "https://saves.example.com", Token: "j"}, config)
	if !strings.Contains(output.String(), "config.ini écrit") || string(read(filepath.Join(sd3, "3ds", "3dssavesync", "config.ini"))) != "url=https://saves.example.com\njeton=j\n" {
		t.Errorf("assistant en console :\n%s", output.String())
	}
}

// Page web : config.ini à télécharger, carte préparée, serveur local proposé au premier lancement.
func TestWebCard(t *testing.T) {
	fakeHome(t)
	detectBefore := detect
	t.Cleanup(func() { detect = detectBefore })
	detect = func(string) []Found { return nil }
	config := filepath.Join(t.TempDir(), "config.json")
	a := &WebSetup{config: config, secret: "s", host: "127.0.0.1:9", end: make(chan struct{})}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:9/s/"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	var state WebState
	json.Unmarshal(request("POST", "api/etat", "{}").Body.Bytes(), &state)
	if !state.Local || state.Address3DS != "" {
		t.Errorf("premier lancement : serveur local attendu, %+v", state)
	}
	if request("GET", "config.ini", "").Code != 404 {
		t.Error("config.ini sans serveur : 404 attendu")
	}

	writeJSON(config, &Config{URL: "https://saves.example.com", Token: "j"}, false)
	json.Unmarshal(request("POST", "api/etat", "{}").Body.Bytes(), &state)
	if state.Local || state.Address3DS != "https://saves.example.com" {
		t.Errorf("serveur personnel gardé : %+v", state)
	}
	if w := request("GET", "config.ini", ""); w.Body.String() != "url=https://saves.example.com\njeton=j\n" || !strings.Contains(w.Header().Get("Content-Disposition"), "config.ini") {
		t.Errorf("config.ini téléchargé : %q", w.Body.String())
	}

	sd := t.TempDir()
	os.Mkdir(filepath.Join(sd, "Nintendo 3DS"), 0o755)
	body, _ := json.Marshal(map[string]string{"Racine": sd})
	var r struct {
		Ok      bool
		Message string
	}
	json.Unmarshal(request("POST", "api/preparer", string(body)).Body.Bytes(), &r)
	if !r.Ok || string(read(filepath.Join(sd, "3ds", "3dssavesync", "config.ini"))) != "url=https://saves.example.com\njeton=j\n" {
		t.Errorf("carte préparée : %+v", r)
	}
}

// Adresses pour la 3DS : IPv4 privées, sans doublon, celle de la box en tête.
func TestLocalAddresses(t *testing.T) {
	// VPN sur la route par défaut, WSL, VirtualBox, box en 192.168 : la box d'abord.
	if got := sortAddresses([]string{"172.20.0.1", "10.8.0.6", "192.168.56.1", "192.168.0.42"}, "10.8.0.6"); !slices.Equal(got,
		[]string{"192.168.0.42", "10.8.0.6", "192.168.56.1", "172.20.0.1"}) {
		t.Errorf("VPN : %v", got)
	}
	// Box en 10.x sans VPN : la route par défaut d'abord, VirtualBox après.
	if got := sortAddresses([]string{"192.168.56.1", "10.0.0.7", "172.17.0.1"}, "10.0.0.7"); !slices.Equal(got,
		[]string{"10.0.0.7", "192.168.56.1", "172.17.0.1"}) {
		t.Errorf("box en 10.x : %v", got)
	}
	l := localAddresses()
	for i, a := range l {
		if ip := net.ParseIP(a); ip == nil || ip.To4() == nil || !ip.IsPrivate() || slices.Contains(l[:i], a) {
			t.Errorf("adresse %q dans %v", a, l)
		}
	}
}

// alias.ini de la 3DS : les alias du PC remplacent ou complètent les lignes, les autres restent.
func TestAliasIni(t *testing.T) {
	old := "Hack 3DS seul=GBA-0AAA\r\nRadical Red (4.10)=GBA-0OLD\r\n"
	got := aliasIni(old, map[string]string{"Radical Red (4.10)": "GBA-0RAD", "Unbound": "GBA-0UNB"})
	if want := "Hack 3DS seul=GBA-0AAA\nRadical Red (4.10)=GBA-0RAD\nUnbound=GBA-0UNB\n"; got != want {
		t.Errorf("alias.ini :\n%q\nattendu\n%q", got, want)
	}
	sd := t.TempDir()
	os.MkdirAll(filepath.Join(sd, "Nintendo 3DS"), 0o755)
	if _, err := prepareCard(sd, "http://192.0.2.5:8765", "abc", map[string]string{"Radical Red (4.10)": "GBA-0RAD"}); err != nil {
		t.Fatal(err)
	}
	if o, _ := os.ReadFile(filepath.Join(sd, "3ds", "3dssavesync", "alias.ini")); string(o) != "Radical Red (4.10)=GBA-0RAD\n" {
		t.Errorf("alias.ini écrit : %q", o)
	}
}

// 3dssavesync.cia sur la carte : écrit, déjà là, ancien copié avant, place libre, FBI absent, .cia absent de ce
// programme ; la date de préparation notée seulement après une préparation réussie.
func TestCopyCia(t *testing.T) {
	ea, ca := freeSpace, embeddedCia
	t.Cleanup(func() { freeSpace, embeddedCia = ea, ca })
	freeSpace = func(string) (uint64, error) { return 1 << 40, nil }
	sd := t.TempDir()
	os.Mkdir(filepath.Join(sd, "Nintendo 3DS"), 0o755)
	cia := filepath.Join(sd, "cias", "3dssavesync.cia")

	l, err := copyCia(sd, nil)
	if err != nil || len(l) != 2 || !strings.Contains(l[0], "ne contient pas 3dssavesync.cia") || !strings.Contains(l[0], "releases/latest") ||
		!strings.Contains(l[1], "FBI n'est pas sur la carte") || exists(filepath.Join(sd, "cias")) {
		t.Errorf("sans .cia dans le programme : %v %q", err, l)
	}
	if l, err := copyCia(sd, []byte("cia 1")); err != nil || len(l) != 2 || l[0] != "3dssavesync.cia dev copié : "+cia || string(read(cia)) != "cia 1" {
		t.Errorf("carte neuve : %v %q", err, l)
	}
	write(t, filepath.Join(sd, "3ds", "FBI.3dsx"), []byte("fbi"))
	if l, err := copyCia(sd, []byte("cia 1")); err != nil || len(l) != 1 || !strings.Contains(l[0], "déjà sur la carte") {
		t.Errorf("même .cia, FBI présent : %v %q", err, l)
	}
	freeSpace = func(string) (uint64, error) { return 1 << 20, nil }
	if _, err := copyCia(sd, []byte("cia 2")); err == nil || !strings.Contains(err.Error(), "Pas assez de place sur la carte SD") || string(read(cia)) != "cia 1" {
		t.Errorf("carte pleine : %v", err)
	}
	freeSpace = func(string) (uint64, error) { return 0, errors.New("inconnu") } // place inconnue : la copie est tentée
	if _, err := copyCia(sd, []byte("cia 2")); err != nil || string(read(cia)) != "cia 2" {
		t.Errorf("nouvelle version : %v", err)
	}
	if c, _ := filepath.Glob(cia + ".avant-3dssavesync-*"); len(c) != 1 || string(read(c[0])) != "cia 1" {
		t.Errorf("copie datée de l'ancien .cia : %v", c)
	}

	errno := syscall.EROFS
	if runtime.GOOS == "windows" {
		errno = syscall.Errno(19) // ERROR_WRITE_PROTECT
	}
	if e := cardError(&fs.PathError{Op: "open", Path: cia, Err: errno}); !strings.HasPrefix(e, "La carte SD est verrouillée") || !strings.Contains(e, "\nDétail : open "+cia) {
		t.Errorf("carte verrouillée : %q", e)
	}

	config := filepath.Join(t.TempDir(), "config.json")
	writeJSON(config, &Config{URL: "https://saves.example.com", Token: "j"}, false)
	var cfg Config
	if ok, l := prepareAndRecord(config, t.TempDir()); ok || len(l) != 1 || !strings.Contains(l[0], "Nintendo 3DS") || readJSON(config, &cfg) != nil || cfg.PreparedCard != "" {
		t.Errorf("carte refusée : %v %q %+v", ok, l, cfg)
	}
	embeddedCia = []byte("cia 3")
	ok, l := prepareAndRecord(config, sd)
	readJSON(config, &cfg)
	if !ok || len(l) != 3 || !strings.HasPrefix(l[0], "config.ini écrit") || !strings.HasPrefix(l[1], "3dssavesync.cia dev copié") ||
		!strings.Contains(l[2], "Dans FBI, ouvrez SD, cias, 3dssavesync.cia et choisissez « Install and delete CIA »") || string(read(cia)) != "cia 3" || len(cfg.PreparedCard) != 16 {
		t.Errorf("carte préparée : %v %q %+v", ok, l, cfg)
	}

	// La 3DS s'est mise à jour seule (installee= de maj.ini) : un .cia plus ancien n'est pas copié, config.ini l'est.
	va := version
	t.Cleanup(func() { version = va })
	version, embeddedCia = "3.7.0", []byte("cia 3.7.0")
	write(t, filepath.Join(sd, "3ds", "3dssavesync", "maj.ini"), []byte("jour=20000\r\nversion=3.8.0\r\ninstallee=3.8.0\r\n"))
	ok, l = prepareAndRecord(config, sd)
	if !ok || len(l) != 3 || !strings.HasPrefix(l[0], "config.ini écrit") ||
		l[1] != "La 3DS a déjà une version plus récente de 3dssavesync (3.8.0). Inutile d'installer la version 3.7.0." ||
		strings.Contains(l[2], "FBI") || string(read(cia)) != "cia 3" {
		t.Errorf("3DS plus récente : %v %q", ok, l)
	}
	write(t, filepath.Join(sd, "3ds", "3dssavesync", "maj.ini"), []byte("jour=20000\nversion=3.8.0\ninstallee=3.6.1\n"))
	if l, err := copyCia(sd, embeddedCia); err != nil || l[0] != "3dssavesync.cia 3.7.0 copié : "+cia || string(read(cia)) != "cia 3.7.0" {
		t.Errorf("3DS plus ancienne (même si GitHub a plus récent) : %v %q", err, l)
	}
}
