package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// Assistant web : accès réservé à la page (hôte et secret), page en français, état, test du serveur, enregistrement.
func TestAssistantWeb(t *testing.T) {
	maisonFactice(t)
	reinitialiser()
	serveur := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "jeton-web"})
	defer serveur.Close()
	exeEmu := filepath.Join(t.TempDir(), "melonDS.exe")
	ecrire(t, exeEmu, []byte("melon"))
	detecterAvant := detecter
	t.Cleanup(func() { detecter = detecterAvant })
	detecter = func(string) []Trouve { return []Trouve{{Profil: "melonds", Exe: exeEmu, Source: "test"}} }
	config := filepath.Join(t.TempDir(), "config.json")
	a := &AssistantWeb{config: config, secret: "s3cret", hote: "127.0.0.1:9", fin: make(chan struct{})}

	requete := func(methode, hote, chemin, corps string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(methode, "http://"+hote+chemin, strings.NewReader(corps))
		r.Host = hote
		if methode == "POST" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	if requete("GET", "127.0.0.1:9", "/autre/", "").Code != 404 || requete("GET", "evil.example:9", "/s3cret/", "").Code != 404 {
		t.Error("sans le secret ou avec un autre hôte : 404 attendu")
	}
	if w := requete("GET", "127.0.0.1:9", "/s3cret/", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Tester la connexion") {
		t.Errorf("page attendue : %d", w.Code)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:9/s3cret/api/etat", strings.NewReader("{}"))
	r.Host = "127.0.0.1:9"
	r.Header.Set("Content-Type", "text/plain") // formulaire d'une autre page : refusé
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Errorf("POST sans JSON : 405 attendu, %d", w.Code)
	}

	var etat EtatWeb
	json.Unmarshal(requete("POST", "127.0.0.1:9", "/s3cret/api/etat", "{}").Body.Bytes(), &etat)
	if etat.Existe || len(etat.Emulateurs) != 1 || !etat.Emulateurs[0].Actif || etat.Emulateurs[0].Systemes[0].Systeme != "DS" {
		t.Fatalf("état initial : %+v", etat)
	}

	var test map[string]any
	json.Unmarshal(requete("POST", "127.0.0.1:9", "/s3cret/api/tester", `{"URL":"`+serveur.URL+`","Jeton":"faux"}`).Body.Bytes(), &test)
	if test["Ok"] != false || !strings.Contains(test["Message"].(string), "refuse ce jeton") {
		t.Errorf("mauvais jeton : %v", test)
	}

	emu := etat.Emulateurs[0]
	emu.Systemes[0].Sauvegardes = filepath.Join(t.TempDir(), "saves")
	choix := ChoixWeb{URL: serveur.URL, Jeton: "jeton-web", Emulateurs: []EmuWeb{emu}}
	corps, _ := json.Marshal(choix)
	var res struct {
		Lignes []string
		Erreur string
	}
	json.Unmarshal(requete("POST", "127.0.0.1:9", "/s3cret/api/appliquer", string(corps)).Body.Bytes(), &res)
	if res.Erreur != "" {
		t.Fatalf("enregistrement : %s", res.Erreur)
	}
	var cfg Config
	lireJSON(config, &cfg)
	if cfg.URL != serveur.URL || cfg.Jeton != "jeton-web" || cfg.Emulateurs["melonds"] == nil ||
		cfg.Emulateurs["melonds"].Sauvegardes["DS"] != emu.Systemes[0].Sauvegardes {
		t.Fatalf("config.json : %+v", cfg)
	}

	choix.Jeton, choix.URL = "", "http://127.0.0.1:1" // serveur injoignable : rien n'est écrit
	corps, _ = json.Marshal(choix)
	avant := string(lire(config))
	json.Unmarshal(requete("POST", "127.0.0.1:9", "/s3cret/api/appliquer", string(corps)).Body.Bytes(), &res)
	if res.Erreur == "" || string(lire(config)) != avant {
		t.Errorf("serveur injoignable : erreur et config.json intact attendus (%q)", res.Erreur)
	}
	json.Unmarshal(requete("POST", "127.0.0.1:9", "/s3cret/api/etat", "{}").Body.Bytes(), &etat)
	if !etat.Existe || !etat.JetonPresent || etat.URL != serveur.URL || strings.Contains(requete("POST", "127.0.0.1:9", "/s3cret/api/etat", "{}").Body.String(), "jeton-web") {
		t.Errorf("état après enregistrement (jamais le jeton) : %+v", etat)
	}

	// « Ouvrir » : l'émulateur configuré, sans jeu, par launch ; un nom inconnu ne lance rien.
	var lance []string
	ouvrirDetache = func(exe string, args []string) error { lance = append([]string{exe}, args...); return nil }
	var ouvre map[string]any
	if len(etat.Configures) != 1 || etat.Configures[0] != "melonds" {
		t.Errorf("boutons « Ouvrir » : %v", etat.Configures)
	}
	json.Unmarshal(requete("POST", "127.0.0.1:9", "/s3cret/api/ouvrir", `{"Nom":"melonds"}`).Body.Bytes(), &ouvre)
	if ouvre["Ok"] != true || strings.Join(lance[1:], " ") != "launch --config "+config+" melonds" {
		t.Errorf("ouvrir melonds : %v, commande %q", ouvre, lance)
	}
	lance = nil
	json.Unmarshal(requete("POST", "127.0.0.1:9", "/s3cret/api/ouvrir", `{"Nom":"inconnu"}`).Body.Bytes(), &ouvre)
	if ouvre["Ok"] != false || lance != nil {
		t.Errorf("émulateur inconnu : refus attendu, rien de lancé (%v, %q)", ouvre, lance)
	}
}

// Émulateur personnalisé dans la page : contrôles, aperçu, enregistrement.
func TestAssistantWebPersonnalise(t *testing.T) {
	maisonFactice(t)
	serveur := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "j"})
	defer serveur.Close()
	detecterAvant := detecter
	t.Cleanup(func() { detecter = detecterAvant })
	detecter = func(string) []Trouve { return nil }
	config := filepath.Join(t.TempDir(), "config.json")
	a := &AssistantWeb{config: config}
	saves := filepath.Join(t.TempDir(), "saves")
	x := EmuWeb{Nom: "gearboy", Profil: "gearboy", Exe: `C:\Emu\gearboy.exe`, Actif: true, Personnalise: true, Commande: `-rom {rom} -batt "{sauvegardes}"`,
		Extension: "sav", Systemes: []SysWeb{{Systeme: "GB", Sauvegardes: saves}}}
	if r := apercu(x); r["Erreur"] != "" || !strings.HasPrefix(r["Commande"], `C:\Emu\gearboy.exe -rom `) || !strings.Contains(r["Commande"], "Pokemon Cristal.gbc") ||
		!strings.Contains(r["Commande"], saves) {
		t.Errorf("aperçu : %v", r)
	}
	for _, mauvais := range []EmuWeb{
		{Nom: "mgba", Personnalise: true, Commande: "{rom}", Systemes: x.Systemes},                                // nom d'un profil intégré
		{Nom: "GEARBOY", Personnalise: true, Commande: "{rom}", Systemes: x.Systemes},                             // majuscules
		{Nom: "gearboy", Personnalise: true, Commande: "-rom", Systemes: x.Systemes},                              // sans {rom}
		{Nom: "gearboy", Personnalise: true, Commande: "{rom}"},                                                   // aucun Système
		{Nom: "gearboy", Personnalise: true, Commande: "{rom}", Systemes: []SysWeb{{Systeme: "GB"}}, Actif: true}, // dossier vide
	} {
		mauvais.Actif = true
		if _, err := a.appliquer(ChoixWeb{URL: serveur.URL, Jeton: "j", Emulateurs: []EmuWeb{mauvais}}); err == nil {
			t.Errorf("refus attendu : %+v", mauvais)
		}
	}
	if _, err := a.appliquer(ChoixWeb{URL: serveur.URL, Jeton: "j", Emulateurs: []EmuWeb{x}}); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	lireJSON(config, &cfg)
	e := cfg.Emulateurs["gearboy"]
	if e == nil || strings.Join(e.Args, "|") != "-rom|{rom}|-batt|{sauvegardes}" || e.Extension != ".sav" || e.Sauvegardes["GB"] != saves {
		t.Fatalf("émulateur personnalisé dans config.json : %+v", e)
	}
	var etat EtatWeb = a.etat()
	if len(etat.Emulateurs) != 1 || !etat.Emulateurs[0].Personnalise || etat.Emulateurs[0].Commande != `-rom {rom} -batt {sauvegardes}` {
		t.Errorf("relu par la page : %+v", etat.Emulateurs)
	}
}

// Section ROM hacks : ajout (Clé créée, copie de config.json avant), liste, retrait.
func TestAliasWeb(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.json")
	if err := ecrireJSON(config, &Config{URL: "http://192.0.2.5", Jeton: "j"}, true); err != nil {
		t.Fatal(err)
	}
	a := &AssistantWeb{config: config}
	r := a.aliasWeb("Radical Red (4.10)", "GBA", false)
	var cfg Config
	lireJSON(config, &cfg)
	if r["Erreur"] != nil || !formesCle["GBA"].MatchString(cfg.Alias["Radical Red (4.10)"]) || strings.Join(r["Noms"].([]string), "|") != "Radical Red (4.10)" {
		t.Errorf("ajout : %v, %v", r, cfg.Alias)
	}
	if c, _ := filepath.Glob(config + ".avant-3dssavesync-*"); len(c) != 1 {
		t.Errorf("copie de config.json avant l'ajout : %v", c)
	}
	if r := a.aliasWeb("a=b", "GBA", false); r["Erreur"] == nil {
		t.Error("nom invalide accepté")
	}
	a.aliasWeb("Radical Red (4.10)", "", true)
	cfg = Config{}
	lireJSON(config, &cfg)
	if len(cfg.Alias) != 0 || cfg.Jeton != "j" {
		t.Errorf("retrait : %+v", cfg)
	}
}
