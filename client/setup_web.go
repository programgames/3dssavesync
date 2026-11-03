package main

// Setup in the browser: the exe serves a page on 127.0.0.1 (random port, secret in the address) and opens the default
// browser. Same logic as the console setup (setup.go); nothing is written without a copy first.

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed assistant.html
var setupPageHTML string

type WebSetup struct {
	config, secret, host string
	lock                 sync.Mutex
	seen                 time.Time // last sign of life from the page
	end                  chan struct{}
	names                map[string]string // Key -> ROM name, read once (history.go)
	namesRead            sync.Once
}

// WebEmu is an emulator as the page shows it and sends it back.
type WebEmu struct {
	Name     string `json:"Nom"`
	Profile  string `json:"Profil"`
	Exe      string
	Process  string `json:"Processus"`
	Source   string
	Active   bool              `json:"Actif"`
	Systems  []WebSystem       `json:"Systemes"`
	Hint     string            `json:"Conseil"`
	Error    string            `json:"Erreur"`
	Problems []string          `json:"Problemes"` // to-do lines of the checks for this config.json emulator
	Cores    map[string]string `json:"Coeurs"`
	Extra    string            `json:"EnPlus"` // built-in Profile: extra arguments, before the ROM
	// Custom emulator: command line ({rom}, {sauvegardes}), extension, name of the file inside the zip.
	Custom       bool   `json:"Personnalise"`
	Command      string `json:"Commande"`
	Extension    string
	InternalName bool `json:"NomInterne"`
}

type WebSystem struct {
	System  string `json:"Systeme"`
	Saves   string `json:"Sauvegardes"` // shown value (explicit, or the emulator's setting)
	Default string `json:"Defaut"`      // the emulator's setting; "" = next to the ROM
	Roms    string
}

type WebState struct {
	English       bool `json:"Anglais"`
	Windows       bool
	Exists        bool `json:"Existe"`
	HasToken      bool `json:"JetonPresent"`
	Local         bool
	URL           string
	Current       string `json:"Courant"`
	Installed     string `json:"Installe"`
	LaunchBox     string
	EsDe          string
	Address3DS    string `json:"Adresse3DS"` // written to the 3DS SD card
	Port          int
	EsDeActive    bool     `json:"EsDeActif"`
	Emulators     []WebEmu `json:"Emulateurs"`
	Profiles      []string `json:"Profils"`
	Configured    []string `json:"Configures"` // config.json emulators: "Open" buttons
	Icon          bool     `json:"Icone"`
	IconAvailable bool     `json:"IconeDisponible"` // tray icon (tray.go)
	Version       string   // of this program (selfupdate.go)
	Update        bool     `json:"Maj"` // update check
}

// openDetached opens an emulator without a game, with Sync (launch), apart from the page. Tests replace it.
var openDetached = startDetached

type WebChoices struct {
	Local     bool
	URL       string
	Token     string `json:"Jeton"`
	Port      int
	Install   bool     `json:"Installer"`
	Emulators []WebEmu `json:"Emulateurs"`
	LaunchBox string
	EsDe      bool
	Shortcuts bool `json:"Raccourcis"`
	Icon      bool `json:"Icone"`
	Update    bool `json:"Maj"`
}

func cmdSetupWeb(config string, noBrowser bool) (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 1, err
	}
	o := make([]byte, 16)
	rand.Read(o)
	a := &WebSetup{config: config, secret: hex.EncodeToString(o), host: l.Addr().String(), seen: time.Now(), end: make(chan struct{})}
	go http.Serve(l, a)
	address := "http://" + a.host + "/" + a.secret + "/"
	if noBrowser {
		fmt.Println(address)
	} else if err := openBrowser(address); err != nil {
		fmt.Println(address)
		notify(tr("Ouvrez cette adresse dans votre navigateur : ", "Open this address in your browser: ") + address)
	}
	// End: "Finish" button, or no sign of life from the page (tab closed) for 2 minutes.
	for {
		select {
		case <-a.end:
			time.Sleep(300 * time.Millisecond) // the response goes out before the program ends
			return 0, nil
		case <-time.After(5 * time.Second):
			a.lock.Lock()
			lost := time.Since(a.seen) > 2*time.Minute
			a.lock.Unlock()
			if lost {
				return 0, nil
			}
		}
	}
}

func openBrowser(address string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Start()
	case "darwin":
		return exec.Command("open", address).Start()
	}
	return exec.Command("xdg-open", address).Start()
}

func (a *WebSetup) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only the page opened by the setup: right host (no DNS rebinding) and secret in the address.
	path, ok := strings.CutPrefix(r.URL.Path, "/"+a.secret+"/")
	if r.Host != a.host || !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	a.lock.Lock()
	defer a.lock.Unlock()
	a.seen = time.Now()
	if path == "" && r.Method == "GET" {
		t := template.Must(template.New("page").Funcs(template.FuncMap{"t": tr}).Parse(setupPageHTML))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		t.Execute(w, map[string]any{"Langue": tr("fr", "en")})
		return
	}
	if path == "config.ini" && r.Method == "GET" { // to copy to the 3DS by FTP
		var cfg Config
		readJSON(a.config, &cfg)
		url, _ := address3DS(&cfg)
		if url == "" || cfg.Token == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="config.ini"`)
		io.WriteString(w, configIni("", url, cfg.Token))
		return
	}
	if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" { // another page can't send this type without CORS
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	var response any
	switch path {
	case "api/etat":
		response = a.state()
	case "api/ping":
		response = true
	case "api/emulateur":
		var t Found
		json.NewDecoder(r.Body).Decode(&t)
		response = webEmu(t.Profile, &Emulator{Exe: t.Exe}, "manuel", true)
	case "api/apercu":
		var x WebEmu
		json.NewDecoder(r.Body).Decode(&x)
		response = preview(x)
	case "api/tester":
		var c WebChoices
		json.NewDecoder(r.Body).Decode(&c)
		response = a.test(c)
	case "api/appliquer":
		var c WebChoices
		json.NewDecoder(r.Body).Decode(&c)
		lines, err := a.apply(c)
		x := map[string]any{"Lignes": lines, "Erreur": errorText(err)}
		var cfg Config
		if err == nil && readJSON(a.config, &cfg) == nil && cfg.LocalServer != nil { // "enter on the 3DS" box
			x["Adresse"], x["Autres"] = address3DS(&cfg)
			x["Jeton"] = cfg.Token
		}
		response = x
	case "api/cartes":
		response = map[string]any{"Cartes": detectCards()}
	case "api/preparer":
		var x struct {
			Root string `json:"Racine"`
		}
		json.NewDecoder(r.Body).Decode(&x)
		ok, lines := prepareAndRecord(a.config, x.Root)
		response = map[string]any{"Ok": ok, "Lignes": lines}
	case "api/reparer":
		var cfg Config
		if err := readConfig(a.config, &cfg); err != nil {
			response = map[string]any{"Erreur": errorText(err)}
		} else {
			response = map[string]any{"Lignes": repair(&cfg, launcherOf(&cfg))}
		}
	case "api/desinstaller":
		var cfg Config
		if err := readConfig(a.config, &cfg); err != nil {
			response = map[string]any{"Erreur": errorText(err)}
			break
		}
		lines, err := uninstall(&cfg, a.config)
		response = map[string]any{"Lignes": lines, "Erreur": errorText(err)}
		if err == nil {
			a.finish() // the response goes out, then the program stops: its exe can go to the Recycle Bin
		}
	case "api/ouvrir":
		var x struct {
			Name string `json:"Nom"`
		}
		json.NewDecoder(r.Body).Decode(&x)
		response = a.open(x.Name)
	case "api/alias":
		var x struct {
			Name   string `json:"Nom"`
			System string `json:"Systeme"`
			Remove bool   `json:"Retirer"`
		}
		json.NewDecoder(r.Body).Decode(&x)
		response = a.aliasWeb(x.Name, x.System, x.Remove)
	case "api/jeux", "api/versions", "api/restaurer", "api/verifications", "api/mettreajour":
		var x struct {
			Key     string `json:"Cle"`
			File    string `json:"Fichier"`
			Current string `json:"Courant"`
		}
		json.NewDecoder(r.Body).Decode(&x)
		func() { // network reads without the lock: the page (ping, other tabs) stays served meanwhile
			a.lock.Unlock()
			defer a.lock.Lock()
			switch path {
			case "api/jeux":
				response = a.webGames()
			case "api/versions":
				response = a.versionsWeb(x.Key)
			case "api/verifications":
				l, update := checks(a.config)
				response = map[string]any{"Liste": l, "Maj": update}
			case "api/mettreajour":
				response = a.selfUpdateWeb()
			default:
				response = a.restoreWeb(x.Key, x.File, x.Current)
			}
		}()
	case "api/terminer":
		a.finish()
		response = true
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// aliasWeb handles ROM hacks that get their own Key (ADR 0014). A non-empty name: add (Key made by addAlias) or
// remove, written to config.json with a dated copy.
func (a *WebSetup) aliasWeb(name, system string, remove bool) map[string]any {
	var cfg Config
	if err := readConfig(a.config, &cfg); err != nil {
		return map[string]any{"Erreur": errorText(err)}
	}
	var err error
	if name = strings.TrimSpace(name); name != "" {
		_, exists := cfg.Alias[name]
		switch {
		case remove && exists:
			delete(cfg.Alias, name)
			err = writeConfigCopy(a.config, &cfg)
		case !remove && !exists:
			if _, err = addAlias(&cfg, system, name); err == nil {
				err = writeConfigCopy(a.config, &cfg)
			}
		}
	}
	names := []string{}
	for n := range cfg.Alias {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	x := map[string]any{"Noms": names, "Cles": cfg.Alias} // Key: the console in the list
	if err != nil {
		x["Erreur"] = errorText(err)
	}
	return x
}

// finish stops the program after the response; a second call (two tabs, two clicks) does nothing.
func (a *WebSetup) finish() {
	select {
	case <-a.end:
	default:
		close(a.end)
	}
}

// webEmu builds the emulator for the page: folders per System (emulator setting, or config.json value), autoload hint.
func webEmu(name string, e *Emulator, source string, active bool) WebEmu {
	x := WebEmu{Name: name, Profile: e.Profile, Exe: e.Exe, Process: e.Process, Source: source, Active: active, Cores: e.Cores,
		Extra: joinArgs(e.ExtraArgs)}
	if x.Profile == "" {
		x.Profile = name
	}
	if _, builtIn := profiles[x.Profile]; !builtIn && e.Args != nil {
		x.Custom, x.Command, x.Extension, x.InternalName, x.Extra = true, joinArgs(e.Args), e.Extension, e.InternalName, ""
	}
	if x.Profile == "retroarch" && len(e.Cores) == 0 {
		e.Cores = retroArchCores(e.Exe)
		x.Cores = e.Cores
	}
	p, err := e.profile(name)
	if err != nil {
		x.Error = err.Error()
		return x
	}
	standalone := &Emulator{Exe: e.Exe, Cores: e.Cores, Profile: e.Profile, Args: e.Args} // without config.json: the emulator's setting
	for _, s := range e.systems(p) {
		d := standalone.dir(p, s, "")
		v := e.Saves[s]
		if v == "" {
			v = d
		}
		x.Systems = append(x.Systems, WebSystem{s, v, d, e.Roms[s]})
	}
	x.Hint = hint(e, p)
	return x
}

var reEmulatorName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,15}$`)

// emulatorOf returns the emulator that the page describes. Returns a clear error for an incomplete custom emulator.
func emulatorOf(x WebEmu) (*Emulator, error) {
	e := &Emulator{Exe: x.Exe, Process: x.Process, Cores: x.Cores}
	if !x.Custom {
		if x.Profile != x.Name {
			e.Profile = x.Profile
		}
		e.ExtraArgs = split(x.Extra)
		return e, nil
	}
	if _, builtIn := profiles[x.Name]; builtIn || !reEmulatorName.MatchString(x.Name) {
		return nil, fmt.Errorf(tr("Le nom « %s » ne convient pas. Utilisez des minuscules, des chiffres et des tirets, et un nom différent des profils intégrés.",
			"The name \"%s\" can't be used. Use lowercase letters, digits and dashes, and a name that isn't a built-in profile."), x.Name)
	}
	e.Args = split(x.Command)
	if !strings.Contains(x.Command, "{rom}") {
		return nil, fmt.Errorf(tr("%s : ajoutez {rom} à la ligne de commande.", "%s: add {rom} to the command line."), x.Name)
	}
	if len(x.Systems) == 0 {
		return nil, fmt.Errorf(tr("%s : cochez au moins un système.", "%s: tick at least one system."), x.Name)
	}
	e.Extension = strings.TrimSpace(x.Extension)
	if e.Extension != "" && !strings.HasPrefix(e.Extension, ".") {
		e.Extension = "." + e.Extension
	}
	e.InternalName = x.InternalName
	return e, nil
}

// preview returns the command started for a sample ROM of the first System (display only).
func preview(x WebEmu) map[string]string {
	e, err := emulatorOf(x)
	if err == nil && len(x.Systems) == 0 {
		err = fmt.Errorf("%s", tr("aucun système", "no system"))
	}
	if err != nil {
		return map[string]string{"Erreur": err.Error()}
	}
	s := x.Systems[0]
	examples := map[string]string{"DS": "Pokemon Platine.nds", "GB": "Pokemon Cristal.gbc", "GBA": "Pokemon Emeraude.gba", "3DS": "Pokemon X.3ds"}
	romsDir := s.Roms
	if romsDir == "" {
		romsDir = filepath.Join(home(), "roms")
	}
	rom := filepath.Join(romsDir, examples[s.System])
	if x.Custom {
		e.Saves = map[string]string{s.System: s.Saves}
	}
	p, err := e.profile(x.Name)
	if err != nil {
		return map[string]string{"Erreur": err.Error()}
	}
	d := s.Saves
	if d == "" {
		d = e.dir(p, s.System, rom)
	}
	return map[string]string{"Commande": joinArgs(e.command(p, s.System, rom, d))}
}

func (a *WebSetup) state() WebState {
	var cfg Config
	cfgExists := readJSON(a.config, &cfg) == nil
	e := WebState{English: english, Windows: runtime.GOOS == "windows", Exists: cfgExists, HasToken: cfg.Token != "",
		Local: cfg.LocalServer != nil || !cfgExists, URL: cfg.URL, Current: executable(), Installed: installedLocation(), Port: 8765}
	if cfg.Token != "" {
		e.Address3DS, _ = address3DS(&cfg)
	}
	if cfg.LocalServer != nil {
		e.Port = cfg.LocalServer.Port
		e.URL = ""
	}
	e.Icon, e.IconAvailable = cfg.trayIcon(), trayAvailable
	e.Version, e.Update = version, cfg.checkUpdate()
	e.LaunchBox = cfg.Frontends["launchbox"]
	if e.LaunchBox == "" && e.Windows {
		e.LaunchBox = findLaunchBox()
	}
	e.EsDe, e.EsDeActive = cfg.Frontends["esde"], cfg.Frontends["esde"] != ""
	if e.EsDe == "" && exists(home("ES-DE")) {
		e.EsDe, e.EsDeActive = home("ES-DE"), true
	}
	var names []string
	for n := range cfg.Emulators {
		names = append(names, n)
	}
	sort.Strings(names)
	e.Configured = names
	problems := map[string][]string{} // same state as the checks (azahar without an sdmc folder: "to check")
	for _, v := range emulatorChecks(&cfg) {
		if !v.Ok {
			problems[v.Emulator] = append(problems[v.Emulator], v.Text)
		}
	}
	for _, n := range names {
		x := webEmu(n, cfg.Emulators[n], tr("configuré", "set up"), true)
		x.Problems = problems[n]
		e.Emulators = append(e.Emulators, x)
	}
	// Found on this PC, missing from config.json: ticked on first run (nothing chosen before), else not ticked, in the
	// collapsed part of the page: nothing is added to an existing configuration without a choice.
	for _, t := range detect(e.LaunchBox) {
		if c := cfg.Emulators[t.Profile]; c != nil && strings.EqualFold(c.Exe, t.Exe) {
			continue
		}
		e.Emulators = append(e.Emulators, webEmu(t.Profile, &Emulator{Exe: t.Exe, Process: t.Process}, t.Source, !cfgExists))
	}
	// Extra arguments: suggested from the original LaunchBox emulator (e.g. -f), written by "Save". Only for an emulator
	// missing from config.json (a saved empty value stays empty). Never for RetroArch (the -L of one platform's core
	// would go to every System) nor NanoBoyAdvance (the ROM must stay the first argument).
	if o, err := os.ReadFile(filepath.Join(e.LaunchBox, "Data", "Emulators.xml")); e.LaunchBox != "" && err == nil {
		for i := range e.Emulators {
			x := &e.Emulators[i]
			var systems []string
			for _, s := range x.Systems {
				systems = append(systems, s.System)
			}
			if !x.Custom && x.Error == "" && x.Extra == "" && cfg.Emulators[x.Name] == nil && x.Profile != "retroarch" && x.Profile != "nanoboyadvance" {
				x.Extra = launchBoxOptions(string(o), e.LaunchBox, x.Exe, systems)
			}
		}
	}
	for p := range profiles {
		e.Profiles = append(e.Profiles, p)
	}
	sort.Strings(e.Profiles)
	return e
}

// open starts the config.json emulator name, without a game, through "launch" (Sync before and after). For the
// LaunchBox "Open mGBA" entry, which starts the Launcher with no argument, so on this page.
func (a *WebSetup) open(name string) map[string]any {
	var cfg Config
	if err := readJSON(a.config, &cfg); err != nil || cfg.Emulators[name] == nil {
		return map[string]any{"Ok": false, "Message": tr("Cet émulateur n'est pas configuré : ", "This emulator isn't set up: ") + name}
	}
	if err := openDetached(launcherOf(&cfg), []string{"launch", "--config", a.config, name}); err != nil {
		return map[string]any{"Ok": false, "Message": errorText(err)}
	}
	return map[string]any{"Ok": true, "Message": fmt.Sprintf(tr("%s s'ouvre avec vos sauvegardes à jour. Elles seront envoyées quand vous le fermerez.",
		"%s is opening with your saves up to date. They'll be uploaded when you close it."), name)}
}

func (a *WebSetup) test(c WebChoices) map[string]any {
	var cfg Config
	readJSON(a.config, &cfg)
	cfg.URL = strings.TrimRight(c.URL, "/")
	if c.Token != "" {
		cfg.Token = c.Token
	}
	if err := testServer(&cfg); err != nil {
		return map[string]any{"Ok": false, "Message": errorText(err)}
	}
	return map[string]any{"Ok": true, "Message": tr("Connexion réussie.", "Connected.")}
}

// apply writes config.json (copy first), program, frontends, shortcuts, local server. Nothing is written if the
// personal server doesn't answer or a choice is invalid.
func (a *WebSetup) apply(c WebChoices) ([]string, error) {
	var cfg Config
	if err := readJSON(a.config, &cfg); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if c.Local {
		if c.Port <= 0 || c.Port > 65535 {
			return nil, fmt.Errorf("%s", tr("Le port doit être un nombre entre 1 et 65535.", "The port must be a number from 1 to 65535."))
		}
		cfg.LocalServer = &LocalServer{Port: c.Port}
		cfg.URL = fmt.Sprintf("http://127.0.0.1:%d", c.Port)
		if c.Token != "" {
			cfg.Token = c.Token
		}
		if cfg.Token == "" {
			o := make([]byte, 16)
			rand.Read(o)
			cfg.Token = hex.EncodeToString(o)
		}
	} else {
		cfg.LocalServer = nil
		cfg.URL = strings.TrimRight(c.URL, "/")
		if c.Token != "" {
			cfg.Token = c.Token
		}
		if err := testServer(&cfg); err != nil {
			return nil, fmt.Errorf("%s %s", errorText(err), tr("Les réglages ne sont pas enregistrés.", "Your settings weren't saved."))
		}
	}

	emus := map[string]*Emulator{}
	for _, x := range c.Emulators {
		if !x.Active {
			continue
		}
		if emus[x.Name] != nil {
			return nil, fmt.Errorf(tr("Deux émulateurs « %s » sont cochés. Gardez-en un seul.", "Two \"%s\" emulators are ticked. Keep only one."), x.Name)
		}
		e, err := emulatorOf(x)
		if err != nil {
			return nil, err
		}
		explicit := x.Custom // the Launcher doesn't know the settings of a custom emulator
		for _, s := range x.Systems {
			explicit = explicit || s.Saves != s.Default
			if s.Roms != "" {
				if e.Roms == nil {
					e.Roms = map[string]string{}
				}
				e.Roms[s.System] = s.Roms
			}
		}
		if explicit { // one folder changed: all written (else the emulator would handle only that one)
			e.Saves = map[string]string{}
			for _, s := range x.Systems {
				if s.Saves == "" {
					return nil, fmt.Errorf(tr("%s : indiquez le dossier des sauvegardes %s.", "%s: enter the %s save folder."), x.Name, s.System)
				}
				e.Saves[s.System] = s.Saves
			}
		}
		if _, err := e.profile(x.Name); err != nil {
			return nil, err
		}
		emus[x.Name] = e
	}
	cfg.Emulators = emus
	cfg.TrayIcon = nil // "icone_horloge": false only when it is removed
	if !c.Icon {
		cfg.TrayIcon = &c.Icon
	}
	cfg.CheckUpdates = nil
	if !c.Update {
		cfg.CheckUpdates = &c.Update
	}

	var output bytes.Buffer
	q := &Questions{out: &output}
	lines := []string{}
	if cfg.Frontends == nil {
		cfg.Frontends = map[string]string{}
	}
	delete(cfg.Frontends, "launchbox")
	if c.LaunchBox != "" {
		if !exists(filepath.Join(c.LaunchBox, "Data", "Emulators.xml")) {
			return nil, fmt.Errorf(tr("LaunchBox introuvable dans %s. Choisissez le dossier principal de LaunchBox.", "Couldn't find LaunchBox in %s. Choose the main LaunchBox folder."), c.LaunchBox)
		}
		cfg.Frontends["launchbox"] = c.LaunchBox
	}
	esde := cfg.Frontends["esde"]
	delete(cfg.Frontends, "esde")
	if c.EsDe {
		if esde == "" {
			esde = home("ES-DE")
		}
		cfg.Frontends["esde"] = esde
	}
	if c.Install { // after the checks: installExe stops the icon, localServer restarts it
		lines = append(lines, installExe(&cfg, filepath.Dir(a.config)))
	} else if cfg.Launcher == "" {
		cfg.Launcher = executable()
	}
	if err := saveConfig(q, a.config, &cfg); err != nil {
		return nil, err
	}
	lines = append(lines, repair(&cfg, launcherOf(&cfg))...)
	if c.Shortcuts {
		createShortcuts(q, &cfg)
	}
	if err := localServer(q, &cfg, a.config); err != nil { // tray icon, local server
		lines = append(lines, err.Error())
	}
	for _, l := range strings.Split(output.String(), "\n") {
		if l = strings.TrimPrefix(strings.TrimSpace(l), "- "); l != "" && !strings.HasPrefix(l, "==") {
			lines = append(lines, l)
		}
	}
	return lines, nil
}
