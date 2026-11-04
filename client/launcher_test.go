package main

// End-to-end Launcher tests: the local Server (server.go) and a temporary folder.
// Test doubles replace the emulator, the windows and the process detection.
// The scenarios follow one another: each one starts from the state that the previous one left.

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	tmp, url                            string
	responses, questions, notifications []string
	play                                func()
	played                              bool
	running                             int // running emulator processes
	launchedCommand                     []string
)

func reset() {
	responses, questions, notifications, play, played, running, launchedCommand = nil, nil, nil, nil, false, 0, nil
}

func init() {
	english = false // the checks read the French texts, whatever the language of the test machine
	ask = func(text string, buttons []string) string {
		questions = append(questions, text)
		if len(responses) == 0 {
			return buttons[len(buttons)-1] + " (question inattendue)"
		}
		r := responses[0]
		responses = responses[1:]
		return r
	}
	notify = func(text string) { notifications = append(notifications, text) }
	launch = func(c []string) error {
		played, launchedCommand = true, c
		if play != nil {
			play()
		}
		return nil
	}
	instances = func(string) int { return running }
}

// testClient returns a Client named name, on a test server, with its configuration folder in dir.
func testClient(srv *httptest.Server, dir, name string) *Client {
	cfg := &Config{URL: srv.URL}
	return (&Session{cfg: cfg, dir: dir, remote: &remote{cfg: cfg, dir: dir, http: srv.Client()}}).client(name)
}

// startTestServer starts the local Server (server.go) on tmp/data, at url. Its HTTP contract is the one of
// server/server.py: TestServerContract runs the Python suite against it.
func startTestServer(t *testing.T) {
	srv := httptest.NewServer(&Server{data: filepath.Join(tmp, "data"), token: "jeton-test"})
	t.Cleanup(srv.Close)
	url = srv.URL
}

// startPythonServer starts the real server/server.py on tmp/data, at url.
func startPythonServer(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	py := "python3"
	if runtime.GOOS == "windows" {
		py = "python"
	}
	// The real server, but on 127.0.0.1 like server/test_server.py, and without socket.getfqdn
	// (called by HTTPServer.server_bind): on the macOS CI machines, these name lookups
	// delayed the start by ~35 s per address (~70 s on 0.0.0.0, 2026-11-02).
	c := exec.Command(py, "-c", "import socket, server; socket.getfqdn = lambda nom='': nom; server.creer(('127.0.0.1', "+fmt.Sprint(port)+"), r'"+filepath.Join(tmp, "data")+"', 'jeton-test').serve_forever()")
	c.Dir = filepath.Join("..", "server")
	output, err := os.Create(filepath.Join(t.TempDir(), "serveur.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	c.Stdout, c.Stderr = output, output
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	go func() { c.Wait(); close(finished) }()
	t.Cleanup(func() { c.Process.Kill(); <-finished })
	url = fmt.Sprintf("http://127.0.0.1:%d", port)
	// The first Python started on a fresh Windows CI machine took more than 10 s (2026-11-02):
	// wait while the process lives, with a wide limit, and stop at once if it dies.
	// Short timeout per attempt: a connection with no response must not use up the whole limit.
	limit := time.After(time.Minute)
	for {
		r, err := (&http.Client{Timeout: time.Second}).Get(url + "/saves")
		if err == nil {
			r.Body.Close()
			return
		}
		select {
		case <-finished:
			t.Fatalf("serveur arrêté au démarrage : %v\n%s", c.ProcessState, read(output.Name()))
		case <-limit:
			t.Fatalf("serveur injoignable (%v)\n%s", err, read(output.Name()))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// api sends direct requests to the server, to play "the other Client".
func api(method, path string, body []byte, headers map[string]string) (int, []byte) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, url+path, r)
	req.Header.Set("Authorization", "Bearer jeton-test")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer rep.Body.Close()
	o, _ := io.ReadAll(rep.Body)
	return rep.StatusCode, o
}

func current(key string) []byte { _, o := api("GET", "/saves/"+key, nil, nil); return o }

func history(key string) (hashes []string) {
	_, o := api("GET", "/saves/"+key+"/history", nil, nil)
	var h []Version
	json.Unmarshal(o, &h)
	for _, v := range h {
		hashes = append(hashes, v.Hash)
	}
	return
}

func codes() string {
	_, o := api("GET", "/saves", nil, nil)
	var l []Version
	json.Unmarshal(o, &l)
	var c []string
	for _, v := range l {
		c = append(c, v.Code)
	}
	return strings.Join(c, ",")
}

func upload3ds(key string, buf, before []byte) int {
	cond := map[string]string{"If-None-Match": "*", "X-Client": "3ds"}
	if before != nil {
		cond = map[string]string{"If-Match": `"` + sha256Hex(before) + `"`, "X-Client": "3ds"}
	}
	st, _ := api("PUT", "/saves/"+key, buf, cond)
	return st
}

func byteRange(start, end int) []byte { // bytes from start to end, both included
	o := []byte{}
	for i := start; i <= end; i++ {
		o = append(o, byte(i))
	}
	return o
}

func random(n int, seed int64) []byte {
	o := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(o)
	return o
}

func write(t *testing.T, path string, o []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, o, 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(path string) []byte { o, _ := os.ReadFile(path); return o }

func romDS(t *testing.T, name, code string) string {
	o := make([]byte, 0x200)
	copy(o[0x0C:], code)
	c := filepath.Join(tmp, "roms", name+".nds")
	write(t, c, o)
	return c
}

// romZip writes a fake ROM in a .zip with a No-Intro name.
func romZip(t *testing.T, name, internal string, o []byte) string {
	c := filepath.Join(tmp, "roms", name+".zip")
	os.MkdirAll(filepath.Dir(c), 0o755)
	f, err := os.Create(c)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, _ := z.Create(internal)
	w.Write(o)
	z.Close()
	f.Close()
	return c
}

func gbHeader(title string, sum uint16) []byte {
	o := make([]byte, 0x150)
	copy(o[0x134:], title)
	o[0x14E], o[0x14F] = byte(sum>>8), byte(sum)
	return o
}

func gbaHeader(code string) []byte { o := make([]byte, 0xC0); copy(o[0xAC:], code); return o }

// rom3DS writes a fake 3DS ROM: NCSD, partition 0 at 0x200, program_id at NCCH + 0x118.
func rom3DS(t *testing.T, name string, id uint64) string {
	o := make([]byte, 0x400)
	copy(o[0x100:], "NCSD")
	binary.LittleEndian.PutUint32(o[0x120:], 1)
	binary.LittleEndian.PutUint64(o[0x200+0x118:], id)
	c := filepath.Join(tmp, "roms", name+".3ds")
	write(t, c, o)
	return c
}

func azaharDir(id uint64) string {
	z := strings.Repeat("0", 32)
	return filepath.Join(tmp, "azahar", "Nintendo 3DS", z, z, "title", fmt.Sprintf("%08x", id>>32), fmt.Sprintf("%08x", uint32(id)), "data", "00000001")
}

func writeConfig(t *testing.T, path string, c Config) string {
	o, _ := json.Marshal(c)
	write(t, path, o)
	return path
}

func namesIn(dir, pattern string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, pattern))
	return m
}

func contains(list []string, text string) bool {
	for _, s := range list {
		if strings.Contains(s, text) {
			return true
		}
	}
	return false
}

// raceTransport makes the 3DS upload just after the first GET /saves of the PC (GET/PUT race).
type raceTransport struct{ after func() }

func (c *raceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rep, err := http.DefaultTransport.RoundTrip(r)
	if c.after != nil && r.Method == "GET" && r.URL.Path == "/saves" {
		c.after()
		c.after = nil
	}
	return rep, err
}

func TestLauncher(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	checks := 0
	verify := func(ok bool, message string) {
		t.Helper()
		checks++
		if !ok {
			t.Errorf("ECHEC %s", message)
		}
	}
	saves, copies := filepath.Join(tmp, "saves"), filepath.Join(tmp, "copies")
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{
		URL: url, Token: "jeton-test", Copies: copies,
		Emulators: map[string]*Emulator{
			"melonds": {Exe: "inutilise.exe", Saves: map[System]string{"DS": saves}},
			"mgba":    {Exe: "inutilise.exe", Saves: map[System]string{"GB": filepath.Join(tmp, "gb"), "GBA": filepath.Join(tmp, "gba")}},
			"azahar":  {Exe: "inutilise.exe", Saves: map[System]string{"3DS": filepath.Join(tmp, "azahar")}},
		},
	})
	run := func(args ...string) int { return realMain(append([]string{"run", "--config", config}, args...)) }
	sav := func(name string) string { return filepath.Join(saves, name+".sav") }
	black := sav("Pokemon Noire")
	localHash := func() string { return sha256Hex(read(black)) }

	t.Log("Premier lancement : la Sauvegarde locale part sur le serveur")
	reset()
	rom := romDS(t, "Pokemon Noire", "IRBF")
	v1 := byteRange(1, 200)
	write(t, black, v1)
	verify(run(rom) == 0, "code de sortie 0")
	verify(played, "melonDS lancé")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v1), "le serveur a la Sauvegarde du PC")
	_, o := api("GET", "/saves", nil, nil)
	verify(strings.Contains(string(o), `"nom": "Pokemon Noire"`), "premier Envoi : le serveur connaît aussi le nom du jeu (ADR 0018)")
	verify(len(questions) == 0, "aucune question")
	verify(strings.Join(launchedCommand, "|") == "inutilise.exe|"+rom, "melonDS reçoit la ROM")

	t.Log("La partie modifie la Sauvegarde : Envoi après la partie")
	reset()
	v2 := byteRange(2, 201)
	play = func() { write(t, black, v2) }
	run(rom)
	verify(sha256Hex(current("IRBF")) == sha256Hex(v2), "le serveur a la Sauvegarde jouée")
	verify(len(history("IRBF")) == 2, "l'Historique garde les 2 Versions")
	_, o = api("GET", "/saves", nil, nil)
	verify(strings.Contains(string(o), `"code": "IRBF"`) && strings.Contains(string(o), `"nom": "Pokemon Noire"`),
		"le serveur connaît le nom du jeu (ADR 0018)")

	t.Log("La 3DS a joué : Récupération avec Copie de sécurité")
	reset()
	v3 := byteRange(3, 202)
	verify(upload3ds("IRBF", v3, v2) == 200, "la 3DS envoie")
	run(rom)
	verify(localHash() == sha256Hex(v3), "le PC a la Sauvegarde de la 3DS")
	c := namesIn(filepath.Join(copies, "melonds"), "*.sav")
	verify(len(c) == 1 && sha256Hex(read(c[0])) == sha256Hex(v2), "Copie de sécurité de l'ancienne Sauvegarde")
	verify(len(questions) == 0, "aucune question")

	t.Log("Conflit avant la partie, Ne pas jouer : rien ne bouge, pas de partie")
	reset()
	v4, v5 := byteRange(4, 203), byteRange(5, 204)
	write(t, black, v4)
	upload3ds("IRBF", v5, v3)
	responses = []string{"Ne pas jouer"}
	run(rom)
	verify(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "une question de Conflit")
	verify(!played, "melonDS non lancé")
	verify(localHash() == sha256Hex(v4), "le PC garde sa Sauvegarde")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v5), "le serveur garde la sienne")

	t.Log("Conflit avant la partie, Prendre celle du serveur")
	reset()
	responses = []string{"Prendre celle du serveur"}
	run(rom)
	verify(played, "melonDS lancé")
	verify(localHash() == sha256Hex(v5), "le PC a la Version du serveur")
	cp := false
	for _, f := range namesIn(filepath.Join(copies, "melonds"), "*.sav") {
		cp = cp || sha256Hex(read(f)) == sha256Hex(v4)
	}
	verify(cp, "Copie de sécurité de la Sauvegarde du PC")

	t.Log("Conflit avant la partie, Garder celle du PC : il envoie la sienne")
	reset()
	v6, v7 := byteRange(6, 205), byteRange(7, 206)
	write(t, black, v6)
	upload3ds("IRBF", v7, v5)
	responses = []string{"Garder celle du PC"}
	run(rom)
	verify(played, "melonDS lancé")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v6), "le serveur a la Sauvegarde du PC")
	verify(contains(history("IRBF"), sha256Hex(v7)), "la Version de la 3DS reste dans l'Historique")

	t.Log("La 3DS envoie pendant la partie (412), Ne rien faire : rien n'est écrasé")
	reset()
	v8, v9 := byteRange(8, 207), byteRange(9, 208)
	play = func() { upload3ds("IRBF", v8, v6); write(t, black, v9) }
	responses = []string{"Ne rien faire"}
	run(rom)
	verify(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "une question de Conflit après la partie")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v8), "le serveur garde la Version de la 3DS")
	verify(localHash() == sha256Hex(v9), "le PC garde la Sauvegarde jouée")

	t.Log("Conflit après la partie, Garder celle du PC : il envoie la sienne")
	reset()
	v10, v11 := byteRange(10, 209), byteRange(11, 210)
	play = func() { upload3ds("IRBF", v10, v8); write(t, black, v11) }
	responses = []string{"Prendre celle du serveur", "Garder celle du PC"}
	run(rom)
	verify(len(questions) == 2, "deux questions de Conflit")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v11), "le serveur a la Sauvegarde du PC")
	verify(contains(history("IRBF"), sha256Hex(v10)), "la Version de la 3DS reste dans l'Historique")

	t.Log("Sauvegarde tronquée pendant la partie : Conflit, rien n'est envoyé")
	reset()
	play = func() { write(t, black, byteRange(1, 100)) }
	responses = []string{"Ne rien faire"}
	run(rom)
	verify(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "une question de Conflit")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v11), "le serveur garde la Version entière")

	t.Log("Course : la 3DS envoie entre le GET et le PUT du PC (412)")
	reset()
	write(t, black, v11) // back to the synced state
	run(rom)
	v12, v13 := byteRange(12, 211), byteRange(13, 212)
	write(t, black, v12)
	reset()
	transport = &raceTransport{after: func() { upload3ds("IRBF", v13, v11) }}
	responses = []string{"Ne pas jouer"}
	run(rom)
	transport = http.DefaultTransport
	verify(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "le 412 devient une question de Conflit")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v13), "la Version de la 3DS n'est pas écrasée")
	verify(localHash() == sha256Hex(v12), "le PC garde sa Sauvegarde")

	t.Log("Conflit sans fenêtre possible (Q16) : rien n'est remplacé, partie, notification")
	reset()
	responses = []string{"", ""}
	run(rom)
	verify(played, "melonDS lancé")
	verify(contains(notifications, "rien n'a été remplacé"), "notification")
	verify(localHash() == sha256Hex(v12) && sha256Hex(current("IRBF")) == sha256Hex(v13), "rien n'a bougé")

	t.Log("melonDS ouvert hors de 3dssavesync : son jeu est inconnu, refus")
	reset()
	running = 1
	verify(run(rom) == 1, "code de sortie 1")
	verify(len(questions) == 0 && contains(notifications, "sans passer par 3dssavesync"), "notification, aucune fenêtre")
	verify(strings.Contains(string(read(filepath.Join(tmp, "cfg", "3dssavesync.log"))), "refus"), "le refus est écrit dans 3dssavesync.log")
	verify(!played, "melonDS non lancé")

	// Lock of another Launcher for this same emulator (live pid: the one of this test).
	lock := func(key string) string {
		f := filepath.Join(tmp, "cfg", "en-cours", fmt.Sprintf("inutilise.%d", os.Getpid()))
		write(t, f, []byte(key))
		return f
	}

	t.Log("Un autre jeu lancé par 3dssavesync tourne dans melonDS : partie possible (échange entre deux jeux)")
	reset()
	running = 1
	f := lock("XXXX")
	verify(run(rom) == 0 && played, "melonDS lancé à côté de l'autre jeu")
	verify(!exists(f), "le verrou de ce Lanceur est retiré après l'envoi")

	t.Log("Le même jeu tourne déjà, lancé par 3dssavesync : refus")
	reset()
	running = 1
	f = lock("IRBF")
	verify(run(rom) == 1 && !played && contains(notifications, "est déjà ouvert dans"), "refus du même jeu")
	os.Remove(f)

	t.Log("Verrou d'un Lanceur disparu (plantage) : retiré ; l'instance est alors inconnue, refus")
	reset()
	running = 1
	finished := exec.Command(os.Args[0], "-test.run=^$")
	if err := finished.Run(); err != nil {
		t.Fatal(err)
	}
	dead := filepath.Join(tmp, "cfg", "en-cours", fmt.Sprintf("inutilise.%d", finished.Process.Pid))
	write(t, dead, []byte("XXXX"))
	verify(run(rom) == 1 && !played && !exists(dead) && contains(notifications, "sans passer par 3dssavesync"), "verrou périmé retiré, refus")

	t.Log("Homebrew (code ####) : partie sans Synchronisation")
	reset()
	homebrew := romDS(t, "Homebrew", "####")
	write(t, sav("Homebrew"), byteRange(1, 10))
	before := codes()
	run(homebrew)
	verify(played, "melonDS lancé")
	verify(codes() == before && len(questions) == 0, "rien envoyé, aucune question")

	t.Log("ROM en .7z (LaunchBox, AutoExtract désactivé) : partie sans Synchronisation, une notification")
	reset()
	seven := filepath.Join(tmp, "roms", "Pokemon Noire.7z")
	write(t, seven, []byte("7z\xbc\xaf\x27\x1c"))
	before = codes()
	verify(run("--emulator", "melonds", seven) == 0 && played && launchedCommand[len(launchedCommand)-1] == seven, "melonDS lancé avec le .7z")
	verify(codes() == before && len(questions) == 0 && len(notifications) == 1 &&
		contains(notifications, "ne sait pas lire ce format de ROM (.7z)"), "rien envoyé, une seule notification")
	reset()
	verify(run(seven) == 1 && !played, "sans --emulator et plusieurs émulateurs : refus")

	t.Log("ROM en .zip : Code jeu et nom lus dans l'archive")
	reset()
	nds := make([]byte, 0x200)
	copy(nds[0x0C:], "IRAF")
	z := romZip(t, "Pokemon Blanche (archive)", "Pokemon Blanche.nds", nds)
	white := byteRange(20, 219)
	write(t, sav("Pokemon Blanche"), white)
	run(z)
	verify(sha256Hex(current("IRAF")) == sha256Hex(white), "le serveur a la Sauvegarde de Pokemon Blanche.sav")

	t.Log("Serveur injoignable avant la partie, Non : pas de partie")
	reset()
	outside := writeConfig(t, filepath.Join(tmp, "cfg-hors", "config.json"), Config{URL: "http://127.0.0.1:1", Token: "x",
		Emulators: map[string]*Emulator{"melonds": {Exe: "inutilise.exe", Saves: map[System]string{"DS": saves}}}})
	responses = []string{"Non"}
	realMain([]string{"run", "--config", outside, rom})
	verify(strings.Contains(strings.Join(questions, ""), "Le serveur refuse la connexion") && !strings.Contains(strings.Join(questions, ""), "dial tcp"),
		"message clair, sans le texte de Go : "+strings.Join(questions, ""))
	verify(len(questions) == 1 && strings.Contains(questions[0], "Synchronisation impossible"), "question « Synchronisation impossible »")
	verify(!played, "melonDS non lancé")

	t.Log("Serveur injoignable, Oui : partie, puis simple information")
	reset()
	responses = []string{"Oui"}
	play = func() { write(t, black, v13) }
	realMain([]string{"run", "--config", outside, rom})
	verify(played, "melonDS lancé")
	verify(len(questions) == 1 && contains(notifications, "reste sur le PC"), "notification après la partie, pas de fenêtre")
	verify(localHash() == sha256Hex(v13), "la Sauvegarde jouée reste sur le PC")

	t.Log("Copie de sécurité impossible : rien n'est écrasé")
	reset()
	blocked := filepath.Join(tmp, "copies-impossibles")
	write(t, blocked, []byte("un fichier, pas un dossier"))
	cfg2 := writeConfig(t, filepath.Join(tmp, "cfg2", "config.json"), Config{URL: url, Token: "jeton-test", Copies: blocked,
		Emulators: map[string]*Emulator{"melonds": {Exe: "inutilise.exe", Saves: map[System]string{"DS": filepath.Join(tmp, "saves2")}}}})
	platinum := romDS(t, "Pokemon Platine", "CPUF")
	p1, p2 := byteRange(30, 229), byteRange(31, 230)
	write(t, filepath.Join(tmp, "saves2", "Pokemon Platine.sav"), p1)
	realMain([]string{"run", "--config", cfg2, platinum}) // first Upload: Base = p1
	upload3ds("CPUF", p2, p1)
	reset()
	responses = []string{"Non"}
	realMain([]string{"run", "--config", cfg2, platinum})
	verify(sha256Hex(read(filepath.Join(tmp, "saves2", "Pokemon Platine.sav"))) == sha256Hex(p1), "la Sauvegarde du PC est intacte")
	verify(len(questions) == 1 && strings.Contains(questions[0], "Synchronisation impossible"), "question « Synchronisation impossible »")
	verify(!played, "melonDS non lancé")

	// --- GB / GBA (ADR 0007, 0008) ----------------------------------------------------------
	t.Log("GB : Clé lue dans l'en-tête, seule la SRAM part sur le serveur")
	reset()
	nameRed := "Pokemon - Version Rouge (France) (SGB Enhanced)"
	red := romZip(t, nameRed, nameRed+".gb", gbHeader("POKEMON RED", 0x91E6))
	savRed := filepath.Join(tmp, "gb", nameRed+".sav")
	sram1, clock := random(32768, 40), byteRange(1, 48)
	write(t, savRed, append(append([]byte{}, sram1...), clock...))
	run(red)
	cr := current("GB-POKEMON_RED-91E6")
	verify(len(cr) == 32768 && sha256Hex(cr) == sha256Hex(sram1), "le serveur a la SRAM seule (32 768 o), sous GB-POKEMON_RED-91E6")
	verify(played, "mGBA lancé")
	verify(strings.Join(launchedCommand, "|") == "inutilise.exe|-C|savegamePath="+filepath.Join(tmp, "gb")+"|"+red, "mGBA reçoit le dossier GB et la ROM")

	t.Log("GB : la Récupération garde le Trailer d'horloge du PC")
	reset()
	sram2 := random(32768, 41)
	upload3ds("GB-POKEMON_RED-91E6", sram2, sram1)
	run(red)
	l := read(savRed)
	verify(len(l) == 32816, "le fichier garde sa taille avec horloge (32 816 o)")
	verify(len(l) == 32816 && sha256Hex(l[:32768]) == sha256Hex(sram2), "la SRAM est celle de la 3DS")
	verify(len(l) == 32816 && bytes.Equal(l[32768:], clock), "le Trailer d'horloge du PC est intact")
	verify(len(namesIn(filepath.Join(copies, "mgba"), "*.sav")) == 1, "Copie de sécurité faite")
	verify(len(questions) == 0, "aucune question (horloge ignorée dans la comparaison)")

	t.Log("GBA : Clé GBA-<code>, .sav nommé comme le zip (règle mGBA)")
	reset()
	emuName := "Pokemon - Version Emeraude (France)"
	emerald := romZip(t, emuName, emuName+".gba", gbaHeader("BPEF"))
	g1 := random(131072, 42)
	write(t, filepath.Join(tmp, "gba", emuName+".sav"), append(append([]byte{}, g1...), byteRange(1, 16)...))
	run(emerald)
	verify(sha256Hex(current("GBA-BPEF")) == sha256Hex(g1), "le serveur a la SRAM d'Émeraude sous GBA-BPEF")

	t.Log("Jeu neuf, sauvegarde créée pendant la partie : le premier Envoi donne aussi le nom (ADR 0018)")
	reset()
	fresh := romZip(t, "Radical Red", "Radical Red.gba", gbaHeader("ZZRR"))
	play = func() { write(t, filepath.Join(tmp, "gba", "Radical Red.sav"), random(131072, 43)) }
	run(fresh)
	_, o = api("GET", "/saves", nil, nil)
	verify(strings.Contains(string(o), `"code": "GBA-ZZRR"`) && strings.Contains(string(o), `"nom": "Radical Red"`), "le serveur connaît le nom du jeu neuf")

	// --- 3DS (ADR 0009) -------------------------------------------------------------------
	t.Log("3DS : Clé et dossier Azahar lus dans la ROM")
	reset()
	x := rom3DS(t, "Pokemon X (Europe)", 0x0004000000055D00)
	dx := azaharDir(0x0004000000055D00)
	m1 := random(415232, 43)
	write(t, filepath.Join(dx, "main"), m1)
	run(x)
	verify(sha256Hex(current("3DS-0004000000055D00")) == sha256Hex(m1), "le serveur a main sous 3DS-0004000000055D00")
	verify(played && strings.Join(launchedCommand, "|") == "inutilise.exe|-f|"+x, "Azahar lancé en plein écran")

	t.Log("3DS : la Récupération n'ajoute rien dans l'archive Azahar")
	reset()
	m2 := random(415232, 44)
	upload3ds("3DS-0004000000055D00", m2, m1)
	run(x)
	verify(sha256Hex(read(filepath.Join(dx, "main"))) == sha256Hex(m2), "main est la Version de la 3DS")
	verify(strings.Join(namesIn(dx, "*"), ",") == filepath.Join(dx, "main"), "l'archive ne contient que main")
	verify(strings.Join(namesIn(filepath.Dir(dx), "*"), ",") == dx, "pas de fichier temporaire oublié")
	verify(len(namesIn(filepath.Join(copies, "azahar"), "*")) == 1, "Copie de sécurité hors de l'archive")

	t.Log("3DS : jeu jamais lancé dans Azahar, format connu : dossier, .metadata et main créés")
	reset()
	y := rom3DS(t, "Pokemon Y (Europe)", 0x0004000000055E00)
	upload3ds("3DS-0004000000055E00", m2, nil)
	run(y)
	dy := azaharDir(0x0004000000055E00)
	verify(len(questions) == 0 && played, "pas de question, partie lancée")
	verify(sha256Hex(read(filepath.Join(dy, "main"))) == sha256Hex(m2), "main est la Version du serveur")
	verify(bytes.Equal(read(dy+".metadata"), []byte{0, 0, 4, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}), "00000001.metadata écrit (16 octets)")
	verify(strings.Join(namesIn(dy, "*"), ",") == filepath.Join(dy, "main"), "l'archive ne contient que main")

	t.Log("3DS : jeu jamais lancé dans Azahar, format inconnu (titres_3ds) : rien n'est créé")
	reset()
	titles3DS["00040000000EC300"] = "Jeu a moi"
	defer delete(titles3DS, "00040000000EC300")
	other := rom3DS(t, "Jeu a moi", 0x00040000000EC300)
	upload3ds("3DS-00040000000EC300", m2, nil)
	responses = []string{"Non"}
	run(other)
	verify(len(questions) == 1 && strings.Contains(questions[0], "Azahar"), "question qui explique de lancer le jeu une fois")
	_, err := os.Stat(azaharDir(0x00040000000EC300))
	verify(err != nil, "aucun dossier créé dans Azahar")
	_, err = os.Stat(azaharDir(0x00040000000EC300) + ".metadata")
	verify(err != nil, "aucun .metadata créé")

	t.Log("3DS hors des 8 Pokémon : partie sans Synchronisation")
	reset()
	run(rom3DS(t, "Zelda (Europe)", 0x0004000000033500))
	verify(played && len(questions) == 0, "Azahar lancé, aucune question")

	// --- Blank Save = absent -------------------------------------------------------------------
	t.Log("Sauvegarde vierge (0xFF) sur le PC, Version sur le serveur : Récupération sans Conflit")
	reset()
	pearl := romDS(t, "Pokemon Perle", "APAF")
	write(t, sav("Pokemon Perle"), bytes.Repeat([]byte{0xFF}, 524288))
	pe := random(524288, 45)
	upload3ds("APAF", pe, nil)
	run(pearl)
	verify(len(questions) == 0, "aucune question de Conflit")
	verify(sha256Hex(read(sav("Pokemon Perle"))) == sha256Hex(pe), "le PC a la Version du serveur")
	verify(len(namesIn(filepath.Join(copies, "melonds"), "Pokemon Perle-*.sav")) == 1, "Copie de sécurité de la Sauvegarde vierge")

	t.Log("Sauvegarde vierge (0x00 + Trailer d'horloge), rien sur le serveur : rien n'est envoyé")
	reset()
	nameGold := "Pokemon - Version Or (France)"
	or := romZip(t, nameGold, nameGold+".gbc", gbHeader("POKEMON_GLDAAUF", 0x6FC6))
	write(t, filepath.Join(tmp, "gb", nameGold+".sav"), append(make([]byte, 32768), byteRange(1, 48)...))
	run(or)
	st, _ := api("GET", "/saves/GB-POKEMON_GLDAAUF-6FC6", nil, nil)
	verify(st == 404, "le serveur n'a rien reçu")
	verify(played, "mGBA lancé")

	t.Log("3DS : un main à 0x00 n'est pas une Sauvegarde vierge (il part)")
	reset()
	moon := rom3DS(t, "Pokemon Lune (Europe)", 0x0004000000175E00)
	write(t, filepath.Join(azaharDir(0x0004000000175E00), "main"), make([]byte, 1024))
	run(moon)
	st, _ = api("GET", "/saves/3DS-0004000000175E00", nil, nil)
	verify(st == 200, "le serveur a main")

	// --- Wrapped command, several emulators, launch ---------------------------------------
	t.Log("run --rom … -- <commande> : la commande quelconque est enveloppée")
	reset()
	v14 := byteRange(14, 213)
	play = func() { write(t, black, v14) } // before: v13 on the PC and on the server, In sync
	run("--rom", rom, "--", "lanceur.exe", "-rom", rom)
	verify(len(questions) == 0, "aucune question")
	verify(strings.Join(launchedCommand, "|") == "lanceur.exe|-rom|"+rom, "la commande donnée est lancée telle quelle")
	verify(sha256Hex(current("IRBF")) == sha256Hex(v14), "la Sauvegarde jouée part sur le serveur")

	t.Log("Deux émulateurs pour le même Système sans --emulator : erreur, rien n'est lancé")
	reset()
	double := writeConfig(t, filepath.Join(tmp, "cfg-double", "config.json"), Config{URL: url, Token: "jeton-test",
		Emulators: map[string]*Emulator{
			"melonds":  {Exe: "a.exe", Saves: map[System]string{"DS": saves}},
			"portable": {Profile: "melonds", Exe: "b.exe", Saves: map[System]string{"DS": filepath.Join(tmp, "saves-portable")}},
		}})
	responses = []string{"OK"}
	verify(realMain([]string{"run", "--config", double, rom}) == 1, "code de sortie 1")
	verify(len(questions) == 1 && strings.Contains(questions[0], "--emulator") && !played, "message qui demande --emulator")
	reset()
	write(t, filepath.Join(tmp, "saves-portable", "Pokemon Noire.sav"), v14)
	realMain([]string{"run", "--config", double, "--emulator", "portable", rom})
	verify(played && launchedCommand[0] == "b.exe", "--emulator choisit l'émulateur")
	verify(len(questions) == 0, "Client distinct : sa Base propre, sans Conflit")

	t.Log("launch : tous les jeux de l'émulateur avant et après, l'émulateur sans ROM")
	reset()
	savesL, romsL := filepath.Join(tmp, "saves-launch"), filepath.Join(tmp, "roms-launch")
	cfgL := writeConfig(t, filepath.Join(tmp, "cfg-launch", "config.json"), Config{URL: url, Token: "jeton-test",
		Emulators: map[string]*Emulator{"melonds": {Exe: "melon.exe", Saves: map[System]string{"DS": savesL}, Roms: map[System]string{"DS": romsL}}}})
	for name, code := range map[string]string{"Pokemon Diamant": "ADAF", "Pokemon HeartGold": "IPKF"} {
		o := make([]byte, 0x200)
		copy(o[0x0C:], code)
		write(t, filepath.Join(romsL, "sous-dossier", name+".nds"), o)
	}
	write(t, filepath.Join(romsL, "notes.txt"), []byte("pas une ROM"))
	diamond, hg := byteRange(40, 239), byteRange(41, 240)
	write(t, filepath.Join(savesL, "Pokemon Diamant.sav"), diamond)
	upload3ds("IPKF", hg, nil)
	diamond2 := byteRange(42, 241)
	play = func() {
		verify(sha256Hex(read(filepath.Join(savesL, "Pokemon HeartGold.sav"))) == sha256Hex(hg), "HeartGold récupéré avant la partie")
		verify(sha256Hex(current("ADAF")) == sha256Hex(diamond), "Diamant envoyé avant la partie")
		write(t, filepath.Join(savesL, "Pokemon Diamant.sav"), diamond2)
	}
	verify(realMain([]string{"launch", "--config", cfgL, "melonds"}) == 0, "code de sortie 0")
	verify(strings.Join(launchedCommand, "|") == "melon.exe", "melonDS lancé sans ROM")
	verify(sha256Hex(current("ADAF")) == sha256Hex(diamond2), "Diamant joué envoyé après la partie")
	verify(len(questions) == 0, "aucune question")

	// --- Profiles ------------------------------------------------------------------------------
	t.Log("RetroArch : dossier lu dans retroarch.cfg, .srm dans le sous-dossier du cœur, -L <cœur>")
	reset()
	ra := filepath.Join(tmp, "retroarch")
	write(t, filepath.Join(ra, "retroarch.cfg"), []byte("savefile_directory = \""+filepath.ToSlash(filepath.Join(tmp, "ra-saves"))+"\"\n"))
	cfgRA := writeConfig(t, filepath.Join(tmp, "cfg-ra", "config.json"), Config{URL: url, Token: "jeton-test",
		Emulators: map[string]*Emulator{"retroarch": {Exe: filepath.Join(ra, "retroarch.exe"), Cores: map[System]string{"GB": "gambatte_libretro.dll"}}}})
	yellow := filepath.Join(tmp, "roms-ra", "Pokemon Jaune.gb")
	write(t, yellow, gbHeader("POKEMON YELLOW", 0x1234))
	sj := random(32768, 50)
	upload3ds("GB-POKEMON_YELLOW-1234", sj, nil)
	realMain([]string{"run", "--config", cfgRA, yellow})
	verify(sha256Hex(read(filepath.Join(tmp, "ra-saves", "Gambatte", "Pokemon Jaune.srm"))) == sha256Hex(sj), "Version récupérée dans ra-saves/Gambatte/Pokemon Jaune.srm")
	verify(strings.Join(launchedCommand, "|") == filepath.Join(ra, "retroarch.exe")+"|-L|gambatte_libretro.dll|"+yellow, "RetroArch lancé avec le cœur et la ROM")
	verify(len(questions) == 0, "aucune question (pas d'état rechargé)")

	t.Log("mGBA autoload=1 : question après une Récupération, rien sinon")
	reset()
	mg := filepath.Join(tmp, "mgba-auto")
	write(t, filepath.Join(mg, "portable.ini"), nil)
	write(t, filepath.Join(mg, "config.ini"), []byte("[ports.qt]\nautoload=1\n"))
	cfgA := writeConfig(t, filepath.Join(tmp, "cfg-auto", "config.json"), Config{URL: url, Token: "jeton-test",
		Emulators: map[string]*Emulator{"mgba": {Exe: filepath.Join(mg, "mGBA.exe"), Saves: map[System]string{"GBA": filepath.Join(tmp, "gba-auto")}}}})
	sapphire := filepath.Join(tmp, "roms-auto", "Pokemon Saphir.gba")
	write(t, sapphire, gbaHeader("AXPF"))
	upload3ds("GBA-AXPF", random(131072, 51), nil)
	responses = []string{"Ne pas jouer"}
	realMain([]string{"run", "--config", cfgA, sapphire})
	verify(len(questions) == 1 && strings.Contains(questions[0], "autoload=0"), "question qui explique autoload")
	verify(!played, "« Ne pas jouer » : mGBA non lancé")
	reset()
	realMain([]string{"run", "--config", cfgA, sapphire})
	verify(played && len(questions) == 0, "Sauvegarde à jour : pas de question, mGBA lancé")
	verify(strings.Contains(string(read(filepath.Join(tmp, "cfg-auto", "3dssavesync.log"))), "avertissement : mGBA"), "l'avertissement est écrit dans le journal")

	t.Log("Émulateur personnalisé : commande avec {rom} et {sauvegardes} ; aucune sauvegarde après la partie : avertissement")
	reset()
	savesP := filepath.Join(tmp, "saves-perso")
	cfgP := writeConfig(t, filepath.Join(tmp, "cfg-perso", "config.json"), Config{URL: url, Token: "jeton-test",
		Emulators: map[string]*Emulator{"perso": {Exe: "perso.exe", Args: []string{"--batt={sauvegardes}", "{rom}"}, Extension: ".srm",
			Saves: map[System]string{"GBA": savesP}}}})
	ruby := filepath.Join(tmp, "roms-perso", "Pokemon Rubis.gba")
	write(t, ruby, gbaHeader("AXVF"))
	realMain([]string{"run", "--config", cfgP, ruby})
	verify(strings.Join(launchedCommand, "|") == "perso.exe|--batt="+savesP+"|"+ruby, "commande personnalisée")
	verify(contains(notifications, filepath.Join(savesP, "Pokemon Rubis.srm")), "avertissement : aucune sauvegarde après la partie")
	reset()
	play = func() { write(t, filepath.Join(savesP, "Pokemon Rubis.srm"), random(131072, 60)) }
	realMain([]string{"run", "--config", cfgP, ruby})
	verify(len(notifications) == 0 && sha256Hex(current("GBA-AXVF")) == sha256Hex(read(filepath.Join(savesP, "Pokemon Rubis.srm"))),
		"sauvegarde écrite pendant la partie : envoyée, aucun avertissement")

	t.Logf("%d vérifications", checks)
}

// TestSRAM uses the same sizes as sram_size in 3ds/test/test_key.c (ADR 0010).
func TestSRAM(t *testing.T) {
	for _, c := range []struct {
		system     System
		size, sram int
	}{
		{"GB", 32816, 32768}, {"GB", 32812, 32768}, {"GB", 32768, 32768}, {"GBA", 131088, 131072},
		{"GB", 512, 512}, {"GBA", 528, 512}, {"GB", 560, 512}, {"GBA", 8208, 8192},
		{"GB", 1536, 1536}, {"GB", 48, 48}, {"DS", 524298, 524298},
	} {
		f := filepath.Join(t.TempDir(), "s.sav")
		write(t, f, make([]byte, c.size))
		l, err := readLocal(&Game{System: c.system, File: f}, nil)
		if err != nil || len(l.SRAM) != c.sram || len(l.Raw) != c.size {
			t.Errorf("%s %d : SRAM %d attendue (%v, %v)", c.system, c.size, c.sram, l, err)
		}
	}
}

// TestClassify maps network, server and file errors to a text for the user; global = one message for the pass.
func TestClassify(t *testing.T) {
	type tcase struct {
		err    error
		text   string
		global bool
	}
	transport := func(err error) error { return &neturl.Error{Op: "Get", URL: "https://x/saves", Err: err} }
	l := []tcase{
		{&httpError{"GET /saves", 401}, "Le serveur refuse ce jeton. Recopiez le jeton exact du serveur.", true},
		{&httpError{"GET /saves", 404}, "Cette adresse ne correspond pas à un serveur 3dssavesync. Vérifiez-la.", true},
		{&httpError{"PUT /saves/IRBF", 413}, "sauvegarde trop grosse pour le serveur", false},
		{&httpError{"PUT /saves/IRBF", 400}, "le serveur a refusé cette sauvegarde", false},
		{&httpError{"GET /saves", 502}, "Le serveur a rencontré une erreur.", true},
		{&httpError{"GET /saves", 418}, "Le serveur a répondu de façon inattendue.", true},
		{transport(&net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}), "Serveur introuvable.", true},
		{transport(&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}), "Le serveur refuse la connexion", true},
		{transport(os.ErrDeadlineExceeded), "Le serveur ne répond pas. Vérifiez votre connexion Internet", true},
		{transport(io.ErrUnexpectedEOF), "Le serveur ne répond pas. Vérifiez votre connexion Internet", true},
		{transport(&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}), "Le certificat du serveur n'est pas valide.", true},
		{fmt.Errorf("copie de sécurité impossible : %w", &fs.PathError{Op: "write", Path: "c.sav", Err: syscall.ENOSPC}), "disque plein", false},
		{&fs.PathError{Op: "open", Path: "x/a.sav", Err: fs.ErrPermission}, "accès refusé à x/a.sav", false},
		{errors.New("jeu jamais lancé dans Azahar"), "jeu jamais lancé dans Azahar", false},
	}
	if runtime.GOOS == "windows" {
		l = append(l,
			tcase{&fs.PathError{Op: "open", Path: "a.sav", Err: syscall.Errno(32)}, "un autre programme utilise ce fichier. Fermez l'émulateur", false},
			tcase{&os.LinkError{Op: "rename", Old: "a.tmp", New: "a.sav", Err: syscall.Errno(112)}, "disque plein", false},
			tcase{transport(&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connectex", syscall.Errno(10061))}), "Le serveur refuse la connexion", true})
	}
	for _, c := range l {
		if text, global := classify(c.err); !strings.Contains(text, c.text) || global != c.global {
			t.Errorf("%v : %q (global %v), attendu %q (global %v)", c.err, text, global, c.text, c.global)
		}
	}
	if errorText(nil) != "" {
		t.Error("sans erreur : texte vide attendu")
	}

	// Frontend that starts a game before the setup: no "The system cannot find the file specified".
	reset()
	realMain([]string{"run", "--config", filepath.Join(t.TempDir(), "absent", "config.json"), "x.nds"})
	if len(questions) != 1 || !strings.Contains(questions[0], "3dssavesync n'est pas encore configuré. Ouvrez « 3dssavesync »") ||
		!strings.Contains(questions[0], helpPage) {
		t.Errorf("configuration absente : %q", questions)
	}
}

// The English flag names are the documented ones; the French names of the first versions set the same value but stay
// out of the help.
func TestHiddenFlagAliases(t *testing.T) {
	fl := flag.NewFlagSet("setup", flag.ContinueOnError)
	text := fl.Bool("text", false, "text setup")
	yes := fl.Bool("yes", false, "no question")
	hiddenAliases(fl, map[string]string{"texte": "text", "oui": "yes"})
	var help strings.Builder
	fl.SetOutput(&help)
	if err := fl.Parse([]string{"--texte", "-yes", "rom"}); err != nil || !*text || !*yes || fl.Arg(0) != "rom" {
		t.Fatalf("--texte and -yes expected to set text and yes: %v %v %v %q", err, *text, *yes, fl.Args())
	}
	fl.Usage()
	if h := help.String(); !strings.Contains(h, "-text") || !strings.Contains(h, "-yes") || strings.Contains(h, "texte") ||
		strings.Contains(h, "oui") {
		t.Errorf("help with the English names only expected:\n%s", h)
	}
}
