package main

// Frontend setup: shared XML helpers, shortcuts, repair; LaunchBox in launchbox.go, ES-DE in esde.go. Each write keeps a dated copy of the file, and running the setup again
// changes nothing when it is already in place (repair).
// Sources: docs/notes/research-v3-frontends.md.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io/fs"
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

// copyBefore makes a dated copy of a file before changing it; nothing if the file doesn't exist. Two changes in the
// same second (add then remove an Alias): -1, -2… appended, a copy is never overwritten.
func copyBefore(path string) error {
	o, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	name := path + ".avant-3dssavesync-" + time.Now().Format("20060102-150405")
	for i := 1; ; i++ {
		err = writeVerified(name, o)
		if !errors.Is(err, fs.ErrExist) || i == 10 {
			return err
		}
		name = fmt.Sprintf("%s.avant-3dssavesync-%s-%d", path, time.Now().Format("20060102-150405"), i)
	}
}

// replaceFile replaces a frontend file: dated copy, verified temp file, then rename.
func replaceFile(path string, content []byte) error {
	if err := copyBefore(path); err != nil {
		return err
	}
	return writeAtomic(path, content)
}

func escapeXML(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// unescapeXML reverses escapeXML (&#39;, &#34;) and LaunchBox texts (&apos;, &quot;, or raw ' and "): all entities.
func unescapeXML(s string) string { return html.UnescapeString(s) }

// xmlFields holds the regexes of field and setField, compiled once per element name.
var xmlFields sync.Map // name -> [2]*regexp.Regexp

func fieldRegexps(name string) [2]*regexp.Regexp {
	if r, ok := xmlFields.Load(name); ok {
		return r.([2]*regexp.Regexp)
	}
	r := [2]*regexp.Regexp{regexp.MustCompile(`<` + name + `>(.*?)</` + name + `>`), regexp.MustCompile(`<` + name + `>.*?</` + name + `>|<` + name + ` />`)}
	xmlFields.Store(name, r)
	return r
}

// field returns the raw text of the first <name> element of an XML block, on one line ("" if none).
func field(block, name string) string {
	m := fieldRegexps(name)[0].FindStringSubmatch(block)
	if m == nil {
		return ""
	}
	return m[1]
}

// setField replaces the <name> elements of a block with an escaped value; empty value: <name />.
func setField(block, name, value string) string {
	r := fieldRegexps(name)[1]
	if value == "" {
		return r.ReplaceAllLiteralString(block, "<"+name+" />")
	}
	return r.ReplaceAllLiteralString(block, "<"+name+">"+escapeXML(value)+"</"+name+">")
}

// --- Shortcuts -----------------------------------------------------------------------------------

// argDesktop quotes an argument of the Exec key of a .desktop file: quotes if needed, \ before " ` $ \; % doubled.
func argDesktop(a string) string {
	a = strings.ReplaceAll(a, "%", "%%")
	if !strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`") {
		return a
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(a) + `"`
}

func desktopFile(name string, exec []string, terminal bool) string {
	var a []string
	for _, x := range exec {
		a = append(a, argDesktop(x))
	}
	return fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s\nTerminal=%t\nCategories=Game;\n", name, strings.Join(a, " "), terminal)
}

// shortcut creates a "name" shortcut that runs launcher + args. Windows: .lnk on the desktop; Linux: .desktop in the
// applications menu; macOS: .app in ~/Applications. console: the setup (terminal on Linux and macOS).
func shortcut(name, launcher string, args []string, console bool) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return lnk("Desktop", name, launcher, args)
	case "darwin":
		app := home("Applications", name+".app")
		script := "#!/bin/sh\nexec " + argShell(launcher) + " " + strings.Join(mapper(args, argShell), " ") + "\n"
		if console {
			script = "#!/bin/sh\nexec osascript -e " + argShell(`tell application "Terminal" to do script "`+strings.ReplaceAll(argShell(launcher), `"`, `\"`)+` `+strings.Join(args, " ")+`"`) + "\n"
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?>` + "\n<plist version=\"1.0\"><dict><key>CFBundleExecutable</key><string>lanceur</string>" +
			"<key>CFBundleName</key><string>" + escapeXML(name) + "</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>\n"
		if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
			return "", err
		}
		return app, os.WriteFile(filepath.Join(app, "Contents", "MacOS", "lanceur"), []byte(script), 0o755)
	}
	f := xdgData("applications", strings.ReplaceAll(strings.ToLower(name), " ", "-")+".desktop")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return "", err
	}
	return f, os.WriteFile(f, []byte(desktopFile(name, append([]string{launcher}, args...), console)), 0o755)
}

// lnk creates a .lnk shortcut in a special Windows folder (Desktop, Startup). Arguments with a space get quotes.
func lnk(dir, name, launcher string, args []string) (string, error) {
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`[Console]::OutputEncoding=[Text.Encoding]::UTF8; $d=[Environment]::GetFolderPath($env:R_DOSSIER); $l=Join-Path $d ($env:R_NOM+'.lnk'); `+
			`$s=(New-Object -ComObject WScript.Shell).CreateShortcut($l); $s.TargetPath=$env:R_CIBLE; $s.Arguments=$env:R_ARGS; `+
			`$s.WorkingDirectory=(Split-Path $env:R_CIBLE); $s.Save(); $l`)
	var a []string
	for _, x := range args {
		if strings.ContainsAny(x, " \t") {
			x = `"` + x + `"`
		}
		a = append(a, x)
	}
	c.Env = append(os.Environ(), "R_DOSSIER="+dir, "R_NOM="+name, "R_CIBLE="+launcher, "R_ARGS="+strings.Join(a, " "))
	o, err := c.Output()
	return strings.TrimSpace(string(o)), err
}

// addAutostart starts launcher + args at login. Windows: .lnk in Startup; Linux: XDG autostart;
// macOS: LaunchAgent.
func addAutostart(name, launcher string, args []string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return lnk("Startup", name, launcher, args)
	case "darwin":
		f := home("Library", "LaunchAgents", "com.3dssavesync.serveur.plist")
		var p strings.Builder
		for _, x := range append([]string{launcher}, args...) {
			p.WriteString("<string>" + escapeXML(x) + "</string>")
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?>` + "\n<plist version=\"1.0\"><dict><key>Label</key><string>com.3dssavesync.serveur</string>" +
			"<key>ProgramArguments</key><array>" + p.String() + "</array><key>RunAtLoad</key><true/></dict></plist>\n"
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			return "", err
		}
		return f, os.WriteFile(f, []byte(plist), 0o644)
	}
	f := xdgConfig("autostart", "3dssavesync-serveur.desktop")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return "", err
	}
	return f, os.WriteFile(f, []byte(desktopFile(name, append([]string{launcher}, args...), false)), 0o644)
}

func argShell(a string) string { return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'" }

func mapper(l []string, f func(string) string) []string {
	r := make([]string, len(l))
	for i, x := range l {
		r[i] = f(x)
	}
	return r
}

// --- Repair --------------------------------------------------------------------------------------

// repair redoes the setup of the frontends listed in config.json. An open frontend is skipped.
func repair(cfg *Config, launcher string) []string {
	var report []string
	lb, emus := frontendEntries(cfg)
	if d := cfg.Frontends["launchbox"]; d != "" {
		f := filepath.Join(d, "Data", "Emulators.xml")
		o, err := os.ReadFile(f)
		switch {
		case activeEmulator("LaunchBox") || activeEmulator("BigBox"):
			report = append(report, tr("LaunchBox est ouvert, rien n'a été modifié. Fermez LaunchBox et BigBox, puis relancez la réparation.",
				"LaunchBox is open, so nothing was changed. Close LaunchBox and BigBox, then run the repair again."))
		case err != nil:
			report = append(report, "LaunchBox : "+errorText(err))
		default:
			newText, r := launchbox(string(o), d, launcher, lb)
			report = append(report, prefix("LaunchBox", r)...)
			if newText != string(o) {
				if err := replaceFile(f, []byte(newText)); err != nil {
					report = append(report, "LaunchBox : "+errorText(err))
				}
			}
		}
	}
	if d := cfg.Frontends["esde"]; d != "" {
		fs, fr := filepath.Join(d, "custom_systems", "es_systems.xml"), filepath.Join(d, "custom_systems", "es_find_rules.xml")
		s, _ := os.ReadFile(fs)
		r, _ := os.ReadFile(fr)
		if activeEmulator("ES-DE") {
			report = append(report, tr("ES-DE est ouvert, rien n'a été modifié. Fermez-le et relancez la réparation.",
				"ES-DE is open, so nothing was changed. Close it and run the repair again."))
		} else {
			ns, nr, rap := esde(string(s), string(r), launcher, emus)
			report = append(report, rap...)
			for f, c := range map[string][2]string{fs: {string(s), ns}, fr: {string(r), nr}} {
				if c[0] != c[1] {
					if err := replaceFile(f, []byte(c[1])); err != nil {
						report = append(report, "ES-DE : "+errorText(err))
					}
				}
			}
		}
	}
	if a := emuDeckWarning(); a != "" {
		report = append(report, a)
	}
	return report
}

// frontendEntries returns the config.json emulators for LaunchBox, and for ES-DE the first (by name) of each System.
func frontendEntries(cfg *Config) ([]LBEntry, map[System]string) {
	var lb []LBEntry
	emus := map[System]string{}
	var names []string
	for n := range cfg.Emulators {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		e := cfg.Emulators[n]
		p, err := e.profile(n)
		if err != nil {
			continue
		}
		lb = append(lb, LBEntry{n, e.Exe, e.systems(p)})
		for _, s := range e.systems(p) {
			if emus[s] == "" {
				emus[s] = n
			}
		}
	}
	return lb, emus
}

func prefix(p string, l []string) []string {
	for i := range l {
		l[i] = p + " " + l[i]
	}
	return l
}

var emuDeckCloudSync = regexp.MustCompile(`(?m)^cloud_sync_status="?true"?`)

// ponytail: cloud_sync_status key of ~/emudeck/settings.sh not verified (docs/notes/research-v3-frontends.md §5);
// warning only.
func emuDeckWarning() string {
	o, err := os.ReadFile(home("emudeck", "settings.sh"))
	if err == nil && emuDeckCloudSync.Match(o) {
		return tr("EmuDeck CloudSync semble actif. Désactivez-le pour DS, GB, GBA et 3DS, sinon les deux outils enverront les mêmes sauvegardes.",
			"EmuDeck CloudSync seems to be on. Turn it off for DS, GB, GBA and 3DS, or both tools will upload the same saves.")
	}
	return ""
}
