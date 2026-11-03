package main

// Liste « Tout est prêt » de la page des réglages : chaque élément est bon, ou dit ce qu'il faut faire. Rien n'est
// écrit, sauf la date de la recherche de mise à jour (maj-pc.json).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Verif : un élément de la liste. Page : l'entrée du menu qui reçoit le compteur ; Emulateur : l'émulateur de
// config.json concerné (sa ligne dans Émulateurs dit « À vérifier »), "" sinon.
type Verif struct {
	Ok                     bool
	Texte, Page, Emulateur string
}

// verifications : la liste, et la mise à jour disponible (Version, Page ; nil sinon). Lectures sur le réseau : la
// page l'appelle sans son verrou.
func verifications(config string) ([]Verif, map[string]string) {
	var cfg Config
	if err := lireConfig(config, &cfg); err != nil {
		return []Verif{{Texte: texteErreur(err), Page: "serveur"}}, nil
	}
	var l []Verif
	ajouter := func(ok bool, page, format string, a ...any) {
		l = append(l, Verif{ok, fmt.Sprintf(format, a...), page, ""})
	}
	detail := tr("Détail : ", "Detail: ")

	if err := testerServeur(&cfg); err != nil {
		ajouter(false, "serveur", "%s%s\n%s%v", tr("Serveur injoignable : ", "Server unreachable: "), texteErreur(err), detail, err)
	} else {
		ajouter(true, "serveur", "%s", tr("Serveur joignable, jeton accepté.", "Server reachable, token accepted."))
	}

	l = append(l, verifsEmulateurs(&cfg)...)

	lb, emus := entreesFrontends(&cfg)
	lanceur := lanceurDe(&cfg)
	if d := cfg.Frontends["launchbox"]; d != "" {
		f := filepath.Join(d, "Data", "Emulators.xml")
		o, err := os.ReadFile(f)
		switch m := manquantsLaunchBox(string(o), lanceur, lb); {
		case err != nil:
			ajouter(false, "frontends", "%s\n%s%v", tr("LaunchBox : Emulators.xml illisible. Vérifiez le dossier de LaunchBox dans Frontends.",
				"LaunchBox: Emulators.xml cannot be read. Check the LaunchBox folder in Frontends."), detail, err)
		case len(m) > 0:
			ajouter(false, "frontends", tr("LaunchBox : %s n'est pas encore l'émulateur par défaut. Fermez LaunchBox et cliquez sur « Réparer les frontends ».",
				"LaunchBox: %s isn't the default emulator yet. Close LaunchBox and click \"Repair frontends\"."), strings.Join(m, ", "))
		default:
			ajouter(true, "frontends", "%s", tr("LaunchBox configuré.", "LaunchBox set up."))
		}
	}
	if d := cfg.Frontends["esde"]; d != "" {
		fs, fr := filepath.Join(d, "custom_systems", "es_systems.xml"), filepath.Join(d, "custom_systems", "es_find_rules.xml")
		s, _ := os.ReadFile(fs)
		r, _ := os.ReadFile(fr)
		if ns, nr, _ := esde(string(s), string(r), lanceur, emus); ns != string(s) || nr != string(r) {
			ajouter(false, "frontends", "%s", tr("ES-DE n'est pas encore configuré. Fermez-le et cliquez sur « Réparer les frontends ».",
				"ES-DE isn't set up yet. Close it and click \"Repair frontends\"."))
		} else {
			ajouter(true, "frontends", "%s", tr("ES-DE configuré.", "ES-DE set up."))
		}
	}

	if cfg.CartePreparee != "" {
		ajouter(true, "carte", tr("Carte SD de la 3DS préparée le %s.", "3DS SD card prepared on %s."), cfg.CartePreparee)
	} else {
		ajouter(false, "carte", "%s", tr("La carte SD de la 3DS n'a pas encore été préparée depuis ce PC. Mettez-la dans le PC et cliquez sur « Préparer la carte SD de la 3DS ».",
			"The 3DS SD card hasn't been prepared from this PC yet. Put it in the PC and click \"Prepare the 3DS SD card\"."))
	}

	if cfg.iconeHorloge() && trayDisponible {
		if trayActif(filepath.Dir(config)) {
			ajouter(true, "frontends", "%s", tr("Icône près de l'horloge en marche.", "Icon near the clock is running."))
		} else {
			ajouter(false, "frontends", "%s", tr("L'icône près de l'horloge n'est pas lancée. Cliquez sur « Enregistrer » pour la démarrer.",
				"The icon near the clock isn't running. Click \"Save\" to start it."))
		}
	}

	var maj map[string]string
	switch {
	case version == "dev":
		ajouter(true, "frontends", "%s", tr("Version de développement : pas de recherche de mise à jour.", "Development build: no update check."))
	case !cfg.verifierMaj():
		ajouter(true, "frontends", tr("3dssavesync %s : recherche des mises à jour désactivée.", "3dssavesync %s: update check turned off."), version)
	default:
		r, err := derniereRelease(filepath.Dir(config), false)
		switch {
		case err != nil:
			ajouter(false, "frontends", "%s %s", tr("Impossible de vérifier les mises à jour.", "Couldn't check for updates."), erreurGitHub(err))
		case plusRecente(version, r.Tag):
			v := strings.TrimPrefix(r.Tag, "v")
			maj = map[string]string{"Version": v, "Page": r.Page}
			ajouter(false, "frontends", tr("La version %s est disponible (vous avez la %s). Cliquez sur « Mettre à jour » en haut de la page.",
				"Version %s is available (you have %s). Click \"Update\" at the top of the page."), v, version)
		default:
			ajouter(true, "frontends", tr("3dssavesync %s est à jour.", "3dssavesync %s is up to date."), version)
		}
	}
	return l, maj
}

// verifsEmulateurs : les dossiers des sauvegardes des émulateurs de config.json (lectures locales seulement : la page
// les montre aussi dans Émulateurs, au même état que dans Vérifications).
func verifsEmulateurs(cfg *Config) []Verif {
	var l []Verif
	n := ""
	ajouter := func(ok bool, page, format string, a ...any) {
		l = append(l, Verif{ok, fmt.Sprintf(format, a...), page, n})
	}
	var noms []string
	for n := range cfg.Emulateurs {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	if len(noms) == 0 {
		ajouter(false, "emulateurs", "%s", tr("Aucun émulateur choisi. Cochez-en un dans Émulateurs et cliquez sur « Enregistrer ».",
			"No emulator selected. Tick one in Emulators and click \"Save\"."))
	}
	for _, n = range noms {
		e := cfg.Emulateurs[n]
		p, err := e.profil(n)
		if err != nil {
			ajouter(false, "emulateurs", "%s : %v", n, err)
			continue
		}
		for _, s := range e.systemes(p) {
			switch d := e.dossier(p, s, ""); {
			case d == "" && s == "3DS":
				ajouter(false, "emulateurs", tr("%s 3DS : dossier sdmc inconnu. Indiquez-le dans Émulateurs, bouton « Régler ».", "%s 3DS: sdmc folder unknown. Set it in Emulators with the \"Configure\" button."), n)
			case d == "":
				ajouter(true, "emulateurs", tr("%s %s : sauvegardes à côté de chaque ROM.", "%s %s: saves next to each ROM."), n, s)
			case existe(d):
				ajouter(true, "emulateurs", tr("%s %s : dossier des sauvegardes trouvé (%s).", "%s %s: save folder found (%s)."), n, s, d)
			default:
				ajouter(false, "emulateurs", tr("%s %s : dossier des sauvegardes introuvable (%s). Lancez un jeu une fois dans l'émulateur, ou indiquez le bon dossier dans Émulateurs, bouton « Régler ».",
					"%s %s: save folder not found (%s). Start a game once in the emulator, or set the right folder in Emulators with the \"Configure\" button."), n, s, d)
			}
		}
	}
	return l
}
