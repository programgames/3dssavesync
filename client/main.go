// 3dssavesync syncs an emulator's saves with the server, before and after playing.
//
//	3dssavesync run [--emulator <name>] <rom>            one game (frontends)
//	3dssavesync run [--emulator <name>] --rom <rom> -- <command…>  wraps any command
//	3dssavesync launch <name>                            all games of an emulator, then the emulator
//	3dssavesync setup [--text] [--no-browser] (or none)  setup (browser, or console)
//	3dssavesync repair                                   redoes the frontend configuration
//	3dssavesync serve                                    local server (local network, for the 3DS)
//	3dssavesync tray                                     tray icon (and local server), at login
//	3dssavesync restore [--yes] [<key> [<n>]]            makes an old Version the Current version again
//
// Common option: --config <file> (default: <config folder>/3dssavesync/config.json).
package main

// Windows exe icon (rsrc_windows_*.syso, ignored outside Windows) from icone/icone.ico (assets/generer.py):
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --manifest none --icon icone/icone.ico

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	URL       string               `json:"url"`
	Token     string               `json:"jeton"`
	Copies    string               `json:"copies,omitempty"` // default: <config folder>/copies
	Emulators map[string]*Emulator `json:"emulateurs"`
	Launcher  string               `json:"lanceur,omitempty"`   // installed exe, which the frontends point to
	Frontends map[string]string    `json:"frontends,omitempty"` // launchbox, esde -> folder (repair)
	// Names of 3DS games: title ID -> name. Before ADR 0028, the only 3DS games synced besides the 8 Pokémon; now
	// every 3DS game with a save in Azahar syncs, and this list only gives names.
	Titles3DS map[string]string `json:"titres_3ds,omitempty"`
	Off3DS    []string          `json:"titres_3ds_off,omitempty"` // 3DS title IDs turned off: never synced (ADR 0028)
	Alias     map[string]string `json:"alias,omitempty"`          // ROM name without extension -> chosen Key (ADR 0014)
	// Local server (3dssavesync serve): same token; url = http://127.0.0.1:<port> for this PC.
	LocalServer  *LocalServer `json:"serveur_local,omitempty"`
	TrayIcon     *bool        `json:"icone_horloge,omitempty"` // missing: tray icon on (tray.go)
	CheckUpdates *bool        `json:"verifier_maj,omitempty"`  // missing: update check on (selfupdate.go)
	AutoSync     *bool        `json:"synchro_auto,omitempty"`  // missing: Sync without the Launcher on (watch.go)
	// Date of the last successful preparation of the 3DS SD card (the "all ready" list), never guessed.
	PreparedCard string `json:"carte_preparee,omitempty"`
}

type LocalServer struct {
	Port int    `json:"port"`
	Data string `json:"donnees,omitempty"` // default: serveur/ next to config.json
}

type Emulator struct {
	Profile string            `json:"profil,omitempty"` // built-in Profile (profiles.go); default: the entry name
	Exe     string            `json:"exe"`
	Saves   map[System]string `json:"sauvegardes,omitempty"` // System -> folder (3DS: sdmc); default: emulator setting
	Roms    map[System]string `json:"roms,omitempty"`        // System -> ROM folder (launch command)
	Cores   map[System]string `json:"coeurs,omitempty"`      // RetroArch: System -> core file
	Process string            `json:"processus,omitempty"`   // default: name of the executable
	// Custom Profile: arguments ({rom}, {sauvegardes}), extension (default .sav), name of the file inside an archive.
	Args         []string `json:"args,omitempty"`
	ExtraArgs    []string `json:"args_en_plus,omitempty"` // built-in Profile: added before the ROM (e.g. -f)
	Extension    string   `json:"extension,omitempty"`
	InternalName bool     `json:"nom_interne,omitempty"`
}

// off3DS reports a 3DS game turned off in titres_3ds_off, from its Key or title ID. A 3DSX extdata is off when its
// ID is titleLow >> 8 of a turned-off game (ADR 0031).
func (c *Config) off3DS(key string) bool {
	id := key[strings.IndexByte(key, '-')+1:]
	return slices.ContainsFunc(c.Off3DS, func(o string) bool {
		o = strings.TrimSpace(o)
		if strings.HasPrefix(key, "3DSX-") {
			return extdataID(o) == id
		}
		return strings.EqualFold(o, id)
	})
}

func (c *Config) copiesDir(configDir string) string {
	if c.Copies != "" {
		return c.Copies
	}
	return filepath.Join(configDir, "copies")
}

// Adapters that the tests replace.
var (
	ask            = askSystem // text, buttons -> chosen button; "" if no window is possible
	notify         = systemNotify
	launch         = runCommand
	activeEmulator = func(name string) bool { return instances(name) > 0 }
	instances      = countProcesses // running processes with this name
	alive          = processAlive   // this pid still runs
	transport      = http.DefaultTransport
)

// hiddenAliases adds alias -> name flags that set the same value as name; the help lists only the names.
func hiddenAliases(fl *flag.FlagSet, aliases map[string]string) {
	help := flag.NewFlagSet(fl.Name(), flag.ContinueOnError)
	fl.VisitAll(func(f *flag.Flag) { help.Var(f.Value, f.Name, f.Usage) })
	fl.Usage = func() {
		fmt.Fprintf(fl.Output(), "Usage of %s:\n", fl.Name())
		help.SetOutput(fl.Output())
		help.PrintDefaults()
	}
	for alias, name := range aliases {
		fl.Var(fl.Lookup(name).Value, alias, "")
	}
}

// runCommand waits for the command to end. Only a failed start counts: after an emulator crash,
// the Upload must still happen.
func runCommand(command []string) error {
	c := exec.Command(command[0], command[1:]...)
	if err := c.Start(); err != nil {
		return err
	}
	c.Wait()
	return nil
}

func main() {
	code := realMain(os.Args[1:])
	waitNotifications()
	os.Exit(code)
}

func realMain(args []string) int {
	// A frontend that stops the program (Lutris, Steam) must not cut the Upload:
	// wait for the emulator to end, then upload. SIGKILL can't be caught.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	if len(args) == 0 { // double-click on the exe: the setup
		args = []string{"setup"}
	}
	fl := flag.NewFlagSet(args[0], flag.ContinueOnError)
	config := fl.String("config", defaultConfig(), "configuration file")
	emulator := fl.String("emulator", "", "emulator of the configuration")
	rom := fl.String("rom", "", "ROM, with a command after --")
	text := fl.Bool("text", false, "setup: text setup in a console, without a browser")
	noBrowser := fl.Bool("no-browser", false, "setup: print the address of the page instead of opening the browser (SSH)")
	yes := fl.Bool("yes", false, "restore, uninstall: no confirmation question")
	// The French names of the first versions still work (shortcuts, scripts).
	hiddenAliases(fl, map[string]string{"texte": "text", "sans-navigateur": "no-browser", "oui": "yes"})
	if err := fl.Parse(args[1:]); err != nil {
		return 2
	}
	dir := filepath.Dir(*config)
	var code int
	var err error
	if d := portableDir(); d != "" {
		err = checkPortable(d)
	}
	if err == nil {
		switch args[0] {
		case "run":
			code, err = cmdRun(*config, *emulator, *rom, fl.Args())
		case "launch":
			if fl.NArg() != 1 {
				err = errors.New(tr("launch attend un nom d'émulateur", "launch needs an emulator name"))
				break
			}
			code, err = cmdLaunch(*config, fl.Arg(0))
		case "setup":
			code, err = cmdSetup(*config, *text, *noBrowser)
		case "repair":
			code, err = cmdRepair(*config)
		case "uninstall":
			code, err = cmdUninstall(*config, *yes)
		case "serve":
			code, err = cmdServe(*config)
		case "tray":
			code, err = cmdTray(*config)
		case "restore":
			code = cmdRestoreConsole(*config, fl.Args(), *yes)
		default:
			err = fmt.Errorf(tr("commande inconnue : %s", "unknown command: %s"), args[0])
		}
	}
	if err != nil {
		logTo(dir, "erreur : "+err.Error()+" ("+strings.Join(args, " ")+")")
		text := errorText(err)
		fmt.Fprintln(os.Stderr, text)
		if j := filepath.Join(dir, "3dssavesync.log"); exists(j) {
			text += "\n\n" + tr("Détails dans le journal : ", "Details in the log: ") + j
		}
		ask(tr("Erreur de 3dssavesync : ", "3dssavesync error: ")+text+"\n"+tr("Aide : ", "Help: ")+helpPage, []string{"OK"})
		return 1
	}
	return code
}

func defaultConfig() string {
	if d := portableDir(); d != "" {
		return filepath.Join(d, "config.json")
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "3dssavesync", "config.json")
	}
	return "config.json"
}

// readConfig: a missing config.json means the setup never finished; the message says what to do.
func readConfig(path string, cfg *Config) error {
	err := readJSON(path, cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.New(tr("3dssavesync n'est pas encore configuré. Ouvrez « 3dssavesync » pour choisir le serveur et les émulateurs.",
			"3dssavesync isn't set up yet. Open \"3dssavesync\" to choose the server and the emulators."))
	}
	return err
}

func newSession(path string) (*Session, error) {
	var cfg Config
	if err := readConfig(path, &cfg); err != nil {
		return nil, err
	}
	for id, name := range cfg.Titles3DS {
		names3DS[strings.ToUpper(id)] = name
	}
	dir := filepath.Dir(path)
	return &Session{cfg: &cfg, config: path, dir: dir, remote: &remote{cfg: &cfg, dir: dir, http: &http.Client{Timeout: 30 * time.Second, Transport: transport}}}, nil
}

// emulatorFor returns the configured emulator that handles this System: the one asked for, or the only possible one.
func (c *Config) emulatorFor(system System, asked string) (string, error) {
	handles := func(name string) bool {
		e := c.Emulators[name]
		if e == nil {
			return false
		}
		p, err := e.profile(name)
		return err == nil && e.handles(p, system)
	}
	if asked != "" {
		if !handles(asked) {
			return "", fmt.Errorf(tr("l'émulateur %s ne gère pas le système %s", "emulator %s doesn't support the %s system"), asked, system)
		}
		return asked, nil
	}
	var names []string
	for name := range c.Emulators {
		if handles(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	switch len(names) {
	case 0:
		return "", fmt.Errorf(tr("aucun émulateur configuré pour %s", "no emulator configured for %s"), system)
	case 1:
		return names[0], nil
	}
	return "", fmt.Errorf(tr("plusieurs émulateurs gèrent %s (%s) : choisissez-en un avec --emulator", "several emulators handle %s (%s): pick one with --emulator"), system, strings.Join(names, ", "))
}

func cmdRun(config, asked, rom string, rest []string) (int, error) {
	command := rest
	if rom == "" {
		if len(rest) != 1 {
			return 2, errors.New(tr("run attend une ROM, ou --rom <rom> -- <commande>", "run needs a ROM, or --rom <rom> -- <command>"))
		}
		rom, command = rest[0], nil
	} else if len(rest) == 0 {
		return 2, errors.New(tr("run --rom : indiquez la commande à lancer après --", "run --rom: add the command to run after --"))
	}
	s, err := newSession(config)
	if err != nil {
		return 1, err
	}
	r, err := readRom(rom)
	system, key, name, internal := r.System, r.Key, r.Name, r.Internal
	s.log(fmt.Sprintf("lancement %s %s : %s", system, key, rom))
	if err != nil {
		return 1, err
	}
	// Unread format (.7z, .rar, .cia…): the game still starts, without Sync, with the emulator asked for
	// (LaunchBox, ES-DE) or the only one in the configuration.
	unread := system == ""
	if unread {
		if asked == "" && len(s.cfg.Emulators) == 1 {
			for n := range s.cfg.Emulators {
				asked = n
			}
		}
		if s.cfg.Emulators[asked] == nil {
			return 1, fmt.Errorf(tr("Extension de ROM inconnue : %s", "Unknown ROM extension: %s"), rom)
		}
	} else if asked, err = s.cfg.emulatorFor(system, asked); err != nil {
		return 1, err
	}
	c := s.client(asked)
	e := s.cfg.Emulators[c.name]
	p, err := e.profile(c.name)
	if err != nil {
		return 1, err
	}
	if l := e.systems(p); unread && len(l) == 1 {
		system = l[0] // for the command (RetroArch core)
	}
	if unread && command == nil && len(e.Cores) > 0 && e.Cores[system] == "" {
		return 1, fmt.Errorf(tr("Extension de ROM inconnue : %s", "Unknown ROM extension: %s"), rom) // RetroArch: which core?
	}
	var why string
	if !unread { // unread format: no Key, so no Alias either
		key, why = s.romKey(system, key, name)
	}
	if system == N3DS && key != "" && s.cfg.off3DS(key) {
		why = tr("ce jeu 3DS est désactivé dans les réglages (titres_3ds_off)", "this 3DS game is turned off in the settings (titres_3ds_off)")
	}
	if s.refusal(e, key, name) {
		return 1, nil
	}
	defer s.acquireLock(e.process(), key)()
	dir := e.dir(p, system, rom)
	if command == nil {
		command = e.command(p, system, rom, dir)
	}
	var j *Game
	if why == "" && key != "" && dir != "" {
		if p.InternalName && internal != "" {
			name = internal
		}
		core := library(e.Cores[system])
		j = &Game{system, key, name, savePath(dir, system, key, name, p.Ext), core, s.cfg.mbc2Size(c.name, core)}
		if system != "3DS" { // Azahar stores the Save by title ID: two ROMs of a 3DS game share it
			why = c.romRefusal(j, rom, true)
		}
	}
	var games []*Game
	switch {
	case why != "": // ROM hack under another game's Key, Key already synced with no ROM kept, invalid Alias (ADR 0014)
		s.log("sans synchronisation : " + why)
		notify(tr("Jeu lancé sans synchronisation : ", "Game started without syncing: ") + why)
	case key == "": // homebrew, or an ignored 3DS game
		s.log("sans synchronisation : Clé illisible ou jeu hors périmètre")
		if !unread {
			notify(tr("Jeu lancé sans synchronisation : 3dssavesync ne reconnaît pas ce jeu (homebrew ou jeu 3DS non pris en charge). Sa sauvegarde reste sur ce PC.",
				"Game started without syncing: 3dssavesync doesn't recognize this game (homebrew or unsupported 3DS game). Its save stays on this PC."))
		}
	case dir == "":
		return 1, fmt.Errorf(tr("dossier des sauvegardes %s de %s introuvable. Indiquez-le dans les réglages.", "can't find the %s save folder of %s. Set it in the settings."), system, c.name)
	default:
		games = []*Game{j}
		if system == N3DS { // the extdata of the game, if Azahar has one (ADR 0031)
			ext := extdataID(key[strings.IndexByte(key, '-')+1:])
			for _, g := range c.extdata3DS(dir, map[string]string{ext: name}) {
				if g.Key == "3DSX-"+ext {
					games = append(games, g)
				}
			}
		}
	}
	if unread {
		notify(fmt.Sprintf(tr("Jeu lancé sans synchronisation : 3dssavesync ne sait pas lire ce format de ROM (%s). Utilisez une ROM .zip ou non compressée.",
			"Game started without syncing: 3dssavesync can't read this ROM format (%s). Use a .zip or uncompressed ROM."), strings.ToLower(filepath.Ext(rom))))
	}
	return c.play(games, command, hint(e, p))
}

// hint names the setting that loads a save state at start: it can replace a downloaded Version with older progress.
func hint(e *Emulator, p Profile) string {
	if p.Autoload == nil {
		return ""
	}
	return p.Autoload(e)
}

func cmdLaunch(config, name string) (int, error) {
	s, err := newSession(config)
	if err != nil {
		return 1, err
	}
	c := s.client(name)
	s.log("lancement de " + name)
	e := s.cfg.Emulators[name]
	if e == nil {
		return 1, fmt.Errorf(tr("émulateur non configuré : %s", "emulator not set up: %s"), name)
	}
	p, err := e.profile(name)
	if err != nil {
		return 1, err
	}
	if s.refusal(e, "*", "") { // no game: the emulator can open any game
		return 1, nil
	}
	defer s.acquireLock(e.process(), "*")()
	// mGBA without a game: only one savegamePath is possible, so only if GB and GBA share the folder.
	var dir string
	for i, sys := range e.systems(p) {
		if d := e.dir(p, sys, ""); i > 0 && d != dir {
			dir = ""
			break
		} else {
			dir = d
		}
	}
	games, refusal, noRoms := c.gamesOf(e, p)
	if len(refusal) > 0 {
		notify(tr("Non synchronisé : ", "Not synced: ") + strings.Join(refusal, "\n"))
	}
	if len(noRoms) > 0 {
		notify(fmt.Sprintf(tr("%s s'ouvre sans synchronisation, car 3dssavesync ne connaît pas votre dossier de jeux %s. "+
			"Lancez vos jeux depuis votre frontend, ou ajoutez ce dossier dans les réglages.",
			"%s is opening without sync because 3dssavesync doesn't know your %s games folder. "+
				"Start your games from your frontend, or add this folder in the settings."), e.process(), strings.Join(noRoms, ", ")))
	}
	return c.play(games, e.command(p, "", "", dir), hint(e, p))
}

// syncAll runs a Sync of every game of the configured emulators (Sync all: settings page, tray menu). Returns the
// text for the user, problems first (a notification is cut at 255 characters).
func syncAll(config string) string {
	r := syncEmulators(config, nil)
	text := append(r.problems, r.noFolder...)
	if len(r.open) > 0 {
		text = append(text, fmt.Sprintf(tr("Jeux laissés de côté, car l'émulateur est ouvert : %s. Fermez-le, puis recommencez.",
			"Games skipped because the emulator is open: %s. Close it, then try again."), strings.Join(r.open, ", ")))
	}
	if len(text) == 0 && r.uploaded+r.downloaded+r.inSync == 0 {
		text = append(text, tr("Aucun jeu trouvé. Vérifiez les dossiers de jeux dans la partie Émulateurs.",
			"No game found. Check the games folders in the Emulators part."))
	}
	return strings.Join(append(text, r.counts()), "\n")
}

// syncReport is the outcome of syncEmulators. problems: for the user, most important first; open: the programs of the
// emulators skipped because they run (or a Launcher of theirs is in progress). noFolder: the systems without a games
// folder, a setting rather than a failure: Sync all shows them, the automatic pass does not notify them (the games
// started from a frontend sync anyway; the settings page lists them).
type syncReport struct {
	syncCount
	problems, open, noFolder []string
}

func (r *syncReport) counts() string {
	return fmt.Sprintf(tr("Envoyées : %d · Récupérées : %d · Déjà à jour : %d", "Uploaded: %d · Downloaded: %d · Already up to date: %d"),
		r.uploaded, r.downloaded, r.inSync)
}

// syncEmulators runs a Sync of every game of the emulators named in only (nil: all), after the pending Uploads. An
// open emulator is skipped, since its game may still write its save; so is one whose Launcher is in progress. A
// Conflict is not resolved here: the next start of the game asks.
func syncEmulators(config string, only []string) syncReport {
	var r syncReport
	s, err := newSession(config)
	if err != nil {
		r.problems = []string{tr("Synchronisation impossible. ", "Sync failed. ") + errorText(err)}
		return r
	}
	if err := s.resume(); err != nil && end(err) {
		r.problems = []string{tr("Synchronisation impossible. ", "Sync failed. ") + errorText(err) + "\n" + tr("Détail : ", "Detail: ") + err.Error()}
		return r
	}
	var refused, failures []string
emulators:
	for _, name := range slices.Sorted(maps.Keys(s.cfg.Emulators)) {
		if only != nil && !slices.Contains(only, name) {
			continue
		}
		e := s.cfg.Emulators[name]
		p, err := e.profile(name)
		if err != nil {
			failures = append(failures, name+" : "+errorText(err))
			continue
		}
		if instances(e.process()) > 0 || len(s.locks(e.process())) > 0 {
			if !slices.Contains(r.open, e.process()) {
				r.open = append(r.open, e.process())
			}
			continue
		}
		c := s.client(name)
		c.batch = &r.syncCount
		games, refusal, noRoms := c.gamesOf(e, p)
		refused = append(refused, refusal...)
		if len(noRoms) > 0 {
			r.noFolder = append(r.noFolder, fmt.Sprintf(tr("%s : dossier de jeux %s inconnu. Ajoutez-le dans les réglages.",
				"%s: unknown %s games folder. Add it in the settings."), name, strings.Join(noRoms, ", ")))
		}
		for _, j := range games {
			if _, err := c.syncGame(j, true); err != nil {
				c.log(j.Key + " synchro groupée : " + err.Error())
				if failures = addFailure(failures, j, err); end(err) {
					break emulators
				}
			}
		}
	}
	if len(r.conflicts) > 0 {
		r.problems = append(r.problems, fmt.Sprintf(tr("Conflit sur %s : lancez ce jeu pour choisir la version à garder.",
			"Conflict on %s: start this game to choose which version to keep."), strings.Join(r.conflicts, ", ")))
	}
	if len(failures) > 0 {
		r.problems = append(r.problems, tr("Non synchronisé :", "Not synced:"))
		r.problems = append(r.problems, failures...)
	}
	r.problems = append(r.problems, refused...)
	s.log("synchro groupée " + strings.Join(only, ",") + " : " + strings.Join(append(append(r.problems, r.noFolder...), r.counts()), " | "))
	return r
}

// refusal: the emulator already runs. Refuse if its game is unknown (instance opened outside 3dssavesync, or
// started without a game) or if it is this game: a Sync would overwrite or upload a game in progress. Two different
// games started by 3dssavesync can run together (a trade between X and Y). key "*": start without a game (launch).
func (s *Session) refusal(e *Emulator, key, game string) bool {
	name := e.process()
	n := instances(name)
	if n == 0 {
		return false
	}
	en := s.locks(name)
	var text string
	switch {
	case key == "*" || slices.Contains(en, "*"):
		text = name + tr(" est déjà ouvert. Fermez-le avant de lancer un jeu.", " is already open. Close it before starting a game.")
	case n > len(en):
		text = name + tr(" a été ouvert sans passer par 3dssavesync. Fermez-le avant de lancer un jeu.",
			" was opened without 3dssavesync. Close it before starting a game.")
	case key != "" && slices.Contains(en, key):
		text = fmt.Sprintf(tr("%s est déjà ouvert dans %s. Fermez-le avant de le relancer.", "%s is already open in %s. Close it before starting it again."), game, name)
	default:
		return false
	}
	s.log("refus : " + text)
	notify(text)
	return true
}

// locks of the games in progress: one file en-cours/<process>.<pid> per active Launcher, holding its game's Key.
// Returns the Keys of the Launchers still alive for this emulator; it removes the lock of a dead Launcher (crash).
// ponytail: a pid reused by another program keeps the lock; it goes away at the next PC restart.
func (s *Session) locks(process string) (keys []string) {
	files, _ := filepath.Glob(filepath.Join(s.dir, "en-cours", process+".*"))
	for _, f := range files {
		pid, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(f), process+"."))
		if err != nil || !alive(pid) {
			os.Remove(f)
			continue
		}
		if o, err := os.ReadFile(f); err == nil {
			keys = append(keys, string(o))
		}
	}
	return keys
}

// acquireLock sets this Launcher's lock; the returned function removes it (after the Upload).
func (s *Session) acquireLock(process, key string) func() {
	f := filepath.Join(s.dir, "en-cours", fmt.Sprintf("%s.%d", process, os.Getpid()))
	if os.MkdirAll(filepath.Dir(f), 0o755) != nil || os.WriteFile(f, []byte(key), 0o644) != nil {
		s.log("verrou impossible : " + f)
		notify(fmt.Sprintf(tr("Le jeu démarre, mais 3dssavesync ne peut pas écrire dans %s. Ne relancez pas ce jeu tant qu'il est ouvert.",
			"The game is starting, but 3dssavesync can't write to %s. Don't start this game again while it's open."), filepath.Dir(f)))
	}
	return func() { os.Remove(f) }
}

// gamesOf returns an emulator's games (launch, Sync all): the ROMs of its roms folders, and the 3DS games seen by
// Azahar. refusal: why some ROMs are not synced; noRoms: the systems with no roms folder.
func (c *Client) gamesOf(e *Emulator, p Profile) (games []*Game, refusal, noRoms []string) {
	byKey, roms := map[string][]*Game{}, map[*Game]string{} // roms: ROM path of each Game
	for _, system := range e.systems(p) {
		if system == N3DS {
			games = append(games, c.games3DS(e.dir(p, system, ""), e.Roms[system])...)
			continue
		}
		root := e.Roms[system]
		if root == "" {
			c.log("pas de dossier roms pour " + string(system) + " : ses jeux ne sont pas synchronisés")
			noRoms = append(noRoms, string(system))
			continue
		}
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			r, err := readRom(path)
			sys, name := r.System, r.Name
			if err != nil || sys != system {
				return nil
			}
			if key, err := c.cfg.aliasKey(sys, name, r.Key); err != nil {
				refusal = append(refusal, err.Error())
			} else if key != "" {
				if p.InternalName && r.Internal != "" {
					name = r.Internal
				}
				core := library(e.Cores[sys])
				byKey[key] = append(byKey[key], &Game{sys, key, name, savePath(e.dir(p, sys, path), sys, key, name, p.Ext), core, c.cfg.mbc2Size(c.name, core)})
				roms[byKey[key][len(byKey[key])-1]] = path
			}
			return nil
		})
	}
	// Several ROMs for one Key, or for one save file (shared save folder and same name in two subfolders):
	// none is synced, like the Duplicate on the 3DS (ADR 0005, 0014, 0015).
	byFile := map[string]int{}
	for _, l := range byKey {
		for _, j := range l {
			byFile[strings.ToLower(j.File)]++
		}
	}
	var duplicates, shares []string
	for _, l := range byKey {
		switch {
		case len(l) > 1:
			for _, j := range l {
				duplicates = append(duplicates, filepath.Base(roms[j]))
			}
		case byFile[strings.ToLower(l[0].File)] > 1:
			shares = append(shares, filepath.Base(roms[l[0]]))
		default:
			if why := c.romRefusal(l[0], roms[l[0]], false); why != "" {
				refusal = append(refusal, why)
			} else {
				games = append(games, l[0])
			}
		}
	}
	if len(duplicates) > 0 {
		sort.Strings(duplicates)
		c.log("Doublons : " + strings.Join(duplicates, ", "))
		refusal = append(refusal, fmt.Sprintf(tr("ces ROM contiennent le même jeu : %s. Gardez-en une seule, ou ajoutez les autres dans les réglages, section ROM hacks.",
			"these ROMs contain the same game: %s. Keep only one, or add the others in the settings, ROM hacks section."), strings.Join(duplicates, ", ")))
	}
	if len(shares) > 0 {
		sort.Strings(shares)
		c.log("même fichier de sauvegarde : " + strings.Join(shares, ", "))
		refusal = append(refusal, fmt.Sprintf(tr("ces ROM utiliseraient le même fichier de sauvegarde : %s. Renommez-en une.",
			"these ROMs would use the same save file: %s. Rename one of them."), strings.Join(shares, ", ")))
	}
	sort.Slice(games, func(a, b int) bool { return games[a].Name < games[b].Name })
	return games, refusal, noRoms
}

// games3DS returns the 3DS games of an Azahar sdmc folder (ADR 0028): the 8 Pokémon games (a never-started one too,
// its folder is created with a known format), and every other application that has a save folder, except the ignored
// and turned-off ones; then the extdata (extdata3DS). Name: titres_3ds, else the ROM in the roms folder (if set),
// else the title ID.
func (c *Client) games3DS(sdmc, roms string) []*Game {
	if sdmc == "" {
		return nil
	}
	names, ids := map[string]string{}, map[string]bool{} // names: title ID -> ROM name
	if roms != "" {
		filepath.WalkDir(roms, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if r, err := readRom(path); err == nil && r.System == N3DS && r.Key != "" {
					names[r.Key[strings.IndexByte(r.Key, '-')+1:]] = r.Name
				}
			}
			return nil
		})
	}
	for id := range titles3DS {
		ids[id] = true
	}
	z := strings.Repeat("0", 32)
	lows, _ := os.ReadDir(filepath.Join(sdmc, "Nintendo 3DS", z, z, "title", "00040000"))
	for _, d := range lows {
		if d.IsDir() {
			ids["00040000"+strings.ToUpper(d.Name())] = true
		}
	}
	var games []*Game
	for id := range ids {
		key := key3DS(id)
		if !keyShapes[N3DS].MatchString(key) || c.cfg.off3DS(key) {
			continue
		}
		name := cmp.Or(titles3DS[id], names3DS[id], names[id], id)
		j := &Game{System: N3DS, Key: key, Name: name, File: savePath(sdmc, N3DS, key, name, "")}
		if exists(savePath(sdmc, N3DS, "3DSA-"+id, "", "")) || formats3DS[id] != nil { // never started, unknown format: skipped
			games = append(games, j)
		}
	}
	// Name of an extdata: its known game whose titleLow >> 8 is the ID, the smallest title ID if several.
	for id := range names3DS {
		ids[id] = true
	}
	for id := range names {
		ids[id] = true
	}
	extNames := map[string]string{}
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		if n := cmp.Or(titles3DS[id], names3DS[id], names[id]); n != "" && extNames[extdataID(id)] == "" {
			extNames[extdataID(id)] = n
		}
	}
	return append(games, c.extdata3DS(sdmc, extNames)...)
}

// extdata3DS returns the extdata of an Azahar sdmc folder (ADR 0031): every folder extdata/00000000/<ID>/ that has a
// user/ subfolder, outside ignoredExtdata and the turned-off games (off3DS). Its Local save is the archive of user/.
// Name: the name of its game (names: extdata ID -> game name) + " (data)", else "Extdata <ID>".
func (c *Client) extdata3DS(sdmc string, names map[string]string) []*Game {
	z := strings.Repeat("0", 32)
	root := filepath.Join(sdmc, "Nintendo 3DS", z, z, "extdata", "00000000")
	dirs, _ := os.ReadDir(root)
	var games []*Game
	for _, d := range dirs {
		id := strings.ToUpper(d.Name())
		key, user := "3DSX-"+id, filepath.Join(root, d.Name(), "user")
		if i, err := os.Stat(user); err != nil || !i.IsDir() || !keyShapes[N3DS].MatchString(key) || ignoredExtdata[id] || c.cfg.off3DS(key) {
			continue
		}
		name := "Extdata " + id
		if n := names[id]; n != "" {
			name = n + tr(" (données)", " (data)")
		}
		games = append(games, &Game{System: N3DS, Key: key, Name: name, File: user})
	}
	return games
}

// play runs the pending Uploads, the Sync before, the game, the Sync after. One read of the server list per pass.
func (c *Client) play(games []*Game, command []string, hint string) (int, error) {
	c.resume() // same pass as the pre-game Sync: network down = a single timeout
	var failures []string
	for _, j := range games {
		cancelled, err := c.syncGame(j, false)
		if cancelled {
			return 0, nil
		}
		if err != nil {
			c.log(j.Key + " avant : " + err.Error())
			if failures = addFailure(failures, j, err); end(err) {
				break
			}
		}
	}
	if len(failures) > 0 {
		no := tr("Non", "No")
		r := ask(tr("Synchronisation impossible", "Sync failed")+"\n\n"+strings.Join(failures, "\n")+"\n"+tr("Aide : ", "Help: ")+helpPage+"\n\n"+
			tr("Jouer avec la sauvegarde du PC ? Elle sera envoyée au prochain lancement.", "Play with the PC save? It'll be uploaded the next time you start a game."), []string{tr("Oui", "Yes"), no})
		if r == no {
			return 0, nil
		}
	}
	if hint != "" && c.downloaded {
		noPlay := tr("Ne pas jouer", "Don't play")
		r := ask(tr("Une sauvegarde vient d'arriver du serveur.", "A save just arrived from the server.")+"\n\n"+hint+"\n\n"+
			tr("Sinon, en sauvegardant dans le jeu, vous risquez d'écraser cette sauvegarde avec une partie plus ancienne.", "Otherwise, saving in the game could overwrite it with older progress."),
			[]string{tr("Jouer quand même", "Play anyway"), noPlay})
		if r == noPlay {
			return 0, nil
		}
		if r == "" { // no window: the game runs, the warning stays visible
			notify(hint)
		}
	} else if hint != "" {
		c.log("avertissement : " + hint)
	}
	// A game with no save before playing: if it still has none after, the folder or the extension may be
	// wrong (custom Profile). One game only (run): launch would have dozens of never-played games.
	noSave := len(games) == 1 && games[0].System != "3DS" && !exists(games[0].File)
	if err := launch(command); err != nil {
		return 1, fmt.Errorf(tr("impossible de lancer %s : %w", "couldn't start %s: %w"), command[0], err)
	}
	if noSave && !exists(games[0].File) {
		c.log("aucune sauvegarde après la partie : " + games[0].File)
		notify(fmt.Sprintf(tr("Aucune sauvegarde trouvée après la partie : %s\nSi vous avez sauvegardé dans le jeu, vérifiez le dossier des sauvegardes et l'extension réglés pour %s.",
			"No save found after playing: %s\nIf you saved in the game, check the save folder and extension set for %s."), games[0].File, c.name))
	}
	failures = nil
	c.listRead = false
	var notUploaded []*Game // retried at the next game start, whatever the error
	for i, j := range games {
		if _, err := c.syncGame(j, true); err != nil {
			c.log(j.Key + " après : " + err.Error())
			if failures = addFailure(failures, j, err); end(err) {
				notUploaded = append(notUploaded, games[i:]...)
				break
			}
			notUploaded = append(notUploaded, j)
		}
	}
	if len(failures) > 0 { // most important first: the notification is cut at 255 characters
		text := tr("Sauvegarde non envoyée. Elle reste sur le PC et sera envoyée au prochain lancement d'un jeu.\n", "Upload failed. The save stays on the PC and will upload the next time you start a game.\n")
		if c.addPending(notUploaded) != nil {
			text = tr("Sauvegarde non envoyée. Elle reste sur le PC : relancez ce jeu pour l'envoyer.\n", "Upload failed. The save stays on the PC. Start this game again to upload it.\n")
		}
		notify(text + strings.Join(failures, "\n"))
	}
	return 0, nil
}

// addFailure adds a failure line for the user: the game name, except when the server or the network is the cause
// (one message for the whole pass).
func addFailure(failures []string, j *Game, err error) []string {
	text, global := classify(err)
	if !global {
		text = j.Name + " : " + text
	}
	return append(failures, text)
}

// end: the server or the network is the cause, so the next games would fail the same way (each after a timeout).
func end(err error) bool {
	_, global := classify(err)
	return global
}

func readJSON(path string, v any) error {
	o, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes.TrimPrefix(o, []byte("\xef\xbb\xbf")), v); err != nil {
		return fmt.Errorf("%s : %w", path, err)
	}
	return nil
}
