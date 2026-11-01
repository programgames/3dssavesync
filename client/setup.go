package main

// Assistant d'installation (phase 4) : serveur et jeton, copie de l'exe à un emplacement fixe, émulateurs,
// frontends. Texte dans une console (ouverte par l'exe sous Windows). Rien n'est écrit sans copie préalable.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

func (q *Questions) dire(format string, a ...any) { fmt.Fprintf(q.out, format+"\n", a...) }

func (q *Questions) ligne() string {
	l, _ := q.in.ReadString('\n')
	return strings.TrimSpace(l)
}

// texte : réponse, ou defaut si Entrée.
func (q *Questions) texte(question, defaut string) string {
	if defaut != "" {
		fmt.Fprintf(q.out, "%s [%s] : ", question, defaut)
	} else {
		fmt.Fprintf(q.out, "%s : ", question)
	}
	if r := q.ligne(); r != "" {
		return strings.Trim(r, `"`)
	}
	return defaut
}

func (q *Questions) oui(question string, defaut bool) bool {
	choix := tr("[O/n]", "[Y/n]")
	if !defaut {
		choix = tr("[o/N]", "[y/N]")
	}
	fmt.Fprintf(q.out, "%s %s ", question, choix)
	switch strings.ToLower(q.ligne()) {
	case "":
		return defaut
	case "o", "oui", "y", "yes":
		return true
	}
	return false
}

func (q *Questions) secret(question string) string {
	fmt.Fprintf(q.out, "%s : ", question)
	if q.terminal {
		defer sansEcho()()
		defer fmt.Fprintln(q.out)
	}
	return q.ligne()
}

// Emplacement fixe de l'exe : les frontends et les raccourcis pointent vers lui.
func emplacementInstalle() string {
	switch runtime.GOOS {
	case "windows":
		return env("LOCALAPPDATA", "Programs", "3dssavesync", "3dssavesync.exe")
	case "darwin":
		return maison("Applications", "3dssavesync", "3dssavesync")
	}
	return maison(".local", "bin", "3dssavesync")
}

// Exe en cours ; vide en test (le binaire de test n'est pas le Lanceur).
var executable = func() string { e, _ := os.Executable(); return e }

func commandeSetup(config string) (int, error) {
	ouvrirConsole()
	q := &Questions{in: bufio.NewReader(os.Stdin), out: os.Stdout, terminal: true}
	err := assistant(q, config)
	if err != nil {
		q.dire("\n%s%s", tr("Erreur : ", "Error: "), err)
	}
	if consolePropre {
		q.dire("\n%s", tr("Appuie sur Entrée pour fermer.", "Press Enter to close."))
		q.ligne()
	}
	if err != nil {
		return 1, nil // déjà affichée dans la console
	}
	return 0, nil
}

func commandeRepair(config string) (int, error) {
	ouvrirConsole()
	q := &Questions{in: bufio.NewReader(os.Stdin), out: os.Stdout}
	var cfg Config
	if err := lireJSON(config, &cfg); err != nil {
		return 1, err
	}
	for _, l := range reparer(&cfg, lanceurDe(&cfg)) {
		q.dire("- %s", l)
	}
	if consolePropre {
		q.dire("\n%s", tr("Appuie sur Entrée pour fermer.", "Press Enter to close."))
		q.ligne()
	}
	return 0, nil
}

// Exe vers lequel pointent les frontends : celui noté à l'installation, sinon l'exe en cours.
func lanceurDe(cfg *Config) string {
	if cfg.Lanceur != "" {
		return cfg.Lanceur
	}
	return executable()
}

func assistant(q *Questions, config string) error {
	q.dire("3dssavesync - %s\n", tr("assistant d'installation", "setup assistant"))
	var cfg Config
	trouvee := false
	if err := lireJSON(config, &cfg); err == nil {
		trouvee = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if cfg.Emulateurs == nil {
		cfg.Emulateurs = map[string]*Emulateur{}
	}
	etapes := []string{"serveur", "installation", "emulateurs", "frontends"}
	if trouvee {
		q.dire("%s %s", tr("Configuration trouvée :", "Configuration found:"), config)
		q.dire("  1. %s", tr("Ajouter ou modifier un émulateur", "Add or change an emulator"))
		q.dire("  2. %s", tr("Réparer la configuration des frontends", "Repair the frontend configuration"))
		q.dire("  3. %s", tr("Changer le serveur ou le jeton", "Change the server or the token"))
		q.dire("  4. %s", tr("Tout refaire (installation complète)", "Redo everything (full setup)"))
		q.dire("  5. %s", tr("Quitter", "Quit"))
		switch q.texte(tr("Choix", "Choice"), "1") {
		case "1":
			etapes = []string{"emulateurs", "frontends"}
		case "2":
			for _, l := range reparer(&cfg, lanceurDe(&cfg)) {
				q.dire("- %s", l)
			}
			return nil
		case "3":
			etapes = []string{"serveur"}
		case "4":
		default:
			return nil
		}
	}
	for _, e := range etapes {
		var err error
		switch e {
		case "serveur":
			err = etapeServeur(q, &cfg)
		case "installation":
			etapeInstallation(q, &cfg)
		case "emulateurs":
			etapeEmulateurs(q, &cfg)
		case "frontends":
			err = enregistrerConfig(q, config, &cfg)
			if err == nil {
				etapeFrontends(q, &cfg)
			}
		}
		if err != nil {
			return err
		}
	}
	if err := enregistrerConfig(q, config, &cfg); err != nil {
		return err
	}
	q.dire("\n%s", tr("Terminé.", "Done."))
	return nil
}

// config.json : copie datée de l'ancien, puis écriture vérifiée. Rien si rien n'a changé.
func enregistrerConfig(q *Questions, chemin string, cfg *Config) error {
	var avant Config
	if lireJSON(chemin, &avant) == nil && jsonDe(&avant) == jsonDe(cfg) {
		return nil
	}
	if err := copieAvant(chemin); err != nil {
		return err
	}
	if err := ecrireJSON(chemin, cfg, true); err != nil {
		return err
	}
	q.dire("%s %s", tr("Configuration écrite :", "Configuration written:"), chemin)
	return nil
}

func jsonDe(c *Config) string { o, _ := json.Marshal(c); return string(o) }

func etapeServeur(q *Questions, cfg *Config) error {
	q.dire("\n== %s ==", tr("Serveur", "Server"))
	for {
		cfg.URL = strings.TrimRight(q.texte(tr("Adresse du serveur (https://…)", "Server address (https://…)"), cfg.URL), "/")
		question := tr("Jeton (la saisie reste cachée)", "Token (input stays hidden)")
		if cfg.Jeton != "" {
			question = tr("Jeton (Entrée = garder l'actuel)", "Token (Enter = keep the current one)")
		}
		if j := q.secret(question); j != "" {
			cfg.Jeton = j
		}
		err := testerServeur(cfg)
		if err == nil {
			q.dire("%s", tr("Connexion réussie.", "Connection OK."))
			return nil
		}
		q.dire("%s %s", tr("Connexion impossible :", "Cannot connect:"), err)
		if !q.oui(tr("Réessayer ?", "Try again?"), true) {
			return fmt.Errorf("%s", tr("serveur injoignable : rien n'est écrit", "server unreachable: nothing is written"))
		}
	}
}

func testerServeur(cfg *Config) error {
	s := &Session{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second, Transport: transport}, profil: "setup"}
	st, _, err := s.api("GET", "/saves", nil, nil)
	if err == nil && st != 200 {
		err = fmt.Errorf("GET /saves : %d", st)
	}
	return err
}

// Copie de l'exe à l'emplacement fixe (un exe téléchargé reste où il est sinon).
func etapeInstallation(q *Questions, cfg *Config) {
	q.dire("\n== %s ==", tr("Installation", "Install"))
	courant, cible := executable(), emplacementInstalle()
	cfg.Lanceur = courant
	if courant == "" || strings.EqualFold(filepath.Clean(courant), filepath.Clean(cible)) {
		q.dire("%s %s", tr("Exe en place :", "Exe in place:"), courant)
		return
	}
	if !q.oui(fmt.Sprintf(tr("Copier le programme dans %s ?", "Copy the program to %s?"), cible), true) {
		return
	}
	o, err := os.ReadFile(courant)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(cible), 0o755)
	}
	if err == nil {
		os.Remove(cible + ".3dssavesync-tmp")
		if err = ecrireVerifie(cible+".3dssavesync-tmp", o); err == nil {
			os.Chmod(cible+".3dssavesync-tmp", 0o755)
			err = os.Rename(cible+".3dssavesync-tmp", cible)
		}
	}
	if err != nil {
		q.dire("%s %s", tr("Copie impossible (le programme tourne peut-être) :", "Copy failed (the program may be running):"), err)
		return
	}
	cfg.Lanceur = cible
	q.dire("%s %s", tr("Programme copié :", "Program copied:"), cible)
}

// --- Émulateurs ----------------------------------------------------------------------------------

// Trouve : un émulateur détecté sur la machine.
type Trouve struct {
	Profil, Exe, Processus, Source string
}

// Noms connus par profil : exécutables, dossiers usuels, Flatpak, application macOS, AppImage (EmuDeck).
var connus = map[string]struct {
	exes, dossiers []string
	flatpak, app   string
	appimage       []string
	processus      string
}{
	"melonds":   {[]string{"melonDS"}, []string{"melonDS"}, "net.kuribo64.melonDS", "melonDS", []string{"melonDS*.AppImage"}, "melonDS"},
	"mgba":      {[]string{"mGBA", "mgba-qt"}, []string{"mGBA"}, "io.mgba.mGBA", "mGBA", []string{"mGBA*.AppImage"}, "mgba-qt"},
	"azahar":    {[]string{"azahar"}, []string{"Azahar", "azahar"}, "org.azahar_emu.Azahar", "Azahar", []string{"azahar*.AppImage", "Azahar*.AppImage"}, "azahar"},
	"retroarch": {[]string{"retroarch"}, []string{"RetroArch", "RetroArch-Win64"}, "org.libretro.RetroArch", "RetroArch", []string{"RetroArch*.AppImage"}, "retroarch"},
	"vbam":      {[]string{"visualboyadvance-m"}, []string{"VisualBoyAdvance-M", "visualboyadvance-m"}, "", "visualboyadvance-m", nil, "visualboyadvance-m"},
	"sameboy":   {[]string{"sameboy"}, []string{"SameBoy"}, "io.github.sameboy.SameBoy", "SameBoy", nil, "sameboy"},
}

// Détection : PATH, dossiers usuels, Flatpak, AppImage, applications macOS, émulateurs de LaunchBox.
var detecter = func(dossierLB string) []Trouve {
	var t []Trouve
	vus := map[string]bool{}
	ajouter := func(profil, exe, processus, source string) {
		if exe == "" || !existe(exe) || vus[strings.ToLower(exe)] {
			return
		}
		vus[strings.ToLower(exe)] = true
		t = append(t, Trouve{profil, exe, processus, source})
	}
	var profilsTries []string
	for p := range connus {
		profilsTries = append(profilsTries, p)
	}
	sort.Strings(profilsTries)
	for _, p := range profilsTries {
		c := connus[p]
		for _, n := range c.exes {
			if e, err := exec.LookPath(n); err == nil {
				ajouter(p, e, "", "PATH")
			}
			for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), env("LOCALAPPDATA", "Programs"), maison("scoop", "apps")} {
				for _, d := range c.dossiers {
					ajouter(p, rejoindre(base, d, n+".exe"), "", base)
					ajouter(p, rejoindre(base, strings.ToLower(d), "current", n+".exe"), "", "scoop")
				}
			}
		}
		if c.flatpak != "" {
			ajouter(p, filepath.Join("/var/lib/flatpak/exports/bin", c.flatpak), c.processus, "Flatpak")
			ajouter(p, maison(".local", "share", "flatpak", "exports", "bin", c.flatpak), c.processus, "Flatpak")
		}
		for _, motif := range c.appimage {
			m, _ := filepath.Glob(maison("Applications", motif))
			for _, f := range m {
				ajouter(p, f, c.processus, "AppImage")
			}
		}
		for _, racine := range []string{"/Applications", maison("Applications")} {
			ajouter(p, rejoindre(racine, c.app+".app", "Contents", "MacOS", c.app), "", "Applications")
		}
	}
	if dossierLB != "" {
		o, _ := os.ReadFile(filepath.Join(dossierLB, "Data", "Emulators.xml"))
		for _, b := range blocEmulateurLB.FindAllString(string(o), -1) {
			app := champ(b, "ApplicationPath")
			if app != "" && !filepath.IsAbs(app) {
				app = filepath.Join(dossierLB, app)
			}
			nom := strings.TrimSuffix(strings.ToLower(filepath.Base(app)), ".exe")
			for _, p := range profilsTries {
				for _, n := range connus[p].exes {
					if nom == strings.ToLower(n) {
						ajouter(p, filepath.Clean(app), "", "LaunchBox")
					}
				}
			}
		}
	}
	return t
}

// Cœurs RetroArch trouvés près de l'exe ou dans le dossier de configuration : Système -> cœur préféré.
func coeursRetroArch(exe string) map[string]string {
	var dossiers []string
	if exe != "" {
		dossiers = append(dossiers, filepath.Join(filepath.Dir(exe), "cores"))
	}
	if f := configRetroArch(&Emulateur{Exe: exe}); f != "" {
		dossiers = append(dossiers, filepath.Join(filepath.Dir(f), "cores"))
	}
	dossiers = append(dossiers, maison("Library", "Application Support", "RetroArch", "cores"))
	trouves := map[string]string{}
	for _, d := range dossiers {
		m, _ := filepath.Glob(filepath.Join(d, "*_libretro.*"))
		for _, f := range m {
			if b := bibliotheque(f); b != "" && trouves[b] == "" {
				trouves[b] = f
			}
		}
	}
	preferes := map[string][]string{"DS": {"melonDS DS"}, "GB": {"Gambatte", "SameBoy", "mGBA", "VBA-M"}, "GBA": {"mGBA", "VBA-M"}}
	coeurs := map[string]string{}
	for s, l := range preferes {
		for _, b := range l {
			if f := trouves[b]; f != "" {
				coeurs[s] = f
				break
			}
		}
	}
	return coeurs
}

func etapeEmulateurs(q *Questions, cfg *Config) {
	q.dire("\n== %s ==", tr("Émulateurs", "Emulators"))
	dossierLB := cfg.Frontends["launchbox"]
	if dossierLB == "" && runtime.GOOS == "windows" {
		dossierLB = trouverLaunchBox()
	}
	trouves := detecter(dossierLB)
	if len(trouves) == 0 {
		q.dire("%s", tr("Aucun émulateur trouvé automatiquement.", "No emulator found automatically."))
	}
	var configures []string
	for n := range cfg.Emulateurs {
		configures = append(configures, n)
	}
	sort.Strings(configures)
	for _, n := range configures {
		e := cfg.Emulateurs[n]
		if q.oui(fmt.Sprintf(tr("%s est configuré (%s). Modifier ses dossiers ?", "%s is set up (%s). Change its folders?"), n, e.Exe), false) {
			ajouterEmulateur(q, cfg, Trouve{Profil: n, Exe: e.Exe, Processus: e.Processus}, false)
		}
	}
	for _, t := range trouves {
		a := cfg.Emulateurs[t.Profil]
		switch {
		case a != nil && strings.EqualFold(a.Exe, t.Exe):
		case a != nil:
			if q.oui(fmt.Sprintf(tr("Autre installation de %s trouvée (%s) : %s. L'utiliser à la place de %s ?", "Another %s install found (%s): %s. Use it instead of %s?"),
				t.Profil, t.Source, t.Exe, a.Exe), false) {
				ajouterEmulateur(q, cfg, t, true)
			}
		case q.oui(fmt.Sprintf(tr("%s trouvé (%s) : %s. L'utiliser ?", "%s found (%s): %s. Use it?"), t.Profil, t.Source, t.Exe), true):
			ajouterEmulateur(q, cfg, t, false)
		}
	}
	noms := make([]string, 0, len(profils))
	for p := range profils {
		noms = append(noms, p)
	}
	sort.Strings(noms)
	for q.oui(tr("Ajouter un émulateur à la main ?", "Add an emulator by hand?"), false) {
		for i, p := range noms {
			q.dire("  %d. %s", i+1, p)
		}
		i, err := strconv.Atoi(q.texte(tr("Numéro du profil", "Profile number"), ""))
		if err != nil || i < 1 || i > len(noms) {
			continue
		}
		if exe := q.texte(tr("Chemin de l'exécutable", "Executable path"), ""); exe != "" {
			ajouterEmulateur(q, cfg, Trouve{Profil: noms[i-1], Exe: exe, Source: "manuel"}, false)
		}
	}
}

// remplacer : l'utilisateur a déjà accepté de remplacer l'émulateur du même nom.
func ajouterEmulateur(q *Questions, cfg *Config, t Trouve, remplacer bool) {
	nom := t.Profil
	if ancien := cfg.Emulateurs[nom]; ancien != nil && !remplacer && !strings.EqualFold(ancien.Exe, t.Exe) {
		if !q.oui(fmt.Sprintf(tr("%s existe déjà (%s). Le remplacer ?", "%s already exists (%s). Replace it?"), nom, ancien.Exe), false) {
			return
		}
	}
	e := &Emulateur{Exe: t.Exe, Processus: t.Processus}
	if ancien := cfg.Emulateurs[nom]; ancien != nil && strings.EqualFold(ancien.Exe, t.Exe) {
		e = ancien // mêmes réglages : on les garde (dossiers, roms)
	}
	if nom == "retroarch" && len(e.Coeurs) == 0 {
		e.Coeurs = coeursRetroArch(t.Exe)
		if len(e.Coeurs) == 0 {
			q.dire("%s", tr("Aucun cœur géré trouvé (melonDS DS, mGBA, Gambatte, SameBoy, VBA-M) : installe-les dans RetroArch, puis relance l'assistant.",
				"No supported core found (melonDS DS, mGBA, Gambatte, SameBoy, VBA-M): install them in RetroArch, then run the assistant again."))
			return
		}
	}
	p, err := e.profil(nom)
	if err != nil {
		q.dire("%s", err)
		return
	}
	for _, s := range e.systemes(p) {
		d := e.dossier(p, s, "")
		affiche := d
		if d == "" {
			affiche = tr("à côté de chaque ROM", "next to each ROM")
		}
		if s == "3DS" && d == "" {
			affiche = ""
		}
		if r := q.texte(fmt.Sprintf(tr("Sauvegardes %s", "%s saves"), s), affiche); r != affiche && r != "" {
			if e.Sauvegardes == nil {
				e.Sauvegardes = map[string]string{}
			}
			e.Sauvegardes[s] = r
		}
		if s != "3DS" {
			if r := q.texte(fmt.Sprintf(tr("Dossier des ROM %s pour la commande launch (Entrée = aucun)", "%s ROM folder for the launch command (Enter = none)"), s), e.Roms[s]); r != "" {
				if e.Roms == nil {
					e.Roms = map[string]string{}
				}
				e.Roms[s] = r
			}
		}
	}
	if conseil := conseil(e, p); conseil != "" {
		q.dire("%s %s", tr("Attention :", "Warning:"), conseil)
	}
	cfg.Emulateurs[nom] = e
	q.dire("%s %s", tr("Émulateur enregistré :", "Emulator saved:"), nom)
}

// --- Frontends -----------------------------------------------------------------------------------

func trouverLaunchBox() string {
	for _, d := range []string{maison("LaunchBox"), `C:\LaunchBox`, `D:\LaunchBox`, `E:\LaunchBox`, env("ProgramFiles", "LaunchBox")} {
		if existe(rejoindre(d, "Data", "Emulators.xml")) {
			return d
		}
	}
	return ""
}

func etapeFrontends(q *Questions, cfg *Config) {
	q.dire("\n== %s ==", tr("Frontends", "Frontends"))
	if cfg.Frontends == nil {
		cfg.Frontends = map[string]string{}
	}
	if runtime.GOOS == "windows" {
		d := cfg.Frontends["launchbox"]
		if d == "" {
			d = trouverLaunchBox()
		}
		d = q.texte(tr("Dossier de LaunchBox (Entrée = pas de LaunchBox)", "LaunchBox folder (Enter = no LaunchBox)"), d)
		if d != "" && existe(filepath.Join(d, "Data", "Emulators.xml")) {
			cfg.Frontends["launchbox"] = d
		} else if d != "" {
			q.dire("%s", tr("Emulators.xml introuvable dans ce dossier : LaunchBox est ignoré.", "Emulators.xml not found in this folder: LaunchBox is skipped."))
		}
	}
	esde := cfg.Frontends["esde"]
	if esde == "" && existe(maison("ES-DE")) {
		esde = maison("ES-DE")
	}
	if esde != "" && q.oui(fmt.Sprintf(tr("Configurer ES-DE (%s) ?", "Set up ES-DE (%s)?"), esde), true) {
		cfg.Frontends["esde"] = esde
	}
	for {
		lb, es := cfg.Frontends["launchbox"] != "" && (emulateurActif("LaunchBox") || emulateurActif("BigBox")), cfg.Frontends["esde"] != "" && emulateurActif("ES-DE")
		if !lb && !es {
			break
		}
		q.texte(tr("Ferme LaunchBox, BigBox et ES-DE, puis appuie sur Entrée", "Close LaunchBox, BigBox and ES-DE, then press Enter"), "")
	}
	for _, l := range reparer(cfg, lanceurDe(cfg)) {
		q.dire("- %s", l)
	}
	if !q.oui(tr("Créer un raccourci par émulateur (synchronise tout, lance l'émulateur) ?", "Create one shortcut per emulator (syncs everything, starts the emulator)?"), true) {
		return
	}
	lanceur := lanceurDe(cfg)
	var noms []string
	for n := range cfg.Emulateurs {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	for _, n := range noms {
		f, err := raccourci("3dssavesync "+n, lanceur, []string{"launch", n}, false)
		afficherRaccourci(q, f, err)
	}
	f, err := raccourci(tr("3dssavesync réglages", "3dssavesync settings"), lanceur, []string{"setup"}, true)
	afficherRaccourci(q, f, err)
	var sansRoms []string
	for _, n := range noms {
		e := cfg.Emulateurs[n]
		if p, err := e.profil(n); err == nil && len(e.Roms) == 0 && !slices.Equal(e.systemes(p), []string{"3DS"}) {
			sansRoms = append(sansRoms, n)
		}
	}
	if len(sansRoms) > 0 {
		q.dire(tr("Note : %s n'ont pas de dossier des ROM : leur raccourci lance l'émulateur sans rien synchroniser. "+
			"Pour en ajouter un : menu 1, « Modifier ses dossiers ».",
			"Note: %s have no ROM folder: their shortcut starts the emulator without syncing anything. "+
				"To add one: menu 1, \"Change its folders\"."), strings.Join(sansRoms, ", "))
	}
}

func afficherRaccourci(q *Questions, f string, err error) {
	if err != nil {
		q.dire("- %s %s", tr("Raccourci impossible :", "Shortcut failed:"), err)
		return
	}
	q.dire("- %s %s", tr("Raccourci :", "Shortcut:"), f)
}
