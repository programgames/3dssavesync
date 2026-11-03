package main

// Configuration des frontends (phase 4) : LaunchBox, ES-DE, raccourcis. Chaque écriture garde une copie datée
// du fichier, et refaire la configuration ne change rien quand elle est déjà en place (réparation).
// Sources : docs/research-v3-frontends.md.

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
)

// Copie datée d'un fichier avant de le modifier ; rien si le fichier n'existe pas. Deux changements dans la même
// seconde (ajout puis retrait d'un alias) : -1, -2… en plus, jamais d'écrasement d'une copie.
func copieAvant(chemin string) error {
	o, err := os.ReadFile(chemin)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	nom := chemin + ".avant-3dssavesync-" + time.Now().Format("20060102-150405")
	for i := 1; ; i++ {
		err = ecrireVerifie(nom, o)
		if !errors.Is(err, fs.ErrExist) || i == 10 {
			return err
		}
		nom = fmt.Sprintf("%s.avant-3dssavesync-%s-%d", chemin, time.Now().Format("20060102-150405"), i)
	}
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

// cheminLB : chemin complet d'un ApplicationPath de LaunchBox (relatif à son dossier quand il est dedans).
func cheminLB(dossierLB, app string) string {
	app = filepath.FromSlash(strings.ReplaceAll(html.UnescapeString(app), `\`, "/")) // chemins Windows, même lus ailleurs (Wine)
	if app != "" && !filepath.IsAbs(app) && !strings.Contains(app, ":") {
		app = filepath.Join(dossierLB, app)
	}
	return filepath.Clean(app)
}

// optionsLaunchBox : les options de l'utilisateur dans l'émulateur LaunchBox d'origine de exe (sa ligne de commande,
// puis celles de ses plateformes de systemes), pour args_en_plus : la copie « (3dssavesync) » a une ligne vide.
// Émulateur modifié sur place (versions 1 et 2) : son exe est le Lanceur, rien n'est trouvé.
func optionsLaunchBox(texte, dossierLB, exe string, systemes []string) string {
	for _, b := range blocEmulateurLB.FindAllString(texte, -1) {
		app := cheminLB(dossierLB, champ(b, "ApplicationPath"))
		if !strings.EqualFold(app, filepath.Clean(exe)) && !strings.EqualFold(filepath.Base(app), filepath.Base(exe)) {
			continue
		}
		var options []string
		ajouter := func(o string) {
			// jamais le cœur RetroArch d'une plateforme (-L) : il irait à tous les systèmes
			if o = strings.TrimSpace(html.UnescapeString(o)); o != "" && !slices.Contains(options, o) && !strings.Contains(" "+o, " -L ") {
				options = append(options, o)
			}
		}
		ajouter(champ(b, "CommandLine"))
		for _, p := range blocPlateformeLB.FindAllString(texte, -1) {
			if champ(p, "Emulator") == champ(b, "ID") && slices.Contains(systemes, plateformesLB[html.UnescapeString(champ(p, "Platform"))]) {
				ajouter(champ(p, "CommandLine"))
			}
		}
		return strings.Join(options, " ")
	}
	return ""
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
	chemin := func(app string) string { return cheminLB(dossierLB, app) }
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
			rapport = append(rapport, fmt.Sprintf(tr("%s : émulateur absent de LaunchBox. Ajoutez-le dans LaunchBox et relancez la réparation.",
				"%s: emulator not found in LaunchBox. Add it in LaunchBox and run the repair again."), e.Nom))
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
		rapport = append(rapport, fmt.Sprintf(tr("%s : émulateur « %s (3dssavesync) » ajouté et mis par défaut. Plateformes : %d",
			"%s: added the \"%s (3dssavesync)\" emulator as the default. Platforms: %d"), e.Nom, unescape(champ(original, "Title")), len(ajout)))
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
			rapport = append(rapport, fmt.Sprintf(tr("ES-DE %s : commande ajoutée en tête de liste", "ES-DE %s: command added at the top of the list"), n))
		default:
			ext := d[2]
			if bloc != "" {
				if e := champ(bloc, "extension"); e != "" { // liste de l'utilisateur gardée, jamais réduite
					ext = e
				}
				systems = strings.Replace(systems, bloc, "", 1)
			}
			nouveau := "    <system>\n        <name>" + n + "</name>\n        <fullname>" + d[1] + "</fullname>\n        <path>%ROMPATH%/" + n +
				"</path>\n        <extension>" + ext + "</extension>\n        " + commande + "\n        <platform>" + n +
				"</platform>\n        <theme>" + n + "</theme>\n    </system>\n"
			systems = strings.Replace(systems, "</systemList>", nouveau+"</systemList>", 1)
			rapport = append(rapport, fmt.Sprintf(tr("ES-DE %s : système ajouté, avec seulement la commande 3dssavesync", "ES-DE %s: system added, with the 3dssavesync command only"), n))
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
		return lnk("Desktop", nom, lanceur, args)
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

// Raccourci .lnk dans un dossier spécial de Windows (Desktop, Startup). Arguments avec espace entre guillemets.
func lnk(dossier, nom, lanceur string, args []string) (string, error) {
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`[Console]::OutputEncoding=[Text.Encoding]::UTF8; $d=[Environment]::GetFolderPath($env:R_DOSSIER); $l=Join-Path $d ($env:R_NOM+'.lnk'); `+
			`$s=(New-Object -ComObject WScript.Shell).CreateShortcut($l); $s.TargetPath=$env:R_CIBLE; $s.Arguments=$env:R_ARGS; `+
			`$s.WorkingDirectory=(Split-Path $env:R_CIBLE); $s.Save(); $l`)
	var a []string
	for _, x := range args {
		if strings.ContainsAny(x, " \t") {
			x = `"` + x + `"`
		}
		a = append(a, x)
	}
	c.Env = append(os.Environ(), "R_DOSSIER="+dossier, "R_NOM="+nom, "R_CIBLE="+lanceur, "R_ARGS="+strings.Join(a, " "))
	o, err := c.Output()
	return strings.TrimSpace(string(o)), err
}

// demarrageAuto : lance lanceur + args à l'ouverture de session. Windows : .lnk dans Démarrage ; Linux : autostart XDG ;
// macOS : LaunchAgent.
func demarrageAuto(nom, lanceur string, args []string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return lnk("Startup", nom, lanceur, args)
	case "darwin":
		f := maison("Library", "LaunchAgents", "com.3dssavesync.serveur.plist")
		var p strings.Builder
		for _, x := range append([]string{lanceur}, args...) {
			p.WriteString("<string>" + echapperXML(x) + "</string>")
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?>` + "\n<plist version=\"1.0\"><dict><key>Label</key><string>com.3dssavesync.serveur</string>" +
			"<key>ProgramArguments</key><array>" + p.String() + "</array><key>RunAtLoad</key><true/></dict></plist>\n"
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			return "", err
		}
		return f, os.WriteFile(f, []byte(plist), 0o644)
	}
	f := xdgConfig("autostart", "3dssavesync-serveur.desktop")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return "", err
	}
	return f, os.WriteFile(f, []byte(fichierDesktop(nom, append([]string{lanceur}, args...), false)), 0o644)
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
	lb, emus := entreesFrontends(cfg)
	if d := cfg.Frontends["launchbox"]; d != "" {
		f := filepath.Join(d, "Data", "Emulators.xml")
		o, err := os.ReadFile(f)
		switch {
		case emulateurActif("LaunchBox") || emulateurActif("BigBox"):
			rapport = append(rapport, tr("LaunchBox est ouvert, rien n'a été modifié. Fermez LaunchBox et BigBox, puis relancez la réparation.",
				"LaunchBox is open, so nothing was changed. Close LaunchBox and BigBox, then run the repair again."))
		case err != nil:
			rapport = append(rapport, "LaunchBox : "+texteErreur(err))
		default:
			nouveau, r := launchbox(string(o), d, lanceur, lb)
			rapport = append(rapport, prefixer("LaunchBox", r)...)
			if nouveau != string(o) {
				if err := remplacerFichier(f, []byte(nouveau)); err != nil {
					rapport = append(rapport, "LaunchBox : "+texteErreur(err))
				}
			}
		}
	}
	if d := cfg.Frontends["esde"]; d != "" {
		fs, fr := filepath.Join(d, "custom_systems", "es_systems.xml"), filepath.Join(d, "custom_systems", "es_find_rules.xml")
		s, _ := os.ReadFile(fs)
		r, _ := os.ReadFile(fr)
		if emulateurActif("ES-DE") {
			rapport = append(rapport, tr("ES-DE est ouvert, rien n'a été modifié. Fermez-le et relancez la réparation.",
				"ES-DE is open, so nothing was changed. Close it and run the repair again."))
		} else {
			ns, nr, rap := esde(string(s), string(r), lanceur, emus)
			rapport = append(rapport, rap...)
			for f, c := range map[string][2]string{fs: {string(s), ns}, fr: {string(r), nr}} {
				if c[0] != c[1] {
					if err := remplacerFichier(f, []byte(c[1])); err != nil {
						rapport = append(rapport, "ES-DE : "+texteErreur(err))
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

// entreesFrontends : les émulateurs de config.json pour LaunchBox, et pour ES-DE le premier (par nom) de chaque Système.
func entreesFrontends(cfg *Config) ([]EntreeLB, map[string]string) {
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
	return lb, emus
}

// manquantsLaunchBox : les entrées dont l'émulateur d'origine est dans LaunchBox, mais sans émulateur
// « (3dssavesync) » par défaut sur une de ses plateformes (réparation à refaire).
func manquantsLaunchBox(texte, lanceur string, entrees []EntreeLB) []string {
	base := func(app string) string { return strings.ToLower(path.Base(strings.ReplaceAll(app, `\`, "/"))) }
	emus, plats := blocEmulateurLB.FindAllString(texte, -1), blocPlateformeLB.FindAllString(texte, -1)
	var manquants []string
	for _, e := range entrees {
		pret, original := false, false
		for _, b := range emus {
			app := base(champ(b, "ApplicationPath"))
			original = original || app == base(e.Exe)
			for _, p := range plats {
				pret = pret || app == base(lanceur) && champ(p, "Emulator") == champ(b, "ID") && champ(p, "Default") == "true" &&
					strings.Contains(" "+champ(p, "CommandLine")+" ", " --emulator "+e.Nom+" ")
			}
		}
		if original && !pret {
			manquants = append(manquants, e.Nom)
		}
	}
	return manquants
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
		return tr("EmuDeck CloudSync semble actif. Désactivez-le pour DS, GB, GBA et 3DS, sinon les deux outils enverront les mêmes sauvegardes.",
			"EmuDeck CloudSync seems to be on. Turn it off for DS, GB, GBA and 3DS, or both tools will upload the same saves.")
	}
	return ""
}
