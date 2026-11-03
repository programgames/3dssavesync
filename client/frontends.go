package main

// Frontend setup: LaunchBox, ES-DE, shortcuts. Each write keeps a dated copy of the file, and running the setup again
// changes nothing when it is already in place (repair).
// Sources: docs/notes/research-v3-frontends.md.

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
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

func newID() string {
	o := make([]byte, 16)
	rand.Read(o)
	o[6], o[8] = o[6]&0x0F|0x40, o[8]&0x3F|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", o[0:4], o[4:6], o[6:8], o[8:10], o[10:])
}

// --- LaunchBox -----------------------------------------------------------------------------------

// lbPlatforms maps LaunchBox platforms -> System.
var lbPlatforms = map[string]string{
	"Nintendo DS": "DS", "Nintendo DSi": "DS", "Nintendo Game Boy": "GB", "Nintendo Game Boy Color": "GB",
	"Nintendo Game Boy Advance": "GBA", "Nintendo 3DS": "3DS",
}

var (
	lbEmulatorBlock = regexp.MustCompile(`(?s)  <Emulator>\r?\n.*?  </Emulator>\r?\n`)
	lbPlatformBlock = regexp.MustCompile(`(?s)  <EmulatorPlatform>\r?\n.*?  </EmulatorPlatform>\r?\n`)
)

func field(block, name string) string {
	m := regexp.MustCompile(`<` + name + `>(.*?)</` + name + `>`).FindStringSubmatch(block)
	if m == nil {
		return ""
	}
	return m[1]
}

func setField(block, name, value string) string {
	r := regexp.MustCompile(`<` + name + `>.*?</` + name + `>|<` + name + ` />`)
	if value == "" {
		return r.ReplaceAllLiteralString(block, "<"+name+" />")
	}
	return r.ReplaceAllLiteralString(block, "<"+name+">"+escapeXML(value)+"</"+name+">")
}

// lbPath returns the full path of a LaunchBox ApplicationPath (relative to its folder when it is inside).
func lbPath(lbDir, app string) string {
	app = filepath.FromSlash(strings.ReplaceAll(html.UnescapeString(app), `\`, "/")) // Windows paths, even when read elsewhere (Wine)
	if app != "" && !filepath.IsAbs(app) && !strings.Contains(app, ":") {
		app = filepath.Join(lbDir, app)
	}
	return filepath.Clean(app)
}

// launchBoxOptions returns the user's options in the original LaunchBox emulator of exe (its command line, then those
// of its platforms for systems), for args_en_plus: the "(3dssavesync)" copy has an empty line.
// An emulator changed in place by an old Launcher: its exe is the Launcher, nothing is found.
func launchBoxOptions(text, lbDir, exe string, systems []string) string {
	for _, b := range lbEmulatorBlock.FindAllString(text, -1) {
		app := lbPath(lbDir, field(b, "ApplicationPath"))
		if !strings.EqualFold(app, filepath.Clean(exe)) && !strings.EqualFold(filepath.Base(app), filepath.Base(exe)) {
			continue
		}
		var options []string
		add := func(o string) {
			// never the RetroArch core of a platform (-L): it would go to every System
			if o = strings.TrimSpace(html.UnescapeString(o)); o != "" && !slices.Contains(options, o) && !strings.Contains(" "+o, " -L ") {
				options = append(options, o)
			}
		}
		add(field(b, "CommandLine"))
		for _, p := range lbPlatformBlock.FindAllString(text, -1) {
			if field(p, "Emulator") == field(b, "ID") && slices.Contains(systems, lbPlatforms[html.UnescapeString(field(p, "Platform"))]) {
				add(field(p, "CommandLine"))
			}
		}
		return strings.Join(options, " ")
	}
	return ""
}

// LBEntry is an emulator of the configuration, to route through 3dssavesync in LaunchBox.
type LBEntry struct {
	Name    string
	Exe     string // name in config.json, executable of the real emulator
	Systems []string
}

// launchbox adds, for each entry, a new "<title> (3dssavesync)" emulator copied from the original emulator, made the
// default emulator of its platforms. The original emulator stays intact (only Default changes).
// Nothing is done if an emulator already goes through 3dssavesync for this entry.
func launchbox(text, lbDir, launcher string, entries []LBEntry) (string, []string) {
	var report []string
	unescape := unescapeXML
	path := func(app string) string { return lbPath(lbDir, app) }
	for _, e := range entries {
		emus := lbEmulatorBlock.FindAllString(text, -1)
		plats := lbPlatformBlock.FindAllString(text, -1)
		alreadyDone, original := false, ""
		for _, b := range emus {
			app := path(field(b, "ApplicationPath"))
			if strings.EqualFold(filepath.Base(app), filepath.Base(launcher)) {
				for _, p := range plats {
					if field(p, "Emulator") == field(b, "ID") && strings.Contains(" "+field(p, "CommandLine")+" ", " --emulator "+e.Name+" ") {
						alreadyDone = true
					}
				}
			} else if original == "" && (strings.EqualFold(app, filepath.Clean(e.Exe)) || strings.EqualFold(filepath.Base(app), filepath.Base(e.Exe))) {
				original = b
			}
		}
		switch {
		case alreadyDone:
			report = append(report, fmt.Sprintf(tr("%s : déjà en place", "%s: already set up"), e.Name))
			continue
		case original == "":
			report = append(report, fmt.Sprintf(tr("%s : émulateur absent de LaunchBox. Ajoutez-le dans LaunchBox et relancez la réparation.",
				"%s: emulator not found in LaunchBox. Add it in LaunchBox and run the repair again."), e.Name))
			continue
		}
		idOrig, id := field(original, "ID"), newID()
		newText := setField(original, "ID", id)
		newText = setField(newText, "Title", unescape(field(original, "Title"))+" (3dssavesync)")
		newText = setField(newText, "ApplicationPath", launcher)
		newText = setField(newText, "CommandLine", "")
		for _, c := range []string{"HideConsole", "NoQuotes", "AutoExtract"} {
			newText = setField(newText, c, "false")
		}
		var added []string
		for _, p := range plats {
			sys := lbPlatforms[unescape(field(p, "Platform"))]
			if field(p, "Emulator") != idOrig || !slices.Contains(e.Systems, sys) {
				continue
			}
			q := setField(p, "Emulator", id)
			q = setField(q, "CommandLine", "run --emulator "+e.Name) // LaunchBox adds the ROM at the end
			q = setField(q, "Default", "true")
			if sys != "3DS" { // the Launcher reads .zip files; not .3ds inside a .zip
				q = setField(q, "AutoExtract", "false")
			}
			added = append(added, q)
			text = strings.Replace(text, p, setField(p, "Default", "false"), 1)
		}
		if len(added) == 0 {
			report = append(report, fmt.Sprintf(tr("%s : aucune plateforme DS, GB, GBA ou 3DS dans LaunchBox", "%s: no DS, GB, GBA or 3DS platform in LaunchBox"), e.Name))
			continue
		}
		i := strings.LastIndex(text, "</LaunchBox>")
		text = text[:i] + newText + strings.Join(added, "") + text[i:]
		report = append(report, fmt.Sprintf(tr("%s : émulateur « %s (3dssavesync) » ajouté et mis par défaut. Plateformes : %d",
			"%s: added the \"%s (3dssavesync)\" emulator as the default. Platforms: %d"), e.Name, unescape(field(original, "Title")), len(added)))
	}
	return text, report
}

// --- ES-DE ---------------------------------------------------------------------------------------

// esdeSystems maps ES-DE systems -> System, full name, extensions read by the Launcher.
var esdeSystems = map[string][3]string{
	"nds":  {"DS", "Nintendo DS", ".nds .NDS .zip .ZIP"},
	"gb":   {"GB", "Nintendo Game Boy", ".gb .GB .zip .ZIP"},
	"gbc":  {"GB", "Nintendo Game Boy Color", ".gbc .GBC .zip .ZIP"},
	"gba":  {"GBA", "Nintendo Game Boy Advance", ".gba .GBA .zip .ZIP"},
	"n3ds": {"3DS", "Nintendo 3DS", ".3ds .3DS .cci .CCI .cxi .CXI"},
}

// esde writes the 3DSSAVESYNC rule in es_find_rules.xml, and a "3dssavesync" command at the top of each handled system
// in es_systems.xml (first command = default emulator). A system missing from the custom file gets a minimal block
// (the block shipped with ES-DE can't be read inside an AppImage).
// emus: System -> name of the config.json entry.
func esde(systems, rules, launcher string, emus map[string]string) (string, string, []string) {
	var report []string
	rule := "  <emulator name=\"3DSSAVESYNC\">\n    <rule type=\"staticpath\">\n      <entry>" + escapeXML(launcher) + "</entry>\n    </rule>\n  </emulator>\n"
	prev := regexp.MustCompile(`(?s)  <emulator name="3DSSAVESYNC">.*?</emulator>\r?\n`)
	switch {
	case prev.MatchString(rules):
		rules = prev.ReplaceAllLiteralString(rules, rule)
	case strings.Contains(rules, "</ruleList>"):
		rules = strings.Replace(rules, "</ruleList>", rule+"</ruleList>", 1)
	default:
		rules = "<?xml version=\"1.0\"?>\n<ruleList>\n" + rule + "</ruleList>\n"
	}
	if !strings.Contains(systems, "</systemList>") {
		systems = "<?xml version=\"1.0\"?>\n<systemList>\n</systemList>\n"
	}
	var names []string
	for n := range esdeSystems {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		d := esdeSystems[n]
		name := emus[d[0]]
		if name == "" {
			continue
		}
		command := `<command label="3dssavesync (` + name + `)">%EMULATOR_3DSSAVESYNC% run --emulator ` + name + ` %ROM%</command>`
		block := regexp.MustCompile(`(?s)<system>\s*<name>` + n + `</name>.*?</system>`).FindString(systems)
		switch {
		case strings.Contains(block, "--emulator "+name+" "):
			report = append(report, fmt.Sprintf(tr("ES-DE %s : déjà en place", "ES-DE %s: already set up"), n))
		case block != "" && strings.Contains(block, "<command"):
			i := strings.Index(block, "<command")
			systems = strings.Replace(systems, block, block[:i]+command+"\n        "+block[i:], 1)
			report = append(report, fmt.Sprintf(tr("ES-DE %s : commande ajoutée en tête de liste", "ES-DE %s: command added at the top of the list"), n))
		default:
			ext := d[2]
			if block != "" {
				if e := field(block, "extension"); e != "" { // the user's list kept, never shortened
					ext = e
				}
				systems = strings.Replace(systems, block, "", 1)
			}
			newText := "    <system>\n        <name>" + n + "</name>\n        <fullname>" + d[1] + "</fullname>\n        <path>%ROMPATH%/" + n +
				"</path>\n        <extension>" + ext + "</extension>\n        " + command + "\n        <platform>" + n +
				"</platform>\n        <theme>" + n + "</theme>\n    </system>\n"
			systems = strings.Replace(systems, "</systemList>", newText+"</systemList>", 1)
			report = append(report, fmt.Sprintf(tr("ES-DE %s : système ajouté, avec seulement la commande 3dssavesync", "ES-DE %s: system added, with the 3dssavesync command only"), n))
		}
	}
	return systems, rules, report
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
func frontendEntries(cfg *Config) ([]LBEntry, map[string]string) {
	var lb []LBEntry
	emus := map[string]string{}
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

// missingLaunchBox returns the entries whose original emulator is in LaunchBox, but with no default "(3dssavesync)"
// emulator on any of its platforms (repair needed).
func missingLaunchBox(text, launcher string, entries []LBEntry) []string {
	base := func(app string) string { return strings.ToLower(path.Base(strings.ReplaceAll(app, `\`, "/"))) }
	emus, plats := lbEmulatorBlock.FindAllString(text, -1), lbPlatformBlock.FindAllString(text, -1)
	var missing []string
	for _, e := range entries {
		ready, original := false, false
		for _, b := range emus {
			app := base(field(b, "ApplicationPath"))
			original = original || app == base(e.Exe)
			for _, p := range plats {
				ready = ready || app == base(launcher) && field(p, "Emulator") == field(b, "ID") && field(p, "Default") == "true" &&
					strings.Contains(" "+field(p, "CommandLine")+" ", " --emulator "+e.Name+" ")
			}
		}
		if original && !ready {
			missing = append(missing, e.Name)
		}
	}
	return missing
}

func prefix(p string, l []string) []string {
	for i := range l {
		l[i] = p + " " + l[i]
	}
	return l
}

// ponytail: cloud_sync_status key of ~/emudeck/settings.sh not verified (docs/notes/research-v3-frontends.md §5);
// warning only.
func emuDeckWarning() string {
	o, err := os.ReadFile(home("emudeck", "settings.sh"))
	if err == nil && regexp.MustCompile(`(?m)^cloud_sync_status="?true"?`).Match(o) {
		return tr("EmuDeck CloudSync semble actif. Désactivez-le pour DS, GB, GBA et 3DS, sinon les deux outils enverront les mêmes sauvegardes.",
			"EmuDeck CloudSync seems to be on. Turn it off for DS, GB, GBA and 3DS, or both tools will upload the same saves.")
	}
	return ""
}
