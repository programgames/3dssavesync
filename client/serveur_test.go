package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Contrat HTTP : la suite de server/test_server.py, lancée contre le serveur Go.
func TestServeurContrat(t *testing.T) {
	srv := httptest.NewServer(&Serveur{data: t.TempDir(), jeton: "jeton-go"})
	defer srv.Close()
	py := "python3"
	if runtime.GOOS == "windows" {
		py = "python"
	}
	c := exec.Command(py, "-m", "unittest", "-q", "test_server.py")
	c.Dir = filepath.Join("..", "server")
	c.Env = append(os.Environ(), "NDSSYNC_URL="+srv.URL, "NDSSYNC_TOKEN=jeton-go")
	o, err := c.CombinedOutput()
	if err != nil || !strings.Contains(string(o), "OK") {
		t.Fatalf("contrat HTTP non respecté :\n%s", o)
	}
	t.Logf("%s", strings.TrimSpace(string(o)))
}

// Ce que la suite Python ne peut pas faire à distance : format exact de la liste (lu par la 3DS), corps tronqué.
func TestServeurLocal(t *testing.T) {
	data := t.TempDir()
	srv := httptest.NewServer(&Serveur{data: data, jeton: "j"})
	defer srv.Close()
	req, _ := http.NewRequest("PUT", srv.URL+"/saves/ADAF", strings.NewReader("partie"))
	req.Header.Set("Authorization", "Bearer j")
	req.Header.Set("If-None-Match", "*")
	req.Header.Set("X-Client", "3ds")
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != 200 {
		t.Fatalf("PUT : %v %v", r, err)
	}
	req, _ = http.NewRequest("GET", srv.URL+"/saves", nil)
	req.Header.Set("Authorization", "Bearer j")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	o := make([]byte, 400)
	n, _ := r.Body.Read(o)
	r.Body.Close()
	if !strings.HasPrefix(string(o[:n]), `[{"code": "ADAF", "version": "000001-`) || !strings.Contains(string(o[:n]), `"client": "3ds", "age": `) {
		t.Errorf("format de la liste différent de server.py : %s", o[:n])
	}

	s, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	s.Write([]byte("PUT /saves/IRBF HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer j\r\nIf-None-Match: *\r\nContent-Length: 100\r\n\r\n" + strings.Repeat("x", 10)))
	s.(*net.TCPConn).CloseWrite()
	o = make([]byte, 100)
	n, _ = s.Read(o)
	s.Close()
	if !strings.Contains(string(o[:n]), " 400 ") {
		t.Errorf("corps tronqué : 400 attendu, reçu %q", o[:n])
	}
	if _, err := os.Stat(filepath.Join(data, "IRBF")); err == nil {
		t.Error("corps tronqué : aucune Version ne doit être écrite")
	}
}

// Assistant, choix « serveur local » : config.json, démarrage automatique, lancement, adresse et jeton pour la 3DS.
func TestAssistantServeurLocal(t *testing.T) {
	maisonFactice(t)
	reinitialiser()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	config := filepath.Join(t.TempDir(), "config.json")
	var auto, lance []string
	autoAvant, lancerAvant, detecterAvant := autostart, lancerServeur, detecter
	t.Cleanup(func() { autostart, lancerServeur, detecter = autoAvant, lancerAvant, detecterAvant })
	detecter = func(string) []Trouve { return nil }
	autostart = func(nom, exe string, args []string) (string, error) { auto = args; return "demarrage", nil }
	lancerServeur = func(exe string, args []string) error {
		lance = args
		var cfg Config
		lireJSON(config, &cfg)
		go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), &Serveur{data: t.TempDir(), jeton: cfg.Jeton})
		return nil
	}
	reponses := []string{"2", fmt.Sprint(port), "n", "", "n"} // serveur local, port, pas de copie, pas d'ajout manuel, frontends…
	var sortie bytes.Buffer
	q := &Questions{in: bufio.NewReader(strings.NewReader(strings.Join(reponses, "\n") + "\n")), out: &sortie}
	if err := assistant(q, config); err != nil {
		t.Fatalf("%v\n%s", err, sortie.String())
	}
	var cfg Config
	lireJSON(config, &cfg)
	if cfg.ServeurLocal == nil || cfg.ServeurLocal.Port != port || cfg.URL != fmt.Sprintf("http://127.0.0.1:%d", port) || len(cfg.Jeton) != 32 {
		t.Fatalf("config.json du serveur local incorrect : %+v", cfg)
	}
	if strings.Join(auto, " ") != "serve --config "+config || strings.Join(lance, " ") != strings.Join(auto, " ") {
		t.Errorf("démarrage automatique et lancement : %q %q", auto, lance)
	}
	if !strings.Contains(sortie.String(), "Serveur local en marche") || !strings.Contains(sortie.String(), "Jeton : "+cfg.Jeton) ||
		!strings.Contains(sortie.String(), fmt.Sprintf(":%d", port)) {
		t.Errorf("adresse et jeton pour la 3DS attendus :\n%s", sortie.String())
	}
}
