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

// Fausse sauvegarde de Pokémon X (taille 0x65600) : temps de jeu et dresseur aux positions de pokemon.go.
func pokemonX(heures int, dresseur string) []byte {
	d := make([]byte, 0x65600)
	le.PutUint16(d[0x1800:], uint16(heures))
	d[0x1802] = 30
	for i, c := range dresseur {
		le.PutUint16(d[0x14048+2*i:], uint16(c))
	}
	return d
}

// Envoi d'un autre appareil (Client « 3ds »).
func envoiAutre(t *testing.T, s *Session, cle string, o, avant []byte) {
	t.Helper()
	autre := *s
	autre.profil = "3ds"
	cond := map[string]string{"If-None-Match": "*"}
	if avant != nil {
		cond = map[string]string{"If-Match": `"` + hachage(avant) + `"`}
	}
	if st, _, err := autre.api("PUT", "/saves/"+cle, o, cond); st != 200 || err != nil {
		t.Fatalf("envoi %s : %d %v", cle, st, err)
	}
}

func couranteDe(s *Session, cle string) []byte {
	_, o, _ := s.api("GET", "/saves/"+cle, nil, nil)
	return o
}

// Restauration contre les deux serveurs : le serveur local (serveur.go) et server.py.
func TestRestauration(t *testing.T) {
	local := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "jeton-test"})
	defer local.Close()
	t.Run("serveur local", func(t *testing.T) { verifierRestauration(t, local.URL) })
	t.Run("server.py", func(t *testing.T) {
		tmp = t.TempDir()
		demarrerServeur(t)
		verifierRestauration(t, url)
	})
}

func verifierRestauration(t *testing.T, adresse string) {
	config := ecrireConfig(t, filepath.Join(t.TempDir(), "config.json"), Config{URL: adresse, Jeton: "jeton-test"})
	s, err := sessionRestauration(config)
	if err != nil {
		t.Fatal(err)
	}
	cle := "3DS-0004000000055D00"
	v1, v2, v3 := pokemonX(10, "Sacha"), pokemonX(152, "Sacha"), pokemonX(160, "Ondine")
	envoiAutre(t, s, cle, v1, nil)
	envoiAutre(t, s, cle, v2, v1)

	lignes, err := s.lignesHisto(cle)
	if err != nil || len(lignes) != 2 {
		t.Fatalf("liste : %v %+v", err, lignes)
	}
	if l := lignes[0]; l.N != 2 || !l.Courante || l.Client != "3ds" || l.Taille != "406 Kio" || l.Partie != "152 h 30 · Sacha" ||
		!regexp.MustCompile(`^\d\d/\d\d/\d{4} \d\d:\d\d$`).MatchString(l.Date) {
		t.Errorf("version la plus récente d'abord : %+v", l)
	}
	if l := lignes[1]; l.N != 1 || l.Courante || l.Partie != "10 h 30 · Sacha" {
		t.Errorf("version 1 : %+v", l)
	}

	if deja, err := s.restaurer(cle, lignes[0].Fichier, lignes[0].Hash); !deja || err != nil {
		t.Errorf("même contenu que la courante : rien à faire attendu (%v %v)", deja, err)
	}
	if n, _ := s.lignesHisto(cle); len(n) != 2 {
		t.Errorf("même contenu : aucune nouvelle Version attendue (%d)", len(n))
	}

	envoiAutre(t, s, cle, v3, v2) // un appareil envoie après l'affichage de la liste
	if _, err := s.restaurer(cle, lignes[1].Fichier, lignes[0].Hash); err == nil || !strings.Contains(err.Error(), "vient de changer sur le serveur") {
		t.Errorf("Envoi concurrent : refus attendu (%v)", err)
	}
	if !bytes.Equal(couranteDe(s, cle), v3) {
		t.Error("Envoi concurrent : la Version de l'appareil doit rester courante")
	}

	lignes, _ = s.lignesHisto(cle)
	if deja, err := s.restaurer(cle, lignes[2].Fichier, lignes[0].Hash); deja || err != nil {
		t.Fatalf("restauration de la version 1 : %v %v", deja, err)
	}
	lignes, _ = s.lignesHisto(cle)
	if len(lignes) != 4 || lignes[0].N != 4 || lignes[0].Hash != hachage(v1) || lignes[0].Client != "restauration" || !bytes.Equal(couranteDe(s, cle), v1) {
		t.Errorf("la version 1 doit être la Version courante n° 4, envoyée par « restauration » : %+v", lignes)
	}
	for _, d := range []string{"bases", "copies"} { // aucune Base, aucune sauvegarde locale touchée
		if _, err := os.Stat(filepath.Join(filepath.Dir(config), d)); err == nil {
			t.Errorf("restauration : %s ne doit pas être créé", d)
		}
	}
	if _, err := s.restaurer(cle, "../../server.py", lignes[0].Hash); err == nil {
		t.Error("nom de version hors de l'Historique : refus attendu")
	}
}

func TestCommandeRestore(t *testing.T) {
	srv := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "j"})
	defer srv.Close()
	config := ecrireConfig(t, filepath.Join(t.TempDir(), "config.json"), Config{URL: srv.URL, Jeton: "j"})
	s, _ := sessionRestauration(config)
	envoiAutre(t, s, "ADAF", []byte("un"), nil)
	envoiAutre(t, s, "ADAF", []byte("deux"), []byte("un"))
	commande := func(entree string, args ...string) (string, error) {
		var sortie bytes.Buffer
		_, err := commandeRestore(config, args, false, &Questions{in: bufio.NewReader(strings.NewReader(entree)), out: &sortie})
		return sortie.String(), err
	}
	if o, err := commande(""); err != nil || !strings.Contains(o, "ADAF") {
		t.Errorf("liste des jeux : %v\n%s", err, o)
	}
	if o, err := commande("", "adaf"); err != nil || !strings.Contains(o, "   2  ") || !strings.Contains(o, "(actuelle)") || !strings.Contains(o, "   1  ") {
		t.Errorf("liste des versions : %v\n%s", err, o)
	}
	if o, _ := commande("n\n", "ADAF", "1"); !strings.Contains(o, "Rien n'a été modifié.") || string(couranteDe(s, "ADAF")) != "deux" {
		t.Errorf("réponse non : rien ne change\n%s", o)
	}
	if o, _ := commande("", "ADAF", "2"); !strings.Contains(o, "identique à la version actuelle") {
		t.Errorf("version actuelle : rien à faire\n%s", o)
	}
	if o, err := commande("", "ADAF", "1", "--oui"); err != nil || !strings.Contains(o, "Version restaurée") || string(couranteDe(s, "ADAF")) != "un" {
		t.Errorf("--oui : restauration sans question attendue : %v\n%s", err, o)
	}
	if _, err := commande("", "ADAF", "9"); err == nil {
		t.Error("numéro inconnu : erreur attendue")
	}
}

// Page : jeux (nom lu dans les dossiers des ROM), versions, restauration, refus après un Envoi concurrent.
func TestAssistantWebHistorique(t *testing.T) {
	srv := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "j"})
	defer srv.Close()
	roms := t.TempDir()
	rom := make([]byte, 0x200)
	copy(rom[0x0C:], "CPUF")
	ecrire(t, filepath.Join(roms, "Pokemon Platine.nds"), rom)
	config := ecrireConfig(t, filepath.Join(t.TempDir(), "config.json"), Config{URL: srv.URL, Jeton: "j",
		Emulateurs: map[string]*Emulateur{"melonds": {Exe: "melonDS.exe", Roms: map[string]string{"DS": roms}}}})
	s, _ := sessionRestauration(config)
	envoiAutre(t, s, "CPUF", []byte("un"), nil)
	envoiAutre(t, s, "CPUF", []byte("deux"), []byte("un"))

	a := &AssistantWeb{config: config, secret: "s", hote: "127.0.0.1:9", fin: make(chan struct{})}
	requete := func(chemin, corps string, v any) {
		r := httptest.NewRequest("POST", "http://127.0.0.1:9/s/api/"+chemin, strings.NewReader(corps))
		r.Host = "127.0.0.1:9"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		json.Unmarshal(w.Body.Bytes(), v)
	}
	var jeux struct{ Jeux []JeuHisto }
	requete("jeux", "{}", &jeux)
	if len(jeux.Jeux) != 1 || jeux.Jeux[0] != (JeuHisto{"CPUF", "Pokemon Platine"}) {
		t.Errorf("jeux : %+v", jeux)
	}
	var v struct{ Versions []LigneHisto }
	requete("versions", `{"Cle":"CPUF"}`, &v)
	if len(v.Versions) != 2 || !v.Versions[0].Courante {
		t.Fatalf("versions : %+v", v)
	}
	var r struct {
		Ok      bool
		Message string
	}
	corps, _ := json.Marshal(map[string]string{"Cle": "CPUF", "Fichier": v.Versions[1].Fichier, "Courant": v.Versions[0].Hash})
	requete("restaurer", string(corps), &r)
	if !r.Ok || !strings.Contains(r.Message, "Version restaurée") || string(couranteDe(s, "CPUF")) != "un" {
		t.Errorf("restauration : %+v", r)
	}
	requete("restaurer", string(corps), &r) // même liste, mais la Version courante a changé depuis
	if r.Ok || !strings.Contains(r.Message, "vient de changer") {
		t.Errorf("liste périmée : refus attendu, %+v", r)
	}
}
