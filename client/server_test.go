package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// HTTP contract: the server/test_server.py suite, run against the Go server.
func TestServerContract(t *testing.T) {
	srv := httptest.NewServer(&Server{data: t.TempDir(), token: "jeton-go"})
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

// What the Python suite can't do remotely: exact list format (read by the 3DS), truncated body.
func TestLocalServer(t *testing.T) {
	data := t.TempDir()
	srv := httptest.NewServer(&Server{data: data, token: "j"})
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

// tests/saves.json (written by server.py, read by the 3DS C test): the same Versions give the same list, field
// by field, on the Go server.
func TestServerListLikeServerPy(t *testing.T) {
	o, err := os.ReadFile(filepath.Join("..", "tests", "saves.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected, received []map[string]any
	if err := json.Unmarshal(o, &expected); err != nil || len(expected) != 3 {
		t.Fatalf("saves.json : %v %d", err, len(expected))
	}
	data := t.TempDir()
	for _, j := range expected {
		write(t, filepath.Join(data, j["code"].(string), j["version"].(string)), make([]byte, int(j["taille"].(float64))))
		if name, ok := j["nom"].(string); ok {
			write(t, filepath.Join(data, j["code"].(string), "nom.txt"), []byte(name))
		}
	}
	srv := httptest.NewServer(&Server{data: data, token: "j"})
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/saves", nil)
	req.Header.Set("Authorization", "Bearer j")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
		t.Fatal(err)
	}
	for _, j := range append(expected, received...) { // age: depends on the test time
		if a, ok := j["age"].(float64); !ok || a <= 0 {
			t.Errorf("age : %v", j["age"])
		}
		delete(j, "age")
	}
	if !reflect.DeepEqual(received, expected) {
		t.Errorf("liste du serveur Go différente de tests/saves.json :\n%v\n%v", received, expected)
	}
}

// Setup, "local server" choice: config.json, autostart, start, address and token for the 3DS.
func TestTextSetupLocalServer(t *testing.T) {
	fakeHome(t)
	reset()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	config := filepath.Join(t.TempDir(), "config.json")
	var auto, launched []string
	autoBefore, launchBefore, detectBefore := autostart, launchServer, detect
	t.Cleanup(func() { autostart, launchServer, detect = autoBefore, launchBefore, detectBefore })
	detect = func(string) []Found { return nil }
	autostart = func(name, exe string, args []string) (string, error) { auto = args; return "demarrage", nil }
	launchServer = func(exe string, args []string) error {
		launched = args
		var cfg Config
		readJSON(config, &cfg)
		go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), &Server{data: t.TempDir(), token: cfg.Token})
		return nil
	}
	responses := []string{"2", fmt.Sprint(port), "n", "", "n"} // local server, port, no copy, no manual add, frontends…
	var output bytes.Buffer
	q := &Questions{in: bufio.NewReader(strings.NewReader(strings.Join(responses, "\n") + "\n")), out: &output}
	if err := textSetup(q, config); err != nil {
		t.Fatalf("%v\n%s", err, output.String())
	}
	var cfg Config
	readJSON(config, &cfg)
	if cfg.LocalServer == nil || cfg.LocalServer.Port != port || cfg.URL != fmt.Sprintf("http://127.0.0.1:%d", port) || len(cfg.Token) != 32 {
		t.Fatalf("config.json du serveur local incorrect : %+v", cfg)
	}
	expected := "tray --config " + config // the tray icon runs the local server (ADR 0022)
	if !trayAvailable {
		expected = "serve --config " + config
	}
	if strings.Join(auto, " ") != expected || strings.Join(launched, " ") != strings.Join(auto, " ") {
		t.Errorf("démarrage automatique et lancement : %q %q", auto, launched)
	}
	if !strings.Contains(output.String(), "Serveur local en marche") || !strings.Contains(output.String(), "Jeton : "+cfg.Token) ||
		!strings.Contains(output.String(), fmt.Sprintf(":%d", port)) {
		t.Errorf("adresse et jeton pour la 3DS attendus :\n%s", output.String())
	}
}
