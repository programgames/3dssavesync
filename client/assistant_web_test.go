package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Assistant web : accès réservé à la page (hôte et secret), page en français, état, test du serveur, enregistrement.
func TestWebSetup(t *testing.T) {
	fakeHome(t)
	reset()
	server := httptest.NewServer(&Server{data: t.TempDir(), token: "jeton-web"})
	defer server.Close()
	emuExe := filepath.Join(t.TempDir(), "melonDS.exe")
	write(t, emuExe, []byte("melon"))
	detectBefore := detect
	t.Cleanup(func() { detect = detectBefore })
	detect = func(string) []Found { return []Found{{Profile: "melonds", Exe: emuExe, Source: "test"}} }
	config := filepath.Join(t.TempDir(), "config.json")
	a := &WebSetup{config: config, secret: "s3cret", host: "127.0.0.1:9", end: make(chan struct{})}

	request := func(method, host, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
		r.Host = host
		if method == "POST" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	if request("GET", "127.0.0.1:9", "/autre/", "").Code != 404 || request("GET", "evil.example:9", "/s3cret/", "").Code != 404 {
		t.Error("sans le secret ou avec un autre hôte : 404 attendu")
	}
	if w := request("GET", "127.0.0.1:9", "/s3cret/", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Tester la connexion") {
		t.Errorf("page attendue : %d", w.Code)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:9/s3cret/api/etat", strings.NewReader("{}"))
	r.Host = "127.0.0.1:9"
	r.Header.Set("Content-Type", "text/plain") // formulaire d'une autre page : refusé
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Errorf("POST sans JSON : 405 attendu, %d", w.Code)
	}

	var state WebState
	json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/etat", "{}").Body.Bytes(), &state)
	// Premier lancement : l'émulateur trouvé est coché (rien à choisir avant). Avec une configuration, il ne l'est pas.
	if state.Exists || len(state.Emulators) != 1 || !state.Emulators[0].Active || state.Emulators[0].Systems[0].System != "DS" {
		t.Fatalf("état initial : %+v", state)
	}

	var test map[string]any
	json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/tester", `{"URL":"`+server.URL+`","Jeton":"faux"}`).Body.Bytes(), &test)
	if test["Ok"] != false || !strings.Contains(test["Message"].(string), "refuse ce jeton") {
		t.Errorf("mauvais jeton : %v", test)
	}

	emu := state.Emulators[0]
	emu.Active = true // coché par l'utilisateur
	emu.Systems[0].Saves = filepath.Join(t.TempDir(), "saves")
	choice := WebChoices{URL: server.URL, Token: "jeton-web", Emulators: []WebEmu{emu}}
	body, _ := json.Marshal(choice)
	var res struct {
		Lines []string `json:"Lignes"`
		Error string   `json:"Erreur"`
	}
	json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/appliquer", string(body)).Body.Bytes(), &res)
	if res.Error != "" {
		t.Fatalf("enregistrement : %s", res.Error)
	}
	var cfg Config
	readJSON(config, &cfg)
	if cfg.URL != server.URL || cfg.Token != "jeton-web" || cfg.Emulators["melonds"] == nil ||
		cfg.Emulators["melonds"].Saves["DS"] != emu.Systems[0].Saves {
		t.Fatalf("config.json : %+v", cfg)
	}

	choice.Token, choice.URL = "", "http://127.0.0.1:1" // serveur injoignable : rien n'est écrit
	body, _ = json.Marshal(choice)
	before := string(read(config))
	json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/appliquer", string(body)).Body.Bytes(), &res)
	if res.Error == "" || string(read(config)) != before {
		t.Errorf("serveur injoignable : erreur et config.json intact attendus (%q)", res.Error)
	}
	json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/etat", "{}").Body.Bytes(), &state)
	if !state.Exists || !state.HasToken || state.URL != server.URL || strings.Contains(request("POST", "127.0.0.1:9", "/s3cret/api/etat", "{}").Body.String(), "jeton-web") {
		t.Errorf("état après enregistrement (jamais le jeton) : %+v", state)
	}
	// Dossier des sauvegardes absent : la ligne de l'émulateur dit ce que dit Vérifications, jamais « Prêt ».
	if p := state.Emulators[0].Problems; len(p) != 1 || !strings.Contains(p[0], "melonds DS : dossier des sauvegardes introuvable") {
		t.Errorf("problème de melonds attendu : %q", p)
	}
	os.MkdirAll(emu.Systems[0].Saves, 0o755)
	if json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/etat", "{}").Body.Bytes(), &state); state.Emulators[0].Problems != nil {
		t.Errorf("dossier créé : plus de problème attendu (%q)", state.Emulators[0].Problems)
	}

	// « Ouvrir » : l'émulateur configuré, sans jeu, par launch ; un nom inconnu ne lance rien.
	var launched []string
	openDetached = func(exe string, args []string) error { launched = append([]string{exe}, args...); return nil }
	var opened map[string]any
	if len(state.Configured) != 1 || state.Configured[0] != "melonds" {
		t.Errorf("boutons « Ouvrir » : %v", state.Configured)
	}
	json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/ouvrir", `{"Nom":"melonds"}`).Body.Bytes(), &opened)
	if opened["Ok"] != true || strings.Join(launched[1:], " ") != "launch --config "+config+" melonds" {
		t.Errorf("ouvrir melonds : %v, commande %q", opened, launched)
	}
	launched = nil
	json.Unmarshal(request("POST", "127.0.0.1:9", "/s3cret/api/ouvrir", `{"Nom":"inconnu"}`).Body.Bytes(), &opened)
	if opened["Ok"] != false || launched != nil {
		t.Errorf("émulateur inconnu : refus attendu, rien de lancé (%v, %q)", opened, launched)
	}
}

// Émulateur personnalisé dans la page : contrôles, aperçu, enregistrement.
func TestWebSetupCustom(t *testing.T) {
	fakeHome(t)
	server := httptest.NewServer(&Server{data: t.TempDir(), token: "j"})
	defer server.Close()
	detectBefore := detect
	t.Cleanup(func() { detect = detectBefore })
	detect = func(string) []Found { return nil }
	config := filepath.Join(t.TempDir(), "config.json")
	a := &WebSetup{config: config}
	saves := filepath.Join(t.TempDir(), "saves")
	x := WebEmu{Name: "gearboy", Profile: "gearboy", Exe: `C:\Emu\gearboy.exe`, Active: true, Custom: true, Command: `-rom {rom} -batt "{sauvegardes}"`,
		Extension: "sav", Systems: []WebSystem{{System: "GB", Saves: saves}}}
	if r := preview(x); r["Erreur"] != "" || !strings.HasPrefix(r["Commande"], `C:\Emu\gearboy.exe -rom `) || !strings.Contains(r["Commande"], "Pokemon Cristal.gbc") ||
		!strings.Contains(r["Commande"], saves) {
		t.Errorf("aperçu : %v", r)
	}
	for _, bad := range []WebEmu{
		{Name: "mgba", Custom: true, Command: "{rom}", Systems: x.Systems},                                    // nom d'un profil intégré
		{Name: "GEARBOY", Custom: true, Command: "{rom}", Systems: x.Systems},                                 // majuscules
		{Name: "gearboy", Custom: true, Command: "-rom", Systems: x.Systems},                                  // sans {rom}
		{Name: "gearboy", Custom: true, Command: "{rom}"},                                                     // aucun Système
		{Name: "gearboy", Custom: true, Command: "{rom}", Systems: []WebSystem{{System: "GB"}}, Active: true}, // dossier vide
	} {
		bad.Active = true
		if _, err := a.apply(WebChoices{URL: server.URL, Token: "j", Emulators: []WebEmu{bad}}); err == nil {
			t.Errorf("refus attendu : %+v", bad)
		}
	}
	if _, err := a.apply(WebChoices{URL: server.URL, Token: "j", Emulators: []WebEmu{x}}); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	readJSON(config, &cfg)
	e := cfg.Emulators["gearboy"]
	if e == nil || strings.Join(e.Args, "|") != "-rom|{rom}|-batt|{sauvegardes}" || e.Extension != ".sav" || e.Saves["GB"] != saves {
		t.Fatalf("émulateur personnalisé dans config.json : %+v", e)
	}
	var state WebState = a.state()
	if len(state.Emulators) != 1 || !state.Emulators[0].Custom || state.Emulators[0].Command != `-rom {rom} -batt {sauvegardes}` {
		t.Errorf("relu par la page : %+v", state.Emulators)
	}
}

// Section ROM hacks : ajout (Clé créée, copie de config.json avant), liste, retrait.
func TestAliasWeb(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.json")
	if err := writeJSON(config, &Config{URL: "http://192.0.2.5", Token: "j"}, true); err != nil {
		t.Fatal(err)
	}
	a := &WebSetup{config: config}
	r := a.aliasWeb("Radical Red (4.10)", "GBA", false)
	var cfg Config
	readJSON(config, &cfg)
	if r["Erreur"] != nil || !keyShapes["GBA"].MatchString(cfg.Alias["Radical Red (4.10)"]) || strings.Join(r["Noms"].([]string), "|") != "Radical Red (4.10)" {
		t.Errorf("ajout : %v, %v", r, cfg.Alias)
	}
	if c, _ := filepath.Glob(config + ".avant-3dssavesync-*"); len(c) != 1 {
		t.Errorf("copie de config.json avant l'ajout : %v", c)
	}
	if r := a.aliasWeb("a=b", "GBA", false); r["Erreur"] == nil {
		t.Error("nom invalide accepté")
	}
	a.aliasWeb("Radical Red (4.10)", "", true)
	cfg = Config{}
	readJSON(config, &cfg)
	if len(cfg.Alias) != 0 || cfg.Token != "j" {
		t.Errorf("retrait : %+v", cfg)
	}
}

// Page : « Arguments en plus » proposé depuis l'émulateur LaunchBox d'origine pour un émulateur trouvé, absent de
// config.json ; jamais à la place d'une valeur de config.json, même vide ; jamais pour RetroArch ni NanoBoyAdvance.
func TestWebSetupLaunchBoxOptions(t *testing.T) {
	fakeHome(t)
	detectBefore := detect
	t.Cleanup(func() { detect = detectBefore })
	lb := filepath.Join(t.TempDir(), "LaunchBox")
	melon := filepath.Join(lb, "Emulators", "MelonDS", "melonDS.exe")
	nba := filepath.Join(lb, "Emulators", "NBA", "NanoBoyAdvance.exe")
	ra := filepath.Join(lb, "Emulators", "RetroArch", "retroarch.exe")
	xml := strings.NewReplacer(`Emulators\Cemu\Cemu.exe`, `Emulators\NBA\NanoBoyAdvance.exe`, "</LaunchBox>",
		"  <Emulator>\r\n    <ApplicationPath>Emulators\\RetroArch\\retroarch.exe</ApplicationPath>\r\n    <CommandLine>-f</CommandLine>\r\n    <ID>cccc</ID>\r\n  </Emulator>\r\n"+
			"  <EmulatorPlatform>\r\n    <Emulator>cccc</Emulator>\r\n    <Platform>Nintendo Game Boy Advance</Platform>\r\n    <CommandLine>-L &quot;cores\\mgba_libretro.dll&quot;</CommandLine>\r\n  </EmulatorPlatform>\r\n</LaunchBox>").Replace(emulatorsXML)
	write(t, filepath.Join(lb, "Data", "Emulators.xml"), []byte(xml))
	for _, x := range []string{melon, nba, ra, filepath.Join(filepath.Dir(ra), "cores", "mgba_libretro.dll")} {
		write(t, x, []byte("exe"))
	}
	config := filepath.Join(t.TempDir(), "config.json")
	a := &WebSetup{config: config}
	for _, c := range []struct {
		emus     map[string]*Emulator
		found    Found
		expected string
	}{
		{map[string]*Emulator{"melonds": {Exe: melon}}, Found{}, ""}, // vide enregistré : reste vide
		{map[string]*Emulator{"melonds": {Exe: melon, ExtraArgs: []string{"-g"}}}, Found{}, "-g"},
		{nil, Found{Profile: "melonds", Exe: melon}, "-f"},
		{nil, Found{Profile: "nanoboyadvance", Exe: nba}, ""},
		{nil, Found{Profile: "retroarch", Exe: ra}, ""},
	} {
		detect = func(string) []Found {
			if c.found.Exe == "" {
				return nil
			}
			return []Found{c.found}
		}
		writeConfig(t, config, Config{Frontends: map[string]string{"launchbox": lb}, Emulators: c.emus})
		if e := a.state().Emulators; len(e) != 1 || e[0].Extra != c.expected {
			t.Errorf("%v %+v : %+v", c.emus, c.found, e)
		}
	}
}
