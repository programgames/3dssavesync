package main

// Lot D : ROM hack et alias (ADR 0014), Envois en attente (D5), une lecture de la liste du serveur par passe (D6).
// Mêmes doublures que lanceur_test.go ; le vrai serveur.

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleAlias(t *testing.T) {
	c := &Config{Alias: map[string]string{"Hack DS": "0rad", "Hack GBA": " GBA-0RAD ", "Hack GB": "GB-MONHACK-0001",
		"Trop long": "GBA-BPRE-RADICALRED", "Mauvais Système": "0RAD"}}
	for _, k := range []struct{ systeme, nom, cle, attendu string }{
		{"DS", "Hack DS", "ADAF", "0RAD"},
		{"GBA", "Hack GBA", "GBA-BPRE", "GBA-0RAD"},
		{"GB", "Hack GB", "GB-POKEMON_RED-91E6", "GB-MONHACK-0001"},
		{"GBA", "Sans alias", "GBA-BPRE", "GBA-BPRE"},
		{"GBA", "Trop long", "GBA-BPRE", ""},
		{"GBA", "Mauvais Système", "GBA-BPRE", ""},
		{"3DS", "Hack DS", "3DS-0004000000055D00", "3DS-0004000000055D00"}, // jamais d'alias 3DS
	} {
		cle, err := c.cleAlias(k.systeme, k.nom, k.cle)
		if cle != k.attendu || (err != nil) != (k.attendu == "") {
			t.Errorf("%s %s : %q %v, attendu %q", k.systeme, k.nom, cle, err, k.attendu)
		}
	}
}

// Transport qui compte les requêtes et peut couper le réseau.
type reseau struct {
	coupe          bool
	listes, autres int // GET /saves, autres GET
	essais         int // toutes les requêtes, coupées ou non
}

func (r *reseau) RoundTrip(q *http.Request) (*http.Response, error) {
	r.essais++
	if r.coupe {
		return nil, errors.New("réseau coupé")
	}
	if q.Method == "GET" && q.URL.Path == "/saves" {
		r.listes++
	} else if q.Method == "GET" {
		r.autres++
	}
	return http.DefaultTransport.RoundTrip(q)
}

func TestRomHackEtAlias(t *testing.T) {
	tmp = t.TempDir()
	maisonFactice(t)
	demarrerServeur(t)
	gba, roms := filepath.Join(tmp, "gba"), filepath.Join(tmp, "roms-gba")
	cfg := Config{URL: url, Jeton: "jeton-test", Copies: filepath.Join(tmp, "copies"),
		Emulateurs: map[string]*Emulateur{"mgba": {Exe: "inutilise.exe", Sauvegardes: map[string]string{"GBA": gba}, Roms: map[string]string{"GBA": roms}}}}
	config := ecrireConfig(t, filepath.Join(tmp, "cfg", "config.json"), cfg)
	run := func(rom string) int { return principal([]string{"run", "--config", config, rom}) }
	rf, rr := filepath.Join(roms, "Pokemon Rouge Feu.gba"), filepath.Join(roms, "Pokemon Radical Red.gba")
	ecrire(t, rf, enteteGBA("BPRE"))
	ecrire(t, rr, enteteGBA("BPRE"))
	savRF, savRR := filepath.Join(gba, "Pokemon Rouge Feu.sav"), filepath.Join(gba, "Pokemon Radical Red.sav")
	f1, r1, r2 := aleatoire(131072, 1), aleatoire(131072, 2), aleatoire(131072, 3)
	ecrire(t, savRF, f1)
	ecrire(t, savRR, r1)
	retenue := func() string {
		m := map[string]string{}
		lireJSON(filepath.Join(tmp, "cfg", "bases", "roms", "mgba.json"), &m)
		return m["GBA-BPRE"]
	}

	t.Log("Le jeu d'origine se synchronise ; sa ROM est retenue à côté de la Base")
	reinitialiser()
	run(rf)
	if hachage(courante("GBA-BPRE")) != hachage(f1) || retenue() != rf {
		t.Fatalf("serveur ou ROM retenue faux : %q", retenue())
	}

	t.Log("Le hack (même Clé, autre fichier qui existe) : partie sans Synchronisation, une notification")
	reinitialiser()
	partie = func() { ecrire(t, savRR, r2) }
	if run(rr) != 0 || !joue {
		t.Error("le hack doit démarrer")
	}
	if len(notifications) != 1 || !contient(notifications, "Pokemon Radical Red.gba est vu comme le même jeu que Pokemon Rouge Feu.gba") ||
		!contient(notifications, "alias") {
		t.Errorf("notification : %q", notifications)
	}
	if hachage(courante("GBA-BPRE")) != hachage(f1) || len(historique("GBA-BPRE")) != 1 || hachage(lire(savRR)) != hachage(r2) {
		t.Error("le serveur doit garder la Sauvegarde d'origine, et le hack la sienne")
	}

	t.Log("ROM retenue disparue (déplacée, extraction temporaire) : la nouvelle est acceptée et retenue")
	reinitialiser()
	ailleurs := filepath.Join(tmp, "extrait", "Pokemon Rouge Feu.gba")
	os.MkdirAll(filepath.Dir(ailleurs), 0o755)
	if err := os.Rename(rf, ailleurs); err != nil {
		t.Fatal(err)
	}
	run(ailleurs)
	if len(notifications) != 0 || len(questions) != 0 || retenue() != ailleurs {
		t.Errorf("acceptée sans message : %q %q, retenue %q", notifications, questions, retenue())
	}

	t.Log("Alias : le hack a sa propre Clé sur le serveur (espaces et minuscules acceptés)")
	reinitialiser()
	cfg.Alias = map[string]string{"Pokemon Radical Red": " gba-0rad "}
	ecrireConfig(t, config, cfg)
	run(rr)
	if hachage(courante("GBA-0RAD")) != hachage(r2) || hachage(courante("GBA-BPRE")) != hachage(f1) || len(notifications) != 0 {
		t.Errorf("le hack doit partir sous GBA-0RAD : %q", notifications)
	}

	t.Log("Alias invalide : partie sans Synchronisation, jamais sous la Clé de l'en-tête")
	reinitialiser()
	cfg.Alias = map[string]string{"Pokemon Radical Red": "GBA-BPRE-RADICALRED"}
	ecrireConfig(t, config, cfg)
	partie = func() { ecrire(t, savRR, aleatoire(131072, 4)) }
	run(rr)
	if !joue || !contient(notifications, "« GBA-BPRE-RADICALRED » de Pokemon Radical Red est invalide") || !contient(notifications, "GBA-0RAD") {
		t.Errorf("notification : %q", notifications)
	}
	if hachage(courante("GBA-BPRE")) != hachage(f1) || hachage(courante("GBA-0RAD")) != hachage(r2) {
		t.Error("rien ne doit partir")
	}

	t.Log("launch : deux ROM de même Clé, aucune synchronisée, une notification ; les autres jeux le sont")
	reinitialiser()
	cfg.Alias = nil
	ecrireConfig(t, config, cfg)
	if err := os.Rename(ailleurs, rf); err != nil {
		t.Fatal(err)
	}
	ecrire(t, filepath.Join(roms, "Pokemon Emeraude.gba"), enteteGBA("BPEE"))
	em := aleatoire(131072, 5)
	ecrire(t, filepath.Join(gba, "Pokemon Emeraude.sav"), em)
	ecrire(t, savRF, aleatoire(131072, 6))
	principal([]string{"launch", "--config", config, "mgba"})
	if len(notifications) != 1 || !contient(notifications, "Pokemon Radical Red.gba, Pokemon Rouge Feu.gba") {
		t.Errorf("notification : %q", notifications)
	}
	if hachage(courante("GBA-BPRE")) != hachage(f1) || hachage(courante("GBA-BPEE")) != hachage(em) {
		t.Error("Rouge Feu ne doit pas partir, Émeraude oui")
	}
}

func TestEnvoisEnAttente(t *testing.T) {
	tmp = t.TempDir()
	maisonFactice(t)
	demarrerServeur(t)
	r := &reseau{}
	transport = r
	t.Cleanup(func() { transport = http.DefaultTransport })
	saves, gba := filepath.Join(tmp, "saves"), filepath.Join(tmp, "gba")
	config := ecrireConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Jeton: "jeton-test", Copies: filepath.Join(tmp, "copies"),
		Emulateurs: map[string]*Emulateur{
			"melonds": {Exe: "melon.exe", Sauvegardes: map[string]string{"DS": saves}},
			"mgba":    {Exe: "mgba.exe", Sauvegardes: map[string]string{"GBA": gba}},
		}})
	run := func(rom string) int { return principal([]string{"run", "--config", config, rom}) }
	noire, savNoire := romDS(t, "Pokemon Noire", "IRBF"), filepath.Join(saves, "Pokemon Noire.sav")
	emeraude := filepath.Join(tmp, "roms", "Pokemon Emeraude.gba")
	ecrire(t, emeraude, enteteGBA("BPEE"))
	attente := func() (l []enAttente) { lireJSON(filepath.Join(tmp, "cfg", "en-attente.json"), &l); return }
	couperPendant := func(o []byte) { partie = func() { ecrire(t, savNoire, o); r.coupe = true } }

	a1, a2, a3, a4, a5 := suite(1, 200), suite(2, 201), suite(3, 202), suite(4, 203), suite(5, 204)
	ecrire(t, savNoire, a1)
	run(noire)

	t.Log("Réseau coupé pendant la partie : l'Envoi est mis en attente")
	reinitialiser()
	couperPendant(a2)
	run(noire)
	if !contient(notifications, "reste sur le PC") || len(attente()) != 1 || attente()[0].Cle != "IRBF" || attente()[0].Profil != "melonds" {
		t.Fatalf("attente : %+v, notifications %q", attente(), notifications)
	}

	t.Log("Un autre jeu est lancé, réseau revenu : l'Envoi en attente part d'abord")
	reinitialiser()
	r.coupe = false
	run(emeraude)
	if hachage(courante("IRBF")) != hachage(a2) || len(attente()) != 0 || !joue || len(questions) != 0 {
		t.Errorf("serveur, attente %+v, questions %q", attente(), questions)
	}

	t.Log("Envoi en attente en Conflit : la question habituelle, rien n'est écrasé")
	reinitialiser()
	couperPendant(a3)
	run(noire)
	r.coupe = false
	envoi3ds("IRBF", a4, a2)
	reinitialiser()
	reponses = []string{"Ne rien faire"}
	run(emeraude)
	if len(questions) != 1 || !strings.Contains(questions[0], "Conflit") || hachage(courante("IRBF")) != hachage(a4) ||
		hachage(lire(savNoire)) != hachage(a3) || len(attente()) != 0 {
		t.Errorf("questions %q, attente %+v", questions, attente())
	}

	t.Log("Émulateur de l'Envoi en attente ouvert : il reste en attente")
	reinitialiser()
	couperPendant(a5)
	run(noire)
	r.coupe = false
	reinitialiser()
	instances = func(nom string) int {
		if nom == "melon" {
			return 1
		}
		return 0
	}
	run(emeraude)
	instances = func(string) int { return actifs }
	if len(attente()) != 1 || hachage(courante("IRBF")) != hachage(a4) || len(questions) != 0 {
		t.Errorf("attente %+v, questions %q", attente(), questions)
	}

	t.Log("Réseau toujours coupé au lancement suivant : un seul essai pour l'attente et l'avant-partie")
	reinitialiser()
	r.coupe, r.essais = true, 0
	reponses = []string{"Non"}
	run(emeraude)
	if r.essais != 1 || len(questions) != 1 || !strings.Contains(questions[0], "Synchronisation impossible") || len(attente()) != 1 {
		t.Errorf("%d requêtes, questions %q, attente %+v", r.essais, questions, attente())
	}

	t.Log("Réseau revenu, Conflit, Garder le PC : la Sauvegarde jouée part")
	reinitialiser()
	r.coupe = false
	reponses = []string{"Garder le PC"}
	run(emeraude)
	if hachage(courante("IRBF")) != hachage(a5) || len(attente()) != 0 {
		t.Errorf("attente %+v, questions %q", attente(), questions)
	}
}

func TestUneListeParPasse(t *testing.T) {
	tmp = t.TempDir()
	maisonFactice(t)
	demarrerServeur(t)
	r := &reseau{}
	transport = r
	t.Cleanup(func() { transport = http.DefaultTransport })
	saves, roms := filepath.Join(tmp, "saves"), filepath.Join(tmp, "roms")
	config := ecrireConfig(t, filepath.Join(tmp, "cfg", "config.json"), Config{URL: url, Jeton: "jeton-test",
		Emulateurs: map[string]*Emulateur{"melonds": {Exe: "melon.exe", Sauvegardes: map[string]string{"DS": saves}, Roms: map[string]string{"DS": roms}}}})
	for i, code := range []string{"ADAF", "APAF", "CPUF"} {
		romDS(t, code, code)
		ecrire(t, filepath.Join(saves, code+".sav"), suite(i, 100+i))
	}
	launch := func() { principal([]string{"launch", "--config", config, "melonds"}) }

	t.Log("Premier launch : trois Envois, la liste relue après chacun")
	reinitialiser()
	launch()
	if codes() != "ADAF,APAF,CPUF" {
		t.Fatalf("serveur : %s", codes())
	}

	t.Log("launch à jour : une lecture de la liste avant, une après, rien d'autre")
	reinitialiser()
	r.listes, r.autres = 0, 0
	launch()
	if r.listes != 2 || r.autres != 0 || len(questions) != 0 {
		t.Errorf("%d listes, %d autres GET, questions %q", r.listes, r.autres, questions)
	}

	t.Log("Réseau coupé : la passe s'arrête à la première erreur")
	reinitialiser()
	r.coupe, r.essais = true, 0
	reponses = []string{"Non"}
	launch()
	if r.essais != 1 || len(questions) != 1 || joue {
		t.Errorf("%d requêtes, questions %q", r.essais, questions)
	}
}
