package main

// ROM hacks and Aliases (ADR 0014), pending Uploads, one read of the server list per pass.
// Same test doubles as launcher_test.go; the real server.

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAliasKey(t *testing.T) {
	c := &Config{Alias: map[string]string{"Hack DS": "0rad", "Hack GBA": " GBA-0RAD ", "Hack GB": "GB-MONHACK-0001",
		"Trop long": "GBA-BPRE-RADICALRED", "Mauvais Système": "0RAD"}}
	for _, k := range []struct{ system, name, key, expected string }{
		{"DS", "Hack DS", "ADAF", "0RAD"},
		{"GBA", "Hack GBA", "GBA-BPRE", "GBA-0RAD"},
		{"GB", "Hack GB", "GB-POKEMON_RED-91E6", "GB-MONHACK-0001"},
		{"GBA", "Sans alias", "GBA-BPRE", "GBA-BPRE"},
		{"GBA", "Trop long", "GBA-BPRE", ""},
		{"GBA", "Mauvais Système", "GBA-BPRE", ""},
		{"3DS", "Hack DS", "3DS-0004000000055D00", "3DS-0004000000055D00"}, // never a 3DS Alias
	} {
		key, err := c.aliasKey(k.system, k.name, k.key)
		if key != k.expected || (err != nil) != (k.expected == "") {
			t.Errorf("%s %s : %q %v, attendu %q", k.system, k.name, key, err, k.expected)
		}
	}
}

// network is a transport that counts the requests and can cut the network.
type network struct {
	cut           bool
	lists, others int // GET /saves, other GETs
	attempts      int // all requests, cut or not
}

func (r *network) RoundTrip(q *http.Request) (*http.Response, error) {
	r.attempts++
	if r.cut {
		return nil, errors.New("réseau coupé")
	}
	if q.Method == "GET" && q.URL.Path == "/saves" {
		r.lists++
	} else if q.Method == "GET" {
		r.others++
	}
	return http.DefaultTransport.RoundTrip(q)
}

func TestRomHackAndAlias(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	gba, roms := filepath.Join(tmp, "gba"), filepath.Join(tmp, "roms-gba")
	cfg := Config{URL: url, Token: "jeton-test", Copies: filepath.Join(tmp, "copies"),
		Emulators: map[string]*Emulator{"mgba": {Exe: "inutilise.exe", Saves: map[string]string{"GBA": gba}, Roms: map[string]string{"GBA": roms}}}}
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), cfg)
	run := func(rom string) int { return realMain([]string{"run", "--config", config, rom}) }
	rf, rr := filepath.Join(roms, "Pokemon Rouge Feu.gba"), filepath.Join(roms, "Pokemon Radical Red.gba")
	write(t, rf, gbaHeader("BPRE"))
	write(t, rr, gbaHeader("BPRE"))
	savRF, savRR := filepath.Join(gba, "Pokemon Rouge Feu.sav"), filepath.Join(gba, "Pokemon Radical Red.sav")
	f1, r1, r2 := random(131072, 1), random(131072, 2), random(131072, 3)
	write(t, savRF, f1)
	write(t, savRR, r1)
	kept := func() string {
		m := map[string]string{}
		readJSON(filepath.Join(tmp, "cfg", "roms.json"), &m)
		return m["GBA-BPRE"]
	}

	t.Log("Le jeu d'origine se synchronise ; sa ROM est retenue à côté de la Base")
	reset()
	run(rf)
	if sha256Hex(current("GBA-BPRE")) != sha256Hex(f1) || kept() != rf {
		t.Fatalf("serveur ou ROM retenue faux : %q", kept())
	}

	t.Log("Le hack (même Clé, autre fichier qui existe), Jouer sans synchroniser : une question, puis une notification")
	reset()
	responses = []string{"Jouer sans synchroniser"}
	play = func() { write(t, savRR, r2) }
	if run(rr) != 0 || !played {
		t.Error("le hack doit démarrer")
	}
	if len(questions) != 1 || !strings.Contains(questions[0], "Pokemon Radical Red.gba ressemble à Pokemon Rouge Feu.gba") {
		t.Errorf("question : %q", questions)
	}
	if len(notifications) != 1 || !contains(notifications, "Synchroniser à part") || !contains(notifications, "ROM hacks") {
		t.Errorf("notification : %q", notifications)
	}
	if sha256Hex(current("GBA-BPRE")) != sha256Hex(f1) || len(history("GBA-BPRE")) != 1 || sha256Hex(read(savRR)) != sha256Hex(r2) {
		t.Error("le serveur doit garder la Sauvegarde d'origine, et le hack la sienne")
	}

	t.Log("Le hack, Synchroniser à part : alias créé dans config.json (copie avant), le hack part sous sa propre Clé")
	reset()
	responses = []string{"Synchroniser à part"}
	run(rr)
	var withAlias Config
	readJSON(config, &withAlias)
	keyRR := withAlias.Alias["Pokemon Radical Red"]
	if !regexp.MustCompile(`^GBA-0[A-Z0-9]{3}$`).MatchString(keyRR) || sha256Hex(current(keyRR)) != sha256Hex(r2) || sha256Hex(current("GBA-BPRE")) != sha256Hex(f1) {
		t.Errorf("alias %q : le hack doit partir sous sa Clé, Rouge Feu rester", keyRR)
	}
	if len(questions) != 1 || !contains(notifications, "a maintenant sa propre sauvegarde") ||
		len(namesIn(filepath.Join(tmp, "cfg"), "config.json.avant-3dssavesync-*")) == 0 {
		t.Errorf("questions %q, notifications %q, copie de config.json attendue", questions, notifications)
	}
	reset()
	run(rr)
	if len(questions) != 0 || len(notifications) != 0 {
		t.Errorf("lancement suivant sans question ni message : %q %q", questions, notifications)
	}

	t.Log("ROM retenue disparue (déplacée, extraction temporaire) : la nouvelle est acceptée et retenue")
	reset()
	elsewhere := filepath.Join(tmp, "extrait", "Pokemon Rouge Feu.gba")
	os.MkdirAll(filepath.Dir(elsewhere), 0o755)
	if err := os.Rename(rf, elsewhere); err != nil {
		t.Fatal(err)
	}
	run(elsewhere)
	if len(notifications) != 0 || len(questions) != 0 || kept() != elsewhere {
		t.Errorf("acceptée sans message : %q %q, retenue %q", notifications, questions, kept())
	}

	t.Log("Alias : le hack a sa propre Clé sur le serveur (espaces et minuscules acceptés)")
	reset()
	cfg.Alias = map[string]string{"Pokemon Radical Red": " gba-0rad "}
	writeConfig(t, config, cfg)
	run(rr)
	if sha256Hex(current("GBA-0RAD")) != sha256Hex(r2) || sha256Hex(current("GBA-BPRE")) != sha256Hex(f1) || len(notifications) != 0 {
		t.Errorf("le hack doit partir sous GBA-0RAD : %q", notifications)
	}

	t.Log("Alias invalide : partie sans Synchronisation, jamais sous la Clé de l'en-tête")
	reset()
	cfg.Alias = map[string]string{"Pokemon Radical Red": "GBA-BPRE-RADICALRED"}
	writeConfig(t, config, cfg)
	play = func() { write(t, savRR, random(131072, 4)) }
	run(rr)
	if !played || !contains(notifications, "« GBA-BPRE-RADICALRED » de Pokemon Radical Red n'est pas valable") || !contains(notifications, "GBA-0RAD") {
		t.Errorf("notification : %q", notifications)
	}
	if sha256Hex(current("GBA-BPRE")) != sha256Hex(f1) || sha256Hex(current("GBA-0RAD")) != sha256Hex(r2) {
		t.Error("rien ne doit partir")
	}

	t.Log("launch : deux ROM de même Clé, aucune synchronisée, une notification ; les autres jeux le sont")
	reset()
	cfg.Alias = nil
	writeConfig(t, config, cfg)
	if err := os.Rename(elsewhere, rf); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(roms, "Pokemon Emeraude.gba"), gbaHeader("BPEE"))
	em := random(131072, 5)
	write(t, filepath.Join(gba, "Pokemon Emeraude.sav"), em)
	write(t, savRF, random(131072, 6))
	realMain([]string{"launch", "--config", config, "mgba"})
	if len(notifications) != 1 || !contains(notifications, "Pokemon Radical Red.gba, Pokemon Rouge Feu.gba") {
		t.Errorf("notification : %q", notifications)
	}
	if sha256Hex(current("GBA-BPRE")) != sha256Hex(f1) || sha256Hex(current("GBA-BPEE")) != sha256Hex(em) {
		t.Error("Rouge Feu ne doit pas partir, Émeraude oui")
	}
}

func TestPendingUploads(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	r := &network{}
	transport = r
	t.Cleanup(func() { transport = http.DefaultTransport })
	saves, gba := filepath.Join(tmp, "saves"), filepath.Join(tmp, "gba")
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Token: "jeton-test", Copies: filepath.Join(tmp, "copies"),
		Emulators: map[string]*Emulator{
			"melonds": {Exe: "melon.exe", Saves: map[string]string{"DS": saves}},
			"mgba":    {Exe: "mgba.exe", Saves: map[string]string{"GBA": gba}},
		}})
	run := func(rom string) int { return realMain([]string{"run", "--config", config, rom}) }
	black, savBlack := romDS(t, "Pokemon Noire", "IRBF"), filepath.Join(saves, "Pokemon Noire.sav")
	emerald := filepath.Join(tmp, "roms", "Pokemon Emeraude.gba")
	write(t, emerald, gbaHeader("BPEE"))
	pending := func() (l []pendingEntry) { readJSON(filepath.Join(tmp, "cfg", "en-attente.json"), &l); return }
	cutDuring := func(o []byte) { play = func() { write(t, savBlack, o); r.cut = true } }

	a1, a2, a3, a4, a5 := byteRange(1, 200), byteRange(2, 201), byteRange(3, 202), byteRange(4, 203), byteRange(5, 204)
	write(t, savBlack, a1)
	run(black)

	t.Log("Réseau coupé pendant la partie : l'Envoi est mis en attente")
	reset()
	cutDuring(a2)
	run(black)
	if !contains(notifications, "reste sur le PC") || len(pending()) != 1 || pending()[0].Key != "IRBF" || pending()[0].Profile != "melonds" {
		t.Fatalf("attente : %+v, notifications %q", pending(), notifications)
	}

	t.Log("Un autre jeu est lancé, réseau revenu : l'Envoi en attente part d'abord")
	reset()
	r.cut = false
	run(emerald)
	if sha256Hex(current("IRBF")) != sha256Hex(a2) || len(pending()) != 0 || !played || len(questions) != 0 {
		t.Errorf("serveur, attente %+v, questions %q", pending(), questions)
	}

	t.Log("Envoi en attente en Conflit : la question habituelle, rien n'est écrasé")
	reset()
	cutDuring(a3)
	run(black)
	r.cut = false
	upload3ds("IRBF", a4, a2)
	reset()
	responses = []string{"Ne rien faire"}
	run(emerald)
	if len(questions) != 1 || !strings.Contains(questions[0], "Conflit") || sha256Hex(current("IRBF")) != sha256Hex(a4) ||
		sha256Hex(read(savBlack)) != sha256Hex(a3) || len(pending()) != 0 {
		t.Errorf("questions %q, attente %+v", questions, pending())
	}

	t.Log("Émulateur de l'Envoi en attente ouvert : il reste en attente")
	reset()
	cutDuring(a5)
	run(black)
	r.cut = false
	reset()
	instances = func(name string) int {
		if name == "melon" {
			return 1
		}
		return 0
	}
	run(emerald)
	instances = func(string) int { return running }
	if len(pending()) != 1 || sha256Hex(current("IRBF")) != sha256Hex(a4) || len(questions) != 0 {
		t.Errorf("attente %+v, questions %q", pending(), questions)
	}

	t.Log("Réseau toujours coupé au lancement suivant : un seul essai pour l'attente et l'avant-partie")
	reset()
	r.cut, r.attempts = true, 0
	responses = []string{"Non"}
	run(emerald)
	if r.attempts != 1 || len(questions) != 1 || !strings.Contains(questions[0], "Synchronisation impossible") || len(pending()) != 1 {
		t.Errorf("%d requêtes, questions %q, attente %+v", r.attempts, questions, pending())
	}

	t.Log("Réseau revenu, Conflit, Garder celle du PC : la Sauvegarde jouée part")
	reset()
	r.cut = false
	responses = []string{"Garder celle du PC"}
	run(emerald)
	if sha256Hex(current("IRBF")) != sha256Hex(a5) || len(pending()) != 0 {
		t.Errorf("attente %+v, questions %q", pending(), questions)
	}
}

func TestOneListPerPass(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	r := &network{}
	transport = r
	t.Cleanup(func() { transport = http.DefaultTransport })
	saves, roms := filepath.Join(tmp, "saves"), filepath.Join(tmp, "roms")
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Token: "jeton-test",
		Emulators: map[string]*Emulator{"melonds": {Exe: "melon.exe", Saves: map[string]string{"DS": saves}, Roms: map[string]string{"DS": roms}}}})
	for i, code := range []string{"ADAF", "APAF", "CPUF"} {
		romDS(t, code, code)
		write(t, filepath.Join(saves, code+".sav"), byteRange(i, 100+i))
	}
	launch := func() { realMain([]string{"launch", "--config", config, "melonds"}) }

	t.Log("Premier launch : trois Envois, la liste relue après chacun")
	reset()
	launch()
	if codes() != "ADAF,APAF,CPUF" {
		t.Fatalf("serveur : %s", codes())
	}

	t.Log("launch à jour : une lecture de la liste avant, une après, rien d'autre")
	reset()
	r.lists, r.others = 0, 0
	launch()
	if r.lists != 2 || r.others != 0 || len(questions) != 0 {
		t.Errorf("%d listes, %d autres GET, questions %q", r.lists, r.others, questions)
	}

	t.Log("Réseau coupé : la passe s'arrête à la première erreur")
	reset()
	r.cut, r.attempts = true, 0
	responses = []string{"Non"}
	launch()
	if r.attempts != 1 || len(questions) != 1 || played {
		t.Errorf("%d requêtes, questions %q", r.attempts, questions)
	}
}

// ROM kept for all the emulators of the PC, and Key already synced with no kept ROM (ADR 0014).
func TestGlobalKeptRom(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	gba, gba2, roms := filepath.Join(tmp, "gba"), filepath.Join(tmp, "gba2"), filepath.Join(tmp, "roms-gba")
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Token: "jeton-test", Copies: filepath.Join(tmp, "copies"),
		Emulators: map[string]*Emulator{
			"mgba":  {Exe: "mgba.exe", Saves: map[string]string{"GBA": gba}},
			"autre": {Profile: "mgba", Exe: "autre.exe", Saves: map[string]string{"GBA": gba2}},
		}})
	run := func(emu, rom string) int {
		return realMain([]string{"run", "--config", config, "--emulator", emu, rom})
	}
	rf, rr := filepath.Join(roms, "Pokemon Rouge Feu.gba"), filepath.Join(roms, "Pokemon Radical Red.gba")
	write(t, rf, gbaHeader("BPRE"))
	write(t, rr, gbaHeader("BPRE"))
	f1 := random(131072, 1)
	write(t, filepath.Join(gba, "Pokemon Rouge Feu.sav"), f1)
	reset()
	run("mgba", rf)

	t.Log("Le hack dans un autre émulateur, sans sauvegarde : refusé, la ROM d'origine est retenue pour tout le PC")
	reset()
	run("autre", rr)
	if !played || !contains(notifications, "ressemble à Pokemon Rouge Feu.gba") || exists(filepath.Join(gba2, "Pokemon Radical Red.sav")) ||
		len(history("GBA-BPRE")) != 1 {
		t.Errorf("notifications %q, le hack ne doit pas recevoir la partie d'origine", notifications)
	}

	t.Log("Aucune ROM retenue (Base d'avant), pas de sauvegarde ici : une question ; refus = rien n'est écrit")
	os.Remove(filepath.Join(tmp, "cfg", "roms.json"))
	reset()
	responses = []string{"Jouer sans synchroniser"}
	run("autre", rr)
	if len(questions) != 1 || !strings.Contains(questions[0], "peut-être avec une autre ROM") || exists(filepath.Join(gba2, "Pokemon Radical Red.sav")) || !played {
		t.Errorf("questions %q", questions)
	}

	t.Log("Même jeu confirmé : Récupération, ROM retenue, plus de question ensuite")
	reset()
	responses = []string{"Synchroniser"}
	run("autre", rf)
	if sha256Hex(read(filepath.Join(gba2, "Pokemon Rouge Feu.sav"))) != sha256Hex(f1) || len(questions) != 1 {
		t.Errorf("Récupération attendue, questions %q", questions)
	}
	reset()
	run("autre", rr) // the question offers "Sync separately"; with no answer, the hack starts without Sync
	if len(questions) != 1 || !contains(notifications, "ressemble à") {
		t.Errorf("questions %q, notifications %q", questions, notifications)
	}
}

// Two ROMs with different Keys, one save file (shared folder): neither is synced.
func TestLaunchSameFile(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	saves, roms := filepath.Join(tmp, "saves"), filepath.Join(tmp, "roms")
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Token: "jeton-test",
		Emulators: map[string]*Emulator{"melonds": {Exe: "melon.exe", Saves: map[string]string{"DS": saves}, Roms: map[string]string{"DS": roms}}}})
	romDS(t, filepath.Join("USA", "Jeu"), "AAAE")
	romDS(t, filepath.Join("EUR", "Jeu"), "AAAP")
	romDS(t, "Autre", "CPUF")
	write(t, filepath.Join(saves, "Jeu.sav"), byteRange(1, 200))
	write(t, filepath.Join(saves, "Autre.sav"), byteRange(2, 201))
	reset()
	realMain([]string{"launch", "--config", config, "melonds"})
	if codes() != "CPUF" || !contains(notifications, "même fichier de sauvegarde : Jeu.nds, Jeu.nds") {
		t.Errorf("serveur %s, notifications %q", codes(), notifications)
	}
}

// faultyTransport refuses the Uploads (400) or answers the list with a Wi-Fi portal page.
type faultyTransport struct{ rejectUploads, captivePortal bool }

func (p *faultyTransport) RoundTrip(q *http.Request) (*http.Response, error) {
	if p.rejectUploads && q.Method == "PUT" || p.captivePortal && q.Method == "GET" && q.URL.Path == "/saves" {
		st, body := 400, ""
		if p.captivePortal {
			st, body = 200, "<html>Connexion au Wi-Fi</html>"
		}
		return &http.Response{StatusCode: st, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: q}, nil
	}
	return http.DefaultTransport.RoundTrip(q)
}

// Every failed Upload after play becomes pending; an unreadable pending list is kept aside.
func TestPendingFailures(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	p := &faultyTransport{}
	transport = p
	t.Cleanup(func() { transport = http.DefaultTransport })
	saves := filepath.Join(tmp, "saves")
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Token: "jeton-test", Copies: filepath.Join(tmp, "copies"),
		Emulators: map[string]*Emulator{"melonds": {Exe: "melon.exe", Saves: map[string]string{"DS": saves}}}})
	run := func(rom string) int { return realMain([]string{"run", "--config", config, rom}) }
	black, sav := romDS(t, "Pokemon Noire", "IRBF"), filepath.Join(saves, "Pokemon Noire.sav")
	list := filepath.Join(tmp, "cfg", "en-attente.json")
	pending := func() (l []pendingEntry) { readJSON(list, &l); return }
	write(t, sav, byteRange(1, 200))
	run(black)

	t.Log("Envoi refusé par le serveur (400) : en attente, comme la notification le dit")
	reset()
	play = func() { write(t, sav, byteRange(2, 201)); p.rejectUploads = true }
	run(black)
	p.rejectUploads = false
	if !contains(notifications, "sera envoyée au prochain lancement") || len(pending()) != 1 {
		t.Errorf("attente %+v, notifications %q", pending(), notifications)
	}

	t.Log("Portail Wi-Fi (page HTML à la place de la liste) et liste en attente illisible : copie gardée, jeu en attente")
	write(t, list, []byte("{pas du json"))
	reset()
	play = func() { write(t, sav, byteRange(3, 202)); p.captivePortal = true }
	run(black)
	p.captivePortal = false
	side := namesIn(filepath.Join(tmp, "cfg"), "en-attente.json.illisible-*")
	if len(side) != 1 || string(read(side[0])) != "{pas du json" || len(pending()) != 1 || !contains(notifications, "Le serveur a répondu de façon inattendue") ||
		!contains(notifications, "était illisible") {
		t.Errorf("copie %q, attente %+v, notifications %q", side, pending(), notifications)
	}
}

// ROM in an unread format (.7z) with an Alias: never synced, as the notification says.
func TestUnreadFormatAlias(t *testing.T) {
	tmp = t.TempDir()
	fakeHome(t)
	startTestServer(t)
	saves := filepath.Join(tmp, "saves")
	config := writeConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Token: "jeton-test", Alias: map[string]string{"Jeu": "0RAD"},
		Emulators: map[string]*Emulator{"melonds": {Exe: "melon.exe", Saves: map[string]string{"DS": saves}}}})
	rom := filepath.Join(tmp, "roms", "Jeu.7z")
	write(t, rom, []byte("7z"))
	write(t, filepath.Join(saves, "Jeu.sav"), byteRange(1, 200))
	reset()
	realMain([]string{"run", "--config", config, "--emulator", "melonds", rom})
	if !played || codes() != "" || !contains(notifications, "ne sait pas lire ce format de ROM") {
		t.Errorf("serveur %q, notifications %q", codes(), notifications)
	}
}

// Key of an Alias created by 3dssavesync: shape of the System, from the name (same ROM, same Key on each PC), free.
func TestAddAlias(t *testing.T) {
	cfg := &Config{}
	a, err := addAlias(cfg, "GBA", "Radical Red (4.10)")
	if err != nil || !regexp.MustCompile(`^GBA-0[A-Z0-9]{3}$`).MatchString(a) {
		t.Fatalf("GBA : %q %v", a, err)
	}
	if b, _ := addAlias(&Config{}, "GBA", "Radical Red (4.10)"); b != a {
		t.Errorf("même nom, autre PC : %q puis %q", a, b)
	}
	if b, _ := addAlias(cfg, "GBA", "Radical Red (4.10)"); b != a || len(cfg.Alias) != 1 {
		t.Errorf("alias existant gardé : %q, %v", b, cfg.Alias)
	}
	taken := &Config{Alias: map[string]string{"Autre hack": a}}
	if b, _ := addAlias(taken, "GBA", "Radical Red (4.10)"); b == a || !keyShapes["GBA"].MatchString(b) {
		t.Errorf("Clé déjà prise par un autre alias : %q", b)
	}
	for system, shape := range map[string]string{"DS": `^0[A-Z0-9]{3}$`, "GB": `^GB-ALIAS-[0-9A-F]{4}$`} {
		if b, err := addAlias(&Config{}, system, "Hack"); err != nil || !regexp.MustCompile(shape).MatchString(b) || !keyShapes[system].MatchString(b) {
			t.Errorf("%s : %q %v", system, b, err)
		}
	}
	for _, name := range []string{"", "  ", "a=b", "a\nb"} {
		if _, err := addAlias(&Config{}, "GBA", name); err == nil {
			t.Errorf("nom %q accepté", name)
		}
	}
	if _, err := addAlias(&Config{}, "3DS", "Hack"); err == nil {
		t.Error("alias 3DS accepté")
	}
}
