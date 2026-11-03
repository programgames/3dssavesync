package main

// Assistant d'installation (phase 4) : serveur et jeton, copie de l'exe à un emplacement fixe, émulateurs,
// frontends. Texte dans une console (ouverte par l'exe sous Windows). Rien n'est écrit sans copie préalable.

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Questions posées dans la console.
type Questions struct {
	in       *bufio.Reader
	out      io.Writer
	terminal bool // saisie cachée possible (jeton)
}

func (q *Questions) say(format string, a ...any) { fmt.Fprintf(q.out, format+"\n", a...) }

func (q *Questions) line() string {
	l, _ := q.in.ReadString('\n')
	return strings.TrimSpace(l)
}

// texte : réponse, ou defaut si Entrée.
func (q *Questions) text(question, dflt string) string {
	if dflt != "" {
		fmt.Fprintf(q.out, "%s [%s] : ", question, dflt)
	} else {
		fmt.Fprintf(q.out, "%s : ", question)
	}
	if r := q.line(); r != "" {
		return strings.Trim(r, `"`)
	}
	return dflt
}

func (q *Questions) yes(question string, dflt bool) bool {
	choice := tr("[O/n]", "[Y/n]")
	if !dflt {
		choice = tr("[o/N]", "[y/N]")
	}
	fmt.Fprintf(q.out, "%s %s ", question, choice)
	switch strings.ToLower(q.line()) {
	case "":
		return dflt
	case "o", "oui", "y", "yes":
		return true
	}
	return false
}

func (q *Questions) secret(question string) string {
	fmt.Fprintf(q.out, "%s : ", question)
	if q.terminal {
		defer noEcho()()
		defer fmt.Fprintln(q.out)
	}
	return q.line()
}

// Emplacement fixe de l'exe : les frontends et les raccourcis pointent vers lui. Version portable : l'exe en cours.
func installedLocation() string {
	if portableDir() != "" {
		return executable()
	}
	switch runtime.GOOS {
	case "windows":
		return env("LOCALAPPDATA", "Programs", "3dssavesync", "3dssavesync.exe")
	case "darwin":
		return home("Applications", "3dssavesync", "3dssavesync")
	}
	return home(".local", "bin", "3dssavesync")
}

// Exe en cours ; vide en test (le binaire de test n'est pas le Lanceur).
var executable = func() string { e, _ := os.Executable(); return e }

// Version portable (ADR 0020) : portable.txt à côté de l'exe. Réglages et données dans le dossier de l'exe, qui n'est
// jamais copié ailleurs. Rend ce dossier, ou "".
func portableDir() string {
	e := executable()
	if e == "" || !exists(filepath.Join(filepath.Dir(e), "portable.txt")) {
		return ""
	}
	return filepath.Dir(e)
}

// Dossier portable en lecture seule (clé verrouillée, Program Files) : une erreur qui dit quoi faire, jamais un repli
// silencieux sur le dossier de l'utilisateur.
func checkPortable(d string) error {
	f, err := os.CreateTemp(d, ".3dssavesync-essai-*")
	if err != nil {
		return fmt.Errorf(tr("Version portable : 3dssavesync ne peut pas écrire dans son dossier %s. Déplacez ce dossier là où vous pouvez écrire (pas dans Program Files ni sur une clé en lecture seule), ou supprimez portable.txt pour garder les réglages dans votre dossier utilisateur.\nDétail : %v",
			"Portable version: 3dssavesync can't write to its folder %s. Move this folder somewhere you can write (not Program Files or a read-only drive), or delete portable.txt to keep the settings in your user folder.\nDetail: %v"), d, err)
	}
	f.Close()
	os.Remove(f.Name())
	return nil
}

func cmdSetup(config string, text, noBrowser bool) (int, error) {
	if !text {
		return cmdSetupWeb(config, noBrowser)
	}
	openConsole()
	q := &Questions{in: bufio.NewReader(os.Stdin), out: os.Stdout, terminal: true}
	err := textSetup(q, config)
	if err != nil {
		q.say("\n%s%s", tr("Erreur : ", "Error: "), errorText(err))
	}
	if cleanConsole {
		q.say("\n%s", tr("Appuyez sur Entrée pour fermer.", "Press Enter to close."))
		q.line()
	}
	if err != nil {
		return 1, nil // déjà affichée dans la console
	}
	return 0, nil
}

func cmdRepair(config string) (int, error) {
	openConsole()
	q := &Questions{in: bufio.NewReader(os.Stdin), out: os.Stdout}
	var cfg Config
	if err := readConfig(config, &cfg); err != nil {
		return 1, err
	}
	for _, l := range repair(&cfg, launcherOf(&cfg)) {
		q.say("- %s", l)
	}
	if cleanConsole {
		q.say("\n%s", tr("Appuyez sur Entrée pour fermer.", "Press Enter to close."))
		q.line()
	}
	return 0, nil
}

// 3dssavesync uninstall [--oui] : la désinstallation (desinstaller.go), après confirmation.
func cmdUninstall(config string, yes bool) (int, error) {
	openConsole()
	q := &Questions{in: bufio.NewReader(os.Stdin), out: os.Stdout}
	var cfg Config
	if err := readConfig(config, &cfg); err != nil {
		return 1, err
	}
	if !yes && !q.yes(tr("Désinstaller 3dssavesync ? LaunchBox et ES-DE lanceront de nouveau vos jeux sans synchronisation. Vos sauvegardes restent en place.",
		"Uninstall 3dssavesync? LaunchBox and ES-DE will start your games without syncing again. Your saves stay where they are."), false) {
		return 0, nil
	}
	lines, err := uninstall(&cfg, config)
	if err != nil {
		return 1, err
	}
	for _, l := range lines {
		q.say("- %s", l)
	}
	if cleanConsole {
		q.say("\n%s", tr("Appuyez sur Entrée pour fermer.", "Press Enter to close."))
		q.line()
	}
	return 0, nil
}

// Exe vers lequel pointent les frontends : celui noté à l'installation, sinon l'exe en cours. Version portable :
// toujours l'exe en cours (la lettre d'une clé USB change).
func launcherOf(cfg *Config) string {
	if cfg.Launcher != "" && portableDir() == "" {
		return cfg.Launcher
	}
	return executable()
}

func textSetup(q *Questions, config string) error {
	q.say("3dssavesync - %s\n", tr("assistant d'installation", "setup assistant"))
	var cfg Config
	foundOne := false
	if err := readJSON(config, &cfg); err == nil {
		foundOne = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if cfg.Emulators == nil {
		cfg.Emulators = map[string]*Emulator{}
	}
	steps := []string{"serveur", "installation", "emulateurs", "frontends"}
	if foundOne {
		q.say("%s %s", tr("Configuration actuelle :", "Current configuration:"), config)
		q.say("  1. %s", tr("Ajouter ou modifier un émulateur", "Add or change an emulator"))
		q.say("  2. %s", tr("Réparer les frontends", "Repair frontends"))
		q.say("  3. %s", tr("Changer le serveur ou le jeton", "Change the server or token"))
		q.say("  4. %s", tr("Tout reconfigurer", "Set up everything again"))
		q.say("  5. %s", tr("Quitter", "Quit"))
		switch q.text(tr("Choix", "Choice"), "1") {
		case "1":
			steps = []string{"emulateurs", "frontends"}
		case "2":
			for _, l := range repair(&cfg, launcherOf(&cfg)) {
				q.say("- %s", l)
			}
			return nil
		case "3":
			steps = []string{"serveur"}
		case "4":
		default:
			return nil
		}
	}
	for _, e := range steps {
		var err error
		switch e {
		case "serveur":
			err = serverStep(q, &cfg)
		case "installation":
			installStep(q, &cfg, config)
		case "emulateurs":
			emulatorsStep(q, &cfg)
		case "frontends":
			err = saveConfig(q, config, &cfg)
			if err == nil {
				frontendsStep(q, &cfg)
			}
		}
		if err != nil {
			return err
		}
	}
	if err := saveConfig(q, config, &cfg); err != nil {
		return err
	}
	if slices.Contains(steps, "serveur") { // icône près de l'horloge (arrêtée par installerExe) et serveur local, comme la page
		if err := localServer(q, &cfg, config); err != nil {
			return err
		}
		if cfg.LocalServer != nil {
			say3DS(q, &cfg)
		}
		sdCardStep(q, &cfg, config)
	}
	q.say("\n%s", tr("Terminé.", "Done."))
	return nil
}

// Adaptateurs remplacés par les tests (aucun raccourci réel, serveur dans le test).
var autostart, launchServer, createShortcut = addAutostart, startDetached, shortcut

// Programmes résidents : démarrage automatique et lancement de l'icône près de l'horloge (tray.go), qui fait aussi
// tourner le serveur local ; sans l'icône, le serveur local seul (serve). Puis le test du serveur local.
func localServer(q *Questions, cfg *Config, config string) error {
	if cfg.LocalServer != nil {
		q.say("\n== %s ==", tr("Serveur local", "Local server"))
	}
	launcher, args := launcherOf(cfg), startupChoice(cfg, config)
	setupAutostart(q, launcher, args, filepath.Dir(config))
	icon := len(args) > 0 && args[0] == "tray"
	if icon { // arrêtée par demarrageAutomatique : elle redémarre avec les nouveaux réglages
		if err := launchServer(launcher, args); err != nil {
			q.say("%s %s", tr("L'icône près de l'horloge n'a pas pu démarrer :", "The icon near the clock couldn't start:"), errorText(err))
		}
	}
	if cfg.LocalServer == nil {
		return nil
	}
	if testServer(cfg) != nil {
		if runtime.GOOS == "windows" {
			q.say("%s", tr("Si Windows demande d'autoriser 3dssavesync sur le réseau, acceptez pour les réseaux privés.",
				"If Windows asks to allow 3dssavesync on the network, allow it for private networks."))
		}
		if !icon { // l'icône vient de démarrer : son serveur local répond dans un instant
			if err := launchServer(launcher, args); err != nil {
				return fmt.Errorf(tr("impossible de lancer le serveur local : %s", "couldn't start the local server: %s"), errorText(err))
			}
		}
		for i := 0; i < 20 && testServer(cfg) != nil; i++ {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if err := testServer(cfg); err != nil {
		return fmt.Errorf(tr("le serveur local ne répond pas. %s", "the local server isn't responding. %s"), errorText(err))
	}
	q.say("%s", tr("Serveur local en marche.", "Local server running."))
	return nil
}

// Console : l'adresse et le jeton à entrer sur la 3DS (la page web les montre dans un cadre à part).
func say3DS(q *Questions, cfg *Config) {
	url, others := address3DS(cfg)
	if url == "" {
		url = tr("impossible de trouver l'adresse du PC sur le réseau. Vérifiez que le PC est connecté à la box", "couldn't find the PC's network address. Check that the PC is connected to the router")
	}
	q.say("\n%s", tr("Sur la 3DS, ouvrez 3dssavesync, appuyez sur SELECT et entrez :", "On the 3DS, open 3dssavesync, press SELECT and enter:"))
	q.say("  %s %s", tr("Serveur :", "Server:"), url)
	q.say("  %s %s", tr("Jeton :", "Token:"), cfg.Token)
	if len(others) > 0 {
		q.say("  %s %s", tr("Si la 3DS ne se connecte pas, essayez :", "If the 3DS can't connect, try:"), strings.Join(others, ", "))
	}
	q.say("%s", tr("Gardez ce jeton pour vous. N'ouvrez jamais ce port sur Internet depuis votre box.",
		"Keep this token private. Never open this port to the Internet on your router."))
}

// config.json : copie datée de l'ancien, puis écriture vérifiée. Rien si rien n'a changé.
func saveConfig(q *Questions, path string, cfg *Config) error {
	var before Config
	if readJSON(path, &before) == nil && jsonOf(&before) == jsonOf(cfg) {
		return nil
	}
	if err := copyBefore(path); err != nil {
		return err
	}
	if err := writeJSON(path, cfg, true); err != nil {
		return err
	}
	q.say("%s %s", tr("Configuration enregistrée :", "Configuration saved:"), path)
	return nil
}

func jsonOf(c *Config) string { o, _ := json.Marshal(c); return string(o) }

func serverStep(q *Questions, cfg *Config) error {
	q.say("\n== %s ==", tr("Serveur", "Server"))
	q.say("  1. %s", tr("Serveur personnel (Docker, adresse https://…) : synchronisation de n'importe où",
		"Personal server (Docker, https://… address): sync from anywhere"))
	q.say("  2. %s", tr("Serveur local sur ce PC : rien à installer, synchronisation à la maison quand le PC est allumé",
		"Local server on this PC: nothing to install, syncs at home while the PC is on"))
	dflt := "1"
	if cfg.LocalServer != nil || cfg.URL == "" { // premier lancement : serveur local, rien d'autre à installer
		dflt = "2"
	}
	if q.text(tr("Choix", "Choice"), dflt) == "2" {
		port := 8765
		if cfg.LocalServer != nil && cfg.LocalServer.Port > 0 {
			port = cfg.LocalServer.Port
		}
		if p, err := strconv.Atoi(q.text(tr("Port", "Port"), strconv.Itoa(port))); err == nil && p > 0 && p < 65536 {
			port = p
		}
		if cfg.LocalServer == nil {
			cfg.LocalServer = &LocalServer{}
		}
		cfg.LocalServer.Port = port
		cfg.URL = fmt.Sprintf("http://127.0.0.1:%d", port)
		if cfg.Token == "" {
			o := make([]byte, 16)
			rand.Read(o)
			cfg.Token = hex.EncodeToString(o)
		}
		return nil // démarré et testé une fois config.json écrit (serveurLocal)
	}
	cfg.LocalServer = nil
	if strings.HasPrefix(cfg.URL, "http://127.0.0.1:") {
		cfg.URL = ""
	}
	for {
		cfg.URL = strings.TrimRight(q.text(tr("Adresse du serveur (https://…)", "Server address (https://…)"), cfg.URL), "/")
		question := tr("Jeton (rien ne s'affiche pendant la saisie)", "Token (nothing shows as you type)")
		if cfg.Token != "" {
			question = tr("Jeton (Entrée pour garder l'actuel)", "Token (Enter to keep the current one)")
		}
		if j := q.secret(question); j != "" {
			cfg.Token = j
		}
		err := testServer(cfg)
		if err == nil {
			q.say("%s", tr("Connexion réussie.", "Connected."))
			return nil
		}
		q.say("%s", errorText(err))
		if !q.yes(tr("Réessayer ?", "Try again?"), true) {
			return fmt.Errorf("%s", tr("serveur injoignable, la configuration n'est pas enregistrée", "server unreachable, so the configuration wasn't saved"))
		}
	}
}

func testServer(cfg *Config) error {
	s := &Session{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second, Transport: transport}, profile: "setup"}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return errors.New(tr("Entrez l'adresse du serveur, avec https:// (ou http:// pour le serveur local).",
			"Enter the server address, with https:// (or http:// for the local server)."))
	}
	st, _, err := s.api("GET", "/saves", nil, nil)
	if err == nil && st != 200 {
		err = &httpError{"GET /saves", st}
	}
	return err
}

// Copie de l'exe à l'emplacement fixe (un exe téléchargé reste où il est sinon).
func installStep(q *Questions, cfg *Config, config string) {
	q.say("\n== %s ==", tr("Installation", "Install"))
	current, target := executable(), installedLocation()
	cfg.Launcher = current
	if current == "" || strings.EqualFold(filepath.Clean(current), filepath.Clean(target)) {
		q.say("%s %s", tr("Programme déjà installé :", "Program already installed:"), current)
		return
	}
	if !q.yes(fmt.Sprintf(tr("Copier le programme dans %s ?", "Copy the program to %s?"), target), true) {
		return
	}
	q.say("%s", installExe(cfg, filepath.Dir(config)))
}

// Copie de l'exe en cours à l'emplacement fixe ; cfg.Lanceur suit. Rend la ligne à afficher. L'icône près de l'horloge
// (dossier des réglages) tourne souvent depuis l'exe installé : elle s'arrête avant la copie (Windows refuse de
// remplacer un exe en marche) ; serveurLocal la redémarre ensuite.
func installExe(cfg *Config, dir string) string {
	current, target := executable(), installedLocation()
	if cfg.Launcher == "" {
		cfg.Launcher = current
	}
	if current == "" || strings.EqualFold(filepath.Clean(current), filepath.Clean(target)) {
		cfg.Launcher = current
		return tr("Programme déjà installé : ", "Program already installed: ") + current
	}
	stopTray(dir) // échec : l'exe installé reste peut-être pris, le message de la copie dit quoi faire
	o, err := os.ReadFile(current)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(target), 0o755)
	}
	if err == nil {
		os.Remove(target + ".3dssavesync-tmp")
		if err = writeVerified(target+".3dssavesync-tmp", o); err == nil {
			os.Chmod(target+".3dssavesync-tmp", 0o755)
			if err = rename(target+".3dssavesync-tmp", target); err != nil { // exe installé en marche
				os.Remove(target + ".3dssavesync-tmp")
				return tr("Impossible de copier le programme : la version installée est encore ouverte. Quittez l'icône près de l'horloge, fermez les autres fenêtres de 3dssavesync et enregistrez de nouveau.",
					"Couldn't copy the program: the installed version is still running. Quit the icon near the clock, close the other 3dssavesync windows and save again.") +
					"\n" + tr("Détail : ", "Detail: ") + err.Error()
			}
		}
	}
	if err != nil {
		return tr("Impossible de copier le programme, il est peut-être encore ouvert : ", "Couldn't copy the program, it may still be running: ") + errorText(err)
	}
	cfg.Launcher = target
	return tr("Programme copié : ", "Program copied: ") + target
}

// --- Émulateurs ----------------------------------------------------------------------------------

// Trouve : un émulateur détecté sur la machine.
type Found struct {
	Profile string `json:"Profil"`
	Exe     string
	Process string `json:"Processus"`
	Source  string
}

// Noms connus par profil : exécutables, dossiers usuels, Flatpak, application macOS, AppImage (EmuDeck).
var known = map[string]struct {
	exes, dirs   []string
	flatpak, app string
	appimage     []string
	process      string
}{
	"melonds":   {[]string{"melonDS"}, []string{"melonDS"}, "net.kuribo64.melonDS", "melonDS", []string{"melonDS*.AppImage"}, "melonDS"},
	"mgba":      {[]string{"mGBA", "mgba-qt"}, []string{"mGBA"}, "io.mgba.mGBA", "mGBA", []string{"mGBA*.AppImage"}, "mgba-qt"},
	"azahar":    {[]string{"azahar"}, []string{"Azahar", "azahar"}, "org.azahar_emu.Azahar", "Azahar", []string{"azahar*.AppImage", "Azahar*.AppImage"}, "azahar"},
	"retroarch": {[]string{"retroarch"}, []string{"RetroArch", "RetroArch-Win64"}, "org.libretro.RetroArch", "RetroArch", []string{"RetroArch*.AppImage"}, "retroarch"},
	"vbam":      {[]string{"visualboyadvance-m"}, []string{"VisualBoyAdvance-M", "visualboyadvance-m"}, "", "visualboyadvance-m", nil, "visualboyadvance-m"},
	"sameboy":   {[]string{"sameboy"}, []string{"SameBoy"}, "io.github.sameboy.SameBoy", "SameBoy", nil, "sameboy"},
	"desmume":   {[]string{"desmume", "DeSmuME_0.9.13_x64"}, []string{"DeSmuME"}, "org.desmume.DeSmuME", "", nil, "desmume"},

	// Ticket #3 (bgb64 : nom de l'exe de bgbw64.zip NON VÉRIFIÉ)
	"skyemu":         {[]string{"SkyEmu", "SkyEmu-v5-Windows"}, []string{"SkyEmu"}, "", "SkyEmu", nil, "SkyEmu"},
	"nanoboyadvance": {[]string{"NanoBoyAdvance"}, []string{"NanoBoyAdvance"}, "", "NanoBoyAdvance", []string{"NanoBoyAdvance*.AppImage"}, "NanoBoyAdvance"},
	"bgb":            {[]string{"bgb", "bgb64"}, []string{"bgb"}, "", "", nil, "bgb"},
}

// Détection : PATH, dossiers usuels, Flatpak, AppImage, applications macOS, émulateurs de LaunchBox.
var detect = func(lbDir string) []Found {
	var t []Found
	seenSet := map[string]bool{}
	add := func(profile, exe, process, source string) {
		exe = shimTarget(exe)
		if exe == "" || !exists(exe) || seenSet[strings.ToLower(exe)] {
			return
		}
		seenSet[strings.ToLower(exe)] = true
		t = append(t, Found{profile, exe, process, source})
	}
	var sortedProfiles []string
	for p := range known {
		sortedProfiles = append(sortedProfiles, p)
	}
	sort.Strings(sortedProfiles)
	for _, p := range sortedProfiles {
		c := known[p]
		for _, n := range c.exes {
			if e, err := exec.LookPath(n); err == nil {
				add(p, e, "", "PATH")
			}
			for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), env("LOCALAPPDATA", "Programs"), home("scoop", "apps")} {
				for _, d := range c.dirs {
					add(p, joinPath(base, d, n+".exe"), "", base)
					add(p, joinPath(base, strings.ToLower(d), "current", n+".exe"), "", "scoop")
				}
			}
		}
		if c.flatpak != "" {
			add(p, filepath.Join("/var/lib/flatpak/exports/bin", c.flatpak), c.process, "Flatpak")
			add(p, home(".local", "share", "flatpak", "exports", "bin", c.flatpak), c.process, "Flatpak")
		}
		for _, pattern := range c.appimage {
			m, _ := filepath.Glob(home("Applications", pattern))
			for _, f := range m {
				add(p, f, c.process, "AppImage")
			}
		}
		for _, root := range []string{"/Applications", home("Applications")} {
			add(p, joinPath(root, c.app+".app", "Contents", "MacOS", c.app), "", "Applications")
		}
	}
	if lbDir != "" {
		o, _ := os.ReadFile(filepath.Join(lbDir, "Data", "Emulators.xml"))
		for _, b := range lbEmulatorBlock.FindAllString(string(o), -1) {
			app := field(b, "ApplicationPath")
			if app != "" && !filepath.IsAbs(app) {
				app = filepath.Join(lbDir, app)
			}
			name := strings.TrimSuffix(strings.ToLower(filepath.Base(app)), ".exe")
			for _, p := range sortedProfiles {
				for _, n := range known[p].exes {
					if name == strings.ToLower(n) {
						add(p, filepath.Clean(app), "", "LaunchBox")
					}
				}
			}
		}
	}
	return t
}

var reShim = regexp.MustCompile(`(?m)^path\s*=\s*"?([^"\r\n]+?)"?\s*$`)

// cibleShim : l'exe réel derrière un shim de Scoop (scoop\shims\x.exe, et x.shim à côté : path = "…") ; sinon exe.
// Sans cela, le même émulateur apparaît deux fois (PATH et scoop).
func shimTarget(exe string) string {
	if !strings.EqualFold(filepath.Ext(exe), ".exe") {
		return exe
	}
	o, err := os.ReadFile(strings.TrimSuffix(exe, filepath.Ext(exe)) + ".shim")
	if m := reShim.FindSubmatch(o); err == nil && m != nil {
		return filepath.Clean(string(m[1]))
	}
	return exe
}

// Cœurs RetroArch trouvés près de l'exe ou dans le dossier de configuration : Système -> cœur préféré.
func retroArchCores(exe string) map[string]string {
	var dirs []string
	if exe != "" {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "cores"))
	}
	if f := configRetroArch(&Emulator{Exe: exe}); f != "" {
		dirs = append(dirs, filepath.Join(filepath.Dir(f), "cores"))
	}
	dirs = append(dirs, home("Library", "Application Support", "RetroArch", "cores"))
	foundList := map[string]string{}
	for _, d := range dirs {
		m, _ := filepath.Glob(filepath.Join(d, "*_libretro.*"))
		for _, f := range m {
			if b := library(f); b != "" && foundList[b] == "" {
				foundList[b] = f
			}
		}
	}
	preferred := map[string][]string{"DS": {"melonDS DS"}, "GB": {"Gambatte", "SameBoy", "mGBA", "VBA-M"}, "GBA": {"mGBA", "VBA-M", "gpSP", "Beetle GBA"}}
	cores := map[string]string{}
	for s, l := range preferred {
		for _, b := range l {
			if f := foundList[b]; f != "" {
				cores[s] = f
				break
			}
		}
	}
	return cores
}

func emulatorsStep(q *Questions, cfg *Config) {
	q.say("\n== %s ==", tr("Émulateurs", "Emulators"))
	lbDir := cfg.Frontends["launchbox"]
	if lbDir == "" && runtime.GOOS == "windows" {
		lbDir = findLaunchBox()
	}
	foundList := detect(lbDir)
	if len(foundList) == 0 {
		q.say("%s", tr("Aucun émulateur trouvé automatiquement.", "No emulator found automatically."))
	}
	var configured []string
	for n := range cfg.Emulators {
		configured = append(configured, n)
	}
	sort.Strings(configured)
	for _, n := range configured {
		e := cfg.Emulators[n]
		if q.yes(fmt.Sprintf(tr("%s est déjà configuré (%s). Modifier ses dossiers ?", "%s is already set up (%s). Change its folders?"), n, e.Exe), false) {
			addEmulator(q, cfg, Found{Profile: n, Exe: e.Exe, Process: e.Process}, false)
		}
	}
	for _, t := range foundList {
		a := cfg.Emulators[t.Profile]
		switch {
		case a != nil && strings.EqualFold(a.Exe, t.Exe):
		case a != nil:
			if q.yes(fmt.Sprintf(tr("Une autre installation de %s a été trouvée (%s) : %s. L'utiliser à la place de %s ?", "Found another %s install (%s): %s. Use it instead of %s?"),
				t.Profile, t.Source, t.Exe, a.Exe), false) {
				addEmulator(q, cfg, t, true)
			}
		case q.yes(fmt.Sprintf(tr("%s trouvé (%s) : %s. L'utiliser ?", "Found %s (%s): %s. Use it?"), t.Profile, t.Source, t.Exe), true):
			addEmulator(q, cfg, t, false)
		}
	}
	names := make([]string, 0, len(profiles))
	for p := range profiles {
		names = append(names, p)
	}
	sort.Strings(names)
	for q.yes(tr("Ajouter un émulateur à la main ?", "Add an emulator by hand?"), false) {
		for i, p := range names {
			q.say("  %d. %s", i+1, p)
		}
		i, err := strconv.Atoi(q.text(tr("Numéro du profil", "Profile number"), ""))
		if err != nil || i < 1 || i > len(names) {
			continue
		}
		if exe := q.text(tr("Chemin de l'exécutable", "Executable path"), ""); exe != "" {
			addEmulator(q, cfg, Found{Profile: names[i-1], Exe: exe, Source: "manuel"}, false)
		}
	}
}

// remplacer : l'utilisateur a déjà accepté de remplacer l'émulateur du même nom.
func addEmulator(q *Questions, cfg *Config, t Found, replace bool) {
	name := t.Profile
	if old := cfg.Emulators[name]; old != nil && !replace && !strings.EqualFold(old.Exe, t.Exe) {
		if !q.yes(fmt.Sprintf(tr("%s existe déjà (%s). Le remplacer ?", "%s already exists (%s). Replace it?"), name, old.Exe), false) {
			return
		}
	}
	e := &Emulator{Exe: t.Exe, Process: t.Process}
	if old := cfg.Emulators[name]; old != nil && strings.EqualFold(old.Exe, t.Exe) {
		e = old // mêmes réglages : on les garde (dossiers, roms)
	}
	if name == "retroarch" && len(e.Cores) == 0 {
		e.Cores = retroArchCores(t.Exe)
		if len(e.Cores) == 0 {
			q.say("%s", tr("Aucun cœur compatible trouvé dans RetroArch. Installez au moins l'un de ceux-ci, puis relancez l'assistant : melonDS DS, mGBA, Gambatte, SameBoy, VBA-M, gpSP, Beetle GBA.",
				"No supported core found in RetroArch. Install at least one of these, then run the assistant again: melonDS DS, mGBA, Gambatte, SameBoy, VBA-M, gpSP, Beetle GBA."))
			return
		}
	}
	p, err := e.profile(name)
	if err != nil {
		q.say("%s", err)
		return
	}
	for _, s := range e.systems(p) {
		d := e.dir(p, s, "")
		shown := d
		if d == "" {
			shown = tr("à côté de chaque ROM", "next to each ROM")
		}
		if s == "3DS" && d == "" {
			shown = ""
		}
		if r := q.text(fmt.Sprintf(tr("Sauvegardes %s", "%s saves"), s), shown); r != shown && r != "" {
			if e.Saves == nil {
				e.Saves = map[string]string{}
			}
			e.Saves[s] = r
		}
		if s != "3DS" {
			if r := q.text(fmt.Sprintf(tr("Dossier des ROM %s, pour la commande launch (Entrée pour passer)", "%s ROM folder, for the launch command (Enter to skip)"), s), e.Roms[s]); r != "" {
				if e.Roms == nil {
					e.Roms = map[string]string{}
				}
				e.Roms[s] = r
			}
		}
	}
	if hint := hint(e, p); hint != "" {
		q.say("%s %s", tr("Attention :", "Warning:"), hint)
	}
	cfg.Emulators[name] = e
	q.say("%s %s", tr("Émulateur enregistré :", "Emulator saved:"), name)
}

// --- Frontends -----------------------------------------------------------------------------------

func findLaunchBox() string {
	for _, d := range []string{home("LaunchBox"), `C:\LaunchBox`, `D:\LaunchBox`, `E:\LaunchBox`, env("ProgramFiles", "LaunchBox")} {
		if exists(joinPath(d, "Data", "Emulators.xml")) {
			return d
		}
	}
	return ""
}

func frontendsStep(q *Questions, cfg *Config) {
	q.say("\n== %s ==", tr("Frontends", "Frontends"))
	if cfg.Frontends == nil {
		cfg.Frontends = map[string]string{}
	}
	if runtime.GOOS == "windows" {
		d := cfg.Frontends["launchbox"]
		if d == "" {
			d = findLaunchBox()
		}
		d = q.text(tr("Dossier de LaunchBox (Entrée si vous n'utilisez pas LaunchBox)", "LaunchBox folder (Enter if you don't use LaunchBox)"), d)
		if d != "" && exists(filepath.Join(d, "Data", "Emulators.xml")) {
			cfg.Frontends["launchbox"] = d
		} else if d != "" {
			q.say("%s", tr("LaunchBox introuvable dans ce dossier, il est ignoré.", "LaunchBox wasn't found in this folder, so it's skipped."))
		}
	}
	esde := cfg.Frontends["esde"]
	if esde == "" && exists(home("ES-DE")) {
		esde = home("ES-DE")
	}
	if esde != "" && q.yes(fmt.Sprintf(tr("Configurer ES-DE (%s) ?", "Set up ES-DE (%s)?"), esde), true) {
		cfg.Frontends["esde"] = esde
	}
	for {
		lb, es := cfg.Frontends["launchbox"] != "" && (activeEmulator("LaunchBox") || activeEmulator("BigBox")), cfg.Frontends["esde"] != "" && activeEmulator("ES-DE")
		if !lb && !es {
			break
		}
		q.text(tr("Fermez LaunchBox, BigBox et ES-DE, et appuyez sur Entrée", "Close LaunchBox, BigBox and ES-DE, and press Enter"), "")
	}
	for _, l := range repair(cfg, launcherOf(cfg)) {
		q.say("- %s", l)
	}
	if !q.yes(tr("Créer un raccourci par émulateur ? Il synchronise tous les jeux et ouvre l'émulateur.", "Create a shortcut for each emulator? It syncs all games and opens the emulator."), true) {
		return
	}
	createShortcuts(q, cfg)
}

// Un raccourci par émulateur (launch) et un pour les réglages. Sans dossier des ROM (hors 3DS), le raccourci ne
// synchroniserait rien : pas de raccourci, les jeux se lancent depuis le frontend.
func createShortcuts(q *Questions, cfg *Config) {
	launcher := launcherOf(cfg)
	var names, noRoms []string
	for n := range cfg.Emulators {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		e := cfg.Emulators[n]
		if p, err := e.profile(n); err == nil && len(e.Roms) == 0 && !slices.Equal(e.systems(p), []string{"3DS"}) {
			noRoms = append(noRoms, e.process())
			continue
		}
		f, err := createShortcut("3dssavesync "+n, launcher, []string{"launch", n}, false)
		showShortcut(q, f, err)
	}
	f, err := createShortcut(tr("3dssavesync réglages", "3dssavesync settings"), launcher, []string{"setup"}, false)
	showShortcut(q, f, err)
	if len(noRoms) > 0 {
		q.say(tr("Pas de raccourci pour %s, car aucun dossier de jeux n'est indiqué. Lancez ces jeux depuis votre frontend.",
			"No shortcut for %s, because no games folder is set. Start these games from your frontend."), strings.Join(noRoms, ", "))
	}
}

func showShortcut(q *Questions, f string, err error) {
	if err != nil {
		q.say("- %s %s", tr("Impossible de créer le raccourci :", "Couldn't create the shortcut:"), errorText(err))
		return
	}
	q.say("- %s %s", tr("Raccourci créé :", "Shortcut created:"), f)
}
