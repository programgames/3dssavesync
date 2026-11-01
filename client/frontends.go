package main

// Configuration des frontends (phase 4) : LaunchBox, ES-DE, raccourcis. Chaque écriture garde une copie datée
// du fichier, et refaire la configuration ne change rien quand elle est déjà en place (réparation).
// Sources : docs/research-v3-frontends.md.

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
)

// Copie datée d'un fichier avant de le modifier ; rien si le fichier n'existe pas.
func copieAvant(chemin string) error {
	o, err := os.ReadFile(chemin)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return ecrireVerifie(chemin+".avant-3dssavesync-"+time.Now().Format("20060102-150405"), o)
}

// Remplacement d'un fichier de frontend : copie datée, fichier temporaire vérifié, puis renommage.
func remplacerFichier(chemin string, contenu []byte) error {
	if err := copieAvant(chemin); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(chemin), 0o755); err != nil {
		return err
	}
	tmp := chemin + ".3dssavesync-tmp"
	os.Remove(tmp)
	if err := ecrireVerifie(tmp, contenu); err != nil {
		return err
	}
	return os.Rename(tmp, chemin)
}

func echapperXML(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func nouvelID() string {
	o := make([]byte, 16)
	rand.Read(o)
	o[6], o[8] = o[6]&0x0F|0x40, o[8]&0x3F|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", o[0:4], o[4:6], o[6:8], o[8:10], o[10:])
}

// --- LaunchBox -----------------------------------------------------------------------------------

// Plateformes LaunchBox -> Système.
var plateformesLB = map[string]string{
	"Nintendo DS": "DS", "Nintendo DSi": "DS", "Nintendo Game Boy": "GB", "Nintendo Game Boy Color": "GB",
	"Nintendo Game Boy Advance": "GBA", "Nintendo 3DS": "3DS",
}

var (
	blocEmulateurLB  = regexp.MustCompile(`(?s)  <Emulator>\r?\n.*?  </Emulator>\r?\n`)
	blocPlateformeLB = regexp.MustCompile(`(?s)  <EmulatorPlatform>\r?\n.*?  </EmulatorPlatform>\r?\n`)
)

func champ(bloc, nom string) string {
	m := regexp.MustCompile(`<` + nom + `>(.*?)</` + nom + `>`).FindStringSubmatch(bloc)
	if m == nil {
		return ""
	}
	return m[1]
}

func fixerChamp(bloc, nom, valeur string) string {
	r := regexp.MustCompile(`<` + nom + `>.*?</` + nom + `>|<` + nom + ` />`)
	if valeur == "" {
		return r.ReplaceAllLiteralString(bloc, "<"+nom+" />")
	}
	return r.ReplaceAllLiteralString(bloc, "<"+nom+">"+echapperXML(valeur)+"</"+nom+">")
}

// EntreeLB : un émulateur de la configuration, à faire passer par 3dssavesync dans LaunchBox.
type EntreeLB struct {
	Nom, Exe string // nom dans config.json, exécutable de l'émulateur réel
	Systemes []string
}

// launchbox : pour chaque entrée, un nouvel émulateur « <titre> (3dssavesync) » copié de l'émulateur d'origine,
// devenu l'émulateur par défaut de ses plateformes. L'émulateur d'origine reste intact (seul Default change).
// Rien n'est fait si un émulateur passe déjà par 3dssavesync pour cette entrée.
func launchbox(texte, dossierLB, lanceur string, entrees []EntreeLB) (string, []string) {
	var rapport []string
	unescape := func(s string) string { return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(s) }
	chemin := func(app string) string {
		app = filepath.FromSlash(strings.ReplaceAll(unescape(app), `\`, "/")) // chemins Windows, même lus ailleurs (Wine)
		if app != "" && !filepath.IsAbs(app) && !strings.Contains(app, ":") {
			app = filepath.Join(dossierLB, app)
		}
		return filepath.Clean(app)
	}
	for _, e := range entrees {
		emus := blocEmulateurLB.FindAllString(texte, -1)
		plats := blocPlateformeLB.FindAllString(texte, -1)
		dejaFait, original := false, ""
		for _, b := range emus {
			app := chemin(champ(b, "ApplicationPath"))
			if strings.EqualFold(filepath.Base(app), filepath.Base(lanceur)) {
				for _, p := range plats {
					if champ(p, "Emulator") == champ(b, "ID") && strings.Contains(" "+champ(p, "CommandLine")+" ", " --emulator "+e.Nom+" ") {
						dejaFait = true
					}
				}
			} else if original == "" && (strings.EqualFold(app, filepath.Clean(e.Exe)) || strings.EqualFold(filepath.Base(app), filepath.Base(e.Exe))) {
				original = b
			}
		}
		switch {
		case dejaFait:
			rapport = append(rapport, fmt.Sprintf(tr("%s : déjà en place", "%s: already set up"), e.Nom))
			continue
		case original == "":
			rapport = append(rapport, fmt.Sprintf(tr("%s : émulateur absent de LaunchBox (ajoute-le dans LaunchBox, puis relance la réparation)",
				"%s: emulator not in LaunchBox (add it in LaunchBox, then run the repair again)"), e.Nom))
			continue
		}
		idOrig, id := champ(original, "ID"), nouvelID()
		nouveau := fixerChamp(original, "ID", id)
		nouveau = fixerChamp(nouveau, "Title", unescape(champ(original, "Title"))+" (3dssavesync)")
		nouveau = fixerChamp(nouveau, "ApplicationPath", lanceur)
		nouveau = fixerChamp(nouveau, "CommandLine", "")
		for _, c := range []string{"HideConsole", "NoQuotes", "AutoExtract"} {
			nouveau = fixerChamp(nouveau, c, "false")
		}
		var ajout []string
		for _, p := range plats {
			sys := plateformesLB[unescape(champ(p, "Platform"))]
			if champ(p, "Emulator") != idOrig || !slices.Contains(e.Systemes, sys) {
				continue
			}
			q := fixerChamp(p, "Emulator", id)
			q = fixerChamp(q, "CommandLine", "run --emulator "+e.Nom) // LaunchBox ajoute la ROM à la fin
			q = fixerChamp(q, "Default", "true")
			if sys != "3DS" { // le Lanceur lit les .zip ; pas les .3ds dans un .zip
				q = fixerChamp(q, "AutoExtract", "false")
			}
			ajout = append(ajout, q)
			texte = strings.Replace(texte, p, fixerChamp(p, "Default", "false"), 1)
		}
		if len(ajout) == 0 {
			rapport = append(rapport, fmt.Sprintf(tr("%s : aucune plateforme DS, GB, GBA ou 3DS dans LaunchBox", "%s: no DS, GB, GBA or 3DS platform in LaunchBox"), e.Nom))
			continue
		}
		i := strings.LastIndex(texte, "</LaunchBox>")
		texte = texte[:i] + nouveau + strings.Join(ajout, "") + texte[i:]
		rapport = append(rapport, fmt.Sprintf(tr("%s : émulateur « %s (3dssavesync) » ajouté, par défaut pour %d plateforme(s)",
			"%s: emulator \"%s (3dssavesync)\" added, default for %d platform(s)"), e.Nom, unescape(champ(original, "Title")), len(ajout)))
	}
	return texte, rapport
}

// --- ES-DE ---------------------------------------------------------------------------------------

// Systèmes ES-DE -> Système, nom complet, extensions lues par le Lanceur.
var systemesESDE = map[string][3]string{
	"nds":  {"DS", "Nintendo DS", ".nds .NDS .zip .ZIP"},
	"gb":   {"GB", "Nintendo Game Boy", ".gb .GB .zip .ZIP"},
	"gbc":  {"GB", "Nintendo Game Boy Color", ".gbc .GBC .zip .ZIP"},
	"gba":  {"GBA", "Nintendo Game Boy Advance", ".gba .GBA .zip .ZIP"},
	"n3ds": {"3DS", "Nintendo 3DS", ".3ds .3DS .cci .CCI .cxi .CXI"},
}

// esde : règle 3DSSAVESYNC dans es_find_rules.xml, et une commande « 3dssavesync » en tête de chaque système géré
// dans es_systems.xml (première commande = émulateur par défaut). Un système absent du fichier personnalisé reçoit
// un bloc minimal (le bloc fourni par ES-DE n'est pas lisible dans une AppImage).
// emus : Système -> nom de l'entrée de config.json.
func esde(systems, regles, lanceur string, emus map[string]string) (string, string, []string) {
	var rapport []string
	regle := "  <emulator name=\"3DSSAVESYNC\">\n    <rule type=\"staticpath\">\n      <entry>" + echapperXML(lanceur) + "</entry>\n    </rule>\n  </emulator>\n"
	ancienne := regexp.MustCompile(`(?s)  <emulator name="3DSSAVESYNC">.*?</emulator>\r?\n`)
	switch {
	case ancienne.MatchString(regles):
		regles = ancienne.ReplaceAllLiteralString(regles, regle)
	case strings.Contains(regles, "</ruleList>"):
		regles = strings.Replace(regles, "</ruleList>", regle+"</ruleList>", 1)
	default:
		regles = "<?xml version=\"1.0\"?>\n<ruleList>\n" + regle + "</ruleList>\n"
	}
	if !strings.Contains(systems, "</systemList>") {
		systems = "<?xml version=\"1.0\"?>\n<systemList>\n</systemList>\n"
	}
	var noms []string
	for n := range systemesESDE {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	for _, n := range noms {
		d := systemesESDE[n]
		nom := emus[d[0]]
		if nom == "" {
			continue
		}
		commande := `<command label="3dssavesync (` + nom + `)">%EMULATOR_3DSSAVESYNC% run --emulator ` + nom + ` %ROM%</command>`
		bloc := regexp.MustCompile(`(?s)<system>\s*<name>` + n + `</name>.*?</system>`).FindString(systems)
		switch {
		case strings.Contains(bloc, "--emulator "+nom+" "):
			rapport = append(rapport, fmt.Sprintf(tr("ES-DE %s : déjà en place", "ES-DE %s: already set up"), n))
		case bloc != "" && strings.Contains(bloc, "<command"):
			i := strings.Index(bloc, "<command")
			systems = strings.Replace(systems, bloc, bloc[:i]+commande+"\n        "+bloc[i:], 1)
			rapport = append(rapport, fmt.Sprintf(tr("ES-DE %s : commande ajoutée en premier", "ES-DE %s: command added first"), n))
		default:
			if bloc != "" {
				systems = strings.Replace(systems, bloc, "", 1)
			}
			nouveau := "    <system>\n        <name>" + n + "</name>\n        <fullname>" + d[1] + "</fullname>\n        <path>%ROMPATH%/" + n +
				"</path>\n        <extension>" + d[2] + "</extension>\n        " + commande + "\n        <platform>" + n +
				"</platform>\n        <theme>" + n + "</theme>\n    </system>\n"
			systems = strings.Replace(systems, "</systemList>", nouveau+"</systemList>", 1)
			rapport = append(rapport, fmt.Sprintf(tr("ES-DE %s : système ajouté (seulement la commande 3dssavesync)", "ES-DE %s: system added (3dssavesync command only)"), n))
		}
	}
	return systems, regles, rapport
}

// --- Raccourcis ----------------------------------------------------------------------------------

// Argument de la clé Exec d'un .desktop : guillemets si besoin, \ devant " ` $ \ ; % doublé.
func argDesktop(a string) string {
	a = strings.ReplaceAll(a, "%", "%%")
	if !strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`") {
		return a
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(a) + `"`
}

func fichierDesktop(nom string, exec []string, terminal bool) string {
	var a []string
	for _, x := range exec {
		a = append(a, argDesktop(x))
	}
	return fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s\nTerminal=%t\nCategories=Game;\n", nom, strings.Join(a, " "), terminal)
}

// raccourci : un lanceur « nom » qui exécute lanceur + args. Windows : .lnk sur le bureau ; Linux : .desktop dans
// le menu des applications ; macOS : .app dans ~/Applications. console : l'assistant (terminal sous Linux et macOS).
func raccourci(nom, lanceur string, args []string, console bool) (string, error) {
	switch runtime.GOOS {
	case "windows":
		c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			`[Console]::OutputEncoding=[Text.Encoding]::UTF8; $d=[Environment]::GetFolderPath('Desktop'); $l=Join-Path $d ($env:R_NOM+'.lnk'); `+
				`$s=(New-Object -ComObject WScript.Shell).CreateShortcut($l); $s.TargetPath=$env:R_CIBLE; $s.Arguments=$env:R_ARGS; `+
				`$s.WorkingDirectory=(Split-Path $env:R_CIBLE); $s.Save(); $l`)
		c.Env = append(os.Environ(), "R_NOM="+nom, "R_CIBLE="+lanceur, "R_ARGS="+strings.Join(args, " "))
		o, err := c.Output()
		return strings.TrimSpace(string(o)), err
	case "darwin":
		app := maison("Applications", nom+".app")
		script := "#!/bin/sh\nexec " + argShell(lanceur) + " " + strings.Join(mapper(args, argShell), " ") + "\n"
		if console {
			script = "#!/bin/sh\nexec osascript -e " + argShell(`tell application "Terminal" to do script "`+strings.ReplaceAll(argShell(lanceur), `"`, `\"`)+` `+strings.Join(args, " ")+`"`) + "\n"
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?>` + "\n<plist version=\"1.0\"><dict><key>CFBundleExecutable</key><string>lanceur</string>" +
			"<key>CFBundleName</key><string>" + echapperXML(nom) + "</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>\n"
		if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
			return "", err
		}
		return app, os.WriteFile(filepath.Join(app, "Contents", "MacOS", "lanceur"), []byte(script), 0o755)
	}
	f := xdgData("applications", strings.ReplaceAll(strings.ToLower(nom), " ", "-")+".desktop")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return "", err
	}
	return f, os.WriteFile(f, []byte(fichierDesktop(nom, append([]string{lanceur}, args...), console)), 0o755)
}

func argShell(a string) string { return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'" }

func mapper(l []string, f func(string) string) []string {
	r := make([]string, len(l))
	for i, x := range l {
		r[i] = f(x)
	}
	return r
}

// --- Réparation ----------------------------------------------------------------------------------

// reparer : refait la configuration des frontends notés dans config.json. Un frontend ouvert est sauté.
func reparer(cfg *Config, lanceur string) []string {
	var rapport []string
	var lb []EntreeLB
	emus := map[string]string{}
	var noms []string
	for n := range cfg.Emulateurs {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	for _, n := range noms {
		e := cfg.Emulateurs[n]
		p, err := e.profil(n)
		if err != nil {
			continue
		}
		lb = append(lb, EntreeLB{n, e.Exe, e.systemes(p)})
		for _, s := range e.systemes(p) {
			if emus[s] == "" {
				emus[s] = n
			}
		}
	}
	if d := cfg.Frontends["launchbox"]; d != "" {
		f := filepath.Join(d, "Data", "Emulators.xml")
		o, err := os.ReadFile(f)
		switch {
		case emulateurActif("LaunchBox") || emulateurActif("BigBox"):
			rapport = append(rapport, tr("LaunchBox : ouvert, rien n'est fait. Ferme LaunchBox et BigBox, puis relance la réparation.",
				"LaunchBox: open, nothing done. Close LaunchBox and BigBox, then run the repair again."))
		case err != nil:
			rapport = append(rapport, "LaunchBox : "+err.Error())
		default:
			nouveau, r := launchbox(string(o), d, lanceur, lb)
			rapport = append(rapport, prefixer("LaunchBox", r)...)
			if nouveau != string(o) {
				if err := remplacerFichier(f, []byte(nouveau)); err != nil {
					rapport = append(rapport, "LaunchBox : "+err.Error())
				}
			}
		}
	}
	if d := cfg.Frontends["esde"]; d != "" {
		fs, fr := filepath.Join(d, "custom_systems", "es_systems.xml"), filepath.Join(d, "custom_systems", "es_find_rules.xml")
		s, _ := os.ReadFile(fs)
		r, _ := os.ReadFile(fr)
		if emulateurActif("ES-DE") {
			rapport = append(rapport, tr("ES-DE : ouvert, rien n'est fait. Ferme ES-DE, puis relance la réparation.",
				"ES-DE: open, nothing done. Close ES-DE, then run the repair again."))
		} else {
			ns, nr, rap := esde(string(s), string(r), lanceur, emus)
			rapport = append(rapport, rap...)
			for f, c := range map[string][2]string{fs: {string(s), ns}, fr: {string(r), nr}} {
				if c[0] != c[1] {
					if err := remplacerFichier(f, []byte(c[1])); err != nil {
						rapport = append(rapport, "ES-DE : "+err.Error())
					}
				}
			}
		}
	}
	if a := avertissementEmuDeck(); a != "" {
		rapport = append(rapport, a)
	}
	return rapport
}

func prefixer(p string, l []string) []string {
	for i := range l {
		l[i] = p + " " + l[i]
	}
	return l
}

// ponytail: clé cloud_sync_status de ~/emudeck/settings.sh non vérifiée (research-v3-frontends §5) ; avertissement seulement.
func avertissementEmuDeck() string {
	o, err := os.ReadFile(maison("emudeck", "settings.sh"))
	if err == nil && regexp.MustCompile(`(?m)^cloud_sync_status="?true"?`).Match(o) {
		return tr("EmuDeck : CloudSync semble actif. Désactive-le pour DS, GB, GBA et 3DS : deux outils enverraient les mêmes sauvegardes.",
			"EmuDeck: CloudSync seems active. Turn it off for DS, GB, GBA and 3DS: two tools would upload the same saves.")
	}
	return ""
}
