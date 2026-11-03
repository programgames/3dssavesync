package main

// Uninstall (ADR 0019): undoes what the install did outside the 3dssavesync folders, so that LaunchBox and ES-DE
// start their games without it again. Nothing is deleted for good: the frontend files are copied first
// (replaceFile), the shortcuts and the program go to the Recycle Bin (Windows). The settings, the Backup copies and
// the local Server data stay: they are the user's data. Portable version (ADR 0020): the program and its folder
// stay, the report says to delete that folder.

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
	// LaunchBox command line of an emulator that goes through the Launcher: "run --emulator <name>".
	emulatorOption = regexp.MustCompile(`--emulator (\S+)`)
)

// Adapters that the tests replace (PowerShell, Recycle Bin, detached process).
var removeShortcuts, removeProgram = removeShortcutsSystem, removeProgramSystem

// undoLaunchbox removes each "<title> (3dssavesync)" copy that starts the Launcher, and its platforms; the emulator
// it copies (same title without "(3dssavesync)") becomes the default emulator of these platforms again. An emulator
// that starts the Launcher under its original title was changed in place (setup of versions 1 and 2): it stays with
// its platforms, and gets back the real exe of config.json (name read from "--emulator <name>"); unknown or missing
// exe: left as is, the report says to fix it by hand. A platform where this emulator is the default keeps it: the
// original emulator of the copy does not become the default there again.
// Two emulators still the default of one platform: the report says to keep one.
func undoLaunchbox(text, lbDir, launcher string, cfg *Config) (string, []string) {
	var report []string
	inPlace := map[string]bool{} // IDs of the emulators changed in place
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
		app := e.Exe // relative under the LaunchBox folder, as LaunchBox writes it
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
				if i > 0 && profile == "retroarch" { // the core's -L, which "run --emulator" replaced
					platform := unescape(field(n, "Platform"))
					if c := e.Cores[lbPlatforms[platform]]; c != "" {
						if r, err := filepath.Rel(filepath.Dir(e.Exe), c); err == nil && !strings.HasPrefix(r, "..") {
							c = r // relative to the RetroArch folder, as LaunchBox writes it
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
			if !ours(o) && unescape(field(o, "Title")) == unescape(title) { // &#39; (escapeXML) against ' (LaunchBox)
				idOrig = field(o, "ID")
			}
		}
		restored, kept := false, false // original back as the default; emulator changed in place still the default
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

// undoEsde removes the 3DSSAVESYNC rule and the 3dssavesync commands. A system with no command left is a minimal
// block that esde added: it is removed, and ES-DE goes back to its own setting for this system.
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

// uninstall undoes the frontends, shortcuts, autostart, then the installed program. Error: a frontend is open, nothing
// is done (it would keep the 3dssavesync emulator in memory and write it again when it closes).
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
		targets = append(targets, installed) // uninstall started from another exe (downloaded again)
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
	if err := stopTray(filepath.Dir(config)); err != nil { // else the exe of the tray icon cannot go to the Recycle Bin
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

// removeShortcutsSystem removes the shortcuts that start the Launcher (desktop, menu, autostart). Windows:
// Recycle Bin; Linux and macOS: the files that shortcut and addAutostart wrote are removed.
func removeShortcutsSystem(launcher string) ([]string, error) {
	if runtime.GOOS == "windows" {
		c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			`[Console]::OutputEncoding=[Text.Encoding]::UTF8; Add-Type -AssemblyName Microsoft.VisualBasic; $s=New-Object -ComObject WScript.Shell; `+
				`foreach ($d in 'Desktop','Startup','Programs') { Get-ChildItem -LiteralPath ([Environment]::GetFolderPath($d)) -Filter *.lnk -ErrorAction SilentlyContinue | `+
				`Where-Object { $s.CreateShortcut($_.FullName).TargetPath -ieq $env:R_CIBLE } | ForEach-Object { `+
				`[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($_.FullName,'OnlyErrorDialogs','SendToRecycleBin'); $_.FullName } }`)
		c.Env = append(os.Environ(), "R_CIBLE="+launcher)
		o, err := c.Output()
		var l []string // one path per line (names can hold spaces)
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

// removeProgramSystem: Windows cannot remove a running exe, so a detached PowerShell waits until it closes, then moves
// it to the Recycle Bin and removes its folder if empty. Linux and macOS: removed at once.
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
