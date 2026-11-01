package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Extrait d'un vrai Emulators.xml (mêmes champs, même indentation, fins de ligne CRLF).
const emulatorsXML = "<?xml version=\"1.0\" standalone=\"yes\"?>\r\n<LaunchBox>\r\n" +
	"  <EmulatorPlatform>\r\n    <Emulator>aaaa</Emulator>\r\n    <Platform>Nintendo DS</Platform>\r\n    <CommandLine />\r\n    <Default>true</Default>\r\n    <M3uDiscLoadEnabled>false</M3uDiscLoadEnabled>\r\n    <AutoExtract>true</AutoExtract>\r\n  </EmulatorPlatform>\r\n" +
	"  <EmulatorPlatform>\r\n    <Emulator>aaaa</Emulator>\r\n    <Platform>Nintendo Entertainment System</Platform>\r\n    <CommandLine />\r\n    <Default>false</Default>\r\n    <M3uDiscLoadEnabled>false</M3uDiscLoadEnabled>\r\n    <AutoExtract>false</AutoExtract>\r\n  </EmulatorPlatform>\r\n" +
	"  <Emulator>\r\n    <ApplicationPath>Emulators\\MelonDS\\melonDS.exe</ApplicationPath>\r\n    <CommandLine>-f</CommandLine>\r\n    <DefaultPlatform />\r\n    <ID>aaaa</ID>\r\n    <Title>MelonDS &amp; co</Title>\r\n    <NoQuotes>false</NoQuotes>\r\n    <HideConsole>true</HideConsole>\r\n    <AutoExtract>true</AutoExtract>\r\n    <UsePauseScreen>true</UsePauseScreen>\r\n  </Emulator>\r\n" +
	"  <Emulator>\r\n    <ApplicationPath>Emulators\\Cemu\\Cemu.exe</ApplicationPath>\r\n    <CommandLine>-f -g</CommandLine>\r\n    <ID>bbbb</ID>\r\n    <Title>Cemu</Title>\r\n  </Emulator>\r\n" +
	"</LaunchBox>"

func xmlValide(t *testing.T, nom, texte string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(texte))
	for {
		_, err := d.Token()
		if err != nil {
			if err.Error() != "EOF" {
				t.Errorf("%s : XML invalide : %v", nom, err)
			}
			return
		}
	}
}

func TestLaunchBox(t *testing.T) {
	lb, lanceur := filepath.Join("L", "LaunchBox"), filepath.Join("P", "3dssavesync.exe")
	entrees := []EntreeLB{
		{"melonds", filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), []string{"DS"}},
		{"mgba", filepath.Join("x", "mGBA.exe"), []string{"GB", "GBA"}},
	}
	nouveau, rapport := launchbox(emulatorsXML, lb, lanceur, entrees)
	xmlValide(t, "Emulators.xml", nouveau)
	emus := blocEmulateurLB.FindAllString(nouveau, -1)
	plats := blocPlateformeLB.FindAllString(nouveau, -1)
	if len(emus) != 3 || len(plats) != 3 {
		t.Fatalf("attendu 3 émulateurs et 3 plateformes : %d, %d\n%s", len(emus), len(plats), nouveau)
	}
	e := emus[2]
	id := champ(e, "ID")
	if champ(e, "ApplicationPath") != lanceur || champ(e, "Title") != "MelonDS &amp; co (3dssavesync)" || id == "aaaa" || len(id) != 36 {
		t.Errorf("nouvel émulateur incorrect :\n%s", e)
	}
	if !strings.Contains(e, "<CommandLine />") || champ(e, "HideConsole") != "false" || champ(e, "UsePauseScreen") != "true" || !strings.Contains(e, "\r\n") {
		t.Errorf("champs copiés ou corrigés incorrects :\n%s", e)
	}
	p := plats[2]
	if champ(p, "Emulator") != id || champ(p, "Platform") != "Nintendo DS" || champ(p, "CommandLine") != "run --emulator melonds" ||
		champ(p, "Default") != "true" || champ(p, "AutoExtract") != "false" {
		t.Errorf("nouvelle plateforme incorrecte :\n%s", p)
	}
	if champ(plats[0], "Default") != "false" || champ(plats[0], "Emulator") != "aaaa" {
		t.Errorf("l'ancienne plateforme DS n'est plus par défaut, et reste à l'émulateur d'origine :\n%s", plats[0])
	}
	if plats[1] != blocPlateformeLB.FindAllString(emulatorsXML, -1)[1] || emus[0] != blocEmulateurLB.FindAllString(emulatorsXML, -1)[0] {
		t.Error("l'émulateur d'origine et la plateforme NES ne changent pas")
	}
	if len(rapport) != 2 || !strings.Contains(rapport[1], "absent") {
		t.Errorf("rapport : %q", rapport)
	}
	encore, rapport := launchbox(nouveau, lb, lanceur, entrees[:1])
	if encore != nouveau || !strings.Contains(rapport[0], "déjà en place") {
		t.Errorf("réparation d'une configuration en place : rien ne doit changer (%q)", rapport)
	}
	// Installation modifiée à la main (bascule de la phase 1) : reconnue, pas de doublon.
	main := strings.Replace(strings.Replace(emulatorsXML, `Emulators\MelonDS\melonDS.exe`, `C:\Users\J\AppData\Local\Programs\3dssavesync\3dssavesync.exe`, 1),
		"<CommandLine />", "<CommandLine>run --emulator melonds</CommandLine>", 1)
	if r, rap := launchbox(main, lb, lanceur, entrees[:1]); r != main || !strings.Contains(rap[0], "déjà en place") {
		t.Errorf("bascule de la phase 1 non reconnue : %q", rap)
	}
}

func TestESDE(t *testing.T) {
	emus := map[string]string{"DS": "melonds", "3DS": "azahar"}
	s, r, rapport := esde("", "", "/home/x/.local/bin/3dssavesync", emus)
	xmlValide(t, "es_systems.xml", s)
	xmlValide(t, "es_find_rules.xml", r)
	if !strings.Contains(r, "<entry>/home/x/.local/bin/3dssavesync</entry>") || strings.Count(s, "<system>") != 2 ||
		!strings.Contains(s, "%EMULATOR_3DSSAVESYNC% run --emulator azahar %ROM%") || len(rapport) != 2 {
		t.Errorf("fichiers créés incorrects :\n%s\n%s\n%q", s, r, rapport)
	}
	s2, r2, _ := esde(s, r, "/home/x/.local/bin/3dssavesync", emus)
	if s2 != s || r2 != r {
		t.Error("deuxième passage : rien ne doit changer")
	}
	_, r3, _ := esde(s, r, "/opt/3dssavesync", emus)
	if strings.Count(r3, "3DSSAVESYNC") != 1 || !strings.Contains(r3, "/opt/3dssavesync") {
		t.Errorf("nouvel emplacement de l'exe : règle remplacée :\n%s", r3)
	}
	emudeck := "<?xml version=\"1.0\"?>\n<systemList>\n    <system>\n        <name>nds</name>\n        <fullname>Nintendo DS</fullname>\n" +
		"        <command label=\"melonDS (Standalone)\">%EMULATOR_MELONDS% -f %ROM%</command>\n" +
		"        <command label=\"DeSmuME\">%EMULATOR_RETROARCH% %ROM%</command>\n    </system>\n</systemList>\n"
	s4, _, _ := esde(emudeck, "", "x", map[string]string{"DS": "melonds"})
	xmlValide(t, "es_systems.xml EmuDeck", s4)
	i, j := strings.Index(s4, "3dssavesync (melonds)"), strings.Index(s4, "melonDS (Standalone)")
	if i < 0 || j < i || !strings.Contains(s4, "DeSmuME") {
		t.Errorf("bloc EmuDeck : commande 3dssavesync en premier, les autres gardées :\n%s", s4)
	}
}

func TestDesktop(t *testing.T) {
	f := fichierDesktop("3dssavesync melonds", []string{"/home/a b/3dssavesync", "launch", "melonds", `100%"$`}, false)
	if !strings.Contains(f, `Exec="/home/a b/3dssavesync" launch melonds "100%%\"\$"`) {
		t.Errorf("Exec mal échappé :\n%s", f)
	}
}

// Assistant complet, puis réparation : vrai serveur, émulateur détecté factice, LaunchBox et ES-DE factices.
func TestAssistant(t *testing.T) {
	tmp = t.TempDir()
	m := maisonFactice(t)
	demarrerServeur(t)
	reinitialiser()
	exeEmu := filepath.Join(tmp, "emus", "melonDS.exe")
	ecrire(t, exeEmu, []byte("melon"))
	fauxExe := filepath.Join(tmp, "telechargements", "3dssavesync.exe")
	ecrire(t, fauxExe, []byte("lanceur"))
	detecterAvant, executableAvant := detecter, executable
	t.Cleanup(func() { detecter, executable = detecterAvant, executableAvant })
	detecter = func(string) []Trouve { return []Trouve{{Profil: "melonds", Exe: exeEmu, Source: "test"}} }
	executable = func() string { return fauxExe }
	lb := filepath.Join(tmp, "LaunchBox")
	ecrire(t, filepath.Join(lb, "Data", "Emulators.xml"), []byte(emulatorsXML))
	if err := os.MkdirAll(filepath.Join(m, "HOME", "ES-DE"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		os.MkdirAll(filepath.Join(m, "USERPROFILE", "ES-DE"), 0o755)
	}
	saves, roms := filepath.Join(tmp, "saves"), filepath.Join(tmp, "roms")
	reponses := []string{"", url, "mauvais-jeton", "", "", "jeton-test", "", "", saves, roms, ""}
	if runtime.GOOS == "windows" {
		reponses = append(reponses, lb)
	}
	reponses = append(reponses, "", "n")
	var sortie bytes.Buffer
	config := filepath.Join(tmp, "cfg", "config.json")
	q := &Questions{in: bufio.NewReader(strings.NewReader(strings.Join(reponses, "\n") + "\n")), out: &sortie}
	if err := assistant(q, config); err != nil {
		t.Fatalf("assistant : %v\n%s", err, sortie.String())
	}
	if strings.Contains(sortie.String(), "jeton-test") {
		t.Error("le jeton ne doit jamais s'afficher")
	}
	if !strings.Contains(sortie.String(), "401") || !strings.Contains(sortie.String(), "Connexion réussie") {
		t.Errorf("mauvais jeton refusé, puis connexion réussie attendus :\n%s", sortie.String())
	}
	var cfg Config
	if err := lireJSON(config, &cfg); err != nil {
		t.Fatal(err)
	}
	e := cfg.Emulateurs["melonds"]
	if cfg.URL != url || cfg.Jeton != "jeton-test" || e == nil || e.Exe != exeEmu || e.Sauvegardes["DS"] != saves || e.Roms["DS"] != roms {
		t.Fatalf("config.json incorrect : %+v\n%s", cfg, sortie.String())
	}
	if cfg.Lanceur != emplacementInstalle() || string(lire(cfg.Lanceur)) != "lanceur" {
		t.Errorf("exe copié à l'emplacement fixe attendu : %s", cfg.Lanceur)
	}
	if !strings.Contains(string(lire(filepath.Join(cfg.Frontends["esde"], "custom_systems", "es_systems.xml"))), "--emulator melonds") {
		t.Errorf("ES-DE configuré attendu :\n%s", sortie.String())
	}
	if runtime.GOOS == "windows" {
		lbx := string(lire(filepath.Join(lb, "Data", "Emulators.xml")))
		if !strings.Contains(lbx, "MelonDS &amp; co (3dssavesync)") || len(nomsDans(filepath.Join(lb, "Data"), "Emulators.xml.avant-3dssavesync-*")) != 1 {
			t.Errorf("LaunchBox configuré, avec une copie, attendu :\n%s", sortie.String())
		}
	}

	// Seconde ouverture : menu, réparation, rien ne change.
	sortie.Reset()
	avantCfg := string(lire(config))
	q = &Questions{in: bufio.NewReader(strings.NewReader("2\n")), out: &sortie}
	if err := assistant(q, config); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sortie.String(), "ES-DE nds : déjà en place") || string(lire(config)) != avantCfg {
		t.Errorf("réparation sans changement attendue :\n%s", sortie.String())
	}
	if runtime.GOOS == "windows" && !strings.Contains(sortie.String(), "LaunchBox melonds : déjà en place") {
		t.Errorf("LaunchBox déjà en place attendu :\n%s", sortie.String())
	}
}
