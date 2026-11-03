package main

// Serveur local (phase 6) : le contrat HTTP de server/server.py (docs/design.md §2), dans l'exe du PC.
// Même format de données : data/<Clé>/<n°>-<date>-<client>-<hash>.sav, rien n'est jamais supprimé ni réécrit.
// HTTP sur le réseau local seulement (jamais ouvert sur Internet) ; la 3DS y accède sans TLS.

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const serverKey = `([A-Z0-9]{4}|GBA-[A-Z0-9]{4}|GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}|3DS-[0-9A-F]{16})`

var (
	reCode    = regexp.MustCompile(`^/saves/` + serverKey + `$`)
	reHistory = regexp.MustCompile(`^/saves/` + serverKey + `/history(/.*)?$`)
	reName    = regexp.MustCompile(`^/saves/` + serverKey + `/name$`)
	reClient  = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
)

const maxSize = 64 << 20 // plus grande mémoire de sauvegarde DS connue

type Server struct {
	data, token string
	lock        sync.Mutex // ponytail: verrou global, comme server.py ; suffit pour quelques Clients
}

func (s *Server) versions(key string) []string {
	f, _ := filepath.Glob(filepath.Join(s.data, key, "*.sav"))
	for i := range f {
		f[i] = filepath.Base(f[i])
	}
	sort.Strings(f)
	return f
}

func (s *Server) current(key string) string {
	if v := s.versions(key); len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

func hashOf(version string) string {
	p := strings.Split(strings.TrimSuffix(version, ".sav"), "-")
	return p[len(p)-1]
}

// Même texte que json.dumps de server.py (", " et ": ") : le homebrew 3DS lit ce format.
func (s *Server) info(key, version string) string {
	p := strings.Split(strings.TrimSuffix(version, ".sav"), "-")
	var size int64
	if st, err := os.Stat(filepath.Join(s.data, key, version)); err == nil {
		size = st.Size()
	}
	var age int64
	if d, err := time.Parse("20060102T150405Z", p[1]); err == nil {
		age = int64(time.Since(d).Seconds())
	}
	return fmt.Sprintf(`{"code": "%s", "version": "%s", "hash": "%s", "taille": %d, "date": "%s", "client": "%s", "age": %d}`,
		key, version, p[3], size, p[1], p[2], age)
}

func answer(w http.ResponseWriter, status int, body []byte, etag string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	w.Write(body)
}

func (s *Server) allowed(w http.ResponseWriter, r *http.Request) bool {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.token)) == 1 {
		return true
	}
	answer(w, 401, nil, "")
	return false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.get(w, r)
	case "PUT":
		s.put(w, r)
	default:
		answer(w, 405, nil, "")
	}
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	if !s.allowed(w, r) {
		return
	}
	list := func(elements []string) {
		w.Header().Set("Content-Type", "application/json")
		answer(w, 200, []byte("["+strings.Join(elements, ", ")+"]"), "")
	}
	path := r.URL.Path
	if path == "/saves" {
		var l []string
		entries, _ := os.ReadDir(s.data)
		for _, e := range entries {
			if v := s.current(e.Name()); e.IsDir() && v != "" {
				i := s.info(e.Name(), v)
				if name, err := os.ReadFile(filepath.Join(s.data, e.Name(), "nom.txt")); err == nil {
					n, _ := json.Marshal(string(name))
					i = strings.TrimSuffix(i, "}") + `, "nom": ` + string(n) + "}"
				}
				l = append(l, i)
			}
		}
		list(l)
		return
	}
	var key, v string
	if m := reHistory.FindStringSubmatch(path); m != nil {
		if m[2] == "" {
			var l []string
			for _, x := range s.versions(m[1]) {
				l = append(l, s.info(m[1], x))
			}
			list(l)
			return
		}
		key = m[1]
		for _, x := range s.versions(key) { // seulement un nom de la liste : jamais de chemin hors du dossier
			if x == m[2][1:] {
				v = x
			}
		}
	} else if m := reCode.FindStringSubmatch(path); m != nil {
		key, v = m[1], s.current(m[1])
	}
	if v == "" {
		answer(w, 404, nil, "")
		return
	}
	o, err := os.ReadFile(filepath.Join(s.data, key, v))
	if err != nil {
		answer(w, 500, nil, "")
		return
	}
	answer(w, 200, o, `"`+hashOf(v)+`"`)
}

func (s *Server) put(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength <= 0 || r.ContentLength > maxSize {
		w.Header().Set("Connection", "close")
		answer(w, 400, nil, "")
		return
	}
	body := make([]byte, r.ContentLength)
	if _, err := io.ReadFull(r.Body, body); err != nil { // connexion coupée : jamais de Version tronquée
		w.Header().Set("Connection", "close")
		answer(w, 400, nil, "")
		return
	}
	if !s.allowed(w, r) {
		return
	}
	if m := reName.FindStringSubmatch(r.URL.Path); m != nil {
		s.setName(w, m[1], body)
		return
	}
	m := reCode.FindStringSubmatch(r.URL.Path)
	if m == nil {
		answer(w, 400, nil, "")
		return
	}
	ifEqual, ifAbsent := r.Header.Get("If-Match"), r.Header.Get("If-None-Match")
	if _, present := r.Header["If-Match"]; !present && ifAbsent != "*" {
		answer(w, 428, nil, "")
		return
	}
	sum := sha256.Sum256(body)
	h := hex.EncodeToString(sum[:])
	client := r.Header.Get("X-Client")
	if !reClient.MatchString(client) {
		client = "inconnu"
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	v := s.current(m[1])
	expected := ""
	if v != "" {
		expected = `"` + hashOf(v) + `"`
	}
	if ifEqual != expected || ifAbsent == "*" && v != "" {
		answer(w, 412, nil, expected)
		return
	}
	if v != "" && hashOf(v) == h {
		answer(w, 200, nil, expected)
		return
	}
	seq := 1
	if v != "" {
		n, _ := strconv.Atoi(v[:6])
		seq = n + 1
	}
	dir := filepath.Join(s.data, m[1])
	name := fmt.Sprintf("%06d-%s-%s-%s.sav", seq, time.Now().UTC().Format("20060102T150405Z"), client, h)
	tmp := filepath.Join(dir, name+".tmp")
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		os.Remove(tmp)
		err = writeVerified(tmp, body) // écrite sur le disque et relue : une Version acceptée survit à une coupure
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, name))
	}
	if err != nil {
		answer(w, 500, nil, "")
		return
	}
	answer(w, 200, nil, `"`+h+`"`)
}

// Nom du jeu (nom de la ROM ou titre 3DS), comme server.py : seulement pour un jeu qui a déjà une Version.
func (s *Server) setName(w http.ResponseWriter, key string, body []byte) {
	name := strings.TrimSpace(string(body))
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 200 ||
		strings.ContainsFunc(name, func(c rune) bool { return c < ' ' || c == 0x7f }) {
		answer(w, 400, nil, "")
		return
	}
	if s.current(key) == "" {
		answer(w, 404, nil, "")
		return
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	tmp := filepath.Join(s.data, key, "nom.txt.tmp")
	os.Remove(tmp)
	err := writeVerified(tmp, []byte(name)) // écrit sur le disque et relu, comme une Version
	if err == nil {
		err = os.Rename(tmp, filepath.Join(s.data, key, "nom.txt"))
	}
	if err != nil {
		answer(w, 500, nil, "")
		return
	}
	answer(w, 204, nil, "")
}

// Adresses IPv4 du PC sur le réseau local : celles à entrer sur la 3DS, la plus probable en tête.
func localAddresses() []string {
	route := ""
	// UDP : « connecter » choisit seulement la route, rien n'est envoyé. 192.0.2.1 : adresse de documentation.
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		route = c.LocalAddr().(*net.UDPAddr).IP.String()
		c.Close()
	}
	var l []string
	addresses, _ := net.InterfaceAddrs()
	for _, a := range addresses {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && ip.IP.IsPrivate() && !slices.Contains(l, ip.IP.String()) {
			l = append(l, ip.IP.String())
		}
	}
	return sortAddresses(l, route)
}

// La carte réseau de la box d'abord : pas en .1 (côté PC d'un réseau virtuel : WSL, Docker, VirtualBox, VMware), puis
// en 192.168 (plage des box), puis celle de la route par défaut (seule, elle désigne un VPN actif : ADR 0017).
func sortAddresses(l []string, route string) []string {
	rank := func(a string) int {
		r := 0
		if strings.HasSuffix(a, ".1") {
			r += 4
		}
		if !strings.HasPrefix(a, "192.168.") {
			r += 2
		}
		if a != route {
			r++
		}
		return r
	}
	slices.SortStableFunc(l, func(a, b string) int { return rank(a) - rank(b) })
	return l
}

// 3dssavesync serve : le serveur local, avec le jeton et le dossier de config.json.
func cmdServe(config string) (int, error) {
	var cfg Config
	if err := readConfig(config, &cfg); err != nil {
		return 1, err
	}
	if cfg.LocalServer == nil {
		return 1, fmt.Errorf("%s", tr("aucun serveur local n'est configuré. Choisissez « Serveur local sur ce PC » dans les réglages.",
			"no local server is set up. Choose \"Local server on this PC\" in the settings."))
	}
	data := cfg.LocalServer.Data
	if data == "" {
		data = filepath.Join(filepath.Dir(config), "serveur")
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		return 1, err
	}
	(&Session{dir: filepath.Dir(config)}).log(fmt.Sprintf("serveur local : port %d, données %s", cfg.LocalServer.Port, data))
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.LocalServer.Port), Handler: &Server{data: data, token: cfg.Token},
		ReadHeaderTimeout: 30 * time.Second}
	err := srv.ListenAndServe()
	var errno syscall.Errno
	if errors.Is(err, syscall.EADDRINUSE) || errors.As(err, &errno) && errno == 10048 { // WSAEADDRINUSE
		// Démarrage à l'ouverture de session alors qu'il tourne déjà : rien à signaler.
		cfg.URL = fmt.Sprintf("http://127.0.0.1:%d", cfg.LocalServer.Port)
		if testServer(&cfg) == nil {
			(&Session{dir: filepath.Dir(config)}).log("serveur local déjà en marche")
			return 0, nil
		}
		return 1, fmt.Errorf(tr("le serveur local ne démarre pas : un autre programme utilise déjà le port %d. Choisissez un autre port dans les réglages et entrez la nouvelle adresse sur la 3DS.",
			"the local server can't start: another program is already using port %d. Choose another port in the settings and enter the new address on the 3DS."), cfg.LocalServer.Port)
	}
	return 1, err
}
