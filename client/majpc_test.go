package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Versions comparées comme sur la 3DS (maj.c) ; « dev » ne propose jamais de mise à jour.
func TestPlusRecente(t *testing.T) {
	for _, c := range []struct {
		courante, distante string
		attendu            bool
	}{
		{"3.6.1", "v3.7.0", true}, {"3.9.1", "v3.10.0", true}, {"3.7.0", "v3.7.0", false}, {"3.8.0", "v3.7.9", false},
		{"3.7.0-rc.1", "v3.7.0", true}, {"3.7.0", "v3.7.0-rc.1", false}, {"dev", "v9.0.0", false}, {"3.7.0", "vx", false},
		{"3.7", "v3.8.0", false}, {"3.7.0", "v3.+8.0", false},
	} {
		if plusRecente(c.courante, c.distante) != c.attendu {
			t.Errorf("plusRecente(%q, %q) : %v attendu", c.courante, c.distante, c.attendu)
		}
	}
	if nomFichierPC("windows", "arm64") != "3dssavesync-windows-arm64.exe" || nomFichierPC("linux", "amd64") != "3dssavesync-linux-amd64" ||
		nomFichierPC("darwin", "arm64") != "3dssavesync-darwin-arm64" {
		t.Error("nom du fichier de la release")
	}
	// Vraie réponse de GitHub (v3.6.1).
	var r Release
	if err := json.Unmarshal(lire("../3ds/test/release_latest.json"), &r); err != nil || r.Tag != "v3.6.1" || r.Page != "https://github.com/programgames/3dssavesync/releases/tag/v3.6.1" ||
		len(r.Assets) != 3 || r.Assets[0].Nom != "3dssavesync-windows-amd64.exe" || r.Assets[0].Taille != 10036224 ||
		r.Assets[0].URL != "https://github.com/programgames/3dssavesync/releases/download/v3.6.1/3dssavesync-windows-amd64.exe" {
		t.Errorf("release_latest.json : %v %+v", err, r)
	}
	if !sommeValide("00  autre\n"+"a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3 *nom\n", "nom", []byte("123")) ||
		sommeValide("a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3  nom\n", "nom", []byte("124")) || sommeValide("", "nom", nil) {
		t.Error("sommeValide")
	}
}

// Mise à jour du PC : recherche une fois par jour, téléchargement vérifié, exe remplacé avec l'ancien gardé à côté ;
// tout échec laisse l'ancien exe en place. Jamais GitHub, jamais le vrai exe : un serveur de test, un dossier temporaire.
func TestMettreAJour(t *testing.T) {
	neuf := []byte("programme 3.7.0")
	h := sha256.Sum256(neuf)
	nom := nomFichierPC(runtime.GOOS, runtime.GOARCH)
	sommes := hex.EncodeToString(h[:]) + "  " + nom + "\n"
	demandes := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			demandes++
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "v3.7.0", "html_url": srv.URL + "/page", "assets": []map[string]any{
				{"name": nom, "size": len(neuf), "browser_download_url": srv.URL + "/exe"},
				{"name": "SHA256SUMS", "size": len(sommes), "browser_download_url": srv.URL + "/sommes"}}})
		case "/exe":
			w.Write(neuf)
		case "/sommes":
			w.Write([]byte(sommes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	va, aa, ea, ra := version, adresseRelease, espaceLibre, renommer
	t.Cleanup(func() { version, adresseRelease, espaceLibre, renommer = va, aa, ea, ra })
	version, adresseRelease = "3.6.1", srv.URL+"/latest"
	espaceLibre = func(string) (uint64, error) { return 1 << 40, nil }

	reglages := t.TempDir()
	r, err := derniereRelease(reglages, false)
	if err != nil || r.Tag != "v3.7.0" || !plusRecente(version, r.Tag) {
		t.Fatalf("derniereRelease : %v %+v", err, r)
	}
	derniereRelease(reglages, false)
	if demandes != 1 {
		t.Errorf("une seule recherche par jour : %d", demandes)
	}
	derniereRelease(reglages, true)
	ecrireJSON(filepath.Join(reglages, "maj-pc.json"), RechercheMaj{time.Now().AddDate(0, 0, -1).Format("2006-01-02"), *r}, true)
	derniereRelease(reglages, false)
	if demandes != 3 {
		t.Errorf("forcée, puis le lendemain : 3 recherches attendues, %d", demandes)
	}

	d := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	exe := filepath.Join(d, "3dssavesync"+ext)
	reste := func(attendu string) {
		t.Helper()
		if string(lire(exe)) != attendu {
			t.Errorf("exe : %q, attendu %q", lire(exe), attendu)
		}
		if f, _ := filepath.Glob(filepath.Join(d, "*tmp*")); len(f) != 0 {
			t.Errorf("fichier temporaire laissé : %v", f)
		}
	}
	ecrire(t, exe, []byte("programme 3.6.1"))

	if _, err := mettreAJour(r, exe, "plan9", "amd64"); err == nil || !strings.Contains(err.Error(), "n'a pas de fichier 3dssavesync-plan9-amd64") {
		t.Errorf("système sans fichier : %v", err)
	}
	espaceLibre = func(string) (uint64, error) { return 1000, nil }
	if _, err := mettreAJour(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "Pas assez de place") {
		t.Errorf("disque plein : %v", err)
	}
	espaceLibre = func(string) (uint64, error) { return 1 << 40, nil }
	sommes = strings.Repeat("0", 64) + "  " + nom + "\n"
	if _, err := mettreAJour(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "abîmé") {
		t.Errorf("SHA-256 différent : %v", err)
	}
	sommes = hex.EncodeToString(h[:]) + "  " + nom + "\n"
	// Le nouvel exe ne prend pas la place : l'ancien revient.
	n := 0
	renommer = func(a, b string) error {
		if n++; n == 2 {
			return errors.New("refusé")
		}
		return os.Rename(a, b)
	}
	if _, err := mettreAJour(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "l'ancien reste en place") {
		t.Errorf("renommage refusé : %v", err)
	}
	reste("programme 3.6.1")
	if f, _ := os.ReadDir(d); len(f) != 1 {
		t.Errorf("seul l'ancien exe doit rester : %v", f)
	}
	renommer = os.Rename

	copie, err := mettreAJour(r, exe, runtime.GOOS, runtime.GOARCH)
	if err != nil || copie != filepath.Join(d, "3dssavesync-3.6.1"+ext) || string(lire(copie)) != "programme 3.6.1" {
		t.Fatalf("mise à jour : %v, %s", err, copie)
	}
	reste("programme 3.7.0")
	// Encore une fois (onglet resté ouvert, autre fenêtre) : l'exe sur le disque est déjà celui de la release.
	if _, err := mettreAJour(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.HasPrefix(err.Error(), "Le programme sur le disque est déjà la version 3.7.0") {
		t.Errorf("déjà à jour : %v", err)
	}
	reste("programme 3.7.0")
	// Encore depuis la même ancienne version : la première copie n'est jamais écrasée.
	ecrire(t, exe, []byte("programme 3.6.1 bis"))
	if copie, err := mettreAJour(r, exe, runtime.GOOS, runtime.GOARCH); err != nil || copie != filepath.Join(d, "3dssavesync-3.6.1-1"+ext) ||
		string(lire(filepath.Join(d, "3dssavesync-3.6.1"+ext))) != "programme 3.6.1" || string(lire(copie)) != "programme 3.6.1 bis" {
		t.Errorf("deuxième copie : %v, %s", err, copie)
	}
}

// Une mise à jour à la fois (deux onglets, deux fenêtres) ; un verrou de plus de 15 minutes est remplacé.
func TestVerrouMaj(t *testing.T) {
	ferme := httptest.NewServer(http.NotFoundHandler()) // jamais GitHub, même si le verrou laissait passer
	ferme.Close()
	aa := adresseRelease
	t.Cleanup(func() { adresseRelease = aa })
	adresseRelease = ferme.URL
	d := t.TempDir()
	liberer, err := verrouMaj(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verrouMaj(d); err == nil || !strings.HasPrefix(err.Error(), "Une mise à jour est déjà en cours") || !strings.Contains(err.Error(), "\nDétail : ") {
		t.Errorf("deuxième mise à jour : refus attendu, %v", err)
	}
	a := &AssistantWeb{config: filepath.Join(d, "config.json"), fin: make(chan struct{})}
	if m := a.mettreAJourWeb()["Message"].(string); !strings.HasPrefix(m, "Une mise à jour est déjà en cours") {
		t.Errorf("page : %q", m)
	}
	liberer()
	if existe(filepath.Join(d, "maj-pc.verrou")) {
		t.Error("verrou retiré attendu")
	}
	liberer, _ = verrouMaj(d)
	vieux := time.Now().Add(-16 * time.Minute)
	os.Chtimes(filepath.Join(d, "maj-pc.verrou"), vieux, vieux)
	if l, err := verrouMaj(d); err != nil {
		t.Errorf("verrou abandonné : remplacé attendu, %v", err)
	} else {
		l()
	}
}

// Échecs de GitHub : un texte par cas (quoi faire), puis le détail technique.
func TestErreurGitHub(t *testing.T) {
	var reponse func(w http.ResponseWriter)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reponse(w) }))
	defer srv.Close()
	ferme := httptest.NewServer(http.NotFoundHandler())
	ferme.Close()
	aa := adresseRelease
	t.Cleanup(func() { adresseRelease = aa })
	for _, c := range []struct {
		quoi, adresse string
		reponse       func(w http.ResponseWriter)
		debut, detail string
	}{
		{"pas de réseau", ferme.URL, nil, "GitHub ne répond pas : pas d'Internet, ou connexion coupée.", "\nDétail : Get \"" + ferme.URL + "\""},
		{"403", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(403) }, "GitHub refuse pour l'instant : trop de demandes depuis cette adresse Internet. Réessayez dans une heure.", "\nDétail : GET " + srv.URL + " : 403"},
		{"429", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(429) }, "GitHub refuse pour l'instant", " : 429"},
		{"404", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(404) }, "Fichier absent de la version sur GitHub : rien n'est changé. Téléchargez la version à la main depuis https://github.com/programgames/3dssavesync/releases/latest.", " : 404"},
		{"500", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(500) }, "GitHub a un problème pour l'instant", " : 500"},
		{"pas du JSON", srv.URL, func(w http.ResponseWriter) { w.Write([]byte("<html>")) }, "Réponse de GitHub illisible : rien n'est changé.", "réponse illisible"},
		{"sans tag_name", srv.URL, func(w http.ResponseWriter) { w.Write([]byte(`{"assets":[]}`)) }, "Réponse de GitHub illisible", `tag_name ""`},
		{"trop grand", srv.URL, func(w http.ResponseWriter) { w.Write(make([]byte, 4<<20+1)) }, "Fichier de GitHub plus grand que prévu : rien n'est changé.", "réponse trop grande (4194304)"},
	} {
		adresseRelease, reponse = c.adresse, c.reponse
		_, err := derniereRelease(t.TempDir(), true)
		if err == nil {
			t.Errorf("%s : échec attendu", c.quoi)
			continue
		}
		if e := erreurGitHub(err); !strings.HasPrefix(e, c.debut) || !strings.Contains(e, c.detail) || !strings.Contains(e, "\nDétail : ") {
			t.Errorf("%s : %q", c.quoi, e)
		}
	}
}
