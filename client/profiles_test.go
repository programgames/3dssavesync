package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeHome makes a fake home folder: no real emulator setting of the machine may count.
func fakeHome(t *testing.T) string {
	m := t.TempDir()
	for _, v := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		t.Setenv(v, filepath.Join(m, v))
	}
	// Shortcuts and autostart: never on the real desktop (Windows places them from the registry, not the environment).
	r, a, o := createShortcut, autostart, openDetached
	t.Cleanup(func() { createShortcut, autostart, openDetached = r, a, o })
	createShortcut = func(name, _ string, _ []string, _ bool) (string, error) { return "test:" + name, nil }
	autostart = func(name, _ string, _ []string) (string, error) { return "test:" + name, nil }
	openDetached = func(string, []string) error { return nil } // never a real emulator
	// Never a real tray icon or a real Server started, never the real Startup folder.
	l, ra := launchServer, removeAutostart
	t.Cleanup(func() { launchServer, removeAutostart = l, ra })
	launchServer = func(string, []string) error { return nil }
	removeAutostart = func(bool) error { return nil }
	rr, rp := removeShortcuts, removeProgram // uninstall: never the real shortcuts or the real program
	t.Cleanup(func() { removeShortcuts, removeProgram = rr, rp })
	removeShortcuts = func(string) ([]string, error) { return nil, nil }
	removeProgram = func(string) error { return nil }
	return m
}

func TestProfiles(t *testing.T) {
	m := fakeHome(t)
	d := func(name string) string { return filepath.Join(m, name) }
	emu := func(profile, exeDir string, e Emulator) (*Emulator, Profile) {
		e.Profile, e.Exe = profile, filepath.Join(d(exeDir), "emu.exe")
		p, err := e.profile(profile)
		if err != nil {
			t.Fatal(err)
		}
		return &e, p
	}
	equal := func(obtained, expected, message string) {
		t.Helper()
		if obtained != expected {
			t.Errorf("%s : %q, attendu %q", message, obtained, expected)
		}
	}
	rom := filepath.Join(m, "roms", "gb", "Pokemon Bleue.gb")

	// melonDS: melonDS.toml next to the exe (portable build)
	e, p := emu("melonds", "melon", Emulator{})
	write(t, filepath.Join(d("melon"), "melonDS.toml"), []byte("[Instance0]\nSaveFilePath = \""+filepath.ToSlash(d("Saves/DS"))+"\"\n[Instance0.Audio]\nSaveFilePath = \"faux\"\n"))
	equal(e.dir(p, "DS", rom), d("Saves/DS"), "melonDS lit SaveFilePath de [Instance0]")
	equal(savePath("x", "DS", "ADAF", "Pokemon Diamant", p.Ext), filepath.Join("x", "Pokemon Diamant.sav"), "melonDS : .sav")

	// Portable mGBA: savegamePath and autoload in config.ini
	e, p = emu("mgba", "mgba", Emulator{})
	equal(e.dir(p, "GB", rom), filepath.Dir(rom), "mGBA sans réglage : à côté de la ROM")
	equal(hint(e, p), "", "mGBA sans config.ini : rien à signaler")
	write(t, filepath.Join(d("mgba"), "portable.ini"), nil)
	write(t, filepath.Join(d("mgba"), "config.ini"), []byte("[gba.input.QT_K]\nautoload=0\n[ports.qt]\nautoload=1\nsavegamePath="+filepath.ToSlash(d("Saves/GB"))+"\n"))
	equal(e.dir(p, "GB", rom), d("Saves/GB"), "mGBA lit savegamePath de [ports.qt]")
	if !strings.Contains(hint(e, p), "autoload=0") {
		t.Errorf("mGBA autoload=1 : conseil attendu, obtenu %q", hint(e, p))
	}
	write(t, filepath.Join(d("mgba"), "config.ini"), []byte("[ports.qt]\nsavegamePath=savegame\n"))
	equal(e.dir(p, "GB", rom), filepath.Join(d("mgba"), "savegame"), "mGBA : savegamePath relatif au dossier de config.ini")
	write(t, filepath.Join(d("mgba"), "config.ini"), []byte("[ports.qt]\nautoload=0\n"))
	equal(hint(e, p), "", "mGBA autoload=0 : rien à signaler")
	write(t, filepath.Join(d("mgba"), "config.ini"), []byte("[ports.qt]\nfullscreen=1\n"))
	if hint(e, p) == "" {
		t.Error("mGBA sans clé autoload : actif par défaut, conseil attendu")
	}

	// Azahar: user folder (portable), then custom storage
	e, p = emu("azahar", "azahar", Emulator{})
	write(t, filepath.Join(d("azahar"), "user", "config", "qt-config.ini"), []byte("[Data%20Storage]\nuse_custom_storage=false\nsdmc_directory=ignore\n"))
	equal(e.dir(p, "3DS", ""), filepath.Join(d("azahar"), "user", "sdmc"), "Azahar portable : user/sdmc")
	write(t, filepath.Join(d("sdmc-perso"), "x"), nil)
	write(t, filepath.Join(d("azahar"), "user", "config", "qt-config.ini"),
		[]byte("[Data%20Storage]\nsdmc_directory\\default=false\nsdmc_directory="+filepath.ToSlash(d("sdmc-perso"))+"\nuse_custom_storage=true\n"))
	equal(e.dir(p, "3DS", ""), d("sdmc-perso"), "Azahar : sdmc_directory si use_custom_storage=true")
	write(t, filepath.Join(m, "APPDATA", "Azahar", "sdmc", "x"), nil)
	e, p = emu("azahar", "azahar-installe", Emulator{})
	equal(e.dir(p, "3DS", ""), filepath.Join(m, "APPDATA", "Azahar", "sdmc"), "Azahar installé : %APPDATA%\\Azahar\\sdmc")

	// RetroArch: savefile_directory + core subfolder; sort by content
	cores := map[System]string{"GB": "cores/gambatte_libretro.dll", "GBA": "cores/mgba_libretro.so", "DS": "melondsds_libretro.dylib"}
	e, p = emu("retroarch", "ra", Emulator{Cores: cores})
	equal(strings.Join(p.Args(e, "GB", rom, ""), "|"), "-L|cores/gambatte_libretro.dll|"+rom, "RetroArch : -L <cœur> <rom>")
	write(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \"default\"\n"))
	if runtime.GOOS != "darwin" { // macOS: ~/Documents/RetroArch/saves
		equal(e.dir(p, "GB", rom), filepath.Join(d("ra"), "saves", "Gambatte"), "RetroArch default : saves/<cœur> à côté de retroarch.cfg")
	}
	write(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \""+filepath.ToSlash(d("ra-saves"))+"\"\nsort_savefiles_by_content_enable = \"true\"\n"))
	equal(e.dir(p, "GBA", rom), filepath.Join(d("ra-saves"), "gb", "mGBA"), "RetroArch : tri par contenu puis par cœur")
	write(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \""+filepath.ToSlash(d("ra-saves"))+"\"\nsort_savefiles_enable = \"false\"\nsavestate_auto_load = \"true\"\n"))
	equal(e.dir(p, "DS", rom), d("ra-saves"), "RetroArch sans tri (EmuDeck) : le dossier seul")
	equal(savePath("x", "GB", "GB-X", "Pokemon Bleue", p.Ext), filepath.Join("x", "Pokemon Bleue.srm"), "RetroArch : .srm")
	if !strings.Contains(hint(e, p), "savestate_auto_load") {
		t.Error("RetroArch savestate_auto_load : conseil attendu")
	}
	write(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \"\"\n"))
	equal(e.dir(p, "GB", rom), filepath.Dir(rom), "RetroArch savefile_directory vide : à côté de la ROM")
	if _, err := (&Emulator{Profile: "retroarch"}).profile("retroarch"); err == nil {
		t.Error("RetroArch sans cœur : erreur attendue")
	}
	if _, err := (&Emulator{Cores: map[System]string{"GBA": "gearboy_libretro.dll"}}).profile("retroarch"); err == nil {
		t.Error("cœur non géré (Gearboy) : erreur attendue")
	}
	equal(library("cores/gpsp_libretro.dll")+"|"+library("mednafen_gba_libretro.so"), "gpSP|Beetle GBA", "cœurs gpSP et Beetle GBA gérés (srm.go)")
	if !e.handles(p, "DS") || e.handles(p, "3DS") {
		t.Error("RetroArch gère les Systèmes de ses cœurs, et eux seuls")
	}

	// VBA-M: relative BatteryDir, %s = System
	e, p = emu("vbam", "vbam", Emulator{})
	write(t, filepath.Join(d("vbam"), "vbam.ini"), []byte("[General]\nAutoLoadLastState=1\nBatteryDir=battery/%s\n"))
	equal(e.dir(p, "GBA", rom), filepath.Join(d("vbam"), "battery", "GameBoy Advance"), "VBA-M : BatteryDir relatif à l'exe, %s remplacé")
	if hint(e, p) == "" {
		t.Error("VBA-M AutoLoadLastState=1 : conseil attendu")
	}

	// SameBoy: always next to the ROM
	e, p = emu("sameboy", "sameboy", Emulator{})
	equal(e.dir(p, "GB", rom), filepath.Dir(rom), "SameBoy : à côté de la ROM")

	// SkyEmu: next to the ROM; folder of search_paths.bin if the .sav is there, or if save_to_path
	// (user_settings.bin) is ticked
	e, p = emu("skyemu", "sky", Emulator{})
	romSky := filepath.Join(m, "roms", "sky", "Pokemon Bleue.gb")
	equal(e.dir(p, "GB", romSky), filepath.Dir(romSky), "SkyEmu sans réglages : à côté de la ROM")
	pref := filepath.Join(m, "APPDATA", "Sky", "SkyEmu")
	write(t, filepath.Join(pref, "search_paths.bin"), append([]byte(filepath.ToSlash(d("sky-saves"))+"/"), make([]byte, 1024*8)...))
	settings := make([]byte, 1024)
	write(t, filepath.Join(pref, "user_settings.bin"), settings)
	equal(e.dir(p, "GB", romSky), filepath.Dir(romSky), "SkyEmu : .sav absent du dossier des réglages, save_to_path décoché")
	write(t, filepath.Join(d("sky-saves"), "Pokemon Bleue.sav"), nil)
	equal(e.dir(p, "GB", romSky), d("sky-saves"), "SkyEmu : .sav déjà dans le dossier des réglages")
	write(t, filepath.Join(filepath.Dir(romSky), "Pokemon Bleue.sav"), nil)
	equal(e.dir(p, "GB", romSky), filepath.Dir(romSky), "SkyEmu : .sav à côté de la ROM prioritaire")
	settings[80] = 1
	write(t, filepath.Join(pref, "user_settings.bin"), settings)
	equal(e.dir(p, "GBA", filepath.Join(m, "roms", "sky", "Pokemon Emeraude.gba")), d("sky-saves"), "SkyEmu save_to_path : dossier des réglages")
	equal(savePath("x", "DS", "ADAF", "Pokemon Diamant", p.Ext), filepath.Join("x", "Pokemon Diamant.sav"), "SkyEmu : .sav")
	write(t, filepath.Join(pref, "search_paths.bin"), []byte("./\x00"))
	equal(e.dir(p, "GBA", filepath.Join(m, "roms", "sky", "Pokemon Emeraude.gba")), filepath.Join(m, "roms", "sky"), "SkyEmu : dossier relatif ignoré")

	// NanoBoyAdvance: general.save_folder of config.toml next to the exe, only if the folder exists
	e, p = emu("nanoboyadvance", "nba", Emulator{})
	write(t, filepath.Join(d("nba"), "config.toml"), []byte("[cartridge]\nsave_folder = \"faux\"\n\n[general]\nbios_path = \"bios.bin\"\nsave_folder = \""+filepath.ToSlash(d("nba-saves"))+"\"\n"))
	equal(e.dir(p, "GBA", rom), filepath.Dir(rom), "NanoBoyAdvance : save_folder absent du disque, à côté de la ROM")
	write(t, filepath.Join(d("nba-saves"), "x"), nil)
	equal(e.dir(p, "GBA", rom), d("nba-saves"), "NanoBoyAdvance lit save_folder de [general]")
	equal(strings.Join(p.Args(e, "GBA", rom, ""), "|"), rom, "NanoBoyAdvance : la ROM seule")

	// BGB: next to the ROM (empty "Save files"); -rom optional, args_en_plus before the ROM
	e, p = emu("bgb", "bgb", Emulator{ExtraArgs: []string{"-nowarn"}})
	equal(e.dir(p, "GB", rom), filepath.Dir(rom), "BGB : à côté de la ROM")
	equal(strings.Join(e.command(p, "GB", rom, ""), "|"), e.Exe+"|-nowarn|"+rom, "BGB : exe, args_en_plus, ROM")
	if e.handles(p, "GBA") || !e.handles(p, "GB") {
		t.Error("BGB : GB seulement")
	}

	// config.json still wins
	e, p = emu("mgba", "mgba", Emulator{Saves: map[System]string{"GBA": "E:/GBA"}})
	equal(e.dir(p, "GBA", rom), "E:/GBA", "dossier de config.json prioritaire")
	if e.handles(p, "GB") {
		t.Error("dossiers dans config.json : seuls ces Systèmes sont gérés")
	}

	// Custom Profile
	e, p = emu("monemu", "perso", Emulator{Args: []string{"--batt={sauvegardes}", "{rom}"}, Extension: ".sav", Saves: map[System]string{"GBA": "S"}})
	equal(strings.Join(p.Args(e, "GBA", rom, "S"), "|"), "--batt=S|"+rom, "profil personnalisé : {sauvegardes} et {rom}")
	equal(strings.Join(p.Args(e, "GBA", "", "S"), "|"), "--batt=S", "profil personnalisé sans jeu : {rom} retiré")
	if _, err := (&Emulator{}).profile("inconnu"); err == nil {
		t.Error("profil inconnu sans args : erreur attendue")
	}
	equal((&Emulator{Exe: filepath.Join("Emu", "mGBA.EXE")}).process(), "mGBA", "processus : nom de l'exe sans .exe")
}

func TestTitles3DS(t *testing.T) {
	tmp = t.TempDir()
	config := writeConfig(t, filepath.Join(tmp, "config.json"), Config{URL: "http://x", Token: "j",
		Titles3DS: map[string]string{"00040000000ec300": "Jeu a moi"}})
	t.Cleanup(func() { delete(titles3DS, "00040000000EC300") })
	if _, err := newSession(config); err != nil {
		t.Fatal(err)
	}
	r, err := readRom(rom3DS(t, "Mon jeu", 0x00040000000EC300))
	if err != nil || r.Key != "3DS-00040000000EC300" || r.Name != "Jeu a moi" {
		t.Errorf("titres_3ds : Clé %q, nom %q, erreur %v", r.Key, r.Name, err)
	}
}

// GB title with a byte >= 0x80: same Key as key.c (3ds/test/test_key.c), no panic.
func TestGBKey(t *testing.T) {
	tmp = t.TempDir()
	rom := filepath.Join(tmp, "Pokemon.gb")
	write(t, rom, gbHeader("Pok\xe9mon\x99", 0x91E6))
	r, err := readRom(rom)
	if err != nil || r.Key != "GB-POK_MON_-91E6" {
		t.Errorf("Clé %q, erreur %v", r.Key, err)
	}
}

func TestCommandAndSplit(t *testing.T) {
	e := &Emulator{Exe: "mGBA.exe", ExtraArgs: []string{"-f"}}
	p, _ := e.profile("mgba")
	if c := strings.Join(e.command(p, "GBA", "r.gba", "S"), "|"); c != "mGBA.exe|-C|savegamePath=S|-f|r.gba" {
		t.Errorf("arguments en plus avant la ROM : %s", c)
	}
	if c := strings.Join(e.command(p, "", "", ""), "|"); c != "mGBA.exe|-f" {
		t.Errorf("launch : arguments en plus à la fin : %s", c)
	}
	a := split(`  -rom "{rom}"  --dir='C:\Mes jeux\saves' -f `)
	if strings.Join(a, "|") != `-rom|{rom}|--dir=C:\Mes jeux\saves|-f` {
		t.Errorf("decouper : %q", a)
	}
	if j := joinArgs(a); j != `-rom {rom} "--dir=C:\Mes jeux\saves" -f` || strings.Join(split(j), "|") != strings.Join(a, "|") {
		t.Errorf("joindre : %s", j)
	}
}

// Scoop shim: scoop\shims\SkyEmu.exe (in the PATH) and SkyEmu.shim next to it, which names the real exe. Only one
// SkyEmu found, the real exe (not two, PATH and scoop).
func TestDetectScoopShim(t *testing.T) {
	m := fakeHome(t)
	real, shims := home("scoop", "apps", "skyemu", "current", "SkyEmu-v5-Windows.exe"), home("scoop", "shims")
	write(t, real, []byte("exe"))
	write(t, filepath.Join(shims, "SkyEmu.exe"), []byte("shim"))
	write(t, filepath.Join(shims, "SkyEmu.shim"), []byte("path = \""+real+"\"\r\n"))
	t.Setenv("PATH", shims)
	if c := shimTarget(filepath.Join(shims, "SkyEmu.exe")); c != real {
		t.Errorf("cible du shim : %q", c)
	}
	if c := shimTarget(real); c != real {
		t.Errorf("exe sans shim : %q", c)
	}
	var l []Found
	for _, x := range detect("") {
		if x.Profile == "skyemu" && strings.HasPrefix(x.Exe, m) {
			l = append(l, x)
		}
	}
	if len(l) != 1 || l[0].Exe != real {
		t.Errorf("un seul SkyEmu, l'exe réel : %+v", l)
	}
}
