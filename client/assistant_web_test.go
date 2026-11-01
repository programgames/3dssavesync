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
	if test["Ok"] != false || !strings.Contains(test["Message"].(string), "401") {
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
}
