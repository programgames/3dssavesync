package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Désinstallation (ADR 0019) : LaunchBox et ES-DE reviennent exactement à leur état d'avant l'installation.
func TestDefaireLaunchBox(t *testing.T) {
	lb, lanceur := filepath.Join("L", "LaunchBox"), filepath.Join("P", "3dssavesync.exe")
	entrees := []EntreeLB{
		{"melonds", filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), []string{"DS"}},
		{"cemu", filepath.Join("x", "Cemu.exe"), []string{"GB"}}, // aucune plateforme GB : rien d'ajouté
	}
	installe, _ := launchbox(emulatorsXML, lb, lanceur, entrees)
	if installe == emulatorsXML {
		t.Fatal("l'installation doit changer Emulators.xml")
	}
	defait, rapport := defaireLaunchbox(installe, lb, lanceur, &Config{})
	if defait != emulatorsXML {
		t.Errorf("Emulators.xml doit revenir à l'identique :\n%s", defait)
	}
	if len(rapport) != 1 || !strings.Contains(rapport[0], "MelonDS & co") {
		t.Errorf("rapport : %q", rapport)
	}
	if encore, r := defaireLaunchbox(emulatorsXML, lb, lanceur, &Config{}); encore != emulatorsXML || len(r) != 0 {
		t.Errorf("rien à retirer : rien ne change (%q)", r)
	}

	t.Log("Émulateur d'origine supprimé depuis dans LaunchBox : l'émulateur 3dssavesync part, le rapport le dit")
	sansOrigine := blocEmulateurLB.ReplaceAllStringFunc(installe, func(b string) string {
		if champ(b, "ID") == "aaaa" {
			return ""
		}
		return b
	})
	d, r := defaireLaunchbox(sansOrigine, lb, lanceur, &Config{})
	xmlValide(t, "Emulators.xml sans origine", d)
	if strings.Contains(d, "3dssavesync") || len(r) != 1 || !strings.Contains(r[0], "choisissez un émulateur par défaut") {
		t.Errorf("rapport %q :\n%s", r, d)
	}
}

// Apostrophe et guillemets (dossier « Jeux d'enfance », titre LaunchBox) : echapperXML écrit &#39; et &#34;, LaunchBox
// les écrit tels quels. La désinstallation compare les textes décodés.
func TestDefaireLaunchBoxApostrophe(t *testing.T) {
	lb := filepath.Join("C:", "Jeux d'enfance", "LaunchBox")
	lanceur := filepath.Join("C:", "Jeux d'enfance", "3dssavesync", "3dssavesync.exe")
	avant := strings.Replace(emulatorsXML, "<Title>MelonDS &amp; co</Title>", `<Title>Jeu d'enfance "DS"</Title>`, 1)
	installe, _ := launchbox(avant, lb, lanceur, []EntreeLB{{"melonds", filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), []string{"DS"}}})
	if !strings.Contains(installe, "<Title>Jeu d&#39;enfance &#34;DS&#34; (3dssavesync)</Title>") {
		t.Fatalf("copie « (3dssavesync) » attendue :\n%s", installe)
	}
	defait, rapport := defaireLaunchbox(installe, lb, lanceur, &Config{})
	if defait != avant {
		t.Errorf("Emulators.xml doit revenir à l'identique (émulateur d'origine de nouveau par défaut) :\n%s", defait)
	}
	if len(rapport) != 1 || !strings.Contains(rapport[0], `Jeu d'enfance "DS" (3dssavesync)`) {
		t.Errorf("rapport : %q", rapport)
	}
}

func TestDefaireESDE(t *testing.T) {
	emus := map[string]string{"DS": "melonds", "3DS": "azahar"}
	emudeck := "<?xml version=\"1.0\"?>\n<systemList>\n    <system>\n        <name>nds</name>\n        <fullname>Nintendo DS</fullname>\n" +
		"        <command label=\"melonDS (Standalone)\">%EMULATOR_MELONDS% -f %ROM%</command>\n" +
		"        <command label=\"DeSmuME\">%EMULATOR_RETROARCH% %ROM%</command>\n    </system>\n</systemList>\n"
	regles := "<?xml version=\"1.0\"?>\n<ruleList>\n  <emulator name=\"MELONDS\">\n    <rule type=\"staticpath\">\n      <entry>/x/melonDS</entry>\n    </rule>\n  </emulator>\n</ruleList>\n"
	s, r, _ := esde(emudeck, regles, "/opt/3dssavesync", emus)
	if !strings.Contains(s, "3dssavesync (melonds)") || !strings.Contains(s, "<name>n3ds</name>") || !strings.Contains(r, "3DSSAVESYNC") {
		t.Fatalf("installation ES-DE attendue :\n%s\n%s", s, r)
	}
	ds, dr, rapport := defaireEsde(s, r)
	if ds != emudeck || dr != regles {
		t.Errorf("ES-DE doit revenir à l'identique :\n%s\n%s", ds, dr)
	}
	if len(rapport) != 2 {
		t.Errorf("rapport (commande nds retirée, système n3ds retiré) : %q", rapport)
	}

	t.Log("Fichiers créés par l'installation : plus aucune trace de 3dssavesync, XML valide")
	s, r, _ = esde("", "", "/opt/3dssavesync", emus)
	ds, dr, _ = defaireEsde(s, r)
	xmlValide(t, "es_systems.xml", ds)
	xmlValide(t, "es_find_rules.xml", dr)
	if strings.Contains(ds+dr, "3dssavesync") || strings.Contains(ds+dr, "3DSSAVESYNC") || strings.Contains(ds, "<system>") {
		t.Errorf("reste de 3dssavesync :\n%s\n%s", ds, dr)
	}
}

// desinstaller : un frontend ouvert bloque tout ; sinon fichiers défaits avec copie, raccourcis et programme retirés,
// données de l'utilisateur gardées et nommées.
func TestDesinstaller(t *testing.T) {
	maisonFactice(t)
	tmp := t.TempDir()
	lb, es := filepath.Join(tmp, "LaunchBox"), filepath.Join(tmp, "es-de")
	lanceur := filepath.Join(tmp, "Programs", "3dssavesync.exe")
	installe, _ := launchbox(emulatorsXML, lb, lanceur, []EntreeLB{{"melonds", filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), []string{"DS"}}})
	s, r, _ := esde("", "", lanceur, map[string]string{"DS": "melonds"})
	ecrire(t, filepath.Join(lb, "Data", "Emulators.xml"), []byte(installe))
	ecrire(t, filepath.Join(es, "custom_systems", "es_systems.xml"), []byte(s))
	ecrire(t, filepath.Join(es, "custom_systems", "es_find_rules.xml"), []byte(r))
	cfg := &Config{Lanceur: lanceur, Frontends: map[string]string{"launchbox": lb, "esde": es}, ServeurLocal: &ServeurLocal{Port: 8765}}
	config := filepath.Join(tmp, "cfg", "config.json")

	var raccourcisPour []string
	var programme string
	retirerRaccourcis = func(l string) ([]string, error) {
		raccourcisPour = append(raccourcisPour, l)
		return []string{"a.lnk"}, nil
	}
	retirerProgramme = func(f string) error { programme = f; return nil }
	ecrire(t, emplacementInstalle(), []byte("exe")) // dans la maison factice
	ouvert, actif := "LaunchBox", emulateurActif
	emulateurActif = func(nom string) bool { return nom == ouvert }
	t.Cleanup(func() { emulateurActif = actif })

	if _, err := desinstaller(cfg, config); err == nil || !strings.Contains(err.Error(), "LaunchBox est ouvert") ||
		string(lire(filepath.Join(lb, "Data", "Emulators.xml"))) != installe || raccourcisPour != nil {
		t.Fatalf("LaunchBox ouvert : rien ne doit changer (%v)", err)
	}
	ouvert = ""
	rapport, err := desinstaller(cfg, config)
	if err != nil {
		t.Fatal(err)
	}
	if string(lire(filepath.Join(lb, "Data", "Emulators.xml"))) != emulatorsXML {
		t.Error("Emulators.xml doit revenir à l'identique")
	}
	if strings.Contains(string(lire(filepath.Join(es, "custom_systems", "es_systems.xml"))), "3dssavesync") {
		t.Error("es_systems.xml ne doit plus lancer 3dssavesync")
	}
	if c, _ := filepath.Glob(filepath.Join(lb, "Data", "Emulators.xml.avant-3dssavesync-*")); len(c) != 1 {
		t.Errorf("copie d'Emulators.xml avant le changement : %v", c)
	}
	texte := strings.Join(rapport, "\n")
	if programme != emplacementInstalle() {
		t.Errorf("programme installé à mettre à la Corbeille : %q", programme)
	}
	// raccourcis vers l'exe noté (Lanceur) et vers l'exe installé
	if strings.Join(raccourcisPour, "|") != lanceur+"|"+emplacementInstalle() || !strings.Contains(texte, "Raccourcis mis à la Corbeille : 2") || !strings.Contains(texte, filepath.Join(tmp, "cfg", "serveur")) ||
		!strings.Contains(texte, filepath.Join(tmp, "cfg", "copies")) || !strings.Contains(texte, "FBI") {
		t.Errorf("rapport :\n%s", texte)
	}
}

// Version portable : frontends et raccourcis défaits, mais le programme et son dossier restent ; le rapport le nomme.
func TestDesinstallerPortable(t *testing.T) {
	maisonFactice(t)
	dossier := filepath.Join(t.TempDir(), "3dssavesync")
	exe := filepath.Join(dossier, "3dssavesync.exe")
	ecrire(t, exe, []byte("exe"))
	ecrire(t, filepath.Join(dossier, "portable.txt"), []byte("portable"))
	executableAvant := executable
	t.Cleanup(func() { executable = executableAvant })
	executable = func() string { return exe }
	var raccourcisPour []string
	programme := false
	retirerRaccourcis = func(l string) ([]string, error) { raccourcisPour = append(raccourcisPour, l); return nil, nil }
	retirerProgramme = func(string) error { programme = true; return nil }

	rapport, err := desinstaller(&Config{Lanceur: exe}, filepath.Join(dossier, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	texte := strings.Join(rapport, "\n")
	if programme || !existe(exe) {
		t.Error("version portable : le programme ne doit pas partir à la Corbeille")
	}
	if strings.Join(raccourcisPour, "|") != exe || !strings.Contains(texte, "Supprimez ce dossier quand vous n'en aurez plus besoin") || !strings.Contains(texte, dossier) {
		t.Errorf("raccourcis %q, rapport :\n%s", raccourcisPour, texte)
	}
}

// Assistant des versions 1 et 2 : MelonDS, Mgba et Azahar modifiés sur place (exe = Lanceur, titre d'origine), plus
// deux copies « (3dssavesync) » par défaut sur les mêmes plateformes. Les émulateurs modifiés restent, avec leurs
// plateformes, et retrouvent leur exe ; les copies partent ; exe inconnu : émulateur laissé tel quel, rapport.
func TestDefaireLaunchBoxSurPlace(t *testing.T) {
	tmp := t.TempDir()
	lb := filepath.Join(tmp, "LaunchBox")
	lanceur := filepath.Join(tmp, "Programs", "3dssavesync", "3dssavesync.exe")
	melon, mgba := filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), filepath.Join(tmp, "mGBA", "mGBA.exe")
	ecrire(t, melon, []byte("exe"))
	ecrire(t, mgba, []byte("exe"))
	ra := filepath.Join(tmp, "RetroArch", "retroarch.exe")
	ecrire(t, ra, []byte("exe"))
	cfg := &Config{Emulateurs: map[string]*Emulateur{"melonds": {Exe: melon}, "mgba": {Exe: mgba}, "azahar": {Exe: filepath.Join(tmp, "absent", "azahar.exe")},
		"retroarch": {Exe: ra, Coeurs: map[string]string{"GBA": filepath.Join(tmp, "RetroArch", "cores", "mgba_libretro.dll")}}}}
	plat := func(emu, p, cmd, def string) string {
		c := "<CommandLine />"
		if cmd != "" {
			c = "<CommandLine>" + cmd + "</CommandLine>"
		}
		return "  <EmulatorPlatform>\r\n    <Emulator>" + emu + "</Emulator>\r\n    <Platform>" + p + "</Platform>\r\n    " + c +
			"\r\n    <Default>" + def + "</Default>\r\n    <AutoExtract>false</AutoExtract>\r\n  </EmulatorPlatform>\r\n"
	}
	emu := func(id, titre, app string) string {
		return "  <Emulator>\r\n    <ApplicationPath>" + app + "</ApplicationPath>\r\n    <CommandLine />\r\n    <ID>" + id +
			"</ID>\r\n    <Title>" + titre + "</Title>\r\n  </Emulator>\r\n"
	}
	xmlLB := "<?xml version=\"1.0\" standalone=\"yes\"?>\r\n<LaunchBox>\r\n" +
		plat("m1", "Nintendo DS", "run --emulator melonds", "true") + plat("m1", "Nintendo DSi", "run --emulator melonds", "true") +
		plat("m1", "Nintendo Entertainment System", "", "false") +
		plat("m2", "Nintendo Game Boy", "run --emulator mgba", "true") + plat("m2", "Nintendo Game Boy Advance", "run --emulator mgba", "true") +
		plat("m2", "Nintendo Game Boy Color", "run --emulator mgba", "true") + plat("m3", "Nintendo 3DS", "run --emulator azahar", "true") +
		plat("r0", "Nintendo Game Boy Advance", "", "false") + plat("r1", "Nintendo Game Boy Advance", "run --emulator retroarch", "true") +
		plat("s0", "Nintendo Game Boy", "", "false") + plat("s1", "Nintendo Game Boy", "run --emulator sameboy", "true") +
		emu("m1", "MelonDS", lanceur) + emu("m2", "Mgba", lanceur) + emu("m3", "Azahar", lanceur) +
		emu("r0", "RetroArch", `C:\RetroArch\retroarch.exe`) + emu("r1", "RetroArch (3dssavesync)", lanceur) +
		emu("s0", "Sameboy", `C:\SameBoy\sameboy.exe`) + emu("s1", "Sameboy (3dssavesync)", lanceur) +
		// RetroArch modifié sur place : cœur GBA connu, GB inconnu ; DeSmuME, aussi par défaut pour Nintendo DS
		plat("m4", "Nintendo Game Boy Advance", "run --emulator retroarch", "false") + plat("m4", "Nintendo Game Boy Color", "run --emulator retroarch", "false") +
		plat("x0", "Nintendo DS", "", "true") + emu("m4", "RetroArch 1.9", lanceur) + emu("x0", "DeSmuME", `C:\DeSmuME\desmume.exe`) +
		"</LaunchBox>"

	d, rapport := defaireLaunchbox(xmlLB, lb, lanceur, cfg)
	xmlValide(t, "Emulators.xml", d)
	emus := map[string]string{}
	for _, b := range blocEmulateurLB.FindAllString(d, -1) {
		emus[champ(b, "Title")] = b
	}
	if len(emus) != 7 || emus["RetroArch (3dssavesync)"] != "" || emus["Sameboy (3dssavesync)"] != "" {
		t.Fatalf("copies retirées, MelonDS, Mgba, Azahar, RetroArch, Sameboy gardés :\n%s", d)
	}
	if a := champ(emus["MelonDS"], "ApplicationPath"); a != filepath.Join("Emulators", "MelonDS", "melonDS.exe") {
		t.Errorf("MelonDS : exe réel, relatif au dossier de LaunchBox : %q", a)
	}
	if a := champ(emus["Mgba"], "ApplicationPath"); a != mgba {
		t.Errorf("Mgba : exe réel : %q", a)
	}
	if emus["Azahar"] != emu("m3", "Azahar", lanceur) {
		t.Errorf("Azahar (exe absent) : laissé tel quel :\n%s", emus["Azahar"])
	}
	plats := blocPlateformeLB.FindAllString(d, -1)
	if len(plats) != 12 || !strings.Contains(d, plat("m3", "Nintendo 3DS", "run --emulator azahar", "true")) {
		t.Fatalf("plateformes de MelonDS, Mgba et Azahar gardées :\n%s", d)
	}
	for _, p := range []string{plat("m1", "Nintendo DS", "", "true"), plat("m1", "Nintendo DSi", "", "true"), plat("m1", "Nintendo Entertainment System", "", "false"),
		plat("m2", "Nintendo Game Boy", "", "true"), plat("m2", "Nintendo Game Boy Advance", "", "true"), plat("m2", "Nintendo Game Boy Color", "", "true"),
		// Mgba, modifié sur place, reste par défaut : RetroArch et Sameboy ne le redeviennent pas (jamais deux par défaut)
		plat("r0", "Nintendo Game Boy Advance", "", "false"), plat("s0", "Nintendo Game Boy", "", "false"),
		// le cœur de config.json revient, relatif au dossier de RetroArch
		plat("m4", "Nintendo Game Boy Advance", "-L &#34;"+filepath.Join("cores", "mgba_libretro.dll")+"&#34;", "false"),
		plat("m4", "Nintendo Game Boy Color", "", "false"), plat("x0", "Nintendo DS", "", "true")} {
		if !strings.Contains(d, p) {
			t.Errorf("plateforme attendue :\n%s", p)
		}
	}
	texte := strings.Join(rapport, "\n")
	// 8 lignes : MelonDS, Mgba, Azahar, RetroArch 1.9 et son cœur GB inconnu, les deux copies retirées, Nintendo DS
	// par défaut deux fois (MelonDS et DeSmuME)
	if len(rapport) != 8 || !strings.Contains(texte, "Azahar") || !strings.Contains(texte, "Tools, Manage, Emulators") || !strings.Contains(texte, mgba) ||
		!strings.Contains(texte, "Nintendo Game Boy Color") || !strings.Contains(texte, "2 émulateurs sont par défaut pour « Nintendo DS »") ||
		strings.Contains(texte, "de nouveau celui par défaut") || strings.Contains(texte, "Nintendo Game Boy Advance") {
		t.Errorf("rapport :\n%s", texte)
	}
}
