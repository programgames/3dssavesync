package main

// LaunchBox (Data/Emulators.xml): install (launchbox), check (missingLaunchBox) and uninstall (undoLaunchbox) of the
// "<title> (3dssavesync)" emulators. Sources: docs/notes/research-v3-frontends.md, ADR 0019.

import (
	"crypto/rand"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// lbPlatforms maps LaunchBox platforms -> System.
var lbPlatforms = map[string]System{
	"Nintendo DS": DS, "Nintendo DSi": DS, "Nintendo Game Boy": GB, "Nintendo Game Boy Color": GB,
	"Nintendo Game Boy Advance": GBA, "Nintendo 3DS": N3DS,
}

var (
	lbEmulatorBlock = regexp.MustCompile(`(?s)  <Emulator>\r?\n.*?  </Emulator>\r?\n`)
	lbPlatformBlock = regexp.MustCompile(`(?s)  <EmulatorPlatform>\r?\n.*?  </EmulatorPlatform>\r?\n`)
	// Command line of an emulator that goes through the Launcher: "run --emulator <name>".
	emulatorOption = regexp.MustCompile(`--emulator (\S+)`)
)

// lbPath returns the full path of a LaunchBox ApplicationPath (relative to its folder when it is inside).
func lbPath(lbDir, app string) string {
	app = filepath.FromSlash(strings.ReplaceAll(unescapeXML(app), `\`, "/")) // Windows paths, even when read elsewhere (Wine)
	if app != "" && !filepath.IsAbs(app) && !strings.Contains(app, ":") {
		app = filepath.Join(lbDir, app)
	}
	return filepath.Clean(app)
}

// launchBoxOptions returns the user's options in the original LaunchBox emulator of exe (its command line, then those
// of its platforms for systems), for args_en_plus: the "(3dssavesync)" copy has an empty line.
// An emulator changed in place by an old Launcher: its exe is the Launcher, nothing is found.
func launchBoxOptions(text, lbDir, exe string, systems []System) string {
	for _, b := range lbEmulatorBlock.FindAllString(text, -1) {
		app := lbPath(lbDir, field(b, "ApplicationPath"))
		if !strings.EqualFold(app, filepath.Clean(exe)) && !strings.EqualFold(filepath.Base(app), filepath.Base(exe)) {
			continue
		}
		var options []string
		add := func(o string) {
			// never the RetroArch core of a platform (-L): it would go to every System
			if o = strings.TrimSpace(unescapeXML(o)); o != "" && !slices.Contains(options, o) && !strings.Contains(" "+o, " -L ") {
				options = append(options, o)
			}
		}
		add(field(b, "CommandLine"))
		for _, p := range lbPlatformBlock.FindAllString(text, -1) {
			if field(p, "Emulator") == field(b, "ID") && slices.Contains(systems, lbPlatforms[unescapeXML(field(p, "Platform"))]) {
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
	Systems []System
}

// launchbox adds, for each entry, a new "<title> (3dssavesync)" emulator copied from the original emulator, made the
// default emulator of its platforms. The original emulator stays intact (only Default changes).
// Nothing is done if an emulator already goes through 3dssavesync for this entry.
func launchbox(text, lbDir, launcher string, entries []LBEntry) (string, []string) {
	var report []string
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
		newText = setField(newText, "Title", unescapeXML(field(original, "Title"))+" (3dssavesync)")
		newText = setField(newText, "ApplicationPath", launcher)
		newText = setField(newText, "CommandLine", "")
		for _, c := range []string{"HideConsole", "NoQuotes", "AutoExtract"} {
			newText = setField(newText, c, "false")
		}
		var added []string
		for _, p := range plats {
			sys := lbPlatforms[unescapeXML(field(p, "Platform"))]
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
			"%s: added the \"%s (3dssavesync)\" emulator as the default. Platforms: %d"), e.Name, unescapeXML(field(original, "Title")), len(added)))
	}
	return text, report
}

func newID() string {
	o := make([]byte, 16)
	rand.Read(o)
	o[6], o[8] = o[6]&0x0F|0x40, o[8]&0x3F|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", o[0:4], o[4:6], o[6:8], o[8:10], o[10:])
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
	ours := func(b string) bool {
		return strings.EqualFold(path.Base(strings.ReplaceAll(unescapeXML(field(b, "ApplicationPath")), `\`, "/")), filepath.Base(launcher))
	}
	cp := func(b string) bool { return strings.HasSuffix(field(b, "Title"), " (3dssavesync)") }
	for _, b := range lbEmulatorBlock.FindAllString(text, -1) {
		if !ours(b) || cp(b) {
			continue
		}
		title, name := unescapeXML(field(b, "Title")), ""
		inPlace[field(b, "ID")] = true
		blocks := []string{b}
		for _, p := range lbPlatformBlock.FindAllString(text, -1) {
			if field(p, "Emulator") == field(b, "ID") {
				blocks = append(blocks, p)
			}
		}
		for _, x := range blocks {
			if m := emulatorOption.FindStringSubmatch(unescapeXML(field(x, "CommandLine"))); m != nil && name == "" {
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
					platform := unescapeXML(field(n, "Platform"))
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
			if !ours(o) && unescapeXML(field(o, "Title")) == unescapeXML(title) { // &#39; (escapeXML) against ' (LaunchBox)
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
				unescapeXML(field(b, "Title")), unescapeXML(title)))
		case idOrig == "" && !kept:
			report = append(report, fmt.Sprintf(tr("Émulateur « %s » retiré. Son émulateur d'origine n'existe plus : choisissez un émulateur par défaut dans LaunchBox.",
				"Removed the \"%s\" emulator. Its original emulator no longer exists, so choose a default emulator in LaunchBox."), unescapeXML(field(b, "Title"))))
		default:
			report = append(report, fmt.Sprintf(tr("Émulateur « %s » retiré.", "Removed the \"%s\" emulator."), unescapeXML(field(b, "Title"))))
		}
	}
	defaults := map[string]int{}
	var platforms []string
	for _, q := range lbPlatformBlock.FindAllString(text, -1) {
		if p := unescapeXML(field(q, "Platform")); field(q, "Default") == "true" {
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
