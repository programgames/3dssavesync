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

type Profil struct {
	Systemes   []string                                                  // nil : ceux de la configuration (cœurs RetroArch, dossiers d'un profil personnalisé)
	Ext        string                                                    // extension de la sauvegarde ; défaut .sav
	NomInterne bool                                                      // ROM en archive : sauvegarde au nom du fichier interne (melonDS), sinon au nom de l'archive
	Args       func(e *Emulateur, systeme, rom, dossier string) []string // rom vide : émulateur sans jeu (launch)
	Dossier    func(e *Emulateur, systeme, rom string) string            // réglage de l'émulateur ; "" = inconnu
	Autoload   func(e *Emulateur) string                                 // conseil si un état est rechargé au lancement
}

var profils = map[string]Profil{
	"melonds": {
		Systemes: []string{"DS"}, NomInterne: true,
		Args: func(_ *Emulateur, _, rom, _ string) []string { return seul(rom) },
		Dossier: func(e *Emulateur, _, _ string) string {
			f := premier(pres(e, "melonDS.toml"), pres(e, "portable", "melonDS.toml"), env("LOCALAPPDATA", "melonDS", "melonDS.toml"),
				xdgConfig("melonDS", "melonDS.toml"), maison("Library", "Preferences", "melonDS", "melonDS.toml"),
				maison(".var", "app", "net.kuribo64.melonDS", "config", "melonDS", "melonDS.toml"),
				maison(".var", "app", "net.retrodeck.retrodeck", "config", "melonDS", "melonDS.toml"))
			return reglage(f, "Instance0", "SaveFilePath")
		},
	},
	"mgba": {
		Systemes: []string{"GB", "GBA"},
		Args: func(_ *Emulateur, _, rom, dossier string) []string {
			if dossier == "" {
				return seul(rom)
			}
			return append([]string{"-C", "savegamePath=" + dossier}, seul(rom)...) // jamais --config (mGBA quitte)
		},
		Dossier: func(e *Emulateur, _, _ string) string { return reglage(configMGBA(e), "ports.qt", "savegamePath") },
		Autoload: func(e *Emulateur) string {
			f := configMGBA(e)
			if f != "" && lire1(f, "ports.qt", "autoload") != "0" { // absent = actif (défaut de mGBA)
				return tr("mGBA recharge l'état du slot 0 au lancement d'un jeu (réglage autoload). Mets autoload=0 dans ", "mGBA reloads the slot 0 state when a game starts (autoload setting). Set autoload=0 in ") + f + "."
			}
			return ""
		},
	},
	"azahar": {
		Systemes: []string{"3DS"},
		Args: func(_ *Emulateur, _, rom, _ string) []string {
			if rom == "" {
				return nil
			}
			return []string{"-f", rom}
		},
		Dossier: func(e *Emulateur, _, _ string) string {
			user := premier(pres(e, "user"), env("APPDATA", "Azahar"), xdgData("azahar-emu"), maison("Library", "Application Support", "Azahar"),
				maison(".var", "app", "org.azahar_emu.Azahar", "data", "azahar-emu"))
			f := premier(rejoindre(user, "config", "qt-config.ini"), xdgConfig("azahar-emu", "qt-config.ini"),
				maison(".var", "app", "org.azahar_emu.Azahar", "config", "azahar-emu", "qt-config.ini"),
				maison(".var", "app", "net.retrodeck.retrodeck", "config", "azahar-emu", "qt-config.ini"))
			if lire1(f, "Data%20Storage", "use_custom_storage") == "true" {
				if d := reglage(f, "Data%20Storage", "sdmc_directory"); existe(d) {
					return d
				}
			}
			return rejoindre(user, "sdmc")
		},
	},
	"retroarch": {
		Ext: ".srm",
		Args: func(e *Emulateur, systeme, rom, _ string) []string {
			if rom == "" {
				return nil
			}
			return []string{"-L", e.Coeurs[systeme], rom}
		},
		Dossier: dossierRetroArch,
		Autoload: func(e *Emulateur) string {
			if f := configRetroArch(e); lire1(f, "", "savestate_auto_load") == "true" {
				return tr("RetroArch recharge l'état automatique au lancement (savestate_auto_load). Mets-le à false dans ", "RetroArch reloads the auto state at start (savestate_auto_load). Set it to false in ") + f + "."
			}
			return ""
		},
	},
	"vbam": {
		Systemes: []string{"GB", "GBA"},
		Args:     func(_ *Emulateur, _, rom, _ string) []string { return seul(rom) },
		Dossier: func(e *Emulateur, systeme, _ string) string {
			d := reglage(configVBAM(e), "General", "BatteryDir")
			if d == "" {
				return ""
			}
			s := "GameBoy"
			if systeme == "GBA" {
				s = "GameBoy Advance"
			}
			d = strings.ReplaceAll(d, "%s", s)
			if !filepath.IsAbs(d) {
				d = filepath.Join(filepath.Dir(e.Exe), d)
			}
			return filepath.Clean(d)
		},
		Autoload: func(e *Emulateur) string {
			if f := configVBAM(e); lire1(f, "General", "AutoLoadLastState") == "1" {
				return tr("VBA-M recharge le dernier état au lancement, sans lire la sauvegarde (AutoLoadLastState). Mets-le à 0 dans ", "VBA-M reloads the last state at start, without reading the save (AutoLoadLastState). Set it to 0 in ") + f + "."
			}
			return ""
		},
	},
	"desmume": { // .dsv : SRAM + pied de page (dsv.go)
		Systemes: []string{"DS"}, Ext: ".dsv", NomInterne: true,
		Args: func(_ *Emulateur, _, rom, _ string) []string { return seul(rom) },
		Dossier: func(e *Emulateur, _, _ string) string {
			switch runtime.GOOS {
			case "windows": // desmume.ini à côté de l'exe ; Battery absent = .\Battery (path.cpp, winutil.cpp)
				if d := reglage(pres(e, "desmume.ini"), "PathSettings", "Battery"); d != "" {
					return d
				}
				return pres(e, "Battery")
			case "linux": // GTK : tout dans le dossier de configuration (path.cpp LoadModulePath)
				if d := premier(xdgConfig("desmume"), maison(".var", "app", "org.desmume.DeSmuME", "config", "desmume")); d != "" {
					return d
				}
				return xdgConfig("desmume")
			}
			return ""
		},
	},
	"sameboy": { // .sav toujours à côté de la ROM
		Systemes: []string{"GB"},
		Args:     func(_ *Emulateur, _, rom, _ string) []string { return seul(rom) },
	},
	"skyemu": { // .sav brut, sans Trailer d'horloge (main.c se_write_save_to_disk)
		Systemes: []string{"DS", "GB", "GBA"},
		Args:     func(_ *Emulateur, _, rom, _ string) []string { return seul(rom) },
		Dossier:  dossierSkyEmu,
	},
	"nanoboyadvance": { // .sav brut ; horloge prise sur le PC, jamais écrite (rtc.cc)
		Systemes: []string{"GBA"},
		// ponytail: la ROM doit rester argv[1] (main.cc) ; args_en_plus casse le lancement, champ args si besoin.
		Args: func(_ *Emulateur, _, rom, _ string) []string { return seul(rom) },
		Dossier: func(e *Emulateur, _, _ string) string {
			// Windows, Linux : config.toml dans le dossier courant, celui de l'exe quand une ROM est donnée (main.cc, PORTABLE_MODE forcé)
			f := premier(pres(e, "config.toml"), maison("Library", "Application Support", "org.github.fleroviux.NanoBoyAdvance", "config.toml"))
			if d := reglage(f, "general", "save_folder"); existe(d) { // vide ou absent : à côté de la ROM (main_window.cc GetSavePath)
				return d
			}
			return ""
		},
	},
	"bgb": { // .sav + Trailer d'horloge de 48 octets ; clé du réglage « Save files » non documentée : champ sauvegardes
		Systemes: []string{"GB"},
		Args:     func(_ *Emulateur, _, rom, _ string) []string { return seul(rom) }, // « [-rom] <fichier> » : -rom facultatif
	},
}

// library_name des cœurs RetroArch gérés : nom du sous-dossier quand sort_savefiles_enable est actif.
var bibliotheques = map[string]string{
	"melondsds": "melonDS DS", "mgba": "mGBA", "gambatte": "Gambatte", "sameboy": "SameBoy", "vbam": "VBA-M",
}

func bibliotheque(coeur string) string {
	b, _, _ := strings.Cut(strings.ToLower(filepath.Base(coeur)), "_libretro")
	return bibliotheques[b]
}

func seul(rom string) []string {
	if rom == "" {
		return nil
	}
	return []string{rom}
}

// Profil d'une entrée de la configuration : intégré, ou personnalisé (champ args).
func (e *Emulateur) profil(nom string) (Profil, error) {
	p := e.Profil
	if p == "" {
		p = nom
	}
	if p == "retroarch" {
		if len(e.Coeurs) == 0 {
			return Profil{}, fmt.Errorf(tr("%s : RetroArch demande un cœur par Système (champ coeurs)", "%s: RetroArch needs one core per system (coeurs field)"), nom)
		}
		for _, c := range e.Coeurs {
			if bibliotheque(c) == "" {
				return Profil{}, fmt.Errorf(tr("cœur RetroArch non géré : %s (gérés : melonDS DS, mGBA, Gambatte, SameBoy, VBA-M)", "RetroArch core not supported: %s (supported: melonDS DS, mGBA, Gambatte, SameBoy, VBA-M)"), c)
			}
		}
	}
	if pr, ok := profils[p]; ok {
		return pr, nil
	}
	if e.Args == nil {
		return Profil{}, fmt.Errorf(tr("profil d'émulateur inconnu : %s (un profil personnalisé demande le champ args)", "unknown emulator profile: %s (a custom profile needs the args field)"), p)
	}
	return Profil{Ext: e.Extension, NomInterne: e.NomInterne, Args: func(e *Emulateur, _, rom, dossier string) []string {
		var a []string
		for _, x := range e.Args {
			if strings.Contains(x, "{rom}") && rom == "" {
				continue
			}
			a = append(a, strings.NewReplacer("{rom}", rom, "{sauvegardes}", dossier).Replace(x))
		}
		return a
	}}, nil
}

// Commande complète : exécutable, arguments du profil, et ArgsEnPlus juste avant la ROM (à la fin sans ROM).
func (e *Emulateur) commande(p Profil, systeme, rom, dossier string) []string {
	a := p.Args(e, systeme, rom, dossier)
	if n := len(a); rom != "" && n > 0 && a[n-1] == rom {
		a = append(append(append([]string{}, a[:n-1]...), e.ArgsEnPlus...), rom)
	} else {
		a = append(a, e.ArgsEnPlus...)
	}
	return append([]string{e.Exe}, a...)
}

// decouper : une ligne de commande saisie par l'utilisateur -> arguments. Espaces = séparateurs, sauf entre
// guillemets doubles ou simples ; la barre oblique inverse reste telle quelle (chemins Windows).
func decouper(ligne string) []string {
	var a []string
	var courant strings.Builder
	guillemet, dans := rune(0), false
	for _, c := range ligne {
		switch {
		case guillemet != 0 && c == guillemet:
			guillemet = 0
		case guillemet == 0 && (c == '"' || c == '\''):
			guillemet, dans = c, true
		case guillemet == 0 && (c == ' ' || c == '\t'):
			if dans {
				a = append(a, courant.String())
				courant.Reset()
				dans = false
			}
		default:
			courant.WriteRune(c)
			dans = true
		}
	}
	if dans {
		a = append(a, courant.String())
	}
	return a
}

// joindre : l'inverse de decouper, pour afficher une commande (guillemets autour d'un argument avec espace).
func joindre(a []string) string {
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
func (e *Emulateur) gere(p Profil, systeme string) bool {
	if len(e.Sauvegardes) > 0 || len(e.Coeurs) > 0 {
		return e.Sauvegardes[systeme] != "" || e.Coeurs[systeme] != ""
	}
	return slices.Contains(p.Systemes, systeme)
}

func (e *Emulateur) systemes(p Profil) (s []string) {
	for _, x := range []string{"DS", "GB", "GBA", "3DS"} {
		if e.gere(p, x) {
			s = append(s, x)
		}
	}
	return s
}

// Dossier des sauvegardes : config.json, sinon le réglage de l'émulateur, sinon à côté de la ROM
// (jamais pour la 3DS : sans sdmc connu, "").
func (e *Emulateur) dossier(p Profil, systeme, rom string) string {
	if d := e.Sauvegardes[systeme]; d != "" {
		return d
	}
	if p.Dossier != nil {
		if d := p.Dossier(e, systeme, rom); d != "" {
			return d
		}
	}
	if rom == "" || systeme == "3DS" {
		return ""
	}
	return filepath.Dir(rom)
}

// Nom du processus à surveiller : champ processus, sinon le nom de l'exécutable.
func (e *Emulateur) processus() string {
	if e.Processus != "" {
		return e.Processus
	}
	b := filepath.Base(e.Exe)
	if strings.EqualFold(filepath.Ext(b), ".exe") {
		b = b[:len(b)-4]
	}
	return b
}

// RetroArch : savefile_directory, puis sous-dossier du contenu et du cœur selon les options de tri.
func dossierRetroArch(e *Emulateur, systeme, rom string) string {
	f := configRetroArch(e)
	if lire1(f, "", "savefiles_in_content_dir") == "true" {
		return ""
	}
	base := reglage(f, "", "savefile_directory")
	switch {
	case base == "" && f != "": // vide = dossier de la ROM
		return ""
	case base == "" || base == "default":
		switch {
		case runtime.GOOS == "darwin":
			base = maison("Documents", "RetroArch", "saves")
		case f != "":
			base = filepath.Join(filepath.Dir(f), "saves")
		case runtime.GOOS == "windows":
			base = pres(e, "saves")
		default:
			base = xdgConfig("retroarch", "saves")
		}
	case strings.HasPrefix(base, ":"): // :\saves = relatif au dossier de retroarch.exe
		base = pres(e, base[1:])
	}
	if lire1(f, "", "sort_savefiles_by_content_enable") == "true" {
		if rom == "" {
			return ""
		}
		base = filepath.Join(base, filepath.Base(filepath.Dir(rom)))
	}
	if t := lire1(f, "", "sort_savefiles_enable"); t == "" || t == "true" { // absent = actif (défaut)
		base = filepath.Join(base, bibliotheque(e.Coeurs[systeme]))
	}
	return base
}

func configRetroArch(e *Emulateur) string {
	return premier(pres(e, "retroarch.cfg"), xdgConfig("retroarch", "retroarch.cfg"),
		maison(".var", "app", "org.libretro.RetroArch", "config", "retroarch", "retroarch.cfg"),
		maison(".var", "app", "net.retrodeck.retrodeck", "config", "retroarch", "retroarch.cfg"),
		maison("Library", "Application Support", "RetroArch", "config", "retroarch.cfg"))
}

func configMGBA(e *Emulateur) string {
	if existe(pres(e, "portable.ini")) {
		return premier(pres(e, "config.ini"))
	}
	return premier(env("APPDATA", "mGBA", "config.ini"), xdgConfig("mgba", "config.ini"),
		maison(".var", "app", "io.mgba.mGBA", "config", "mgba", "config.ini"))
}

// ponytail: ordre des emplacements de vbam.ini non vérifié (research §3.6) ; `--print-cfg-path` si un cas réel diffère.
func configVBAM(e *Emulateur) string {
	return premier(pres(e, "vbam.ini"), env("APPDATA", "visualboyadvance-m", "vbam.ini"), env("LOCALAPPDATA", "visualboyadvance-m", "vbam.ini"),
		maison(".vbam", "vbam.ini"), xdgConfig("visualboyadvance-m", "vbam.ini"),
		maison("Library", "Application Support", "visualboyadvance-m", "vbam.ini"))
}

// SkyEmu (main.c se_load_rom) : <ROM>.sav à côté de la ROM s'il existe ; sinon le dossier « Save File/State Path »
// si le .sav y est déjà ou si « Create new files in paths » est coché. Réglages binaires dans SDL_GetPrefPath("Sky", "SkyEmu") :
// search_paths.bin commence par ce dossier (texte fini par 0) ; save_to_path est le 21e uint32 de user_settings.bin
// (structure figée, « keep alignment and ordering the same »).
func dossierSkyEmu(_ *Emulateur, _, rom string) string {
	if rom == "" {
		return ""
	}
	sav := strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom)) + ".sav"
	pref := premier(env("APPDATA", "Sky", "SkyEmu"), xdgData("Sky", "SkyEmu"), maison("Library", "Application Support", "Sky", "SkyEmu"))
	p, _ := os.ReadFile(rejoindre(pref, "search_paths.bin"))
	d, _, _ := strings.Cut(string(p), "\x00")
	if existe(filepath.Join(filepath.Dir(rom), sav)) || !filepath.IsAbs(d) { // vide = "./" (dossier courant) : ignoré
		return ""
	}
	r, _ := os.ReadFile(rejoindre(pref, "user_settings.bin"))
	if existe(filepath.Join(d, sav)) || len(r) >= 84 && r[80]|r[81]|r[82]|r[83] != 0 {
		return filepath.Clean(d)
	}
	return ""
}

// lire1 : valeur brute d'une clé (INI, TOML ou retroarch.cfg). section "" = pas de section.
// ponytail: lecture ligne à ligne, sans échappements TOML ou INI complets ; suffit pour des chemins et des booléens.
func lire1(fichier, section, cle string) string {
	o, err := os.ReadFile(fichier)
	if fichier == "" || err != nil {
		return ""
	}
	dans := section == ""
	for _, l := range strings.Split(string(o), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			dans = section == "" || l == "["+section+"]"
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if dans && ok && strings.TrimSpace(k) == cle {
			return strings.Trim(strings.TrimSpace(v), `"'`) // pas d'échappements : retroarch.cfg n'en a pas
		}
	}
	return ""
}

// reglage : comme lire1, pour un chemin (~ développé ; \\ des formats TOML et wx ramenés à un séparateur).
func reglage(fichier, section, cle string) string {
	v := lire1(fichier, section, cle)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "~/") {
		v = maison(v[2:])
	}
	if !filepath.IsAbs(v) && !strings.HasPrefix(v, ":") && v != "default" { // relatif (mGBA portable : savegamePath=savegame) : au dossier du réglage
		v = filepath.Join(filepath.Dir(fichier), v)
	}
	return filepath.Clean(v)
}

func existe(chemin string) bool {
	if chemin == "" {
		return false
	}
	_, err := os.Stat(chemin)
	return err == nil
}

// Premier chemin existant ; "" sinon.
func premier(chemins ...string) string {
	for _, c := range chemins {
		if existe(c) {
			return c
		}
	}
	return ""
}

// Chemins candidats : "" si la base manque (jamais un chemin relatif au dossier courant).
func rejoindre(base string, elements ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, elements...)...)
}

func pres(e *Emulateur, elements ...string) string {
	if e.Exe == "" {
		return ""
	}
	return rejoindre(filepath.Dir(e.Exe), elements...)
}

func env(variable string, elements ...string) string {
	return rejoindre(os.Getenv(variable), elements...)
}

func maison(elements ...string) string {
	h, _ := os.UserHomeDir()
	return rejoindre(h, elements...)
}

func xdgConfig(elements ...string) string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return rejoindre(d, elements...)
	}
	return maison(append([]string{".config"}, elements...)...)
}

func xdgData(elements ...string) string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return rejoindre(d, elements...)
	}
	return maison(append([]string{".local", "share"}, elements...)...)
}
