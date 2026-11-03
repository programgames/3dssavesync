package main

// Profils d'émulateurs (phase 2) : lancement, dossier et nom des sauvegardes, rechargement d'un état.
// Sources : docs/research-v3-emulateurs.md (chemins par système, Flatpak, EmuDeck, RetroDECK).

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type Profile struct {
	Systems      []string                                            // nil : ceux de la configuration (cœurs RetroArch, dossiers d'un profil personnalisé)
	Ext          string                                              // extension de la sauvegarde ; défaut .sav
	InternalName bool                                                // ROM en archive : sauvegarde au nom du fichier interne (melonDS), sinon au nom de l'archive
	Args         func(e *Emulator, system, rom, dir string) []string // rom vide : émulateur sans jeu (launch)
	Dir          func(e *Emulator, system, rom string) string        // réglage de l'émulateur ; "" = inconnu
	Autoload     func(e *Emulator) string                            // conseil si un état est rechargé au lancement
}

var profiles = map[string]Profile{
	"melonds": {
		Systems: []string{"DS"}, InternalName: true,
		Args: func(_ *Emulator, _, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, _, _ string) string {
			f := first(nextTo(e, "melonDS.toml"), nextTo(e, "portable", "melonDS.toml"), env("LOCALAPPDATA", "melonDS", "melonDS.toml"),
				xdgConfig("melonDS", "melonDS.toml"), home("Library", "Preferences", "melonDS", "melonDS.toml"),
				home(".var", "app", "net.kuribo64.melonDS", "config", "melonDS", "melonDS.toml"),
				home(".var", "app", "net.retrodeck.retrodeck", "config", "melonDS", "melonDS.toml"))
			return setting(f, "Instance0", "SaveFilePath")
		},
	},
	"mgba": {
		Systems: []string{"GB", "GBA"},
		Args: func(_ *Emulator, _, rom, dir string) []string {
			if dir == "" {
				return romArgs(rom)
			}
			return append([]string{"-C", "savegamePath=" + dir}, romArgs(rom)...) // jamais --config (mGBA quitte)
		},
		Dir: func(e *Emulator, _, _ string) string { return setting(configMGBA(e), "ports.qt", "savegamePath") },
		Autoload: func(e *Emulator) string {
			f := configMGBA(e)
			if f != "" && read1(f, "ports.qt", "autoload") != "0" { // absent = actif (défaut de mGBA)
				return tr("mGBA recharge automatiquement l'état du slot 0 au lancement. Mettez autoload=0 dans ", "mGBA automatically reloads the slot 0 save state at launch. Set autoload=0 in ") + f + "."
			}
			return ""
		},
	},
	"azahar": {
		Systems: []string{"3DS"},
		Args: func(_ *Emulator, _, rom, _ string) []string {
			if rom == "" {
				return nil
			}
			return []string{"-f", rom}
		},
		Dir: func(e *Emulator, _, _ string) string {
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
		Args: func(e *Emulator, system, rom, _ string) []string {
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
		Systems: []string{"GB", "GBA"},
		Args:    func(_ *Emulator, _, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, system, _ string) string {
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
	"desmume": { // .dsv : SRAM + pied de page (dsv.go)
		Systems: []string{"DS"}, Ext: ".dsv", InternalName: true,
		Args: func(_ *Emulator, _, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, _, _ string) string {
			switch runtime.GOOS {
			case "windows": // desmume.ini à côté de l'exe ; Battery absent = .\Battery (path.cpp, winutil.cpp)
				if d := setting(nextTo(e, "desmume.ini"), "PathSettings", "Battery"); d != "" {
					return d
				}
				return nextTo(e, "Battery")
			case "linux": // GTK : tout dans le dossier de configuration (path.cpp LoadModulePath)
				if d := first(xdgConfig("desmume"), home(".var", "app", "org.desmume.DeSmuME", "config", "desmume")); d != "" {
					return d
				}
				return xdgConfig("desmume")
			}
			return ""
		},
	},
	"sameboy": { // .sav toujours à côté de la ROM
		Systems: []string{"GB"},
		Args:    func(_ *Emulator, _, rom, _ string) []string { return romArgs(rom) },
	},
	"skyemu": { // .sav brut, sans Trailer d'horloge (main.c se_write_save_to_disk)
		Systems: []string{"DS", "GB", "GBA"},
		Args:    func(_ *Emulator, _, rom, _ string) []string { return romArgs(rom) },
		Dir:     skyEmuDir,
	},
	"nanoboyadvance": { // .sav brut ; horloge prise sur le PC, jamais écrite (rtc.cc)
		Systems: []string{"GBA"},
		// ponytail: la ROM doit rester argv[1] (main.cc) ; args_en_plus casse le lancement, champ args si besoin.
		Args: func(_ *Emulator, _, rom, _ string) []string { return romArgs(rom) },
		Dir: func(e *Emulator, _, _ string) string {
			// Windows, Linux : config.toml dans le dossier courant, celui de l'exe quand une ROM est donnée (main.cc, PORTABLE_MODE forcé)
			f := first(nextTo(e, "config.toml"), home("Library", "Application Support", "org.github.fleroviux.NanoBoyAdvance", "config.toml"))
			if d := setting(f, "general", "save_folder"); exists(d) { // vide ou absent : à côté de la ROM (main_window.cc GetSavePath)
				return d
			}
			return ""
		},
	},
	"bgb": { // .sav + Trailer d'horloge de 48 octets ; clé du réglage « Save files » non documentée : champ sauvegardes
		Systems: []string{"GB"},
		Args:    func(_ *Emulator, _, rom, _ string) []string { return romArgs(rom) }, // « [-rom] <fichier> » : -rom facultatif
	},
}

// library_name des cœurs RetroArch gérés : nom du sous-dossier quand sort_savefiles_enable est actif.
var libraries = map[string]string{
	"melondsds": "melonDS DS", "mgba": "mGBA", "gambatte": "Gambatte", "sameboy": "SameBoy", "vbam": "VBA-M",
	"gpsp": "gpSP", "mednafen_gba": "Beetle GBA", // .srm de taille fixe, converti (srm.go)
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

// Profil d'une entrée de la configuration : intégré, ou personnalisé (champ args).
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
	return Profile{Ext: e.Extension, InternalName: e.InternalName, Args: func(e *Emulator, _, rom, dir string) []string {
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

// Commande complète : exécutable, arguments du profil, et ArgsEnPlus juste avant la ROM (à la fin sans ROM).
func (e *Emulator) command(p Profile, system, rom, dir string) []string {
	a := p.Args(e, system, rom, dir)
	if n := len(a); rom != "" && n > 0 && a[n-1] == rom {
		a = append(append(append([]string{}, a[:n-1]...), e.ExtraArgs...), rom)
	} else {
		a = append(a, e.ExtraArgs...)
	}
	return append([]string{e.Exe}, a...)
}

// decouper : une ligne de commande saisie par l'utilisateur -> arguments. Espaces = séparateurs, sauf entre
// guillemets doubles ou simples ; la barre oblique inverse reste telle quelle (chemins Windows).
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

// joindre : l'inverse de decouper, pour afficher une commande (guillemets autour d'un argument avec espace).
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

// Systèmes gérés : ceux de la configuration s'il y en a, sinon ceux du profil.
func (e *Emulator) handles(p Profile, system string) bool {
	if len(e.Saves) > 0 || len(e.Cores) > 0 {
		return e.Saves[system] != "" || e.Cores[system] != ""
	}
	return slices.Contains(p.Systems, system)
}

func (e *Emulator) systems(p Profile) (s []string) {
	for _, x := range []string{"DS", "GB", "GBA", "3DS"} {
		if e.handles(p, x) {
			s = append(s, x)
		}
	}
	return s
}

// Dossier des sauvegardes : config.json, sinon le réglage de l'émulateur, sinon à côté de la ROM
// (jamais pour la 3DS : sans sdmc connu, "").
func (e *Emulator) dir(p Profile, system, rom string) string {
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

// Nom du processus à surveiller : champ processus, sinon le nom de l'exécutable.
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

// RetroArch : savefile_directory, puis sous-dossier du contenu et du cœur selon les options de tri.
func retroArchDir(e *Emulator, system, rom string) string {
	f := configRetroArch(e)
	if read1(f, "", "savefiles_in_content_dir") == "true" {
		return ""
	}
	base := setting(f, "", "savefile_directory")
	switch {
	case base == "" && f != "": // vide = dossier de la ROM
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
	case strings.HasPrefix(base, ":"): // :\saves = relatif au dossier de retroarch.exe
		base = nextTo(e, base[1:])
	}
	if read1(f, "", "sort_savefiles_by_content_enable") == "true" {
		if rom == "" {
			return ""
		}
		base = filepath.Join(base, filepath.Base(filepath.Dir(rom)))
	}
	if t := read1(f, "", "sort_savefiles_enable"); t == "" || t == "true" { // absent = actif (défaut)
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

// ponytail: ordre des emplacements de vbam.ini non vérifié (research §3.6) ; `--print-cfg-path` si un cas réel diffère.
func configVBAM(e *Emulator) string {
	return first(nextTo(e, "vbam.ini"), env("APPDATA", "visualboyadvance-m", "vbam.ini"), env("LOCALAPPDATA", "visualboyadvance-m", "vbam.ini"),
		home(".vbam", "vbam.ini"), xdgConfig("visualboyadvance-m", "vbam.ini"),
		home("Library", "Application Support", "visualboyadvance-m", "vbam.ini"))
}

// SkyEmu (main.c se_load_rom) : <ROM>.sav à côté de la ROM s'il existe ; sinon le dossier « Save File/State Path »
// si le .sav y est déjà ou si « Create new files in paths » est coché. Réglages binaires dans SDL_GetPrefPath("Sky", "SkyEmu") :
// search_paths.bin commence par ce dossier (texte fini par 0) ; save_to_path est le 21e uint32 de user_settings.bin
// (structure figée, « keep alignment and ordering the same »).
func skyEmuDir(_ *Emulator, _, rom string) string {
	if rom == "" {
		return ""
	}
	sav := strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom)) + ".sav"
	pref := first(env("APPDATA", "Sky", "SkyEmu"), xdgData("Sky", "SkyEmu"), home("Library", "Application Support", "Sky", "SkyEmu"))
	p, _ := os.ReadFile(joinPath(pref, "search_paths.bin"))
	d, _, _ := strings.Cut(string(p), "\x00")
	if exists(filepath.Join(filepath.Dir(rom), sav)) || !filepath.IsAbs(d) { // vide = "./" (dossier courant) : ignoré
		return ""
	}
	r, _ := os.ReadFile(joinPath(pref, "user_settings.bin"))
	if exists(filepath.Join(d, sav)) || len(r) >= 84 && r[80]|r[81]|r[82]|r[83] != 0 {
		return filepath.Clean(d)
	}
	return ""
}

// lire1 : valeur brute d'une clé (INI, TOML ou retroarch.cfg). section "" = pas de section.
// ponytail: lecture ligne à ligne, sans échappements TOML ou INI complets ; suffit pour des chemins et des booléens.
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
			return strings.Trim(strings.TrimSpace(v), `"'`) // pas d'échappements : retroarch.cfg n'en a pas
		}
	}
	return ""
}

// reglage : comme lire1, pour un chemin (~ développé ; \\ des formats TOML et wx ramenés à un séparateur).
func setting(file, section, key string) string {
	v := read1(file, section, key)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "~/") {
		v = home(v[2:])
	}
	if !filepath.IsAbs(v) && !strings.HasPrefix(v, ":") && v != "default" { // relatif (mGBA portable : savegamePath=savegame) : au dossier du réglage
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

// Premier chemin existant ; "" sinon.
func first(paths ...string) string {
	for _, c := range paths {
		if exists(c) {
			return c
		}
	}
	return ""
}

// Chemins candidats : "" si la base manque (jamais un chemin relatif au dossier courant).
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
