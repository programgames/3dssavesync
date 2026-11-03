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

// Excerpt of a real Emulators.xml (same fields, same indentation, CRLF line endings).
const emulatorsXML = "<?xml version=\"1.0\" standalone=\"yes\"?>\r\n<LaunchBox>\r\n" +
	"  <EmulatorPlatform>\r\n    <Emulator>aaaa</Emulator>\r\n    <Platform>Nintendo DS</Platform>\r\n    <CommandLine />\r\n    <Default>true</Default>\r\n    <M3uDiscLoadEnabled>false</M3uDiscLoadEnabled>\r\n    <AutoExtract>true</AutoExtract>\r\n  </EmulatorPlatform>\r\n" +
	"  <EmulatorPlatform>\r\n    <Emulator>aaaa</Emulator>\r\n    <Platform>Nintendo Entertainment System</Platform>\r\n    <CommandLine />\r\n    <Default>false</Default>\r\n    <M3uDiscLoadEnabled>false</M3uDiscLoadEnabled>\r\n    <AutoExtract>false</AutoExtract>\r\n  </EmulatorPlatform>\r\n" +
	"  <Emulator>\r\n    <ApplicationPath>Emulators\\MelonDS\\melonDS.exe</ApplicationPath>\r\n    <CommandLine>-f</CommandLine>\r\n    <DefaultPlatform />\r\n    <ID>aaaa</ID>\r\n    <Title>MelonDS &amp; co</Title>\r\n    <NoQuotes>false</NoQuotes>\r\n    <HideConsole>true</HideConsole>\r\n    <AutoExtract>true</AutoExtract>\r\n    <UsePauseScreen>true</UsePauseScreen>\r\n  </Emulator>\r\n" +
	"  <Emulator>\r\n    <ApplicationPath>Emulators\\Cemu\\Cemu.exe</ApplicationPath>\r\n    <CommandLine>-f -g</CommandLine>\r\n    <ID>bbbb</ID>\r\n    <Title>Cemu</Title>\r\n  </Emulator>\r\n" +
	"</LaunchBox>"

func validXML(t *testing.T, name, text string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(text))
	for {
		_, err := d.Token()
		if err != nil {
			if err.Error() != "EOF" {
				t.Errorf("%s : XML invalide : %v", name, err)
			}
			return
		}
	}
}

func TestLaunchBox(t *testing.T) {
	lb, launcher := filepath.Join("L", "LaunchBox"), filepath.Join("P", "3dssavesync.exe")
	entries := []LBEntry{
		{"melonds", filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe"), []System{"DS"}},
		{"mgba", filepath.Join("x", "mGBA.exe"), []System{"GB", "GBA"}},
	}
	newText, report := launchbox(emulatorsXML, lb, launcher, entries)
	validXML(t, "Emulators.xml", newText)
	emus := lbEmulatorBlock.FindAllString(newText, -1)
	plats := lbPlatformBlock.FindAllString(newText, -1)
	if len(emus) != 3 || len(plats) != 3 {
		t.Fatalf("attendu 3 émulateurs et 3 plateformes : %d, %d\n%s", len(emus), len(plats), newText)
	}
	e := emus[2]
	id := field(e, "ID")
	if field(e, "ApplicationPath") != launcher || field(e, "Title") != "MelonDS &amp; co (3dssavesync)" || id == "aaaa" || len(id) != 36 {
		t.Errorf("nouvel émulateur incorrect :\n%s", e)
	}
	if !strings.Contains(e, "<CommandLine />") || field(e, "HideConsole") != "false" || field(e, "UsePauseScreen") != "true" || !strings.Contains(e, "\r\n") {
		t.Errorf("champs copiés ou corrigés incorrects :\n%s", e)
	}
	p := plats[2]
	if field(p, "Emulator") != id || field(p, "Platform") != "Nintendo DS" || field(p, "CommandLine") != "run --emulator melonds" ||
		field(p, "Default") != "true" || field(p, "AutoExtract") != "false" {
		t.Errorf("nouvelle plateforme incorrecte :\n%s", p)
	}
	if field(plats[0], "Default") != "false" || field(plats[0], "Emulator") != "aaaa" {
		t.Errorf("l'ancienne plateforme DS n'est plus par défaut, et reste à l'émulateur d'origine :\n%s", plats[0])
	}
	if plats[1] != lbPlatformBlock.FindAllString(emulatorsXML, -1)[1] || emus[0] != lbEmulatorBlock.FindAllString(emulatorsXML, -1)[0] {
		t.Error("l'émulateur d'origine et la plateforme NES ne changent pas")
	}
	if len(report) != 2 || !strings.Contains(report[1], "absent") {
		t.Errorf("rapport : %q", report)
	}
	again, report := launchbox(newText, lb, launcher, entries[:1])
	if again != newText || !strings.Contains(report[0], "déjà en place") {
		t.Errorf("réparation d'une configuration en place : rien ne doit changer (%q)", report)
	}
	// Install changed by hand (emulator switched to the Launcher in place): recognized, no duplicate.
	main := strings.Replace(strings.Replace(emulatorsXML, `Emulators\MelonDS\melonDS.exe`, `C:\Users\J\AppData\Local\Programs\3dssavesync\3dssavesync.exe`, 1),
		"<CommandLine />", "<CommandLine>run --emulator melonds</CommandLine>", 1)
	if r, rap := launchbox(main, lb, launcher, entries[:1]); r != main || !strings.Contains(rap[0], "déjà en place") {
		t.Errorf("bascule de la phase 1 non reconnue : %q", rap)
	}
}

func TestESDE(t *testing.T) {
	emus := map[System]string{"DS": "melonds", "3DS": "azahar"}
	s, r, report := esde("", "", "/home/x/.local/bin/3dssavesync", emus)
	validXML(t, "es_systems.xml", s)
	validXML(t, "es_find_rules.xml", r)
	if !strings.Contains(r, "<entry>/home/x/.local/bin/3dssavesync</entry>") || strings.Count(s, "<system>") != 2 ||
		!strings.Contains(s, "%EMULATOR_3DSSAVESYNC% run --emulator azahar %ROM%") || len(report) != 2 {
		t.Errorf("fichiers créés incorrects :\n%s\n%s\n%q", s, r, report)
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
	s4, _, _ := esde(emudeck, "", "x", map[System]string{"DS": "melonds"})
	validXML(t, "es_systems.xml EmuDeck", s4)
	i, j := strings.Index(s4, "3dssavesync (melonds)"), strings.Index(s4, "melonDS (Standalone)")
	if i < 0 || j < i || !strings.Contains(s4, "DeSmuME") {
		t.Errorf("bloc EmuDeck : commande 3dssavesync en premier, les autres gardées :\n%s", s4)
	}
	noCommand := "<?xml version=\"1.0\"?>\n<systemList>\n    <system>\n        <name>nds</name>\n        <extension>.nds .7z .rar</extension>\n    </system>\n</systemList>\n"
	s5, _, _ := esde(noCommand, "", "x", map[System]string{"DS": "melonds"})
	if !strings.Contains(s5, "<extension>.nds .7z .rar</extension>") || !strings.Contains(s5, "3dssavesync (melonds)") {
		t.Errorf("bloc sans commande : extensions de l'utilisateur gardées :\n%s", s5)
	}
}

func TestDesktop(t *testing.T) {
	f := desktopFile("3dssavesync melonds", []string{"/home/a b/3dssavesync", "launch", "melonds", `100%"$`}, false)
	if !strings.Contains(f, `Exec="/home/a b/3dssavesync" launch melonds "100%%\"\$"`) {
		t.Errorf("Exec mal échappé :\n%s", f)
	}
}

// Full setup, then repair: real server, fake detected emulator, fake LaunchBox and ES-DE.
func TestTextSetup(t *testing.T) {
	tmp = t.TempDir()
	m := fakeHome(t)
	startTestServer(t)
	reset()
	emuExe := filepath.Join(tmp, "emus", "melonDS.exe")
	write(t, emuExe, []byte("melon"))
	fakeExe := filepath.Join(tmp, "telechargements", "3dssavesync.exe")
	write(t, fakeExe, []byte("lanceur"))
	detectBefore, executableBefore := detect, executable
	t.Cleanup(func() { detect, executable = detectBefore, executableBefore })
	detect = func(string) []Found { return []Found{{Profile: "melonds", Exe: emuExe, Source: "test"}} }
	executable = func() string { return fakeExe }
	lb := filepath.Join(tmp, "LaunchBox")
	write(t, filepath.Join(lb, "Data", "Emulators.xml"), []byte(emulatorsXML))
	if err := os.MkdirAll(filepath.Join(m, "HOME", "ES-DE"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		os.MkdirAll(filepath.Join(m, "USERPROFILE", "ES-DE"), 0o755)
	}
	saves, roms := filepath.Join(tmp, "saves"), filepath.Join(tmp, "roms")
	responses := []string{"1", url, "mauvais-jeton", "", "", "jeton-test", "", "", saves, roms, ""}
	if runtime.GOOS == "windows" {
		responses = append(responses, lb)
	}
	responses = append(responses, "", "n")
	var output bytes.Buffer
	config := filepath.Join(tmp, "cfg", "config.json")
	q := &Questions{in: bufio.NewReader(strings.NewReader(strings.Join(responses, "\n") + "\n")), out: &output}
	if err := textSetup(q, config); err != nil {
		t.Fatalf("assistant : %v\n%s", err, output.String())
	}
	if strings.Contains(output.String(), "jeton-test") {
		t.Error("le jeton ne doit jamais s'afficher")
	}
	if !strings.Contains(output.String(), "refuse ce jeton") || !strings.Contains(output.String(), "Connexion réussie") {
		t.Errorf("mauvais jeton refusé, puis connexion réussie attendus :\n%s", output.String())
	}
	var cfg Config
	if err := readJSON(config, &cfg); err != nil {
		t.Fatal(err)
	}
	e := cfg.Emulators["melonds"]
	if cfg.URL != url || cfg.Token != "jeton-test" || e == nil || e.Exe != emuExe || e.Saves["DS"] != saves || e.Roms["DS"] != roms {
		t.Fatalf("config.json incorrect : %+v\n%s", cfg, output.String())
	}
	if cfg.Launcher != installedLocation() || string(read(cfg.Launcher)) != "lanceur" {
		t.Errorf("exe copié à l'emplacement fixe attendu : %s", cfg.Launcher)
	}
	if !strings.Contains(string(read(filepath.Join(cfg.Frontends["esde"], "custom_systems", "es_systems.xml"))), "--emulator melonds") {
		t.Errorf("ES-DE configuré attendu :\n%s", output.String())
	}
	if runtime.GOOS == "windows" {
		lbx := string(read(filepath.Join(lb, "Data", "Emulators.xml")))
		if !strings.Contains(lbx, "MelonDS &amp; co (3dssavesync)") || len(namesIn(filepath.Join(lb, "Data"), "Emulators.xml.avant-3dssavesync-*")) != 1 {
			t.Errorf("LaunchBox configuré, avec une copie, attendu :\n%s", output.String())
		}
	}

	// Second opening: menu, repair, nothing changes.
	output.Reset()
	cfgBefore := string(read(config))
	q = &Questions{in: bufio.NewReader(strings.NewReader("2\n")), out: &output}
	if err := textSetup(q, config); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "ES-DE nds : déjà en place") || string(read(config)) != cfgBefore {
		t.Errorf("réparation sans changement attendue :\n%s", output.String())
	}
	if runtime.GOOS == "windows" && !strings.Contains(output.String(), "LaunchBox melonds : déjà en place") {
		t.Errorf("LaunchBox déjà en place attendu :\n%s", output.String())
	}
}

// Portable version (ADR 0020): portable.txt next to the exe. Settings in the exe's folder, exe never copied,
// frontends point to the running exe, read-only folder refused with a message.
func TestPortable(t *testing.T) {
	m := fakeHome(t)
	dir := filepath.Join(t.TempDir(), "LaunchBox", "ThirdParty", "3dssavesync")
	exe := filepath.Join(dir, "3dssavesync.exe")
	write(t, exe, []byte("lanceur"))
	executableBefore := executable
	t.Cleanup(func() { executable = executableBefore })
	executable = func() string { return exe }

	normal := installedLocation()
	if defaultConfig() == filepath.Join(dir, "config.json") || !strings.HasPrefix(defaultConfig(), m) || normal == exe {
		t.Fatalf("sans portable.txt : réglages dans la maison, pas à côté de l'exe (%s, %s)", defaultConfig(), normal)
	}
	write(t, filepath.Join(dir, "portable.txt"), []byte("portable"))
	if defaultConfig() != filepath.Join(dir, "config.json") {
		t.Errorf("config.json à côté de l'exe attendu : %s", defaultConfig())
	}
	if installedLocation() != exe || launcherOf(&Config{Launcher: filepath.Join("E:", "ancien", "3dssavesync.exe")}) != exe {
		t.Errorf("frontends et raccourcis vers l'exe en cours attendus : %s, %s", installedLocation(), launcherOf(&Config{Launcher: "x"}))
	}
	cfg := &Config{}
	if l := installExe(cfg, t.TempDir()); !strings.Contains(l, "Programme déjà installé") || cfg.Launcher != exe || exists(normal) {
		t.Errorf("l'exe ne doit pas être copié : %q, Lanceur %q", l, cfg.Launcher)
	}
	if err := checkPortable(dir); err != nil {
		t.Errorf("dossier modifiable : %v", err)
	}
	if n, _ := filepath.Glob(filepath.Join(dir, ".3dssavesync-essai-*")); len(n) != 0 {
		t.Errorf("fichier d'essai laissé : %v", n)
	}
	err := checkPortable(filepath.Join(dir, "absent"))
	if err == nil || !strings.Contains(err.Error(), "Déplacez ce dossier là où vous pouvez écrire") || !strings.Contains(err.Error(), "supprimez portable.txt") ||
		!strings.Contains(err.Error(), "\nDétail : ") {
		t.Errorf("dossier où rien ne s'écrit : message attendu, %v", err)
	}
}

// Options of the original LaunchBox emulator (the emulator's line, then its platforms for the systems): for
// args_en_plus, because the "(3dssavesync)" copy has an empty line.
func TestLaunchBoxOptions(t *testing.T) {
	lb, launcher := filepath.Join("L", "LaunchBox"), filepath.Join("P", "3dssavesync.exe")
	melon := filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe")
	if o := launchBoxOptions(emulatorsXML, lb, melon, []System{"DS"}); o != "-f" {
		t.Errorf("options de MelonDS : %q", o)
	}
	platform := strings.Replace(emulatorsXML, "<CommandLine />", "<CommandLine>--plein-ecran</CommandLine>", 1)
	if o := launchBoxOptions(platform, lb, melon, []System{"DS"}); o != "-f --plein-ecran" {
		t.Errorf("options de l'émulateur et de la plateforme DS : %q", o)
	}
	core := strings.Replace(emulatorsXML, "<CommandLine />", `<CommandLine>-L &quot;cores\melonds_libretro.dll&quot;</CommandLine>`, 1)
	if o := launchBoxOptions(core, lb, melon, []System{"DS"}); o != "-f" {
		t.Errorf("cœur RetroArch d'une plateforme repris : %q", o)
	}
	if o := launchBoxOptions(platform, lb, melon, []System{"GB"}); o != "-f" {
		t.Errorf("plateforme d'un autre système ignorée : %q", o)
	}
	installed, _ := launchbox(emulatorsXML, lb, launcher, []LBEntry{{"melonds", melon, []System{"DS"}}})
	if o := launchBoxOptions(installed, lb, melon, []System{"DS"}); o != "-f" {
		t.Errorf("après l'installation, lu dans l'émulateur d'origine : %q", o)
	}
	inPlace := strings.Replace(emulatorsXML, `Emulators\MelonDS\melonDS.exe`, launcher, 1)
	if o := launchBoxOptions(inPlace, lb, melon, []System{"DS"}); o != "" {
		t.Errorf("émulateur modifié sur place : rien à reprendre : %q", o)
	}
}
