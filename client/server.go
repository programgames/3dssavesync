package main

// Local server: the HTTP contract of server/server.py (docs/design.md §2), inside the PC exe.
// Same data format: data/<Key>/<n>-<date>-<client>-<hash>.sav; nothing is ever deleted or rewritten.
// HTTP on the local network only (never open to the Internet); the 3DS reaches it without TLS.

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

var (
	reCode    = regexp.MustCompile(`^/saves/` + keyPattern + `$`)
	reHistory = regexp.MustCompile(`^/saves/` + keyPattern + `/history(/.*)?$`)
	reName    = regexp.MustCompile(`^/saves/` + keyPattern + `/name$`)
	reClient  = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
)

const maxSize = 64 << 20 // largest known DS save memory

type Server struct {
	data, token string
	lock        sync.Mutex // ponytail: global lock, like server.py; enough for a few Clients
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

// info writes the same text as json.dumps in server.py (", " and ": "): the 3DS homebrew reads this format.
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
		for _, x := range s.versions(key) { // only a name from the list: never a path outside the folder
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
	if _, err := io.ReadFull(r.Body, body); err != nil { // connection cut: never a truncated Version
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
	// Written to disk and read back: an accepted Version survives a power cut.
	if err := writeAtomic(filepath.Join(dir, name), body); err != nil {
		answer(w, 500, nil, "")
		return
	}
	answer(w, 200, nil, `"`+h+`"`)
}

// setName stores the game name (ROM name or 3DS title), like server.py: only for a game that already has a Version.
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
	if err := writeAtomic(filepath.Join(s.data, key, "nom.txt"), []byte(name)); err != nil { // like a Version
		answer(w, 500, nil, "")
		return
	}
	answer(w, 204, nil, "")
}

// localAddresses returns the PC's IPv4 addresses on the local network: the ones to enter on the 3DS, most likely first.
func localAddresses() []string {
	route := ""
	// UDP: "connect" only picks the route, nothing is sent. 192.0.2.1: a documentation address.
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

// sortAddresses puts the home router's network first: not ending in .1 (PC side of a virtual network: WSL,
// Docker, VirtualBox, VMware), then in 192.168 (home router range), then the default route (alone, it means an
// active VPN: ADR 0017).
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

// cmdServe: 3dssavesync serve, the local server, with the token and the folder of config.json.
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
	logTo(filepath.Dir(config), fmt.Sprintf("serveur local : port %d, données %s", cfg.LocalServer.Port, data))
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.LocalServer.Port), Handler: &Server{data: data, token: cfg.Token},
		ReadHeaderTimeout: 30 * time.Second}
	err := srv.ListenAndServe()
	var errno syscall.Errno
	if errors.Is(err, syscall.EADDRINUSE) || errors.As(err, &errno) && errno == 10048 { // WSAEADDRINUSE
		// Started at login while it already runs: nothing to report.
		cfg.URL = fmt.Sprintf("http://127.0.0.1:%d", cfg.LocalServer.Port)
		if testServer(&cfg) == nil {
			logTo(filepath.Dir(config), "serveur local déjà en marche")
			return 0, nil
		}
		return 1, fmt.Errorf(tr("le serveur local ne démarre pas : un autre programme utilise déjà le port %d. Choisissez un autre port dans les réglages et entrez la nouvelle adresse sur la 3DS.",
			"the local server can't start: another program is already using port %d. Choose another port in the settings and enter the new address on the 3DS."), cfg.LocalServer.Port)
	}
	return 1, err
}
