package main

// Désinstallation (ADR 0019) : défait ce que l'installation a fait hors des dossiers de 3dssavesync, pour que LaunchBox
// et ES-DE lancent de nouveau leurs jeux sans lui. Rien n'est effacé définitivement : les fichiers des frontends sont
// copiés avant (remplacerFichier), les raccourcis et le programme vont à la Corbeille (Windows). Les réglages, les copies
// de sécurité et les données du serveur local restent : ce sont les données de l'utilisateur. Version portable
// (ADR 0020) : le programme et son dossier restent, le rapport dit de supprimer ce dossier.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	regleESDE    = regexp.MustCompile(`(?s)  <emulator name="3DSSAVESYNC">.*?</emulator>\r?\n`)
	commandeESDE = regexp.MustCompile(`<command label="3dssavesync \([^"]*\)">[^<]*</command>\r?\n[ \t]*`)
	systemeESDE  = regexp.MustCompile(`(?s)[ \t]*<system>.*?</system>\r?\n?`)
	// Ligne de commande LaunchBox d'un émulateur qui passe par le Lanceur : « run --emulator <nom> ».
	optionEmulateur = regexp.MustCompile(`--emulator (\S+)`)
)

// Adaptateurs remplacés par les tests (PowerShell, Corbeille, processus détaché).
var retirerRaccourcis, retirerProgramme = retirerRaccourcisSysteme, retirerProgrammeSysteme

// defaireLaunchbox : retire chaque copie « <titre> (3dssavesync) » qui lance le Lanceur, et ses plateformes ;
// l'émulateur dont elle est la copie (même titre sans « (3dssavesync) ») redevient l'émulateur par défaut de ces
// plateformes. Un émulateur qui lance le Lanceur sous son titre d'origine a été modifié sur place (assistant des
// versions 1 et 2) : il reste avec ses plateformes, et retrouve l'exe réel de config.json (nom lu dans
// « --emulator <nom> ») ; exe inconnu ou absent : laissé tel quel, le rapport dit de le corriger à la main.
func defaireLaunchbox(texte, dossierLB, lanceur string, cfg *Config) (string, []string) {
	var rapport []string
	unescape := func(s string) string { return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(s) }
	notre := func(b string) bool {
		return strings.EqualFold(path.Base(strings.ReplaceAll(unescape(champ(b, "ApplicationPath")), `\`, "/")), filepath.Base(lanceur))
	}
	copie := func(b string) bool { return strings.HasSuffix(champ(b, "Title"), " (3dssavesync)") }
	for _, b := range blocEmulateurLB.FindAllString(texte, -1) {
		if !notre(b) || copie(b) {
			continue
		}
		titre, nom := unescape(champ(b, "Title")), ""
		blocs := []string{b}
		for _, p := range blocPlateformeLB.FindAllString(texte, -1) {
			if champ(p, "Emulator") == champ(b, "ID") {
				blocs = append(blocs, p)
			}
		}
		for _, x := range blocs {
			if m := optionEmulateur.FindStringSubmatch(unescape(champ(x, "CommandLine"))); m != nil && nom == "" {
				nom = m[1]
			}
		}
		e := cfg.Emulateurs[nom]
		if e == nil || !existe(e.Exe) {
			rapport = append(rapport, fmt.Sprintf(tr("émulateur « %[1]s » laissé tel quel : il lance encore 3dssavesync et son vrai exe est inconnu. Corrigez-le à la main dans LaunchBox : Tools, Manage, Emulators, « %[1]s » : Application Path",
				"emulator \"%[1]s\" left as is: it still starts 3dssavesync and its real exe is unknown. Fix it by hand in LaunchBox: Tools, Manage, Emulators, \"%[1]s\": Application Path"), titre))
			continue
		}
		app := e.Exe // relatif sous le dossier de LaunchBox, comme LaunchBox l'écrit
		if r, err := filepath.Rel(dossierLB, e.Exe); err == nil && !strings.HasPrefix(r, "..") {
			app = r
		}
		for i, x := range blocs {
			n := x
			if i == 0 {
				n = fixerChamp(n, "ApplicationPath", app)
			}
			if optionEmulateur.MatchString(champ(n, "CommandLine")) {
				n = fixerChamp(n, "CommandLine", "")
			}
			texte = strings.Replace(texte, x, n, 1)
		}
		rapport = append(rapport, fmt.Sprintf(tr("émulateur « %s » de nouveau sur %s", "emulator \"%s\" back to %s"), titre, e.Exe))
	}
	emus := blocEmulateurLB.FindAllString(texte, -1)
	for _, b := range emus {
		if !notre(b) || !copie(b) {
			continue
		}
		id, titre := champ(b, "ID"), strings.TrimSuffix(champ(b, "Title"), " (3dssavesync)")
		idOrig := ""
		for _, o := range emus {
			if !notre(o) && champ(o, "Title") == titre {
				idOrig = champ(o, "ID")
			}
		}
		for _, p := range blocPlateformeLB.FindAllString(texte, -1) {
			if champ(p, "Emulator") != id {
				continue
			}
			if champ(p, "Default") == "true" {
				for _, q := range blocPlateformeLB.FindAllString(texte, -1) {
					if idOrig != "" && champ(q, "Emulator") == idOrig && champ(q, "Platform") == champ(p, "Platform") {
						texte = strings.Replace(texte, q, fixerChamp(q, "Default", "true"), 1)
						break
					}
				}
			}
			texte = strings.Replace(texte, p, "", 1)
		}
		texte = strings.Replace(texte, b, "", 1)
		if idOrig == "" {
			rapport = append(rapport, fmt.Sprintf(tr("Émulateur « %s » retiré. Son émulateur d'origine n'existe plus : choisissez un émulateur par défaut dans LaunchBox.",
				"Removed the \"%s\" emulator. Its original emulator no longer exists, so choose a default emulator in LaunchBox."), unescape(champ(b, "Title"))))
		} else {
			rapport = append(rapport, fmt.Sprintf(tr("Émulateur « %s » retiré, « %s » est de nouveau celui par défaut", "Removed the \"%s\" emulator. \"%s\" is the default again"),
				unescape(champ(b, "Title")), unescape(titre)))
		}
	}
	return texte, rapport
}

// defaireEsde : retire la règle 3DSSAVESYNC et les commandes 3dssavesync. Un système qui n'a plus de commande est un
// bloc minimal ajouté par esde : il est retiré, et ES-DE reprend son propre réglage pour ce système.
func defaireEsde(systems, regles string) (string, string, []string) {
	var rapport []string
	if regleESDE.MatchString(regles) {
		regles = regleESDE.ReplaceAllLiteralString(regles, "")
	}
	systems = systemeESDE.ReplaceAllStringFunc(systems, func(b string) string {
		if !commandeESDE.MatchString(b) {
			return b
		}
		n := champ(b, "name")
		b = commandeESDE.ReplaceAllLiteralString(b, "")
		if !strings.Contains(b, "<command") {
			rapport = append(rapport, fmt.Sprintf(tr("%s : système ajouté par 3dssavesync retiré", "%s: removed the system added by 3dssavesync"), n))
			return ""
		}
		rapport = append(rapport, fmt.Sprintf(tr("%s : commande 3dssavesync retirée", "%s: removed the 3dssavesync command"), n))
		return b
	})
	return systems, regles, rapport
}

// desinstaller : frontends, raccourcis, démarrage automatique, puis le programme installé. Erreur : un frontend est
// ouvert, rien n'est fait (il garderait en mémoire l'émulateur 3dssavesync et le réécrirait en se fermant).
func desinstaller(cfg *Config, config string) ([]string, error) {
	lb, es := cfg.Frontends["launchbox"], cfg.Frontends["esde"]
	switch {
	case lb != "" && (emulateurActif("LaunchBox") || emulateurActif("BigBox")):
		return nil, errors.New(tr("LaunchBox est ouvert. Fermez LaunchBox et BigBox, puis réessayez.",
			"LaunchBox is open. Close LaunchBox and BigBox, then try again."))
	case es != "" && emulateurActif("ES-DE"):
		return nil, errors.New(tr("ES-DE est ouvert. Fermez-le et réessayez.", "ES-DE is open. Close it and try again."))
	}
	lanceur := lanceurDe(cfg)
	var rapport []string
	ecrire := func(nom, f, avant, apres string) {
		if avant != apres {
			if err := remplacerFichier(f, []byte(apres)); err != nil {
				rapport = append(rapport, nom+" : "+texteErreur(err))
			}
		}
	}
	if lb != "" {
		f := filepath.Join(lb, "Data", "Emulators.xml")
		if o, err := os.ReadFile(f); err == nil {
			n, r := defaireLaunchbox(string(o), lb, lanceur, cfg)
			rapport = append(rapport, prefixer("LaunchBox", r)...)
			ecrire("LaunchBox", f, string(o), n)
		}
	}
	if es != "" {
		fs, fr := filepath.Join(es, "custom_systems", "es_systems.xml"), filepath.Join(es, "custom_systems", "es_find_rules.xml")
		s, _ := os.ReadFile(fs)
		r, _ := os.ReadFile(fr)
		ns, nr, rap := defaireEsde(string(s), string(r))
		rapport = append(rapport, prefixer("ES-DE", rap)...)
		ecrire("ES-DE", fs, string(s), ns)
		ecrire("ES-DE", fr, string(r), nr)
	}
	cibles := []string{lanceur}
	if installe := emplacementInstalle(); !strings.EqualFold(installe, lanceur) {
		cibles = append(cibles, installe) // désinstallation lancée depuis un autre exe (téléchargé à nouveau)
	}
	retires := 0
	for _, c := range cibles {
		l, err := retirerRaccourcis(c)
		if err != nil {
			rapport = append(rapport, tr("Raccourcis : ", "Shortcuts: ")+texteErreur(err))
		}
		retires += len(l)
	}
	if retires > 0 {
		rapport = append(rapport, fmt.Sprintf(tr("Raccourcis mis à la Corbeille : %d", "Shortcuts moved to the Recycle Bin: %d"), retires))
	}
	if err := arreterTray(filepath.Dir(config)); err != nil { // sinon l'exe de l'icône ne peut pas aller à la Corbeille
		rapport = append(rapport, err.Error())
	}
	if portable := dossierPortable(); portable != "" {
		rapport = append(rapport, fmt.Sprintf(tr("Version portable : le programme reste dans %s. Supprimez ce dossier quand vous n'en aurez plus besoin. Il contient aussi vos réglages.",
			"Portable version: the program stays in %s. Delete this folder when you no longer need it. It also holds your settings."), portable))
	} else if installe := emplacementInstalle(); existe(installe) {
		if err := retirerProgramme(installe); err != nil {
			rapport = append(rapport, tr("Programme : ", "Program: ")+texteErreur(err))
		} else {
			rapport = append(rapport, fmt.Sprintf(tr("Programme : %s ira à la Corbeille dès sa fermeture", "Program: %s goes to the Recycle Bin when it closes"), installe))
		}
	}
	rapport = append(rapport, fmt.Sprintf(tr("Conservés : vos réglages (%s) et vos copies de sécurité (%s)", "Kept: your settings (%s) and your backups (%s)"),
		filepath.Dir(config), cfg.dossierCopies(filepath.Dir(config))))
	if cfg.ServeurLocal != nil {
		donnees := cfg.ServeurLocal.Donnees
		if donnees == "" {
			donnees = filepath.Join(filepath.Dir(config), "serveur")
		}
		rapport = append(rapport, fmt.Sprintf(tr("Conservées : les versions du serveur local (%s). Le serveur local s'arrêtera au prochain redémarrage du PC.",
			"Kept: the local server versions (%s). The local server stops the next time you restart the PC."), donnees))
	}
	rapport = append(rapport, tr("Sur la 3DS : dans FBI, ouvrez Titles, 3dssavesync et choisissez Delete Title. Vos sauvegardes de la 3DS restent.",
		"On the 3DS: in FBI, open Titles, 3dssavesync and choose Delete Title. Your 3DS saves stay."))
	return rapport, nil
}

// retirerRaccourcisSysteme : les raccourcis qui lancent le Lanceur (bureau, menu, démarrage automatique). Windows :
// Corbeille ; Linux et macOS : fichiers créés par raccourci et demarrageAuto, retirés.
func retirerRaccourcisSysteme(lanceur string) ([]string, error) {
	if runtime.GOOS == "windows" {
		c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			`[Console]::OutputEncoding=[Text.Encoding]::UTF8; Add-Type -AssemblyName Microsoft.VisualBasic; $s=New-Object -ComObject WScript.Shell; `+
				`foreach ($d in 'Desktop','Startup','Programs') { Get-ChildItem -LiteralPath ([Environment]::GetFolderPath($d)) -Filter *.lnk -ErrorAction SilentlyContinue | `+
				`Where-Object { $s.CreateShortcut($_.FullName).TargetPath -ieq $env:R_CIBLE } | ForEach-Object { `+
				`[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($_.FullName,'OnlyErrorDialogs','SendToRecycleBin'); $_.FullName } }`)
		c.Env = append(os.Environ(), "R_CIBLE="+lanceur)
		o, err := c.Output()
		var l []string // un chemin par ligne (des espaces dans les noms)
		for _, x := range strings.Split(strings.TrimSpace(string(o)), "\n") {
			if x = strings.TrimSpace(x); x != "" {
				l = append(l, x)
			}
		}
		return l, err
	}
	motifs := []string{xdgData("applications", "3dssavesync*.desktop"), xdgConfig("autostart", "3dssavesync-serveur.desktop")}
	if runtime.GOOS == "darwin" {
		motifs = []string{maison("Applications", "3dssavesync*.app"), maison("Library", "LaunchAgents", "com.3dssavesync.serveur.plist")}
	}
	var retires []string
	for _, m := range motifs {
		l, _ := filepath.Glob(m)
		for _, f := range l {
			if err := os.RemoveAll(f); err != nil {
				return retires, err
			}
			retires = append(retires, f)
		}
	}
	return retires, nil
}

// retirerProgrammeSysteme : Windows ne retire pas un exe en cours : un PowerShell détaché attend sa fermeture, puis le
// met à la Corbeille et retire son dossier s'il est vide. Linux et macOS : retiré tout de suite.
func retirerProgrammeSysteme(installe string) error {
	if runtime.GOOS != "windows" {
		return os.Remove(installe)
	}
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command",
		`Add-Type -AssemblyName Microsoft.VisualBasic; for ($i=0; $i -lt 60; $i++) { Start-Sleep 2; `+
			`try { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($env:R_EXE,'OnlyErrorDialogs','SendToRecycleBin'); break } catch {} }; `+
			`try { [IO.Directory]::Delete((Split-Path $env:R_EXE)) } catch {}`)
	c.Env = append(os.Environ(), "R_EXE="+installe)
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
