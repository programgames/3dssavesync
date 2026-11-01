package main

// Assistant dans le navigateur : l'exe sert une page sur 127.0.0.1 (port au hasard, secret dans l'adresse) et ouvre
// le navigateur par défaut. Même logique que l'assistant en console (setup.go) ; rien n'est écrit sans copie préalable.

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed assistant.html
var pageAssistant string

type AssistantWeb struct {
	config, secret, hote string
	verrou               sync.Mutex
	vu                   time.Time // dernier signe de vie de la page
	fin                  chan struct{}
}

// Émulateur tel que la page l'affiche et le renvoie.
type EmuWeb struct {
	Nom, Profil, Exe, Processus, Source string
	Actif                               bool
	Systemes                            []SysWeb
	Conseil, Erreur                     string
	Coeurs                              map[string]string
}

type SysWeb struct {
	Systeme     string
	Sauvegardes string // valeur affichée (explicite, ou réglage de l'émulateur)
	Defaut      string // réglage de l'émulateur ; "" = à côté de la ROM
	Roms        string
}

type EtatWeb struct {
	Anglais, Windows, Existe, JetonPresent, Local bool
	URL, Courant, Installe, LaunchBox, EsDe       string
	Port                                          int
	EsDeActif                                     bool
	Emulateurs                                    []EmuWeb
	Profils                                       []string
}

type ChoixWeb struct {
	Local            bool
	URL, Jeton       string
	Port             int
	Installer        bool
	Emulateurs       []EmuWeb
	LaunchBox        string
	EsDe, Raccourcis bool
}

func commandeSetupWeb(config string, sansNavigateur bool) (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 1, err
	}
	o := make([]byte, 16)
	rand.Read(o)
	a := &AssistantWeb{config: config, secret: hex.EncodeToString(o), hote: l.Addr().String(), vu: time.Now(), fin: make(chan struct{})}
	go http.Serve(l, a)
	adresse := "http://" + a.hote + "/" + a.secret + "/"
	if sansNavigateur {
		fmt.Println(adresse)
	} else if err := ouvrirNavigateur(adresse); err != nil {
		fmt.Println(adresse)
		notifier(tr("Ouvre cette adresse dans ton navigateur : ", "Open this address in your browser: ") + adresse)
	}
	// Fin : bouton « Terminer », ou plus de signe de vie de la page (onglet fermé) pendant 2 minutes.
	for {
		select {
		case <-a.fin:
			time.Sleep(300 * time.Millisecond) // la réponse part avant la fin du programme
			return 0, nil
		case <-time.After(5 * time.Second):
			a.verrou.Lock()
			perdu := time.Since(a.vu) > 2*time.Minute
			a.verrou.Unlock()
			if perdu {
				return 0, nil
			}
		}
	}
}

func ouvrirNavigateur(adresse string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", adresse).Start()
	case "darwin":
		return exec.Command("open", adresse).Start()
	}
	return exec.Command("xdg-open", adresse).Start()
}

func (a *AssistantWeb) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Seulement la page ouverte par l'assistant : bon hôte (pas de rebinding DNS) et secret dans l'adresse.
	chemin, ok := strings.CutPrefix(r.URL.Path, "/"+a.secret+"/")
	if r.Host != a.hote || !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	a.verrou.Lock()
	defer a.verrou.Unlock()
	a.vu = time.Now()
	if chemin == "" && r.Method == "GET" {
		t := template.Must(template.New("page").Funcs(template.FuncMap{"t": tr}).Parse(pageAssistant))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		t.Execute(w, map[string]any{"Langue": tr("fr", "en")})
		return
	}
	if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" { // une autre page ne peut pas envoyer ce type sans CORS
		http.Error(w, "", 405)
		return
	}
	var reponse any
	switch chemin {
	case "api/etat":
		reponse = a.etat()
	case "api/ping":
		reponse = true
	case "api/emulateur":
		var t Trouve
		json.NewDecoder(r.Body).Decode(&t)
		reponse = emuWeb(t.Profil, &Emulateur{Exe: t.Exe}, "manuel", true)
	case "api/tester":
		var c ChoixWeb
		json.NewDecoder(r.Body).Decode(&c)
		reponse = a.tester(c)
	case "api/appliquer":
		var c ChoixWeb
		json.NewDecoder(r.Body).Decode(&c)
		lignes, err := a.appliquer(c)
		reponse = map[string]any{"Lignes": lignes, "Erreur": texteErreur(err)}
	case "api/reparer":
		var cfg Config
		if err := lireJSON(a.config, &cfg); err != nil {
			reponse = map[string]any{"Erreur": err.Error()}
		} else {
			reponse = map[string]any{"Lignes": reparer(&cfg, lanceurDe(&cfg))}
		}
	case "api/terminer":
		close(a.fin)
		reponse = true
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reponse)
}

func texteErreur(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Émulateur pour la page : dossiers par Système (réglage de l'émulateur, ou valeur de config.json), conseil autoload.
func emuWeb(nom string, e *Emulateur, source string, actif bool) EmuWeb {
	x := EmuWeb{Nom: nom, Profil: e.Profil, Exe: e.Exe, Processus: e.Processus, Source: source, Actif: actif, Coeurs: e.Coeurs}
	if x.Profil == "" {
		x.Profil = nom
	}
	if x.Profil == "retroarch" && len(e.Coeurs) == 0 {
		e.Coeurs = coeursRetroArch(e.Exe)
		x.Coeurs = e.Coeurs
	}
	p, err := e.profil(nom)
	if err != nil {
		x.Erreur = err.Error()
		return x
	}
	seul := &Emulateur{Exe: e.Exe, Coeurs: e.Coeurs, Profil: e.Profil, Args: e.Args} // sans config.json : le réglage de l'émulateur
	for _, s := range e.systemes(p) {
		d := seul.dossier(p, s, "")
		v := e.Sauvegardes[s]
		if v == "" {
			v = d
		}
		x.Systemes = append(x.Systemes, SysWeb{s, v, d, e.Roms[s]})
	}
	x.Conseil = conseil(e, p)
	return x
}

func (a *AssistantWeb) etat() EtatWeb {
	var cfg Config
	existeCfg := lireJSON(a.config, &cfg) == nil
	e := EtatWeb{Anglais: anglais, Windows: runtime.GOOS == "windows", Existe: existeCfg, JetonPresent: cfg.Jeton != "",
		Local: cfg.ServeurLocal != nil, URL: cfg.URL, Courant: executable(), Installe: emplacementInstalle(), Port: 8765}
	if cfg.ServeurLocal != nil {
		e.Port = cfg.ServeurLocal.Port
		e.URL = ""
	}
	e.LaunchBox = cfg.Frontends["launchbox"]
	if e.LaunchBox == "" && e.Windows {
		e.LaunchBox = trouverLaunchBox()
	}
	e.EsDe, e.EsDeActif = cfg.Frontends["esde"], cfg.Frontends["esde"] != ""
	if e.EsDe == "" && existe(maison("ES-DE")) {
		e.EsDe, e.EsDeActif = maison("ES-DE"), true
	}
	var noms []string
	for n := range cfg.Emulateurs {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	for _, n := range noms {
		e.Emulateurs = append(e.Emulateurs, emuWeb(n, cfg.Emulateurs[n], tr("configuré", "set up"), true))
	}
	for _, t := range detecter(e.LaunchBox) {
		if c := cfg.Emulateurs[t.Profil]; c != nil && strings.EqualFold(c.Exe, t.Exe) {
			continue
		}
		e.Emulateurs = append(e.Emulateurs, emuWeb(t.Profil, &Emulateur{Exe: t.Exe, Processus: t.Processus}, t.Source, cfg.Emulateurs[t.Profil] == nil))
	}
	for p := range profils {
		e.Profils = append(e.Profils, p)
	}
	sort.Strings(e.Profils)
	return e
}

func (a *AssistantWeb) tester(c ChoixWeb) map[string]any {
	var cfg Config
	lireJSON(a.config, &cfg)
	cfg.URL = strings.TrimRight(c.URL, "/")
	if c.Jeton != "" {
		cfg.Jeton = c.Jeton
	}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return map[string]any{"Ok": false, "Message": tr("Entre l'adresse du serveur (https://…).", "Enter the server address (https://…).")}
	}
	if err := testerServeur(&cfg); err != nil {
		return map[string]any{"Ok": false, "Message": tr("Connexion impossible : ", "Cannot connect: ") + err.Error()}
	}
	return map[string]any{"Ok": true, "Message": tr("Connexion réussie.", "Connection OK.")}
}

// appliquer : config.json (copie avant), programme, frontends, raccourcis, serveur local. Rien n'est écrit si le
// serveur personnel ne répond pas ou si un choix est invalide.
func (a *AssistantWeb) appliquer(c ChoixWeb) ([]string, error) {
	var cfg Config
	if err := lireJSON(a.config, &cfg); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if c.Local {
		if c.Port <= 0 || c.Port > 65535 {
			return nil, fmt.Errorf("%s", tr("port invalide", "invalid port"))
		}
		cfg.ServeurLocal = &ServeurLocal{Port: c.Port}
		cfg.URL = fmt.Sprintf("http://127.0.0.1:%d", c.Port)
		if c.Jeton != "" {
			cfg.Jeton = c.Jeton
		}
		if cfg.Jeton == "" {
			o := make([]byte, 16)
			rand.Read(o)
			cfg.Jeton = hex.EncodeToString(o)
		}
	} else {
		cfg.ServeurLocal = nil
		cfg.URL = strings.TrimRight(c.URL, "/")
		if c.Jeton != "" {
			cfg.Jeton = c.Jeton
		}
		if err := testerServeur(&cfg); err != nil {
			return nil, fmt.Errorf(tr("serveur injoignable (%v) : rien n'est écrit", "server unreachable (%v): nothing is written"), err)
		}
	}

	emus := map[string]*Emulateur{}
	for _, x := range c.Emulateurs {
		if !x.Actif {
			continue
		}
		if emus[x.Nom] != nil {
			return nil, fmt.Errorf(tr("deux émulateurs « %s » sont cochés : garde-en un seul", "two \"%s\" emulators are ticked: keep only one"), x.Nom)
		}
		e := &Emulateur{Exe: x.Exe, Processus: x.Processus, Coeurs: x.Coeurs}
		if ancien := cfg.Emulateurs[x.Nom]; ancien != nil && strings.EqualFold(ancien.Exe, x.Exe) {
			e = ancien // mêmes réglages (args d'un profil personnalisé…)
		}
		if x.Profil != x.Nom {
			e.Profil = x.Profil
		}
		e.Sauvegardes, e.Roms = nil, nil
		explicite := false
		for _, s := range x.Systemes {
			explicite = explicite || s.Sauvegardes != s.Defaut
			if s.Roms != "" {
				if e.Roms == nil {
					e.Roms = map[string]string{}
				}
				e.Roms[s.Systeme] = s.Roms
			}
		}
		if explicite { // un dossier changé : tous écrits (sinon l'émulateur ne gérerait que celui-là)
			e.Sauvegardes = map[string]string{}
			for _, s := range x.Systemes {
				if s.Sauvegardes == "" {
					return nil, fmt.Errorf(tr("%s : dossier des sauvegardes %s vide", "%s: empty %s save folder"), x.Nom, s.Systeme)
				}
				e.Sauvegardes[s.Systeme] = s.Sauvegardes
			}
		}
		if _, err := e.profil(x.Nom); err != nil {
			return nil, err
		}
		emus[x.Nom] = e
	}
	cfg.Emulateurs = emus

	var sortie bytes.Buffer
	q := &Questions{out: &sortie}
	lignes := []string{}
	if c.Installer {
		lignes = append(lignes, installerExe(&cfg))
	} else if cfg.Lanceur == "" {
		cfg.Lanceur = executable()
	}
	if cfg.Frontends == nil {
		cfg.Frontends = map[string]string{}
	}
	delete(cfg.Frontends, "launchbox")
	if c.LaunchBox != "" {
		if !existe(filepath.Join(c.LaunchBox, "Data", "Emulators.xml")) {
			return nil, fmt.Errorf(tr("Emulators.xml introuvable dans %s", "Emulators.xml not found in %s"), c.LaunchBox)
		}
		cfg.Frontends["launchbox"] = c.LaunchBox
	}
	esde := cfg.Frontends["esde"]
	delete(cfg.Frontends, "esde")
	if c.EsDe {
		if esde == "" {
			esde = maison("ES-DE")
		}
		cfg.Frontends["esde"] = esde
	}
	if err := enregistrerConfig(q, a.config, &cfg); err != nil {
		return nil, err
	}
	lignes = append(lignes, reparer(&cfg, lanceurDe(&cfg))...)
	if c.Raccourcis {
		creerRaccourcis(q, &cfg)
	}
	if cfg.ServeurLocal != nil {
		if err := serveurLocal(q, &cfg, a.config); err != nil {
			lignes = append(lignes, err.Error())
		}
	}
	for _, l := range strings.Split(sortie.String(), "\n") {
		if l = strings.TrimPrefix(strings.TrimSpace(l), "- "); l != "" && !strings.HasPrefix(l, "==") {
			lignes = append(lignes, l)
		}
	}
	return lignes, nil
}
