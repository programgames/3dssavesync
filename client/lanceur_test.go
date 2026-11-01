package main

// Reprise de pc/test_lanceur.ps1 : le vrai serveur (server/server.py) et un dossier temporaire.
// L'émulateur, les fenêtres et la détection de processus sont remplacés par des doublures.
// Les scénarios se suivent : chacun part de l'état laissé par le précédent.

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var (
	tmp, url                           string
	reponses, questions, notifications []string
	partie                             func()
	joue, actif                        bool
	commandeLancee                     []string
)

func reinitialiser() {
	reponses, questions, notifications, partie, joue, actif, commandeLancee = nil, nil, nil, nil, false, false, nil
}

func init() {
	demander = func(texte string, boutons []string) string {
		questions = append(questions, texte)
		if len(reponses) == 0 {
			return boutons[len(boutons)-1] + " (question inattendue)"
		}
		r := reponses[0]
		reponses = reponses[1:]
		return r
	}
	notifier = func(texte string) { notifications = append(notifications, texte) }
	lancer = func(c []string) error {
		joue, commandeLancee = true, c
		if partie != nil {
			partie()
		}
		return nil
	}
	emulateurActif = func(string) bool { return actif }
}

func demarrerServeur(t *testing.T) {
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
	c := exec.Command(py, filepath.Join("..", "server", "server.py"))
	c.Env = append(os.Environ(), "NDSSYNC_TOKEN=jeton-test", "NDSSYNC_DATA="+filepath.Join(tmp, "data"), fmt.Sprintf("NDSSYNC_PORT=%d", port))
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Process.Kill(); c.Wait() })
	url = fmt.Sprintf("http://127.0.0.1:%d", port)
	for range 100 {
		if r, err := http.Get(url + "/saves"); err == nil {
			r.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("serveur injoignable")
}

// Requêtes directes au serveur, pour jouer « l'autre Client ».
func api(methode, chemin string, corps []byte, entetes map[string]string) (int, []byte) {
	var r io.Reader
	if corps != nil {
		r = bytes.NewReader(corps)
	}
	req, _ := http.NewRequest(methode, url+chemin, r)
	req.Header.Set("Authorization", "Bearer jeton-test")
	for k, v := range entetes {
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

func courante(cle string) []byte { _, o := api("GET", "/saves/"+cle, nil, nil); return o }

func historique(cle string) (hashes []string) {
	_, o := api("GET", "/saves/"+cle+"/history", nil, nil)
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

func envoi3ds(cle string, octets, avant []byte) int {
	cond := map[string]string{"If-None-Match": "*", "X-Client": "3ds"}
	if avant != nil {
		cond = map[string]string{"If-Match": `"` + hachage(avant) + `"`, "X-Client": "3ds"}
	}
	st, _ := api("PUT", "/saves/"+cle, octets, cond)
	return st
}

func suite(debut, fin int) []byte { // [byte[]](debut..fin) de PowerShell
	o := []byte{}
	for i := debut; i <= fin; i++ {
		o = append(o, byte(i))
	}
	return o
}

func aleatoire(n int, graine int64) []byte {
	o := make([]byte, n)
	rand.New(rand.NewSource(graine)).Read(o)
	return o
}

func ecrire(t *testing.T, chemin string, o []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(chemin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chemin, o, 0o644); err != nil {
		t.Fatal(err)
	}
}

func lire(chemin string) []byte { o, _ := os.ReadFile(chemin); return o }

func romDS(t *testing.T, nom, code string) string {
	o := make([]byte, 0x200)
	copy(o[0x0C:], code)
	c := filepath.Join(tmp, "roms", nom+".nds")
	ecrire(t, c, o)
	return c
}

// Fausse ROM dans un .zip au nom No-Intro.
func romZip(t *testing.T, nom, interne string, o []byte) string {
	c := filepath.Join(tmp, "roms", nom+".zip")
	os.MkdirAll(filepath.Dir(c), 0o755)
	f, err := os.Create(c)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, _ := z.Create(interne)
	w.Write(o)
	z.Close()
	f.Close()
	return c
}

func enteteGB(titre string, somme uint16) []byte {
	o := make([]byte, 0x150)
	copy(o[0x134:], titre)
	o[0x14E], o[0x14F] = byte(somme>>8), byte(somme)
	return o
}

func enteteGBA(code string) []byte { o := make([]byte, 0xC0); copy(o[0xAC:], code); return o }

// Fausse ROM 3DS : NCSD, partition 0 à 0x200, program_id à NCCH + 0x118.
func rom3DS(t *testing.T, nom string, id uint64) string {
	o := make([]byte, 0x400)
	copy(o[0x100:], "NCSD")
	binary.LittleEndian.PutUint32(o[0x120:], 1)
	binary.LittleEndian.PutUint64(o[0x200+0x118:], id)
	c := filepath.Join(tmp, "roms", nom+".3ds")
	ecrire(t, c, o)
	return c
}

func dossierAzahar(id uint64) string {
	z := strings.Repeat("0", 32)
	return filepath.Join(tmp, "azahar", "Nintendo 3DS", z, z, "title", fmt.Sprintf("%08x", id>>32), fmt.Sprintf("%08x", uint32(id)), "data", "00000001")
}

func ecrireConfig(t *testing.T, chemin string, c Config) string {
	o, _ := json.Marshal(c)
	ecrire(t, chemin, o)
	return chemin
}

func nomsDans(dossier, motif string) []string {
	m, _ := filepath.Glob(filepath.Join(dossier, motif))
	return m
}

func contient(liste []string, texte string) bool {
	for _, s := range liste {
		if strings.Contains(s, texte) {
			return true
		}
	}
	return false
}

// Transport qui fait envoyer la 3DS juste après le premier GET /saves du PC (course GET/PUT).
type course struct{ apres func() }

func (c *course) RoundTrip(r *http.Request) (*http.Response, error) {
	rep, err := http.DefaultTransport.RoundTrip(r)
	if c.apres != nil && r.Method == "GET" && r.URL.Path == "/saves" {
		c.apres()
		c.apres = nil
	}
	return rep, err
}

func TestLanceur(t *testing.T) {
	tmp = t.TempDir()
	maisonFactice(t)
	demarrerServeur(t)
	verifications := 0
	verifier := func(ok bool, message string) {
		t.Helper()
		verifications++
		if !ok {
			t.Errorf("ECHEC %s", message)
		}
	}
	saves, copies := filepath.Join(tmp, "saves"), filepath.Join(tmp, "copies")
	config := ecrireConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{
		URL: url, Jeton: "jeton-test", Copies: copies,
		Emulateurs: map[string]*Emulateur{
			"melonds": {Exe: "inutilise.exe", Sauvegardes: map[string]string{"DS": saves}},
			"mgba":    {Exe: "inutilise.exe", Sauvegardes: map[string]string{"GB": filepath.Join(tmp, "gb"), "GBA": filepath.Join(tmp, "gba")}},
			"azahar":  {Exe: "inutilise.exe", Sauvegardes: map[string]string{"3DS": filepath.Join(tmp, "azahar")}},
		},
	})
	run := func(args ...string) int { return principal(append([]string{"run", "--config", config}, args...)) }
	sav := func(nom string) string { return filepath.Join(saves, nom+".sav") }
	noire := sav("Pokemon Noire")
	localHash := func() string { return hachage(lire(noire)) }

	t.Log("Premier lancement : la Sauvegarde locale part sur le serveur")
	reinitialiser()
	rom := romDS(t, "Pokemon Noire", "IRBF")
	v1 := suite(1, 200)
	ecrire(t, noire, v1)
	verifier(run(rom) == 0, "code de sortie 0")
	verifier(joue, "melonDS lancé")
	verifier(hachage(courante("IRBF")) == hachage(v1), "le serveur a la Sauvegarde du PC")
	verifier(len(questions) == 0, "aucune question")
	verifier(strings.Join(commandeLancee, "|") == "inutilise.exe|"+rom, "melonDS reçoit la ROM")

	t.Log("La partie modifie la Sauvegarde : Envoi après la partie")
	reinitialiser()
	v2 := suite(2, 201)
	partie = func() { ecrire(t, noire, v2) }
	run(rom)
	verifier(hachage(courante("IRBF")) == hachage(v2), "le serveur a la Sauvegarde jouée")
	verifier(len(historique("IRBF")) == 2, "l'Historique garde les 2 Versions")

	t.Log("La 3DS a joué : Récupération avec Copie de sécurité")
	reinitialiser()
	v3 := suite(3, 202)
	verifier(envoi3ds("IRBF", v3, v2) == 200, "la 3DS envoie")
	run(rom)
	verifier(localHash() == hachage(v3), "le PC a la Sauvegarde de la 3DS")
	c := nomsDans(filepath.Join(copies, "melonds"), "*.sav")
	verifier(len(c) == 1 && hachage(lire(c[0])) == hachage(v2), "Copie de sécurité de l'ancienne Sauvegarde")
	verifier(len(questions) == 0, "aucune question")

	t.Log("Conflit avant la partie, Ne pas jouer : rien ne bouge, pas de partie")
	reinitialiser()
	v4, v5 := suite(4, 203), suite(5, 204)
	ecrire(t, noire, v4)
	envoi3ds("IRBF", v5, v3)
	reponses = []string{"Ne pas jouer"}
	run(rom)
	verifier(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "une question de Conflit")
	verifier(!joue, "melonDS non lancé")
	verifier(localHash() == hachage(v4), "le PC garde sa Sauvegarde")
	verifier(hachage(courante("IRBF")) == hachage(v5), "le serveur garde la sienne")

	t.Log("Conflit avant la partie, Prendre le serveur")
	reinitialiser()
	reponses = []string{"Prendre le serveur"}
	run(rom)
	verifier(joue, "melonDS lancé")
	verifier(localHash() == hachage(v5), "le PC a la Version du serveur")
	copie := false
	for _, f := range nomsDans(filepath.Join(copies, "melonds"), "*.sav") {
		copie = copie || hachage(lire(f)) == hachage(v4)
	}
	verifier(copie, "Copie de sécurité de la Sauvegarde du PC")

	t.Log("Conflit avant la partie, Garder le PC : il envoie la sienne")
	reinitialiser()
	v6, v7 := suite(6, 205), suite(7, 206)
	ecrire(t, noire, v6)
	envoi3ds("IRBF", v7, v5)
	reponses = []string{"Garder le PC"}
	run(rom)
	verifier(joue, "melonDS lancé")
	verifier(hachage(courante("IRBF")) == hachage(v6), "le serveur a la Sauvegarde du PC")
	verifier(contient(historique("IRBF"), hachage(v7)), "la Version de la 3DS reste dans l'Historique")

	t.Log("La 3DS envoie pendant la partie (412), Ne rien faire : rien n'est écrasé")
	reinitialiser()
	v8, v9 := suite(8, 207), suite(9, 208)
	partie = func() { envoi3ds("IRBF", v8, v6); ecrire(t, noire, v9) }
	reponses = []string{"Ne rien faire"}
	run(rom)
	verifier(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "une question de Conflit après la partie")
	verifier(hachage(courante("IRBF")) == hachage(v8), "le serveur garde la Version de la 3DS")
	verifier(localHash() == hachage(v9), "le PC garde la Sauvegarde jouée")

	t.Log("Conflit après la partie, Garder le PC : il envoie la sienne")
	reinitialiser()
	v10, v11 := suite(10, 209), suite(11, 210)
	partie = func() { envoi3ds("IRBF", v10, v8); ecrire(t, noire, v11) }
	reponses = []string{"Prendre le serveur", "Garder le PC"}
	run(rom)
	verifier(len(questions) == 2, "deux questions de Conflit")
	verifier(hachage(courante("IRBF")) == hachage(v11), "le serveur a la Sauvegarde du PC")
	verifier(contient(historique("IRBF"), hachage(v10)), "la Version de la 3DS reste dans l'Historique")

	t.Log("Sauvegarde tronquée pendant la partie : Conflit, rien n'est envoyé")
	reinitialiser()
	partie = func() { ecrire(t, noire, suite(1, 100)) }
	reponses = []string{"Ne rien faire"}
	run(rom)
	verifier(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "une question de Conflit")
	verifier(hachage(courante("IRBF")) == hachage(v11), "le serveur garde la Version entière")

	t.Log("Course : la 3DS envoie entre le GET et le PUT du PC (412)")
	reinitialiser()
	ecrire(t, noire, v11) // retour à l'état synchronisé
	run(rom)
	v12, v13 := suite(12, 211), suite(13, 212)
	ecrire(t, noire, v12)
	reinitialiser()
	transport = &course{apres: func() { envoi3ds("IRBF", v13, v11) }}
	reponses = []string{"Ne pas jouer"}
	run(rom)
	transport = http.DefaultTransport
	verifier(len(questions) == 1 && strings.Contains(questions[0], "Conflit"), "le 412 devient une question de Conflit")
	verifier(hachage(courante("IRBF")) == hachage(v13), "la Version de la 3DS n'est pas écrasée")
	verifier(localHash() == hachage(v12), "le PC garde sa Sauvegarde")

	t.Log("Conflit sans fenêtre possible (Q16) : rien n'est remplacé, partie, notification")
	reinitialiser()
	reponses = []string{"", ""}
	run(rom)
	verifier(joue, "melonDS lancé")
	verifier(contient(notifications, "rien n'est remplacé"), "notification")
	verifier(localHash() == hachage(v12) && hachage(courante("IRBF")) == hachage(v13), "rien n'a bougé")

	t.Log("melonDS tourne déjà : refus")
	reinitialiser()
	actif = true
	verifier(run(rom) == 1, "code de sortie 1")
	verifier(len(questions) == 0 && contient(notifications, "tourne déjà"), "notification, aucune fenêtre")
	verifier(strings.Contains(string(lire(filepath.Join(tmp, "cfg", "3dssavesync.log"))), "refus"), "le refus est écrit dans 3dssavesync.log")
	verifier(!joue, "melonDS non lancé")

	t.Log("Homebrew (code ####) : partie sans Synchronisation")
	reinitialiser()
	homebrew := romDS(t, "Homebrew", "####")
	ecrire(t, sav("Homebrew"), suite(1, 10))
	avant := codes()
	run(homebrew)
	verifier(joue, "melonDS lancé")
	verifier(codes() == avant && len(questions) == 0, "rien envoyé, aucune question")

	t.Log("ROM en .zip : Code jeu et nom lus dans l'archive")
	reinitialiser()
	nds := make([]byte, 0x200)
	copy(nds[0x0C:], "IRAF")
	z := romZip(t, "Pokemon Blanche (archive)", "Pokemon Blanche.nds", nds)
	blanche := suite(20, 219)
	ecrire(t, sav("Pokemon Blanche"), blanche)
	run(z)
	verifier(hachage(courante("IRAF")) == hachage(blanche), "le serveur a la Sauvegarde de Pokemon Blanche.sav")

	t.Log("Serveur injoignable avant la partie, Non : pas de partie")
	reinitialiser()
	hors := ecrireConfig(t, filepath.Join(tmp, "cfg-hors", "config.json"), Config{URL: "http://127.0.0.1:1", Jeton: "x",
		Emulateurs: map[string]*Emulateur{"melonds": {Exe: "inutilise.exe", Sauvegardes: map[string]string{"DS": saves}}}})
	reponses = []string{"Non"}
	principal([]string{"run", "--config", hors, rom})
	verifier(len(questions) == 1 && strings.Contains(questions[0], "Synchronisation impossible"), "question « Synchronisation impossible »")
	verifier(!joue, "melonDS non lancé")

	t.Log("Serveur injoignable, Oui : partie, puis simple information")
	reinitialiser()
	reponses = []string{"Oui"}
	partie = func() { ecrire(t, noire, v13) }
	principal([]string{"run", "--config", hors, rom})
	verifier(joue, "melonDS lancé")
	verifier(len(questions) == 1 && contient(notifications, "reste sur le PC"), "notification après la partie, pas de fenêtre")
	verifier(localHash() == hachage(v13), "la Sauvegarde jouée reste sur le PC")

	t.Log("Copie de sécurité impossible : rien n'est écrasé")
	reinitialiser()
	bloque := filepath.Join(tmp, "copies-impossibles")
	ecrire(t, bloque, []byte("un fichier, pas un dossier"))
	cfg2 := ecrireConfig(t, filepath.Join(tmp, "cfg2", "config.json"), Config{URL: url, Jeton: "jeton-test", Copies: bloque,
		Emulateurs: map[string]*Emulateur{"melonds": {Exe: "inutilise.exe", Sauvegardes: map[string]string{"DS": filepath.Join(tmp, "saves2")}}}})
	platine := romDS(t, "Pokemon Platine", "CPUF")
	p1, p2 := suite(30, 229), suite(31, 230)
	ecrire(t, filepath.Join(tmp, "saves2", "Pokemon Platine.sav"), p1)
	principal([]string{"run", "--config", cfg2, platine}) // premier Envoi : Base = p1
	envoi3ds("CPUF", p2, p1)
	reinitialiser()
	reponses = []string{"Non"}
	principal([]string{"run", "--config", cfg2, platine})
	verifier(hachage(lire(filepath.Join(tmp, "saves2", "Pokemon Platine.sav"))) == hachage(p1), "la Sauvegarde du PC est intacte")
	verifier(len(questions) == 1 && strings.Contains(questions[0], "Synchronisation impossible"), "question « Synchronisation impossible »")
	verifier(!joue, "melonDS non lancé")

	// --- GB / GBA (ADR 0007, 0008) ----------------------------------------------------------
	t.Log("GB : Clé lue dans l'en-tête, seule la SRAM part sur le serveur")
	reinitialiser()
	nomRouge := "Pokemon - Version Rouge (France) (SGB Enhanced)"
	rouge := romZip(t, nomRouge, nomRouge+".gb", enteteGB("POKEMON RED", 0x91E6))
	savRouge := filepath.Join(tmp, "gb", nomRouge+".sav")
	sram1, horloge := aleatoire(32768, 40), suite(1, 48)
	ecrire(t, savRouge, append(append([]byte{}, sram1...), horloge...))
	run(rouge)
	cr := courante("GB-POKEMON_RED-91E6")
	verifier(len(cr) == 32768 && hachage(cr) == hachage(sram1), "le serveur a la SRAM seule (32 768 o), sous GB-POKEMON_RED-91E6")
	verifier(joue, "mGBA lancé")
	verifier(strings.Join(commandeLancee, "|") == "inutilise.exe|-C|savegamePath="+filepath.Join(tmp, "gb")+"|"+rouge, "mGBA reçoit le dossier GB et la ROM")

	t.Log("GB : la Récupération garde le Trailer d'horloge du PC")
	reinitialiser()
	sram2 := aleatoire(32768, 41)
	envoi3ds("GB-POKEMON_RED-91E6", sram2, sram1)
	run(rouge)
	l := lire(savRouge)
	verifier(len(l) == 32816, "le fichier garde sa taille avec horloge (32 816 o)")
	verifier(len(l) == 32816 && hachage(l[:32768]) == hachage(sram2), "la SRAM est celle de la 3DS")
	verifier(len(l) == 32816 && bytes.Equal(l[32768:], horloge), "le Trailer d'horloge du PC est intact")
	verifier(len(nomsDans(filepath.Join(copies, "mgba"), "*.sav")) == 1, "Copie de sécurité faite")
	verifier(len(questions) == 0, "aucune question (horloge ignorée dans la comparaison)")

	t.Log("GBA : Clé GBA-<code>, .sav nommé comme le zip (règle mGBA)")
	reinitialiser()
	nomEm := "Pokemon - Version Emeraude (France)"
	emeraude := romZip(t, nomEm, nomEm+".gba", enteteGBA("BPEF"))
	g1 := aleatoire(131072, 42)
	ecrire(t, filepath.Join(tmp, "gba", nomEm+".sav"), append(append([]byte{}, g1...), suite(1, 16)...))
	run(emeraude)
	verifier(hachage(courante("GBA-BPEF")) == hachage(g1), "le serveur a la SRAM d'Émeraude sous GBA-BPEF")

	// --- 3DS (ADR 0009) -------------------------------------------------------------------
	t.Log("3DS : Clé et dossier Azahar lus dans la ROM")
	reinitialiser()
	x := rom3DS(t, "Pokemon X (Europe)", 0x0004000000055D00)
	dx := dossierAzahar(0x0004000000055D00)
	m1 := aleatoire(415232, 43)
	ecrire(t, filepath.Join(dx, "main"), m1)
	run(x)
	verifier(hachage(courante("3DS-0004000000055D00")) == hachage(m1), "le serveur a main sous 3DS-0004000000055D00")
	verifier(joue && strings.Join(commandeLancee, "|") == "inutilise.exe|-f|"+x, "Azahar lancé en plein écran")

	t.Log("3DS : la Récupération n'ajoute rien dans l'archive Azahar")
	reinitialiser()
	m2 := aleatoire(415232, 44)
	envoi3ds("3DS-0004000000055D00", m2, m1)
	run(x)
	verifier(hachage(lire(filepath.Join(dx, "main"))) == hachage(m2), "main est la Version de la 3DS")
	verifier(strings.Join(nomsDans(dx, "*"), ",") == filepath.Join(dx, "main"), "l'archive ne contient que main")
	verifier(strings.Join(nomsDans(filepath.Dir(dx), "*"), ",") == dx, "pas de fichier temporaire oublié")
	verifier(len(nomsDans(filepath.Join(copies, "azahar"), "*")) == 1, "Copie de sécurité hors de l'archive")

	t.Log("3DS : jeu jamais lancé dans Azahar, rien n'est créé")
	reinitialiser()
	y := rom3DS(t, "Pokemon Y (Europe)", 0x0004000000055E00)
	envoi3ds("3DS-0004000000055E00", m2, nil)
	reponses = []string{"Non"}
	run(y)
	verifier(len(questions) == 1 && strings.Contains(questions[0], "Azahar"), "question qui explique de lancer le jeu une fois")
	_, err := os.Stat(dossierAzahar(0x0004000000055E00))
	verifier(err != nil, "aucun dossier créé dans Azahar")

	t.Log("3DS hors des 8 Pokémon : partie sans Synchronisation")
	reinitialiser()
	run(rom3DS(t, "Zelda (Europe)", 0x0004000000033500))
	verifier(joue && len(questions) == 0, "Azahar lancé, aucune question")

	// --- Sauvegarde vierge = absente --------------------------------------------------------
	t.Log("Sauvegarde vierge (0xFF) sur le PC, Version sur le serveur : Récupération sans Conflit")
	reinitialiser()
	perle := romDS(t, "Pokemon Perle", "APAF")
	ecrire(t, sav("Pokemon Perle"), bytes.Repeat([]byte{0xFF}, 524288))
	pe := aleatoire(524288, 45)
	envoi3ds("APAF", pe, nil)
	run(perle)
	verifier(len(questions) == 0, "aucune question de Conflit")
	verifier(hachage(lire(sav("Pokemon Perle"))) == hachage(pe), "le PC a la Version du serveur")
	verifier(len(nomsDans(filepath.Join(copies, "melonds"), "Pokemon Perle-*.sav")) == 1, "Copie de sécurité de la Sauvegarde vierge")

	t.Log("Sauvegarde vierge (0x00 + Trailer d'horloge), rien sur le serveur : rien n'est envoyé")
	reinitialiser()
	nomOr := "Pokemon - Version Or (France)"
	or := romZip(t, nomOr, nomOr+".gbc", enteteGB("POKEMON_GLDAAUF", 0x6FC6))
	ecrire(t, filepath.Join(tmp, "gb", nomOr+".sav"), append(make([]byte, 32768), suite(1, 48)...))
	run(or)
	st, _ := api("GET", "/saves/GB-POKEMON_GLDAAUF-6FC6", nil, nil)
	verifier(st == 404, "le serveur n'a rien reçu")
	verifier(joue, "mGBA lancé")

	t.Log("3DS : un main à 0x00 n'est pas une Sauvegarde vierge (il part)")
	reinitialiser()
	lune := rom3DS(t, "Pokemon Lune (Europe)", 0x0004000000175E00)
	ecrire(t, filepath.Join(dossierAzahar(0x0004000000175E00), "main"), make([]byte, 1024))
	run(lune)
	st, _ = api("GET", "/saves/3DS-0004000000175E00", nil, nil)
	verifier(st == 200, "le serveur a main")

	// --- Nouveautés v3 ----------------------------------------------------------------------
	t.Log("run --rom … -- <commande> : la commande quelconque est enveloppée")
	reinitialiser()
	v14 := suite(14, 213)
	partie = func() { ecrire(t, noire, v14) } // avant : v13 sur le PC et sur le serveur, à jour
	run("--rom", rom, "--", "lanceur.exe", "-rom", rom)
	verifier(len(questions) == 0, "aucune question")
	verifier(strings.Join(commandeLancee, "|") == "lanceur.exe|-rom|"+rom, "la commande donnée est lancée telle quelle")
	verifier(hachage(courante("IRBF")) == hachage(v14), "la Sauvegarde jouée part sur le serveur")

	t.Log("Deux émulateurs pour le même Système sans --emulator : erreur, rien n'est lancé")
	reinitialiser()
	double := ecrireConfig(t, filepath.Join(tmp, "cfg-double", "config.json"), Config{URL: url, Jeton: "jeton-test",
		Emulateurs: map[string]*Emulateur{
			"melonds":  {Exe: "a.exe", Sauvegardes: map[string]string{"DS": saves}},
			"portable": {Profil: "melonds", Exe: "b.exe", Sauvegardes: map[string]string{"DS": filepath.Join(tmp, "saves-portable")}},
		}})
	reponses = []string{"OK"}
	verifier(principal([]string{"run", "--config", double, rom}) == 1, "code de sortie 1")
	verifier(len(questions) == 1 && strings.Contains(questions[0], "--emulator") && !joue, "message qui demande --emulator")
	reinitialiser()
	ecrire(t, filepath.Join(tmp, "saves-portable", "Pokemon Noire.sav"), v14)
	principal([]string{"run", "--config", double, "--emulator", "portable", rom})
	verifier(joue && commandeLancee[0] == "b.exe", "--emulator choisit l'émulateur")
	verifier(len(questions) == 0, "Client distinct : sa Base propre, sans Conflit")

	t.Log("launch : tous les jeux de l'émulateur avant et après, l'émulateur sans ROM")
	reinitialiser()
	savesL, romsL := filepath.Join(tmp, "saves-launch"), filepath.Join(tmp, "roms-launch")
	cfgL := ecrireConfig(t, filepath.Join(tmp, "cfg-launch", "config.json"), Config{URL: url, Jeton: "jeton-test",
		Emulateurs: map[string]*Emulateur{"melonds": {Exe: "melon.exe", Sauvegardes: map[string]string{"DS": savesL}, Roms: map[string]string{"DS": romsL}}}})
	for nom, code := range map[string]string{"Pokemon Diamant": "ADAF", "Pokemon HeartGold": "IPKF"} {
		o := make([]byte, 0x200)
		copy(o[0x0C:], code)
		ecrire(t, filepath.Join(romsL, "sous-dossier", nom+".nds"), o)
	}
	ecrire(t, filepath.Join(romsL, "notes.txt"), []byte("pas une ROM"))
	diamant, hg := suite(40, 239), suite(41, 240)
	ecrire(t, filepath.Join(savesL, "Pokemon Diamant.sav"), diamant)
	envoi3ds("IPKF", hg, nil)
	diamant2 := suite(42, 241)
	partie = func() {
		verifier(hachage(lire(filepath.Join(savesL, "Pokemon HeartGold.sav"))) == hachage(hg), "HeartGold récupéré avant la partie")
		verifier(hachage(courante("ADAF")) == hachage(diamant), "Diamant envoyé avant la partie")
		ecrire(t, filepath.Join(savesL, "Pokemon Diamant.sav"), diamant2)
	}
	verifier(principal([]string{"launch", "--config", cfgL, "melonds"}) == 0, "code de sortie 0")
	verifier(strings.Join(commandeLancee, "|") == "melon.exe", "melonDS lancé sans ROM")
	verifier(hachage(courante("ADAF")) == hachage(diamant2), "Diamant joué envoyé après la partie")
	verifier(len(questions) == 0, "aucune question")

	// --- Profils (phase 2) -------------------------------------------------------------------
	t.Log("RetroArch : dossier lu dans retroarch.cfg, .srm dans le sous-dossier du cœur, -L <cœur>")
	reinitialiser()
	ra := filepath.Join(tmp, "retroarch")
	ecrire(t, filepath.Join(ra, "retroarch.cfg"), []byte("savefile_directory = \""+filepath.ToSlash(filepath.Join(tmp, "ra-saves"))+"\"\n"))
	cfgRA := ecrireConfig(t, filepath.Join(tmp, "cfg-ra", "config.json"), Config{URL: url, Jeton: "jeton-test",
		Emulateurs: map[string]*Emulateur{"retroarch": {Exe: filepath.Join(ra, "retroarch.exe"), Coeurs: map[string]string{"GB": "gambatte_libretro.dll"}}}})
	jaune := filepath.Join(tmp, "roms-ra", "Pokemon Jaune.gb")
	ecrire(t, jaune, enteteGB("POKEMON YELLOW", 0x1234))
	sj := aleatoire(32768, 50)
	envoi3ds("GB-POKEMON_YELLOW-1234", sj, nil)
	principal([]string{"run", "--config", cfgRA, jaune})
	verifier(hachage(lire(filepath.Join(tmp, "ra-saves", "Gambatte", "Pokemon Jaune.srm"))) == hachage(sj), "Version récupérée dans ra-saves/Gambatte/Pokemon Jaune.srm")
	verifier(strings.Join(commandeLancee, "|") == filepath.Join(ra, "retroarch.exe")+"|-L|gambatte_libretro.dll|"+jaune, "RetroArch lancé avec le cœur et la ROM")
	verifier(len(questions) == 0, "aucune question (pas d'état rechargé)")

	t.Log("mGBA autoload=1 : question après une Récupération, rien sinon")
	reinitialiser()
	mg := filepath.Join(tmp, "mgba-auto")
	ecrire(t, filepath.Join(mg, "portable.ini"), nil)
	ecrire(t, filepath.Join(mg, "config.ini"), []byte("[ports.qt]\nautoload=1\n"))
	cfgA := ecrireConfig(t, filepath.Join(tmp, "cfg-auto", "config.json"), Config{URL: url, Jeton: "jeton-test",
		Emulateurs: map[string]*Emulateur{"mgba": {Exe: filepath.Join(mg, "mGBA.exe"), Sauvegardes: map[string]string{"GBA": filepath.Join(tmp, "gba-auto")}}}})
	saphir := filepath.Join(tmp, "roms-auto", "Pokemon Saphir.gba")
	ecrire(t, saphir, enteteGBA("AXPF"))
	envoi3ds("GBA-AXPF", aleatoire(131072, 51), nil)
	reponses = []string{"Ne pas jouer"}
	principal([]string{"run", "--config", cfgA, saphir})
	verifier(len(questions) == 1 && strings.Contains(questions[0], "autoload=0"), "question qui explique autoload")
	verifier(!joue, "« Ne pas jouer » : mGBA non lancé")
	reinitialiser()
	principal([]string{"run", "--config", cfgA, saphir})
	verifier(joue && len(questions) == 0, "Sauvegarde à jour : pas de question, mGBA lancé")
	verifier(strings.Contains(string(lire(filepath.Join(tmp, "cfg-auto", "3dssavesync.log"))), "avertissement : mGBA"), "l'avertissement est écrit dans le journal")

	t.Log("migrate-v2 : sync.config.json et base.json repris, rien n'est remplacé")
	reinitialiser()
	v2dir := filepath.Join(tmp, "v2")
	ecrire(t, filepath.Join(v2dir, "sync.config.json"), []byte("\xef\xbb\xbf"+`{"url":"https://saves.example","jeton":"j","melonds":"m.exe","sauvegardes":"D:\\S\\DS",
		"mgba":"g.exe","sauvegardes_gb":"D:\\S\\GB","sauvegardes_gba":"D:\\S\\GBA","azahar":"a.exe","azahar_sdmc":"D:\\S\\sdmc","copies_3ds":"D:\\S\\c"}`))
	ecrire(t, filepath.Join(v2dir, "base.json"), []byte("\xef\xbb\xbf"+`{"CPUF":"h1","GB-POKEMON_BLUE-56A4":"h2","GBA-BPEF":"h3","3DS-00040000001B5000":"h4"}`))
	cible := filepath.Join(tmp, "migre", "config.json")
	verifier(principal([]string{"migrate-v2", "--config", cible, v2dir}) == 0, "migration réussie")
	var mc Config
	lireJSON(cible, &mc)
	verifier(mc.URL == "https://saves.example" && mc.Jeton == "j" && len(mc.Emulateurs) == 3, "URL, jeton, 3 émulateurs")
	verifier(mc.Emulateurs["mgba"].Sauvegardes["GBA"] == `D:\S\GBA` && mc.Emulateurs["azahar"].Sauvegardes["3DS"] == `D:\S\sdmc`, "dossiers repris")
	b := map[string]map[string]string{}
	for _, e := range []string{"melonds", "mgba", "azahar"} {
		m := map[string]string{}
		lireJSON(filepath.Join(tmp, "migre", "bases", e+".json"), &m)
		b[e] = m
	}
	verifier(b["melonds"]["CPUF"] == "h1" && b["mgba"]["GB-POKEMON_BLUE-56A4"] == "h2" && b["mgba"]["GBA-BPEF"] == "h3" && b["azahar"]["3DS-00040000001B5000"] == "h4", "Bases réparties par émulateur")
	avantMigr := string(lire(cible))
	reponses = []string{"OK"}
	verifier(principal([]string{"migrate-v2", "--config", cible, v2dir}) == 1 && string(lire(cible)) == avantMigr, "seconde migration refusée, rien de remplacé")

	t.Logf("%d vérifications", verifications)
}
