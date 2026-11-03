package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Dossier personnel factice : aucun réglage réel d'un émulateur de la machine ne doit compter.
func maisonFactice(t *testing.T) string {
	m := t.TempDir()
	for _, v := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		t.Setenv(v, filepath.Join(m, v))
	}
	// Raccourcis et démarrage automatique : jamais sur le vrai bureau (Windows les place selon le registre, pas l'environnement).
	r, a, o := creerRaccourci, autostart, ouvrirDetache
	t.Cleanup(func() { creerRaccourci, autostart, ouvrirDetache = r, a, o })
	creerRaccourci = func(nom, _ string, _ []string, _ bool) (string, error) { return "test:" + nom, nil }
	autostart = func(nom, _ string, _ []string) (string, error) { return "test:" + nom, nil }
	ouvrirDetache = func(string, []string) error { return nil } // jamais un vrai émulateur
	rr, rp := retirerRaccourcis, retirerProgramme               // désinstallation : jamais les vrais raccourcis ni le vrai programme
	t.Cleanup(func() { retirerRaccourcis, retirerProgramme = rr, rp })
	retirerRaccourcis = func(string) ([]string, error) { return nil, nil }
	retirerProgramme = func(string) error { return nil }
	return m
}

func TestProfils(t *testing.T) {
	m := maisonFactice(t)
	d := func(nom string) string { return filepath.Join(m, nom) }
	emu := func(profil, dossierExe string, e Emulateur) (*Emulateur, Profil) {
		e.Profil, e.Exe = profil, filepath.Join(d(dossierExe), "emu.exe")
		p, err := e.profil(profil)
		if err != nil {
			t.Fatal(err)
		}
		return &e, p
	}
	egal := func(obtenu, attendu, message string) {
		t.Helper()
		if obtenu != attendu {
			t.Errorf("%s : %q, attendu %q", message, obtenu, attendu)
		}
	}
	rom := filepath.Join(m, "roms", "gb", "Pokemon Bleue.gb")

	// melonDS : melonDS.toml à côté de l'exe (build portable)
	e, p := emu("melonds", "melon", Emulateur{})
	ecrire(t, filepath.Join(d("melon"), "melonDS.toml"), []byte("[Instance0]\nSaveFilePath = \""+filepath.ToSlash(d("Saves/DS"))+"\"\n[Instance0.Audio]\nSaveFilePath = \"faux\"\n"))
	egal(e.dossier(p, "DS", rom), d("Saves/DS"), "melonDS lit SaveFilePath de [Instance0]")
	egal(fichierSauvegarde("x", "DS", "ADAF", "Pokemon Diamant", p.Ext), filepath.Join("x", "Pokemon Diamant.sav"), "melonDS : .sav")

	// mGBA portable : savegamePath et autoload dans config.ini
	e, p = emu("mgba", "mgba", Emulateur{})
	egal(e.dossier(p, "GB", rom), filepath.Dir(rom), "mGBA sans réglage : à côté de la ROM")
	egal(conseil(e, p), "", "mGBA sans config.ini : rien à signaler")
	ecrire(t, filepath.Join(d("mgba"), "portable.ini"), nil)
	ecrire(t, filepath.Join(d("mgba"), "config.ini"), []byte("[gba.input.QT_K]\nautoload=0\n[ports.qt]\nautoload=1\nsavegamePath="+filepath.ToSlash(d("Saves/GB"))+"\n"))
	egal(e.dossier(p, "GB", rom), d("Saves/GB"), "mGBA lit savegamePath de [ports.qt]")
	if !strings.Contains(conseil(e, p), "autoload=0") {
		t.Errorf("mGBA autoload=1 : conseil attendu, obtenu %q", conseil(e, p))
	}
	ecrire(t, filepath.Join(d("mgba"), "config.ini"), []byte("[ports.qt]\nsavegamePath=savegame\n"))
	egal(e.dossier(p, "GB", rom), filepath.Join(d("mgba"), "savegame"), "mGBA : savegamePath relatif au dossier de config.ini")
	ecrire(t, filepath.Join(d("mgba"), "config.ini"), []byte("[ports.qt]\nautoload=0\n"))
	egal(conseil(e, p), "", "mGBA autoload=0 : rien à signaler")
	ecrire(t, filepath.Join(d("mgba"), "config.ini"), []byte("[ports.qt]\nfullscreen=1\n"))
	if conseil(e, p) == "" {
		t.Error("mGBA sans clé autoload : actif par défaut, conseil attendu")
	}

	// Azahar : dossier user (portable), puis stockage personnalisé
	e, p = emu("azahar", "azahar", Emulateur{})
	ecrire(t, filepath.Join(d("azahar"), "user", "config", "qt-config.ini"), []byte("[Data%20Storage]\nuse_custom_storage=false\nsdmc_directory=ignore\n"))
	egal(e.dossier(p, "3DS", ""), filepath.Join(d("azahar"), "user", "sdmc"), "Azahar portable : user/sdmc")
	ecrire(t, filepath.Join(d("sdmc-perso"), "x"), nil)
	ecrire(t, filepath.Join(d("azahar"), "user", "config", "qt-config.ini"),
		[]byte("[Data%20Storage]\nsdmc_directory\\default=false\nsdmc_directory="+filepath.ToSlash(d("sdmc-perso"))+"\nuse_custom_storage=true\n"))
	egal(e.dossier(p, "3DS", ""), d("sdmc-perso"), "Azahar : sdmc_directory si use_custom_storage=true")
	ecrire(t, filepath.Join(m, "APPDATA", "Azahar", "sdmc", "x"), nil)
	e, p = emu("azahar", "azahar-installe", Emulateur{})
	egal(e.dossier(p, "3DS", ""), filepath.Join(m, "APPDATA", "Azahar", "sdmc"), "Azahar installé : %APPDATA%\\Azahar\\sdmc")

	// RetroArch : savefile_directory + sous-dossier du cœur ; tri par contenu
	coeurs := map[string]string{"GB": "cores/gambatte_libretro.dll", "GBA": "cores/mgba_libretro.so", "DS": "melondsds_libretro.dylib"}
	e, p = emu("retroarch", "ra", Emulateur{Coeurs: coeurs})
	egal(strings.Join(p.Args(e, "GB", rom, ""), "|"), "-L|cores/gambatte_libretro.dll|"+rom, "RetroArch : -L <cœur> <rom>")
	ecrire(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \"default\"\n"))
	if runtime.GOOS != "darwin" { // macOS : ~/Documents/RetroArch/saves
		egal(e.dossier(p, "GB", rom), filepath.Join(d("ra"), "saves", "Gambatte"), "RetroArch default : saves/<cœur> à côté de retroarch.cfg")
	}
	ecrire(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \""+filepath.ToSlash(d("ra-saves"))+"\"\nsort_savefiles_by_content_enable = \"true\"\n"))
	egal(e.dossier(p, "GBA", rom), filepath.Join(d("ra-saves"), "gb", "mGBA"), "RetroArch : tri par contenu puis par cœur")
	ecrire(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \""+filepath.ToSlash(d("ra-saves"))+"\"\nsort_savefiles_enable = \"false\"\nsavestate_auto_load = \"true\"\n"))
	egal(e.dossier(p, "DS", rom), d("ra-saves"), "RetroArch sans tri (EmuDeck) : le dossier seul")
	egal(fichierSauvegarde("x", "GB", "GB-X", "Pokemon Bleue", p.Ext), filepath.Join("x", "Pokemon Bleue.srm"), "RetroArch : .srm")
	if !strings.Contains(conseil(e, p), "savestate_auto_load") {
		t.Error("RetroArch savestate_auto_load : conseil attendu")
	}
	ecrire(t, filepath.Join(d("ra"), "retroarch.cfg"), []byte("savefile_directory = \"\"\n"))
	egal(e.dossier(p, "GB", rom), filepath.Dir(rom), "RetroArch savefile_directory vide : à côté de la ROM")
	if _, err := (&Emulateur{Profil: "retroarch"}).profil("retroarch"); err == nil {
		t.Error("RetroArch sans cœur : erreur attendue")
	}
	if _, err := (&Emulateur{Coeurs: map[string]string{"GBA": "gearboy_libretro.dll"}}).profil("retroarch"); err == nil {
		t.Error("cœur non géré (Gearboy) : erreur attendue")
	}
	egal(bibliotheque("cores/gpsp_libretro.dll")+"|"+bibliotheque("mednafen_gba_libretro.so"), "gpSP|Beetle GBA", "cœurs gpSP et Beetle GBA gérés (srm.go)")
	if !e.gere(p, "DS") || e.gere(p, "3DS") {
		t.Error("RetroArch gère les Systèmes de ses cœurs, et eux seuls")
	}

	// VBA-M : BatteryDir relatif, %s = système
	e, p = emu("vbam", "vbam", Emulateur{})
	ecrire(t, filepath.Join(d("vbam"), "vbam.ini"), []byte("[General]\nAutoLoadLastState=1\nBatteryDir=battery/%s\n"))
	egal(e.dossier(p, "GBA", rom), filepath.Join(d("vbam"), "battery", "GameBoy Advance"), "VBA-M : BatteryDir relatif à l'exe, %s remplacé")
	if conseil(e, p) == "" {
		t.Error("VBA-M AutoLoadLastState=1 : conseil attendu")
	}

	// SameBoy : toujours à côté de la ROM
	e, p = emu("sameboy", "sameboy", Emulateur{})
	egal(e.dossier(p, "GB", rom), filepath.Dir(rom), "SameBoy : à côté de la ROM")

	// SkyEmu : à côté de la ROM ; dossier de search_paths.bin si le .sav y est, ou si save_to_path (user_settings.bin) est coché
	e, p = emu("skyemu", "sky", Emulateur{})
	romSky := filepath.Join(m, "roms", "sky", "Pokemon Bleue.gb")
	egal(e.dossier(p, "GB", romSky), filepath.Dir(romSky), "SkyEmu sans réglages : à côté de la ROM")
	pref := filepath.Join(m, "APPDATA", "Sky", "SkyEmu")
	ecrire(t, filepath.Join(pref, "search_paths.bin"), append([]byte(filepath.ToSlash(d("sky-saves"))+"/"), make([]byte, 1024*8)...))
	reglages := make([]byte, 1024)
	ecrire(t, filepath.Join(pref, "user_settings.bin"), reglages)
	egal(e.dossier(p, "GB", romSky), filepath.Dir(romSky), "SkyEmu : .sav absent du dossier des réglages, save_to_path décoché")
	ecrire(t, filepath.Join(d("sky-saves"), "Pokemon Bleue.sav"), nil)
	egal(e.dossier(p, "GB", romSky), d("sky-saves"), "SkyEmu : .sav déjà dans le dossier des réglages")
	ecrire(t, filepath.Join(filepath.Dir(romSky), "Pokemon Bleue.sav"), nil)
	egal(e.dossier(p, "GB", romSky), filepath.Dir(romSky), "SkyEmu : .sav à côté de la ROM prioritaire")
	reglages[80] = 1
	ecrire(t, filepath.Join(pref, "user_settings.bin"), reglages)
	egal(e.dossier(p, "GBA", filepath.Join(m, "roms", "sky", "Pokemon Emeraude.gba")), d("sky-saves"), "SkyEmu save_to_path : dossier des réglages")
	egal(fichierSauvegarde("x", "DS", "ADAF", "Pokemon Diamant", p.Ext), filepath.Join("x", "Pokemon Diamant.sav"), "SkyEmu : .sav")
	ecrire(t, filepath.Join(pref, "search_paths.bin"), []byte("./\x00"))
	egal(e.dossier(p, "GBA", filepath.Join(m, "roms", "sky", "Pokemon Emeraude.gba")), filepath.Join(m, "roms", "sky"), "SkyEmu : dossier relatif ignoré")

	// NanoBoyAdvance : general.save_folder de config.toml à côté de l'exe, seulement si le dossier existe
	e, p = emu("nanoboyadvance", "nba", Emulateur{})
	ecrire(t, filepath.Join(d("nba"), "config.toml"), []byte("[cartridge]\nsave_folder = \"faux\"\n\n[general]\nbios_path = \"bios.bin\"\nsave_folder = \""+filepath.ToSlash(d("nba-saves"))+"\"\n"))
	egal(e.dossier(p, "GBA", rom), filepath.Dir(rom), "NanoBoyAdvance : save_folder absent du disque, à côté de la ROM")
	ecrire(t, filepath.Join(d("nba-saves"), "x"), nil)
	egal(e.dossier(p, "GBA", rom), d("nba-saves"), "NanoBoyAdvance lit save_folder de [general]")
	egal(strings.Join(p.Args(e, "GBA", rom, ""), "|"), rom, "NanoBoyAdvance : la ROM seule")

	// BGB : à côté de la ROM (« Save files » vide) ; -rom facultatif, args_en_plus avant la ROM
	e, p = emu("bgb", "bgb", Emulateur{ArgsEnPlus: []string{"-nowarn"}})
	egal(e.dossier(p, "GB", rom), filepath.Dir(rom), "BGB : à côté de la ROM")
	egal(strings.Join(e.commande(p, "GB", rom, ""), "|"), e.Exe+"|-nowarn|"+rom, "BGB : exe, args_en_plus, ROM")
	if e.gere(p, "GBA") || !e.gere(p, "GB") {
		t.Error("BGB : GB seulement")
	}

	// config.json reste prioritaire
	e, p = emu("mgba", "mgba", Emulateur{Sauvegardes: map[string]string{"GBA": "E:/GBA"}})
	egal(e.dossier(p, "GBA", rom), "E:/GBA", "dossier de config.json prioritaire")
	if e.gere(p, "GB") {
		t.Error("dossiers dans config.json : seuls ces Systèmes sont gérés")
	}

	// Profil personnalisé
	e, p = emu("monemu", "perso", Emulateur{Args: []string{"--batt={sauvegardes}", "{rom}"}, Extension: ".sav", Sauvegardes: map[string]string{"GBA": "S"}})
	egal(strings.Join(p.Args(e, "GBA", rom, "S"), "|"), "--batt=S|"+rom, "profil personnalisé : {sauvegardes} et {rom}")
	egal(strings.Join(p.Args(e, "GBA", "", "S"), "|"), "--batt=S", "profil personnalisé sans jeu : {rom} retiré")
	if _, err := (&Emulateur{}).profil("inconnu"); err == nil {
		t.Error("profil inconnu sans args : erreur attendue")
	}
	egal((&Emulateur{Exe: filepath.Join("Emu", "mGBA.EXE")}).processus(), "mGBA", "processus : nom de l'exe sans .exe")
}

func TestTitres3DS(t *testing.T) {
	tmp = t.TempDir()
	config := ecrireConfig(t, filepath.Join(tmp, "config.json"), Config{URL: "http://x", Jeton: "j",
		Titres3DS: map[string]string{"00040000000ec300": "Jeu a moi"}})
	t.Cleanup(func() { delete(titres3DS, "00040000000EC300") })
	if _, err := nouvelleSession(config); err != nil {
		t.Fatal(err)
	}
	_, cle, nom, _, err := lireRom(rom3DS(t, "Mon jeu", 0x00040000000EC300))
	if err != nil || cle != "3DS-00040000000EC300" || nom != "Jeu a moi" {
		t.Errorf("titres_3ds : Clé %q, nom %q, erreur %v", cle, nom, err)
	}
}

// Titre GB avec un octet >= 0x80 : même Clé que cle.c (3ds/test/test_cle.c), sans panique.
func TestCleGB(t *testing.T) {
	tmp = t.TempDir()
	rom := filepath.Join(tmp, "Pokemon.gb")
	ecrire(t, rom, enteteGB("Pok\xe9mon\x99", 0x91E6))
	_, cle, _, _, err := lireRom(rom)
	if err != nil || cle != "GB-POK_MON_-91E6" {
		t.Errorf("Clé %q, erreur %v", cle, err)
	}
}

func TestCommandeEtDecoupage(t *testing.T) {
	e := &Emulateur{Exe: "mGBA.exe", ArgsEnPlus: []string{"-f"}}
	p, _ := e.profil("mgba")
	if c := strings.Join(e.commande(p, "GBA", "r.gba", "S"), "|"); c != "mGBA.exe|-C|savegamePath=S|-f|r.gba" {
		t.Errorf("arguments en plus avant la ROM : %s", c)
	}
	if c := strings.Join(e.commande(p, "", "", ""), "|"); c != "mGBA.exe|-f" {
		t.Errorf("launch : arguments en plus à la fin : %s", c)
	}
	a := decouper(`  -rom "{rom}"  --dir='C:\Mes jeux\saves' -f `)
	if strings.Join(a, "|") != `-rom|{rom}|--dir=C:\Mes jeux\saves|-f` {
		t.Errorf("decouper : %q", a)
	}
	if j := joindre(a); j != `-rom {rom} "--dir=C:\Mes jeux\saves" -f` || strings.Join(decouper(j), "|") != strings.Join(a, "|") {
		t.Errorf("joindre : %s", j)
	}
}
