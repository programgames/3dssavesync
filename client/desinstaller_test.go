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
	defait, rapport := defaireLaunchbox(installe, lanceur)
	if defait != emulatorsXML {
		t.Errorf("Emulators.xml doit revenir à l'identique :\n%s", defait)
	}
	if len(rapport) != 1 || !strings.Contains(rapport[0], "MelonDS & co") {
		t.Errorf("rapport : %q", rapport)
	}
	if encore, r := defaireLaunchbox(emulatorsXML, lanceur); encore != emulatorsXML || len(r) != 0 {
		t.Errorf("rien à retirer : rien ne change (%q)", r)
	}

	t.Log("Émulateur d'origine supprimé depuis dans LaunchBox : l'émulateur 3dssavesync part, le rapport le dit")
	sansOrigine := blocEmulateurLB.ReplaceAllStringFunc(installe, func(b string) string {
		if champ(b, "ID") == "aaaa" {
			return ""
		}
		return b
	})
	d, r := defaireLaunchbox(sansOrigine, lanceur)
	xmlValide(t, "Emulators.xml sans origine", d)
	if strings.Contains(d, "3dssavesync") || len(r) != 1 || !strings.Contains(r[0], "choisissez un émulateur par défaut") {
		t.Errorf("rapport %q :\n%s", r, d)
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
	if strings.Join(raccourcisPour, "|") != lanceur+"|"+emplacementInstalle() || !strings.Contains(texte, "2 raccourci(s)") || !strings.Contains(texte, filepath.Join(tmp, "cfg", "serveur")) ||
		!strings.Contains(texte, filepath.Join(tmp, "cfg", "copies")) || !strings.Contains(texte, "FBI") {
		t.Errorf("rapport :\n%s", texte)
	}
}
