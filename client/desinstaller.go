package main

// Désinstallation (ADR 0019) : défait ce que l'installation a fait hors des dossiers de 3dssavesync, pour que LaunchBox
// et ES-DE lancent de nouveau leurs jeux sans lui. Rien n'est effacé définitivement : les fichiers des frontends sont
// copiés avant (remplacerFichier), les raccourcis et le programme vont à la Corbeille (Windows). Les réglages, les copies
// de sécurité et les données du serveur local restent : ce sont les données de l'utilisateur. Version portable
// (ADR 0020) : le programme et son dossier restent, le rapport dit de supprimer ce dossier.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	esdeRule    = regexp.MustCompile(`(?s)  <emulator name="3DSSAVESYNC">.*?</emulator>\r?\n`)
	esdeCommand = regexp.MustCompile(`<command label="3dssavesync \([^"]*\)">[^<]*</command>\r?\n[ \t]*`)
	esdeSystem  = regexp.MustCompile(`(?s)[ \t]*<system>.*?</system>\r?\n?`)
	// Ligne de commande LaunchBox d'un émulateur qui passe par le Lanceur : « run --emulator <nom> ».
	emulatorOption = regexp.MustCompile(`--emulator (\S+)`)
)

// Adaptateurs remplacés par les tests (PowerShell, Corbeille, processus détaché).
var removeShortcuts, removeProgram = removeShortcutsSystem, removeProgramSystem

// defaireLaunchbox : retire chaque copie « <titre> (3dssavesync) » qui lance le Lanceur, et ses plateformes ;
// l'émulateur dont elle est la copie (même titre sans « (3dssavesync) ») redevient l'émulateur par défaut de ces
// plateformes. Un émulateur qui lance le Lanceur sous son titre d'origine a été modifié sur place (assistant des
// versions 1 et 2) : il reste avec ses plateformes, et retrouve l'exe réel de config.json (nom lu dans
// « --emulator <nom> ») ; exe inconnu ou absent : laissé tel quel, le rapport dit de le corriger à la main. Une
// plateforme où cet émulateur est par défaut le reste : l'émulateur d'origine de la copie n'y redevient pas par défaut.
// Deux émulateurs encore par défaut d'une même plateforme : le rapport dit d'en garder un.
func undoLaunchbox(text, lbDir, launcher string, cfg *Config) (string, []string) {
	var report []string
	inPlace := map[string]bool{} // ID des émulateurs modifiés sur place
	unescape := unescapeXML
	ours := func(b string) bool {
		return strings.EqualFold(path.Base(strings.ReplaceAll(unescape(field(b, "ApplicationPath")), `\`, "/")), filepath.Base(launcher))
	}
	cp := func(b string) bool { return strings.HasSuffix(field(b, "Title"), " (3dssavesync)") }
	for _, b := range lbEmulatorBlock.FindAllString(text, -1) {
		if !ours(b) || cp(b) {
			continue
		}
		title, name := unescape(field(b, "Title")), ""
		inPlace[field(b, "ID")] = true
		blocks := []string{b}
		for _, p := range lbPlatformBlock.FindAllString(text, -1) {
			if field(p, "Emulator") == field(b, "ID") {
				blocks = append(blocks, p)
			}
		}
		for _, x := range blocks {
			if m := emulatorOption.FindStringSubmatch(unescape(field(x, "CommandLine"))); m != nil && name == "" {
				name = m[1]
			}
		}
		e := cfg.Emulators[name]
		if e == nil || !exists(e.Exe) {
			report = append(report, fmt.Sprintf(tr("émulateur « %[1]s » laissé tel quel : il lance encore 3dssavesync et son vrai exe est inconnu. Corrigez-le à la main dans LaunchBox : Tools, Manage, Emulators, « %[1]s » : Application Path",
				"emulator \"%[1]s\" left as is: it still starts 3dssavesync and its real exe is unknown. Fix it by hand in LaunchBox: Tools, Manage, Emulators, \"%[1]s\": Application Path"), title))
			continue
		}
		app := e.Exe // relatif sous le dossier de LaunchBox, comme LaunchBox l'écrit
		if r, err := filepath.Rel(lbDir, e.Exe); err == nil && !strings.HasPrefix(r, "..") {
			app = r
		}
		profile := e.Profile
		if profile == "" {
			profile = name
		}
		for i, x := range blocks {
			n := x
			if i == 0 {
				n = setField(n, "ApplicationPath", app)
			}
			if emulatorOption.MatchString(field(n, "CommandLine")) {
				line := ""
				if i > 0 && profile == "retroarch" { // le -L du cœur, que « run --emulator » avait remplacé
					platform := unescape(field(n, "Platform"))
					if c := e.Cores[lbPlatforms[platform]]; c != "" {
						if r, err := filepath.Rel(filepath.Dir(e.Exe), c); err == nil && !strings.HasPrefix(r, "..") {
							c = r // relatif au dossier de RetroArch, comme LaunchBox l'écrit
						}
						line = `-L "` + c + `"`
					} else {
						report = append(report, fmt.Sprintf(tr("« %s » : cœur RetroArch inconnu. Choisissez de nouveau le cœur de « %s » dans LaunchBox : Tools, Manage, Emulators, « %s ».",
							"\"%s\": unknown RetroArch core. Choose the core for \"%s\" again in LaunchBox: Tools, Manage, Emulators, \"%s\"."), title, platform, title))
					}
				}
				n = setField(n, "CommandLine", line)
			}
			text = strings.Replace(text, x, n, 1)
		}
		report = append(report, fmt.Sprintf(tr("émulateur « %s » de nouveau sur %s", "emulator \"%s\" back to %s"), title, e.Exe))
	}
	emus := lbEmulatorBlock.FindAllString(text, -1)
	for _, b := range emus {
		if !ours(b) || !cp(b) {
			continue
		}
		id, title := field(b, "ID"), strings.TrimSuffix(field(b, "Title"), " (3dssavesync)")
		idOrig := ""
		for _, o := range emus {
			if !ours(o) && unescape(field(o, "Title")) == unescape(title) { // &#39; (echapperXML) contre ' (LaunchBox)
				idOrig = field(o, "ID")
			}
		}
		restored, kept := false, false // origine de nouveau par défaut ; émulateur modifié sur place resté par défaut
		for _, p := range lbPlatformBlock.FindAllString(text, -1) {
			if field(p, "Emulator") != id {
				continue
			}
			defaultInPlace := false
			for _, q := range lbPlatformBlock.FindAllString(text, -1) {
				defaultInPlace = defaultInPlace || inPlace[field(q, "Emulator")] && field(q, "Platform") == field(p, "Platform") && field(q, "Default") == "true"
			}
			kept = kept || defaultInPlace
			if field(p, "Default") == "true" && !defaultInPlace {
				for _, q := range lbPlatformBlock.FindAllString(text, -1) {
					if idOrig != "" && field(q, "Emulator") == idOrig && field(q, "Platform") == field(p, "Platform") {
						text = strings.Replace(text, q, setField(q, "Default", "true"), 1)
						restored = true
						break
					}
				}
			}
			text = strings.Replace(text, p, "", 1)
		}
		text = strings.Replace(text, b, "", 1)
		switch {
		case restored:
			report = append(report, fmt.Sprintf(tr("Émulateur « %s » retiré, « %s » est de nouveau celui par défaut", "Removed the \"%s\" emulator. \"%s\" is the default again"),
				unescape(field(b, "Title")), unescape(title)))
		case idOrig == "" && !kept:
			report = append(report, fmt.Sprintf(tr("Émulateur « %s » retiré. Son émulateur d'origine n'existe plus : choisissez un émulateur par défaut dans LaunchBox.",
				"Removed the \"%s\" emulator. Its original emulator no longer exists, so choose a default emulator in LaunchBox."), unescape(field(b, "Title"))))
		default:
			report = append(report, fmt.Sprintf(tr("Émulateur « %s » retiré.", "Removed the \"%s\" emulator."), unescape(field(b, "Title"))))
		}
	}
	defaults := map[string]int{}
	var platforms []string
	for _, q := range lbPlatformBlock.FindAllString(text, -1) {
		if p := unescape(field(q, "Platform")); field(q, "Default") == "true" {
			if defaults[p]++; defaults[p] == 2 {
				platforms = append(platforms, p)
			}
		}
	}
	for _, p := range platforms {
		report = append(report, fmt.Sprintf(tr("%d émulateurs sont par défaut pour « %s ». Gardez-en un seul dans LaunchBox : Tools, Manage, Emulators.",
			"%d emulators are the default for \"%s\". Keep only one in LaunchBox: Tools, Manage, Emulators."), defaults[p], p))
	}
	return text, report
}

// defaireEsde : retire la règle 3DSSAVESYNC et les commandes 3dssavesync. Un système qui n'a plus de commande est un
// bloc minimal ajouté par esde : il est retiré, et ES-DE reprend son propre réglage pour ce système.
func undoEsde(systems, rules string) (string, string, []string) {
	var report []string
	if esdeRule.MatchString(rules) {
		rules = esdeRule.ReplaceAllLiteralString(rules, "")
	}
	systems = esdeSystem.ReplaceAllStringFunc(systems, func(b string) string {
		if !esdeCommand.MatchString(b) {
			return b
		}
		n := field(b, "name")
		b = esdeCommand.ReplaceAllLiteralString(b, "")
		if !strings.Contains(b, "<command") {
			report = append(report, fmt.Sprintf(tr("%s : système ajouté par 3dssavesync retiré", "%s: removed the system added by 3dssavesync"), n))
			return ""
		}
		report = append(report, fmt.Sprintf(tr("%s : commande 3dssavesync retirée", "%s: removed the 3dssavesync command"), n))
		return b
	})
	return systems, rules, report
}

// desinstaller : frontends, raccourcis, démarrage automatique, puis le programme installé. Erreur : un frontend est
// ouvert, rien n'est fait (il garderait en mémoire l'émulateur 3dssavesync et le réécrirait en se fermant).
func uninstall(cfg *Config, config string) ([]string, error) {
	lb, es := cfg.Frontends["launchbox"], cfg.Frontends["esde"]
	switch {
	case lb != "" && (activeEmulator("LaunchBox") || activeEmulator("BigBox")):
		return nil, errors.New(tr("LaunchBox est ouvert. Fermez LaunchBox et BigBox, puis réessayez.",
			"LaunchBox is open. Close LaunchBox and BigBox, then try again."))
	case es != "" && activeEmulator("ES-DE"):
		return nil, errors.New(tr("ES-DE est ouvert. Fermez-le et réessayez.", "ES-DE is open. Close it and try again."))
	}
	launcher := launcherOf(cfg)
	var report []string
	write := func(name, f, before, after string) {
		if before != after {
			if err := replaceFile(f, []byte(after)); err != nil {
				report = append(report, name+" : "+errorText(err))
			}
		}
	}
	if lb != "" {
		f := filepath.Join(lb, "Data", "Emulators.xml")
		if o, err := os.ReadFile(f); err == nil {
			n, r := undoLaunchbox(string(o), lb, launcher, cfg)
			report = append(report, prefix("LaunchBox", r)...)
			write("LaunchBox", f, string(o), n)
		}
	}
	if es != "" {
		fs, fr := filepath.Join(es, "custom_systems", "es_systems.xml"), filepath.Join(es, "custom_systems", "es_find_rules.xml")
		s, _ := os.ReadFile(fs)
		r, _ := os.ReadFile(fr)
		ns, nr, rap := undoEsde(string(s), string(r))
		report = append(report, prefix("ES-DE", rap)...)
		write("ES-DE", fs, string(s), ns)
		write("ES-DE", fr, string(r), nr)
	}
	targets := []string{launcher}
	if installed := installedLocation(); !strings.EqualFold(installed, launcher) {
		targets = append(targets, installed) // désinstallation lancée depuis un autre exe (téléchargé à nouveau)
	}
	removed := 0
	for _, c := range targets {
		l, err := removeShortcuts(c)
		if err != nil {
			report = append(report, tr("Raccourcis : ", "Shortcuts: ")+errorText(err))
		}
		removed += len(l)
	}
	if removed > 0 {
		report = append(report, fmt.Sprintf(tr("Raccourcis mis à la Corbeille : %d", "Shortcuts moved to the Recycle Bin: %d"), removed))
	}
	if err := stopTray(filepath.Dir(config)); err != nil { // sinon l'exe de l'icône ne peut pas aller à la Corbeille
		report = append(report, err.Error())
	}
	if portable := portableDir(); portable != "" {
		report = append(report, fmt.Sprintf(tr("Version portable : le programme reste dans %s. Supprimez ce dossier quand vous n'en aurez plus besoin. Il contient aussi vos réglages.",
			"Portable version: the program stays in %s. Delete this folder when you no longer need it. It also holds your settings."), portable))
	} else if installed := installedLocation(); exists(installed) {
		if err := removeProgram(installed); err != nil {
			report = append(report, tr("Programme : ", "Program: ")+errorText(err))
		} else {
			report = append(report, fmt.Sprintf(tr("Programme : %s ira à la Corbeille dès sa fermeture", "Program: %s goes to the Recycle Bin when it closes"), installed))
		}
	}
	report = append(report, fmt.Sprintf(tr("Conservés : vos réglages (%s) et vos copies de sécurité (%s)", "Kept: your settings (%s) and your backups (%s)"),
		filepath.Dir(config), cfg.copiesDir(filepath.Dir(config))))
	if cfg.LocalServer != nil {
		dataDir := cfg.LocalServer.Data
		if dataDir == "" {
			dataDir = filepath.Join(filepath.Dir(config), "serveur")
		}
		report = append(report, fmt.Sprintf(tr("Conservées : les versions du serveur local (%s). Le serveur local s'arrêtera au prochain redémarrage du PC.",
			"Kept: the local server versions (%s). The local server stops the next time you restart the PC."), dataDir))
	}
	report = append(report, tr("Sur la 3DS : dans FBI, ouvrez Titles, 3dssavesync et choisissez Delete Title. Vos sauvegardes de la 3DS restent.",
		"On the 3DS: in FBI, open Titles, 3dssavesync and choose Delete Title. Your 3DS saves stay."))
	return report, nil
}

// retirerRaccourcisSysteme : les raccourcis qui lancent le Lanceur (bureau, menu, démarrage automatique). Windows :
// Corbeille ; Linux et macOS : fichiers créés par raccourci et demarrageAuto, retirés.
func removeShortcutsSystem(launcher string) ([]string, error) {
	if runtime.GOOS == "windows" {
		c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			`[Console]::OutputEncoding=[Text.Encoding]::UTF8; Add-Type -AssemblyName Microsoft.VisualBasic; $s=New-Object -ComObject WScript.Shell; `+
				`foreach ($d in 'Desktop','Startup','Programs') { Get-ChildItem -LiteralPath ([Environment]::GetFolderPath($d)) -Filter *.lnk -ErrorAction SilentlyContinue | `+
				`Where-Object { $s.CreateShortcut($_.FullName).TargetPath -ieq $env:R_CIBLE } | ForEach-Object { `+
				`[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($_.FullName,'OnlyErrorDialogs','SendToRecycleBin'); $_.FullName } }`)
		c.Env = append(os.Environ(), "R_CIBLE="+launcher)
		o, err := c.Output()
		var l []string // un chemin par ligne (des espaces dans les noms)
		for _, x := range strings.Split(strings.TrimSpace(string(o)), "\n") {
			if x = strings.TrimSpace(x); x != "" {
				l = append(l, x)
			}
		}
		return l, err
	}
	patterns := []string{xdgData("applications", "3dssavesync*.desktop"), xdgConfig("autostart", "3dssavesync-serveur.desktop")}
	if runtime.GOOS == "darwin" {
		patterns = []string{home("Applications", "3dssavesync*.app"), home("Library", "LaunchAgents", "com.3dssavesync.serveur.plist")}
	}
	var removed []string
	for _, m := range patterns {
		l, _ := filepath.Glob(m)
		for _, f := range l {
			if err := os.RemoveAll(f); err != nil {
				return removed, err
			}
			removed = append(removed, f)
		}
	}
	return removed, nil
}

// retirerProgrammeSysteme : Windows ne retire pas un exe en cours : un PowerShell détaché attend sa fermeture, puis le
// met à la Corbeille et retire son dossier s'il est vide. Linux et macOS : retiré tout de suite.
func removeProgramSystem(installed string) error {
	if runtime.GOOS != "windows" {
		return os.Remove(installed)
	}
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command",
		`Add-Type -AssemblyName Microsoft.VisualBasic; for ($i=0; $i -lt 60; $i++) { Start-Sleep 2; `+
			`try { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($env:R_EXE,'OnlyErrorDialogs','SendToRecycleBin'); break } catch {} }; `+
			`try { [IO.Directory]::Delete((Split-Path $env:R_EXE)) } catch {}`)
	c.Env = append(os.Environ(), "R_EXE="+installed)
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
