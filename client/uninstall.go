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
	"path/filepath"
	"runtime"
	"strings"
)

// Adapters that the tests replace (PowerShell, Recycle Bin, detached process).
var removeShortcuts, removeProgram = removeShortcutsSystem, removeProgramSystem

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
