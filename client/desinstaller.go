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
)

// Adaptateurs remplacés par les tests (PowerShell, Corbeille, processus détaché).
var retirerRaccourcis, retirerProgramme = retirerRaccourcisSysteme, retirerProgrammeSysteme

// defaireLaunchbox : retire chaque émulateur qui lance le Lanceur, et ses plateformes ; l'émulateur dont il est la
// copie (même titre sans « (3dssavesync) ») redevient l'émulateur par défaut de ces plateformes.
func defaireLaunchbox(texte, lanceur string) (string, []string) {
	var rapport []string
	unescape := func(s string) string { return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(s) }
	notre := func(b string) bool {
		return strings.EqualFold(path.Base(strings.ReplaceAll(unescape(champ(b, "ApplicationPath")), `\`, "/")), filepath.Base(lanceur))
	}
	emus := blocEmulateurLB.FindAllString(texte, -1)
	for _, b := range emus {
		if !notre(b) {
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
			rapport = append(rapport, fmt.Sprintf(tr("émulateur « %s » retiré ; son émulateur d'origine n'existe plus : choisissez un émulateur par défaut dans LaunchBox",
				"emulator \"%s\" removed; its original emulator no longer exists: choose a default emulator in LaunchBox"), unescape(champ(b, "Title"))))
		} else {
			rapport = append(rapport, fmt.Sprintf(tr("émulateur « %s » retiré, « %s » de nouveau par défaut", "emulator \"%s\" removed, \"%s\" is the default again"),
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
			rapport = append(rapport, fmt.Sprintf(tr("%s : système ajouté par 3dssavesync retiré", "%s: system added by 3dssavesync removed"), n))
			return ""
		}
		rapport = append(rapport, fmt.Sprintf(tr("%s : commande 3dssavesync retirée", "%s: 3dssavesync command removed"), n))
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
		return nil, errors.New(tr("LaunchBox est ouvert : fermez LaunchBox et BigBox, puis recommencez. Rien n'est fait.",
			"LaunchBox is open: close LaunchBox and BigBox, then try again. Nothing is done."))
	case es != "" && emulateurActif("ES-DE"):
		return nil, errors.New(tr("ES-DE est ouvert : fermez ES-DE, puis recommencez. Rien n'est fait.", "ES-DE is open: close ES-DE, then try again. Nothing is done."))
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
			n, r := defaireLaunchbox(string(o), lanceur)
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
		rapport = append(rapport, fmt.Sprintf(tr("%d raccourci(s) mis à la Corbeille", "%d shortcut(s) moved to the Recycle Bin"), retires))
	}
	if err := arreterTray(filepath.Dir(config)); err != nil { // sinon l'exe de l'icône ne peut pas aller à la Corbeille
		rapport = append(rapport, err.Error())
	}
	if portable := dossierPortable(); portable != "" {
		rapport = append(rapport, fmt.Sprintf(tr("Version portable : le programme reste dans %s. Supprimez ce dossier vous-même quand vous n'en avez plus besoin (il contient aussi vos réglages).",
			"Portable version: the program stays in %s. Delete this folder yourself when you no longer need it (it also holds your settings)."), portable))
	} else if installe := emplacementInstalle(); existe(installe) {
		if err := retirerProgramme(installe); err != nil {
			rapport = append(rapport, tr("Programme : ", "Program: ")+texteErreur(err))
		} else {
			rapport = append(rapport, fmt.Sprintf(tr("Programme : %s part à la Corbeille dès sa fermeture", "Program: %s goes to the Recycle Bin as soon as it closes"), installe))
		}
	}
	rapport = append(rapport, fmt.Sprintf(tr("Gardés (vos données) : réglages %s, copies de sécurité %s", "Kept (your data): settings %s, backup copies %s"),
		filepath.Dir(config), cfg.dossierCopies(filepath.Dir(config))))
	if cfg.ServeurLocal != nil {
		donnees := cfg.ServeurLocal.Donnees
		if donnees == "" {
			donnees = filepath.Join(filepath.Dir(config), "serveur")
		}
		rapport = append(rapport, fmt.Sprintf(tr("Gardées : les versions du serveur local, %s. Le serveur local s'arrête au prochain redémarrage du PC.",
			"Kept: the local server versions, %s. The local server stops at the next restart of the PC."), donnees))
	}
	rapport = append(rapport, tr("3DS : dans FBI, Titles, 3dssavesync, Delete Title. Vos sauvegardes de la 3DS ne changent pas.",
		"3DS: in FBI, Titles, 3dssavesync, Delete Title. Your 3DS saves do not change."))
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
