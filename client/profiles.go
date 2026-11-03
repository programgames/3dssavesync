package main

// Emulator Profiles: how to start the emulator, folder and name of the Saves, save state reloaded at start.
// Sources: docs/notes/research-v3-emulateurs.md (paths per System, Flatpak, EmuDeck, RetroDECK).

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type Profile struct {
	Systems      []System                                                   // nil: the ones of the configuration (RetroArch cores, folders of a custom Profile)
	Ext          string                                                     // extension of the Save; default .sav
	InternalName bool                                                       // ROM in an archive: Save named after the inner file (melonDS), else after the archive
	Args         func(e *Emulator, system System, rom, dir string) []string // empty rom: emulator with no game (launch)
	Dir          func(e *Emulator, system System, rom string) string        // setting of the emulator; "" = unknown
	Autoload     func(e *Emulator) string                                   // hint if a save state is reloaded at start
}

var profiles = map[string]Profile{
	"melonds": {
		Systems: []System{"DS"}, InternalName: true,
		Args: func(_ *Emulator, _ System, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, _ System, _ string) string {
			f := first(nextTo(e, "melonDS.toml"), nextTo(e, "portable", "melonDS.toml"), env("LOCALAPPDATA", "melonDS", "melonDS.toml"),
				xdgConfig("melonDS", "melonDS.toml"), home("Library", "Preferences", "melonDS", "melonDS.toml"),
				home(".var", "app", "net.kuribo64.melonDS", "config", "melonDS", "melonDS.toml"),
				home(".var", "app", "net.retrodeck.retrodeck", "config", "melonDS", "melonDS.toml"))
			return setting(f, "Instance0", "SaveFilePath")
		},
	},
	"mgba": {
		Systems: []System{"GB", "GBA"},
		Args: func(_ *Emulator, _ System, rom, dir string) []string {
			if dir == "" {
				return romArgs(rom)
			}
			return append([]string{"-C", "savegamePath=" + dir}, romArgs(rom)...) // never --config (mGBA quits)
		},
		Dir: func(e *Emulator, _ System, _ string) string {
			return setting(configMGBA(e), "ports.qt", "savegamePath")
		},
		Autoload: func(e *Emulator) string {
			f := configMGBA(e)
			if f != "" && read1(f, "ports.qt", "autoload") != "0" { // absent = on (mGBA default)
				return tr("mGBA recharge automatiquement l'état du slot 0 au lancement. Mettez autoload=0 dans ", "mGBA automatically reloads the slot 0 save state at launch. Set autoload=0 in ") + f + "."
			}
			return ""
		},
	},
	"azahar": {
		Systems: []System{"3DS"},
		Args: func(_ *Emulator, _ System, rom, _ string) []string {
			if rom == "" {
				return nil
			}
			return []string{"-f", rom}
		},
		Dir: func(e *Emulator, _ System, _ string) string {
			user := first(nextTo(e, "user"), env("APPDATA", "Azahar"), xdgData("azahar-emu"), home("Library", "Application Support", "Azahar"),
				home(".var", "app", "org.azahar_emu.Azahar", "data", "azahar-emu"))
			f := first(joinPath(user, "config", "qt-config.ini"), xdgConfig("azahar-emu", "qt-config.ini"),
				home(".var", "app", "org.azahar_emu.Azahar", "config", "azahar-emu", "qt-config.ini"),
				home(".var", "app", "net.retrodeck.retrodeck", "config", "azahar-emu", "qt-config.ini"))
			if read1(f, "Data%20Storage", "use_custom_storage") == "true" {
				if d := setting(f, "Data%20Storage", "sdmc_directory"); exists(d) {
					return d
				}
			}
			return joinPath(user, "sdmc")
		},
	},
	"retroarch": {
		Ext: ".srm",
		Args: func(e *Emulator, system System, rom, _ string) []string {
			if rom == "" {
				return nil
			}
			return []string{"-L", e.Cores[system], rom}
		},
		Dir: retroArchDir,
		Autoload: func(e *Emulator) string {
			if f := configRetroArch(e); read1(f, "", "savestate_auto_load") == "true" {
				return tr("RetroArch recharge automatiquement le dernier état au lancement. Mettez savestate_auto_load à false dans ", "RetroArch automatically reloads the last save state at launch. Set savestate_auto_load to false in ") + f + "."
			}
			return ""
		},
	},
	"vbam": {
		Systems: []System{"GB", "GBA"},
		Args:    func(_ *Emulator, _ System, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, system System, _ string) string {
			d := setting(configVBAM(e), "General", "BatteryDir")
			if d == "" {
				return ""
			}
			s := "GameBoy"
			if system == "GBA" {
				s = "GameBoy Advance"
			}
			d = strings.ReplaceAll(d, "%s", s)
			if !filepath.IsAbs(d) {
				d = filepath.Join(filepath.Dir(e.Exe), d)
			}
			return filepath.Clean(d)
		},
		Autoload: func(e *Emulator) string {
			if f := configVBAM(e); read1(f, "General", "AutoLoadLastState") == "1" {
				return tr("VBA-M recharge le dernier état au lancement, sans lire la sauvegarde. Mettez AutoLoadLastState à 0 dans ", "VBA-M reloads the last save state at launch and ignores the save. Set AutoLoadLastState to 0 in ") + f + "."
			}
			return ""
		},
	},
	"desmume": { // .dsv: SRAM + footer (dsv.go)
		Systems: []System{"DS"}, Ext: ".dsv", InternalName: true,
		Args: func(_ *Emulator, _ System, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, _ System, _ string) string {
			switch runtime.GOOS {
			case "windows": // desmume.ini next to the exe; no Battery = .\Battery (path.cpp, winutil.cpp)
				if d := setting(nextTo(e, "desmume.ini"), "PathSettings", "Battery"); d != "" {
					return d
				}
				return nextTo(e, "Battery")
			case "linux": // GTK: everything in the configuration folder (path.cpp LoadModulePath)
				if d := first(xdgConfig("desmume"), home(".var", "app", "org.desmume.DeSmuME", "config", "desmume")); d != "" {
					return d
				}
				return xdgConfig("desmume")
			}
			return ""
		},
	},
	"sameboy": { // .sav always next to the ROM
		Systems: []System{"GB"},
		Args:    func(_ *Emulator, _ System, rom, _ string) []string { return romArgs(rom) },
	},
	"skyemu": { // raw .sav, no Clock trailer (main.c se_write_save_to_disk)
		Systems: []System{"DS", "GB", "GBA"},
		Args:    func(_ *Emulator, _ System, rom, _ string) []string { return romArgs(rom) },
		Dir:     skyEmuDir,
	},
	"nanoboyadvance": { // raw .sav; clock taken from the PC, never written (rtc.cc)
		Systems: []System{"GBA"},
		// ponytail: the ROM must stay argv[1] (main.cc); args_en_plus breaks the start, use the args field if needed.
		Args: func(_ *Emulator, _ System, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, _ System, _ string) string {
			// Windows, Linux: config.toml in the current folder, which is the exe folder when a ROM is given
			// (main.cc, PORTABLE_MODE forced)
			f := first(nextTo(e, "config.toml"), home("Library", "Application Support", "org.github.fleroviux.NanoBoyAdvance", "config.toml"))
			if d := setting(f, "general", "save_folder"); exists(d) { // empty or absent: next to the ROM (main_window.cc GetSavePath)
				return d
			}
			return ""
		},
	},
	"bgb": { // .sav + 48-byte Clock trailer; the key of the "Save files" setting is undocumented: use the sauvegardes field
		Systems: []System{"GB"},
		Args:    func(_ *Emulator, _ System, rom, _ string) []string { return romArgs(rom) }, // "[-rom] <file>": -rom is optional
	},
}

// library_name of the supported RetroArch cores: name of the subfolder when sort_savefiles_enable is on.
var libraries = map[string]string{
	"melondsds": "melonDS DS", "mgba": "mGBA", "gambatte": "Gambatte", "sameboy": "SameBoy", "vbam": "VBA-M",
	"gpsp": "gpSP", "mednafen_gba": "Beetle GBA", // fixed-size .srm, converted (srm.go)
}

func library(core string) string {
	b, _, _ := strings.Cut(strings.ToLower(filepath.Base(core)), "_libretro")
	return libraries[b]
}

func romArgs(rom string) []string {
	if rom == "" {
		return nil
	}
	return []string{rom}
}

// profile returns the Profile of a configuration entry: built in, or custom (args field).
func (e *Emulator) profile(name string) (Profile, error) {
	p := e.Profile
	if p == "" {
		p = name
	}
	if p == "retroarch" {
		if len(e.Cores) == 0 {
			return Profile{}, fmt.Errorf(tr("%s : indiquez un cœur RetroArch par système (champ coeurs)", "%s: set one RetroArch core per system (coeurs field)"), name)
		}
		for _, c := range e.Cores {
			if library(c) == "" {
				return Profile{}, fmt.Errorf(tr("cœur RetroArch non pris en charge : %s. Cœurs pris en charge : melonDS DS, mGBA, Gambatte, SameBoy, VBA-M, gpSP, Beetle GBA", "unsupported RetroArch core: %s. Supported cores: melonDS DS, mGBA, Gambatte, SameBoy, VBA-M, gpSP, Beetle GBA"), c)
			}
		}
	}
	if pr, ok := profiles[p]; ok {
		return pr, nil
	}
	if e.Args == nil {
		return Profile{}, fmt.Errorf(tr("profil d'émulateur inconnu : %s. Un profil personnalisé a besoin du champ args.", "unknown emulator profile: %s. A custom profile needs the args field."), p)
	}
	return Profile{Ext: e.Extension, InternalName: e.InternalName, Args: func(e *Emulator, _ System, rom, dir string) []string {
		var a []string
		for _, x := range e.Args {
			if strings.Contains(x, "{rom}") && rom == "" {
				continue
			}
			a = append(a, strings.NewReplacer("{rom}", rom, "{sauvegardes}", dir).Replace(x))
		}
		return a
	}}, nil
}

// command returns the full command: exe, Profile arguments, and ExtraArgs just before the ROM (at the end with no ROM).
func (e *Emulator) command(p Profile, system System, rom, dir string) []string {
	a := p.Args(e, system, rom, dir)
	if n := len(a); rom != "" && n > 0 && a[n-1] == rom {
		a = append(append(append([]string{}, a[:n-1]...), e.ExtraArgs...), rom)
	} else {
		a = append(a, e.ExtraArgs...)
	}
	return append([]string{e.Exe}, a...)
}

// split turns a command line typed by the user into arguments. Spaces separate them, except inside double or
// single quotes; a backslash stays as is (Windows paths).
func split(line string) []string {
	var a []string
	var current strings.Builder
	quote, inside := rune(0), false
	for _, c := range line {
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote, inside = c, true
		case quote == 0 && (c == ' ' || c == '\t'):
			if inside {
				a = append(a, current.String())
				current.Reset()
				inside = false
			}
		default:
			current.WriteRune(c)
			inside = true
		}
	}
	if inside {
		a = append(a, current.String())
	}
	return a
}

// joinArgs is the reverse of split, to show a command (quotes around an argument with a space).
func joinArgs(a []string) string {
	l := make([]string, len(a))
	for i, x := range a {
		if x == "" || strings.ContainsAny(x, " \t") {
			x = `"` + x + `"`
		}
		l[i] = x
	}
	return strings.Join(l, " ")
}

// handles reports a handled System: the ones of the configuration if any, else the ones of the Profile.
func (e *Emulator) handles(p Profile, system System) bool {
	if len(e.Saves) > 0 || len(e.Cores) > 0 {
		return e.Saves[system] != "" || e.Cores[system] != ""
	}
	return slices.Contains(p.Systems, system)
}

func (e *Emulator) systems(p Profile) (s []System) {
	for _, x := range []System{DS, GB, GBA, N3DS} {
		if e.handles(p, x) {
			s = append(s, x)
		}
	}
	return s
}

// dir returns the Save folder: config.json, else the emulator setting, else next to the ROM
// (never for the 3DS: with no known sdmc, "").
func (e *Emulator) dir(p Profile, system System, rom string) string {
	if d := e.Saves[system]; d != "" {
		return d
	}
	if p.Dir != nil {
		if d := p.Dir(e, system, rom); d != "" {
			return d
		}
	}
	if rom == "" || system == "3DS" {
		return ""
	}
	return filepath.Dir(rom)
}

// process returns the name of the process to watch: processus field, else the exe name.
func (e *Emulator) process() string {
	if e.Process != "" {
		return e.Process
	}
	b := filepath.Base(e.Exe)
	if strings.EqualFold(filepath.Ext(b), ".exe") {
		b = b[:len(b)-4]
	}
	return b
}

// retroArchDir returns savefile_directory, then the content and core subfolders as the sort options say.
func retroArchDir(e *Emulator, system System, rom string) string {
	f := configRetroArch(e)
	if read1(f, "", "savefiles_in_content_dir") == "true" {
		return ""
	}
	base := setting(f, "", "savefile_directory")
	switch {
	case base == "" && f != "": // empty = ROM folder
		return ""
	case base == "" || base == "default":
		switch {
		case runtime.GOOS == "darwin":
			base = home("Documents", "RetroArch", "saves")
		case f != "":
			base = filepath.Join(filepath.Dir(f), "saves")
		case runtime.GOOS == "windows":
			base = nextTo(e, "saves")
		default:
			base = xdgConfig("retroarch", "saves")
		}
	case strings.HasPrefix(base, ":"): // :\saves = relative to the retroarch.exe folder
		base = nextTo(e, base[1:])
	}
	if read1(f, "", "sort_savefiles_by_content_enable") == "true" {
		if rom == "" {
			return ""
		}
		base = filepath.Join(base, filepath.Base(filepath.Dir(rom)))
	}
	if t := read1(f, "", "sort_savefiles_enable"); t == "" || t == "true" { // absent = on (default)
		base = filepath.Join(base, library(e.Cores[system]))
	}
	return base
}

func configRetroArch(e *Emulator) string {
	return first(nextTo(e, "retroarch.cfg"), xdgConfig("retroarch", "retroarch.cfg"),
		home(".var", "app", "org.libretro.RetroArch", "config", "retroarch", "retroarch.cfg"),
		home(".var", "app", "net.retrodeck.retrodeck", "config", "retroarch", "retroarch.cfg"),
		home("Library", "Application Support", "RetroArch", "config", "retroarch.cfg"))
}

func configMGBA(e *Emulator) string {
	if exists(nextTo(e, "portable.ini")) {
		return first(nextTo(e, "config.ini"))
	}
	return first(env("APPDATA", "mGBA", "config.ini"), xdgConfig("mgba", "config.ini"),
		home(".var", "app", "io.mgba.mGBA", "config", "mgba", "config.ini"))
}

// ponytail: order of the vbam.ini locations not checked (docs/notes/research-v3-emulateurs.md §3.6);
// `--print-cfg-path` if a real case differs.
func configVBAM(e *Emulator) string {
	return first(nextTo(e, "vbam.ini"), env("APPDATA", "visualboyadvance-m", "vbam.ini"), env("LOCALAPPDATA", "visualboyadvance-m", "vbam.ini"),
		home(".vbam", "vbam.ini"), xdgConfig("visualboyadvance-m", "vbam.ini"),
		home("Library", "Application Support", "visualboyadvance-m", "vbam.ini"))
}

// skyEmuDir (main.c se_load_rom): <ROM>.sav next to the ROM if it exists; else the "Save File/State Path" folder
// if the .sav is already there or if "Create new files in paths" is ticked. Binary settings in
// SDL_GetPrefPath("Sky", "SkyEmu"): search_paths.bin starts with this folder (text ended by 0); save_to_path is the
// 21st uint32 of user_settings.bin (frozen struct, "keep alignment and ordering the same").
func skyEmuDir(_ *Emulator, _ System, rom string) string {
	if rom == "" {
		return ""
	}
	sav := strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom)) + ".sav"
	pref := first(env("APPDATA", "Sky", "SkyEmu"), xdgData("Sky", "SkyEmu"), home("Library", "Application Support", "Sky", "SkyEmu"))
	p, _ := os.ReadFile(joinPath(pref, "search_paths.bin"))
	d, _, _ := strings.Cut(string(p), "\x00")
	if exists(filepath.Join(filepath.Dir(rom), sav)) || !filepath.IsAbs(d) { // empty = "./" (current folder): ignored
		return ""
	}
	r, _ := os.ReadFile(joinPath(pref, "user_settings.bin"))
	if exists(filepath.Join(d, sav)) || len(r) >= 84 && r[80]|r[81]|r[82]|r[83] != 0 {
		return filepath.Clean(d)
	}
	return ""
}

// read1 returns the raw value of a key (INI, TOML or retroarch.cfg). section "" = no section.
// ponytail: line by line, without full TOML or INI escapes; enough for paths and booleans.
func read1(file, section, key string) string {
	o, err := os.ReadFile(file)
	if file == "" || err != nil {
		return ""
	}
	inside := section == ""
	for _, l := range strings.Split(string(o), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			inside = section == "" || l == "["+section+"]"
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if inside && ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`) // no escapes: retroarch.cfg has none
		}
	}
	return ""
}

// setting is read1 for a path (~ expanded; \\ of the TOML and wx formats turned into one separator).
func setting(file, section, key string) string {
	v := read1(file, section, key)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "~/") {
		v = home(v[2:])
	}
	if !filepath.IsAbs(v) && !strings.HasPrefix(v, ":") && v != "default" { // relative (portable mGBA: savegamePath=savegame): to the folder of the settings file
		v = filepath.Join(filepath.Dir(file), v)
	}
	return filepath.Clean(v)
}

func exists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// first returns the first path that exists; "" if none.
func first(paths ...string) string {
	for _, c := range paths {
		if exists(c) {
			return c
		}
	}
	return ""
}

// joinPath builds a candidate path: "" if base is missing (never a path relative to the current folder).
func joinPath(base string, elements ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, elements...)...)
}

func nextTo(e *Emulator, elements ...string) string {
	if e.Exe == "" {
		return ""
	}
	return joinPath(filepath.Dir(e.Exe), elements...)
}

func env(variable string, elements ...string) string {
	return joinPath(os.Getenv(variable), elements...)
}

func home(elements ...string) string {
	h, _ := os.UserHomeDir()
	return joinPath(h, elements...)
}

func xdgConfig(elements ...string) string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return joinPath(d, elements...)
	}
	return home(append([]string{".config"}, elements...)...)
}

func xdgData(elements ...string) string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return joinPath(d, elements...)
	}
	return home(append([]string{".local", "share"}, elements...)...)
}
