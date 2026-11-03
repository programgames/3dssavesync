package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pokemonX builds a fake Pokémon X save (size 0x65600): play time and trainer at the offsets of pokemon.go.
func pokemonX(hours int, trainer string) []byte {
	d := make([]byte, 0x65600)
	le.PutUint16(d[0x1800:], uint16(hours))
	d[0x1802] = 30
	for i, c := range trainer {
		le.PutUint16(d[0x14048+2*i:], uint16(c))
	}
	return d
}

// uploadFromOther uploads from another device (Client "3ds").
func uploadFromOther(t *testing.T, s *Session, key string, o, before []byte) {
	t.Helper()
	other := *s
	other.profile = "3ds"
	cond := map[string]string{"If-None-Match": "*"}
	if before != nil {
		cond = map[string]string{"If-Match": `"` + sha256Hex(before) + `"`}
	}
	if st, _, err := other.api("PUT", "/saves/"+key, o, cond); st != 200 || err != nil {
		t.Fatalf("envoi %s : %d %v", key, st, err)
	}
}

func currentOf(s *Session, key string) []byte {
	_, o, _ := s.api("GET", "/saves/"+key, nil, nil)
	return o
}

// Restore against both servers: the local server (server.go) and server.py.
func TestRestore(t *testing.T) {
	local := httptest.NewServer(&Server{data: t.TempDir(), token: "jeton-test"})
	defer local.Close()
	t.Run("serveur local", func(t *testing.T) { checkRestore(t, local.URL) })
	t.Run("server.py", func(t *testing.T) {
		tmp = t.TempDir()
		startTestServer(t)
		checkRestore(t, url)
	})
}

func checkRestore(t *testing.T, address string) {
	config := writeConfig(t, filepath.Join(t.TempDir(), "config.json"), Config{URL: address, Token: "jeton-test"})
	s, err := restoreSession(config)
	if err != nil {
		t.Fatal(err)
	}
	key := "3DS-0004000000055D00"
	v1, v2, v3 := pokemonX(10, "Sacha"), pokemonX(152, "Sacha"), pokemonX(160, "Ondine")
	uploadFromOther(t, s, key, v1, nil)
	uploadFromOther(t, s, key, v2, v1)

	lines, err := s.historyLines(key)
	if err != nil || len(lines) != 2 {
		t.Fatalf("liste : %v %+v", err, lines)
	}
	if l := lines[0]; l.N != 2 || !l.Current || l.Client != "3ds" || l.Size != "406 Kio" || l.PlayInfo != "152 h 30 · Sacha" ||
		!regexp.MustCompile(`^\d\d/\d\d/\d{4} \d\d:\d\d$`).MatchString(l.Date) {
		t.Errorf("version la plus récente d'abord : %+v", l)
	}
	if l := lines[1]; l.N != 1 || l.Current || l.PlayInfo != "10 h 30 · Sacha" {
		t.Errorf("version 1 : %+v", l)
	}

	if already, err := s.restore(key, lines[0].File, lines[0].Hash); !already || err != nil {
		t.Errorf("même contenu que la courante : rien à faire attendu (%v %v)", already, err)
	}
	if n, _ := s.historyLines(key); len(n) != 2 {
		t.Errorf("même contenu : aucune nouvelle Version attendue (%d)", len(n))
	}

	uploadFromOther(t, s, key, v3, v2) // a device uploads after the list was shown
	if _, err := s.restore(key, lines[1].File, lines[0].Hash); err == nil || !strings.Contains(err.Error(), "vient de changer sur le serveur") {
		t.Errorf("Envoi concurrent : refus attendu (%v)", err)
	}
	if !bytes.Equal(currentOf(s, key), v3) {
		t.Error("Envoi concurrent : la Version de l'appareil doit rester courante")
	}

	lines, _ = s.historyLines(key)
	if already, err := s.restore(key, lines[2].File, lines[0].Hash); already || err != nil {
		t.Fatalf("restauration de la version 1 : %v %v", already, err)
	}
	lines, _ = s.historyLines(key)
	if len(lines) != 4 || lines[0].N != 4 || lines[0].Hash != sha256Hex(v1) || lines[0].Client != "restauration" || !bytes.Equal(currentOf(s, key), v1) {
		t.Errorf("la version 1 doit être la Version courante n° 4, envoyée par « restauration » : %+v", lines)
	}
	for _, d := range []string{"bases", "copies"} { // no Base and no Local save touched
		if _, err := os.Stat(filepath.Join(filepath.Dir(config), d)); err == nil {
			t.Errorf("restauration : %s ne doit pas être créé", d)
		}
	}
	if _, err := s.restore(key, "../../server.py", lines[0].Hash); err == nil {
		t.Error("nom de version hors de l'Historique : refus attendu")
	}
}

func TestRestoreCommand(t *testing.T) {
	srv := httptest.NewServer(&Server{data: t.TempDir(), token: "j"})
	defer srv.Close()
	config := writeConfig(t, filepath.Join(t.TempDir(), "config.json"), Config{URL: srv.URL, Token: "j"})
	s, _ := restoreSession(config)
	uploadFromOther(t, s, "ADAF", []byte("un"), nil)
	uploadFromOther(t, s, "ADAF", []byte("deux"), []byte("un"))
	command := func(entry string, args ...string) (string, error) {
		var output bytes.Buffer
		_, err := cmdRestore(config, args, false, &Questions{in: bufio.NewReader(strings.NewReader(entry)), out: &output})
		return output.String(), err
	}
	if o, err := command(""); err != nil || !strings.Contains(o, "ADAF") {
		t.Errorf("liste des jeux : %v\n%s", err, o)
	}
	if o, err := command("", "adaf"); err != nil || !strings.Contains(o, "   2  ") || !strings.Contains(o, "(actuelle)") || !strings.Contains(o, "   1  ") {
		t.Errorf("liste des versions : %v\n%s", err, o)
	}
	if o, _ := command("n\n", "ADAF", "1"); !strings.Contains(o, "Rien n'a été modifié.") || string(currentOf(s, "ADAF")) != "deux" {
		t.Errorf("réponse non : rien ne change\n%s", o)
	}
	if o, _ := command("", "ADAF", "2"); !strings.Contains(o, "identique à la version actuelle") {
		t.Errorf("version actuelle : rien à faire\n%s", o)
	}
	if o, err := command("", "ADAF", "1", "--oui"); err != nil || !strings.Contains(o, "Version restaurée") || string(currentOf(s, "ADAF")) != "un" {
		t.Errorf("--oui : restauration sans question attendue : %v\n%s", err, o)
	}
	if _, err := command("", "ADAF", "9"); err == nil {
		t.Error("numéro inconnu : erreur attendue")
	}
}

// Page: games (name read in the ROM folders), Versions, Restore, refusal after a concurrent Upload.
func TestWebSetupHistory(t *testing.T) {
	srv := httptest.NewServer(&Server{data: t.TempDir(), token: "j"})
	defer srv.Close()
	roms := t.TempDir()
	rom := make([]byte, 0x200)
	copy(rom[0x0C:], "CPUF")
	write(t, filepath.Join(roms, "Pokemon Platine.nds"), rom)
	config := writeConfig(t, filepath.Join(t.TempDir(), "config.json"), Config{URL: srv.URL, Token: "j",
		Emulators: map[string]*Emulator{"melonds": {Exe: "melonDS.exe", Roms: map[string]string{"DS": roms}}}})
	s, _ := restoreSession(config)
	uploadFromOther(t, s, "CPUF", []byte("un"), nil)
	uploadFromOther(t, s, "CPUF", []byte("deux"), []byte("un"))

	a := &WebSetup{config: config, secret: "s", host: "127.0.0.1:9", end: make(chan struct{})}
	request := func(path, body string, v any) {
		r := httptest.NewRequest("POST", "http://127.0.0.1:9/s/api/"+path, strings.NewReader(body))
		r.Host = "127.0.0.1:9"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		json.Unmarshal(w.Body.Bytes(), v)
	}
	var games struct {
		Games []HistoryGame `json:"Jeux"`
	}
	request("jeux", "{}", &games)
	if len(games.Games) != 1 || games.Games[0] != (HistoryGame{"CPUF", "Pokemon Platine"}) {
		t.Errorf("jeux : %+v", games)
	}
	var v struct{ Versions []HistoryLine }
	request("versions", `{"Cle":"CPUF"}`, &v)
	if len(v.Versions) != 2 || !v.Versions[0].Current {
		t.Fatalf("versions : %+v", v)
	}
	var r struct {
		Ok      bool
		Message string
	}
	body, _ := json.Marshal(map[string]string{"Cle": "CPUF", "Fichier": v.Versions[1].File, "Courant": v.Versions[0].Hash})
	request("restaurer", string(body), &r)
	if !r.Ok || !strings.Contains(r.Message, "Version restaurée") || string(currentOf(s, "CPUF")) != "un" {
		t.Errorf("restauration : %+v", r)
	}
	request("restaurer", string(body), &r) // same list, but the Current version changed since
	if r.Ok || !strings.Contains(r.Message, "vient de changer") {
		t.Errorf("liste périmée : refus attendu, %+v", r)
	}
}
