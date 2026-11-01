package main

// Serveur local (phase 6) : le contrat HTTP de server/server.py (docs/design.md §2), dans l'exe du PC.
// Même format de données : data/<Clé>/<n°>-<date>-<client>-<hash>.sav, rien n'est jamais supprimé ni réécrit.
// HTTP sur le réseau local seulement (jamais ouvert sur Internet) ; la 3DS y accède sans TLS.

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const cleServeur = `([A-Z0-9]{4}|GBA-[A-Z0-9]{4}|GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}|3DS-[0-9A-F]{16})`

var (
	reCode       = regexp.MustCompile(`^/saves/` + cleServeur + `$`)
	reHistorique = regexp.MustCompile(`^/saves/` + cleServeur + `/history(/.*)?$`)
	reClient     = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
)

const tailleMax = 64 << 20 // plus grande mémoire de sauvegarde DS connue

type Serveur struct {
	data, jeton string
	verrou      sync.Mutex // ponytail: verrou global, comme server.py ; suffit pour quelques Clients
}

func (s *Serveur) versions(cle string) []string {
	f, _ := filepath.Glob(filepath.Join(s.data, cle, "*.sav"))
	for i := range f {
		f[i] = filepath.Base(f[i])
	}
	sort.Strings(f)
	return f
}

func (s *Serveur) courante(cle string) string {
	if v := s.versions(cle); len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

func hashDe(version string) string {
	p := strings.Split(strings.TrimSuffix(version, ".sav"), "-")
	return p[len(p)-1]
}

// Même texte que json.dumps de server.py (", " et ": ") : le homebrew 3DS lit ce format.
func (s *Serveur) info(cle, version string) string {
	p := strings.Split(strings.TrimSuffix(version, ".sav"), "-")
	var taille int64
	if st, err := os.Stat(filepath.Join(s.data, cle, version)); err == nil {
		taille = st.Size()
	}
	var age int64
	if d, err := time.Parse("20060102T150405Z", p[1]); err == nil {
		age = int64(time.Since(d).Seconds())
	}
	return fmt.Sprintf(`{"code": "%s", "version": "%s", "hash": "%s", "taille": %d, "date": "%s", "client": "%s", "age": %d}`,
		cle, version, p[3], taille, p[1], p[2], age)
}

func repondre(w http.ResponseWriter, statut int, corps []byte, etag string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(corps)))
	w.WriteHeader(statut)
	w.Write(corps)
}

func (s *Serveur) autorise(w http.ResponseWriter, r *http.Request) bool {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.jeton)) == 1 {
		return true
	}
	repondre(w, 401, nil, "")
	return false
}

func (s *Serveur) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.get(w, r)
	case "PUT":
		s.put(w, r)
	default:
		repondre(w, 405, nil, "")
	}
}

func (s *Serveur) get(w http.ResponseWriter, r *http.Request) {
	if !s.autorise(w, r) {
		return
	}
	liste := func(elements []string) {
		w.Header().Set("Content-Type", "application/json")
		repondre(w, 200, []byte("["+strings.Join(elements, ", ")+"]"), "")
	}
	chemin := r.URL.Path
	if chemin == "/saves" {
		var l []string
		entrees, _ := os.ReadDir(s.data)
		for _, e := range entrees {
			if v := s.courante(e.Name()); e.IsDir() && v != "" {
				l = append(l, s.info(e.Name(), v))
			}
		}
		liste(l)
		return
	}
	var cle, v string
	if m := reHistorique.FindStringSubmatch(chemin); m != nil {
		if m[2] == "" {
			var l []string
			for _, x := range s.versions(m[1]) {
				l = append(l, s.info(m[1], x))
			}
			liste(l)
			return
		}
		cle = m[1]
		for _, x := range s.versions(cle) { // seulement un nom de la liste : jamais de chemin hors du dossier
			if x == m[2][1:] {
				v = x
			}
		}
	} else if m := reCode.FindStringSubmatch(chemin); m != nil {
		cle, v = m[1], s.courante(m[1])
	}
	if v == "" {
		repondre(w, 404, nil, "")
		return
	}
	o, err := os.ReadFile(filepath.Join(s.data, cle, v))
	if err != nil {
		repondre(w, 500, nil, "")
		return
	}
	repondre(w, 200, o, `"`+hashDe(v)+`"`)
}

func (s *Serveur) put(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength <= 0 || r.ContentLength > tailleMax {
		w.Header().Set("Connection", "close")
		repondre(w, 400, nil, "")
		return
	}
	corps := make([]byte, r.ContentLength)
	if _, err := io.ReadFull(r.Body, corps); err != nil { // connexion coupée : jamais de Version tronquée
		w.Header().Set("Connection", "close")
		repondre(w, 400, nil, "")
		return
	}
	if !s.autorise(w, r) {
		return
	}
	m := reCode.FindStringSubmatch(r.URL.Path)
	if m == nil {
		repondre(w, 400, nil, "")
		return
	}
	siEgal, siAbsent := r.Header.Get("If-Match"), r.Header.Get("If-None-Match")
	if _, present := r.Header["If-Match"]; !present && siAbsent != "*" {
		repondre(w, 428, nil, "")
		return
	}
	somme := sha256.Sum256(corps)
	h := hex.EncodeToString(somme[:])
	client := r.Header.Get("X-Client")
	if !reClient.MatchString(client) {
		client = "inconnu"
	}
	s.verrou.Lock()
	defer s.verrou.Unlock()
	v := s.courante(m[1])
	attendu := ""
	if v != "" {
		attendu = `"` + hashDe(v) + `"`
	}
	if siEgal != attendu || siAbsent == "*" && v != "" {
		repondre(w, 412, nil, attendu)
		return
	}
	if v != "" && hashDe(v) == h {
		repondre(w, 200, nil, attendu)
		return
	}
	seq := 1
	if v != "" {
		n, _ := strconv.Atoi(v[:6])
		seq = n + 1
	}
	dossier := filepath.Join(s.data, m[1])
	nom := fmt.Sprintf("%06d-%s-%s-%s.sav", seq, time.Now().UTC().Format("20060102T150405Z"), client, h)
	tmp := filepath.Join(dossier, nom+".tmp")
	err := os.MkdirAll(dossier, 0o755)
	if err == nil {
		os.Remove(tmp)
		err = ecrireVerifie(tmp, corps) // écrite sur le disque et relue : une Version acceptée survit à une coupure
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dossier, nom))
	}
	if err != nil {
		repondre(w, 500, nil, "")
		return
	}
	repondre(w, 200, nil, `"`+h+`"`)
}

// Adresses IPv4 du PC sur le réseau local : celles à entrer sur la 3DS.
func adressesLocales() []string {
	var l []string
	adresses, _ := net.InterfaceAddrs()
	for _, a := range adresses {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && ip.IP.IsPrivate() {
			l = append(l, ip.IP.String())
		}
	}
	return l
}

// 3dssavesync serve : le serveur local, avec le jeton et le dossier de config.json.
func commandeServe(config string) (int, error) {
	var cfg Config
	if err := lireJSON(config, &cfg); err != nil {
		return 1, err
	}
	if cfg.ServeurLocal == nil {
		return 1, fmt.Errorf("%s", tr("pas de serveur local dans config.json (lance l'assistant)", "no local server in config.json (run the assistant)"))
	}
	data := cfg.ServeurLocal.Donnees
	if data == "" {
		data = filepath.Join(filepath.Dir(config), "serveur")
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		return 1, err
	}
	(&Session{dossier: filepath.Dir(config)}).journal(fmt.Sprintf("serveur local : port %d, données %s", cfg.ServeurLocal.Port, data))
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.ServeurLocal.Port), Handler: &Serveur{data: data, jeton: cfg.Jeton},
		ReadHeaderTimeout: 30 * time.Second}
	return 1, srv.ListenAndServe()
}
