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
func TestNewest(t *testing.T) {
	for _, c := range []struct {
		current, remote string
		expected        bool
	}{
		{"3.6.1", "v3.7.0", true}, {"3.9.1", "v3.10.0", true}, {"3.7.0", "v3.7.0", false}, {"3.8.0", "v3.7.9", false},
		{"3.7.0-rc.1", "v3.7.0", true}, {"3.7.0", "v3.7.0-rc.1", false}, {"dev", "v9.0.0", false}, {"3.7.0", "vx", false},
		{"3.7", "v3.8.0", false}, {"3.7.0", "v3.+8.0", false},
	} {
		if newest(c.current, c.remote) != c.expected {
			t.Errorf("plusRecente(%q, %q) : %v attendu", c.current, c.remote, c.expected)
		}
	}
	if pcFileName("windows", "arm64") != "3dssavesync-windows-arm64.exe" || pcFileName("linux", "amd64") != "3dssavesync-linux-amd64" ||
		pcFileName("darwin", "arm64") != "3dssavesync-darwin-arm64" {
		t.Error("nom du fichier de la release")
	}
	// Vraie réponse de GitHub (v3.6.1).
	var r Release
	if err := json.Unmarshal(read("../3ds/test/release_latest.json"), &r); err != nil || r.Tag != "v3.6.1" || r.Page != "https://github.com/programgames/3dssavesync/releases/tag/v3.6.1" ||
		len(r.Assets) != 3 || r.Assets[0].Name != "3dssavesync-windows-amd64.exe" || r.Assets[0].Size != 10036224 ||
		r.Assets[0].URL != "https://github.com/programgames/3dssavesync/releases/download/v3.6.1/3dssavesync-windows-amd64.exe" {
		t.Errorf("release_latest.json : %v %+v", err, r)
	}
	if !validSum("00  autre\n"+"a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3 *nom\n", "nom", []byte("123")) ||
		validSum("a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3  nom\n", "nom", []byte("124")) || validSum("", "nom", nil) {
		t.Error("sommeValide")
	}
}

// Mise à jour du PC : recherche une fois par jour, téléchargement vérifié, exe remplacé avec l'ancien gardé à côté ;
// tout échec laisse l'ancien exe en place. Jamais GitHub, jamais le vrai exe : un serveur de test, un dossier temporaire.
func TestSelfUpdate(t *testing.T) {
	fresh := []byte("programme 3.7.0")
	h := sha256.Sum256(fresh)
	name := pcFileName(runtime.GOOS, runtime.GOARCH)
	sums := hex.EncodeToString(h[:]) + "  " + name + "\n"
	requests := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			requests++
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "v3.7.0", "html_url": srv.URL + "/page", "assets": []map[string]any{
				{"name": name, "size": len(fresh), "browser_download_url": srv.URL + "/exe"},
				{"name": "SHA256SUMS", "size": len(sums), "browser_download_url": srv.URL + "/sommes"}}})
		case "/exe":
			w.Write(fresh)
		case "/sommes":
			w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	va, aa, ea, ra := version, releaseURL, freeSpace, rename
	t.Cleanup(func() { version, releaseURL, freeSpace, rename = va, aa, ea, ra })
	version, releaseURL = "3.6.1", srv.URL+"/latest"
	freeSpace = func(string) (uint64, error) { return 1 << 40, nil }

	settings := t.TempDir()
	r, err := latestRelease(settings, false)
	if err != nil || r.Tag != "v3.7.0" || !newest(version, r.Tag) {
		t.Fatalf("derniereRelease : %v %+v", err, r)
	}
	latestRelease(settings, false)
	if requests != 1 {
		t.Errorf("une seule recherche par jour : %d", requests)
	}
	latestRelease(settings, true)
	writeJSON(filepath.Join(settings, "maj-pc.json"), UpdateCheck{time.Now().AddDate(0, 0, -1).Format("2006-01-02"), *r}, true)
	latestRelease(settings, false)
	if requests != 3 {
		t.Errorf("forcée, puis le lendemain : 3 recherches attendues, %d", requests)
	}

	d := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	exe := filepath.Join(d, "3dssavesync"+ext)
	rest := func(expected string) {
		t.Helper()
		if string(read(exe)) != expected {
			t.Errorf("exe : %q, attendu %q", read(exe), expected)
		}
		if f, _ := filepath.Glob(filepath.Join(d, "*tmp*")); len(f) != 0 {
			t.Errorf("fichier temporaire laissé : %v", f)
		}
	}
	write(t, exe, []byte("programme 3.6.1"))

	if _, err := selfUpdate(r, exe, "plan9", "amd64"); err == nil || !strings.Contains(err.Error(), "(fichier 3dssavesync-plan9-amd64 ou SHA256SUMS manquant)") {
		t.Errorf("système sans fichier : %v", err)
	}
	freeSpace = func(string) (uint64, error) { return 1000, nil }
	if _, err := selfUpdate(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "Pas assez de place") {
		t.Errorf("disque plein : %v", err)
	}
	freeSpace = func(string) (uint64, error) { return 1 << 40, nil }
	sums = strings.Repeat("0", 64) + "  " + name + "\n"
	if _, err := selfUpdate(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "abîmé") {
		t.Errorf("SHA-256 différent : %v", err)
	}
	sums = hex.EncodeToString(h[:]) + "  " + name + "\n"
	// Le nouvel exe ne prend pas la place : l'ancien revient.
	n := 0
	rename = func(a, b string) error {
		if n++; n == 2 {
			return errors.New("refusé")
		}
		return os.Rename(a, b)
	}
	if _, err := selfUpdate(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "l'ancien reste en place") {
		t.Errorf("renommage refusé : %v", err)
	}
	rest("programme 3.6.1")
	if f, _ := os.ReadDir(d); len(f) != 1 {
		t.Errorf("seul l'ancien exe doit rester : %v", f)
	}
	rename = os.Rename

	cp, err := selfUpdate(r, exe, runtime.GOOS, runtime.GOARCH)
	if err != nil || cp != filepath.Join(d, "3dssavesync-3.6.1"+ext) || string(read(cp)) != "programme 3.6.1" {
		t.Fatalf("mise à jour : %v, %s", err, cp)
	}
	rest("programme 3.7.0")
	// Encore une fois (onglet resté ouvert, autre fenêtre) : l'exe sur le disque est déjà celui de la release.
	if _, err := selfUpdate(r, exe, runtime.GOOS, runtime.GOARCH); err == nil || !strings.HasPrefix(err.Error(), "La version 3.7.0 est déjà installée") {
		t.Errorf("déjà à jour : %v", err)
	}
	rest("programme 3.7.0")
	// Encore depuis la même ancienne version : la première copie n'est jamais écrasée.
	write(t, exe, []byte("programme 3.6.1 bis"))
	if cp, err := selfUpdate(r, exe, runtime.GOOS, runtime.GOARCH); err != nil || cp != filepath.Join(d, "3dssavesync-3.6.1-1"+ext) ||
		string(read(filepath.Join(d, "3dssavesync-3.6.1"+ext))) != "programme 3.6.1" || string(read(cp)) != "programme 3.6.1 bis" {
		t.Errorf("deuxième copie : %v, %s", err, cp)
	}
}

// Une mise à jour à la fois (deux onglets, deux fenêtres) ; un verrou de plus de 15 minutes est remplacé.
func TestUpdateLock(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler()) // jamais GitHub, même si le verrou laissait passer
	closed.Close()
	aa := releaseURL
	t.Cleanup(func() { releaseURL = aa })
	releaseURL = closed.URL
	d := t.TempDir()
	release, err := updateLock(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updateLock(d); err == nil || !strings.HasPrefix(err.Error(), "Une mise à jour est déjà en cours") || !strings.Contains(err.Error(), "\nDétail : ") {
		t.Errorf("deuxième mise à jour : refus attendu, %v", err)
	}
	a := &WebSetup{config: filepath.Join(d, "config.json"), end: make(chan struct{})}
	if m := a.selfUpdateWeb()["Message"].(string); !strings.HasPrefix(m, "Une mise à jour est déjà en cours") {
		t.Errorf("page : %q", m)
	}
	release()
	if exists(filepath.Join(d, "maj-pc.verrou")) {
		t.Error("verrou retiré attendu")
	}
	release, _ = updateLock(d)
	stale := time.Now().Add(-16 * time.Minute)
	os.Chtimes(filepath.Join(d, "maj-pc.verrou"), stale, stale)
	if l, err := updateLock(d); err != nil {
		t.Errorf("verrou abandonné : remplacé attendu, %v", err)
	} else {
		l()
	}
}

// Échecs de GitHub : un texte par cas (quoi faire), puis le détail technique.
func TestGitHubError(t *testing.T) {
	var response func(w http.ResponseWriter)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { response(w) }))
	defer srv.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	aa := releaseURL
	t.Cleanup(func() { releaseURL = aa })
	for _, c := range []struct {
		what, address string
		response      func(w http.ResponseWriter)
		start, detail string
	}{
		{"pas de réseau", closed.URL, nil, "GitHub ne répond pas. Vérifiez votre connexion Internet et réessayez.", "\nDétail : Get \"" + closed.URL + "\""},
		{"403", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(403) }, "GitHub limite pour l'instant les demandes venant de votre connexion. Réessayez dans une heure.", "\nDétail : GET " + srv.URL + " : 403"},
		{"429", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(429) }, "GitHub limite pour l'instant", " : 429"},
		{"404", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(404) }, "Il manque un fichier dans cette version sur GitHub, la mise à jour n'a pas été faite. Téléchargez-la vous-même sur https://github.com/programgames/3dssavesync/releases/latest.", " : 404"},
		{"500", srv.URL, func(w http.ResponseWriter) { w.WriteHeader(500) }, "GitHub rencontre un problème.", " : 500"},
		{"pas du JSON", srv.URL, func(w http.ResponseWriter) { w.Write([]byte("<html>")) }, "GitHub a envoyé une réponse illisible. Réessayez plus tard.", "réponse illisible"},
		{"sans tag_name", srv.URL, func(w http.ResponseWriter) { w.Write([]byte(`{"assets":[]}`)) }, "GitHub a envoyé une réponse illisible", `tag_name ""`},
		{"trop grand", srv.URL, func(w http.ResponseWriter) { w.Write(make([]byte, 4<<20+1)) }, "Le fichier de GitHub est plus gros que prévu, la mise à jour n'a pas été faite.", "réponse trop grande (4194304)"},
	} {
		releaseURL, response = c.address, c.response
		_, err := latestRelease(t.TempDir(), true)
		if err == nil {
			t.Errorf("%s : échec attendu", c.what)
			continue
		}
		if e := gitHubError(err); !strings.HasPrefix(e, c.start) || !strings.Contains(e, c.detail) || !strings.Contains(e, "\nDétail : ") {
			t.Errorf("%s : %q", c.what, e)
		}
	}
}
