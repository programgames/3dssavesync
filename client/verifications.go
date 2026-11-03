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

// Verif : un élément de la liste. Page : l'entrée du menu qui reçoit le point orange.
type Verif struct {
	Ok          bool
	Texte, Page string
}

// verifications : la liste, et la mise à jour disponible (Version, Page ; nil sinon). Lectures sur le réseau : la
// page l'appelle sans son verrou.
func verifications(config string) ([]Verif, map[string]string) {
	var cfg Config
	if err := lireConfig(config, &cfg); err != nil {
		return []Verif{{false, texteErreur(err), "serveur"}}, nil
	}
	var l []Verif
	ajouter := func(ok bool, page, format string, a ...any) {
		l = append(l, Verif{ok, fmt.Sprintf(format, a...), page})
	}
	detail := tr("Détail : ", "Detail: ")

	if err := testerServeur(&cfg); err != nil {
		ajouter(false, "serveur", "%s%s\n%s%v", tr("Serveur injoignable : ", "Server unreachable: "), texteErreur(err), detail, err)
	} else {
		ajouter(true, "serveur", "%s", tr("Serveur joignable, jeton accepté.", "Server reachable, token accepted."))
	}

	var noms []string
	for n := range cfg.Emulateurs {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	if len(noms) == 0 {
		ajouter(false, "emulateurs", "%s", tr("Aucun émulateur : cochez-en un dans Émulateurs, puis cliquez sur « Enregistrer ».",
			"No emulator: tick one in Emulators, then click \"Save\"."))
	}
	for _, n := range noms {
		e := cfg.Emulateurs[n]
		p, err := e.profil(n)
		if err != nil {
			ajouter(false, "emulateurs", "%s : %v", n, err)
			continue
		}
		for _, s := range e.systemes(p) {
			switch d := e.dossier(p, s, ""); {
			case d == "" && s == "3DS":
				ajouter(false, "emulateurs", tr("%s 3DS : dossier sdmc inconnu. Choisissez-le dans Émulateurs, « Régler ».", "%s 3DS: sdmc folder unknown. Choose it in Emulators, \"Settings\"."), n)
			case d == "":
				ajouter(true, "emulateurs", tr("%s %s : sauvegardes à côté de chaque ROM.", "%s %s: saves next to each ROM."), n, s)
			case existe(d):
				ajouter(true, "emulateurs", tr("%s %s : dossier des sauvegardes trouvé (%s).", "%s %s: save folder found (%s)."), n, s, d)
			default:
				ajouter(false, "emulateurs", tr("%s %s : dossier des sauvegardes introuvable (%s). Lancez un jeu une fois dans l'émulateur, ou choisissez le bon dossier dans Émulateurs, « Régler ».",
					"%s %s: save folder not found (%s). Start a game once in the emulator, or choose the right folder in Emulators, \"Settings\"."), n, s, d)
			}
		}
	}

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
			ajouter(false, "frontends", tr("LaunchBox : %s pas encore par défaut. Fermez LaunchBox, puis cliquez sur « Réparer les frontends ».",
				"LaunchBox: %s not default yet. Close LaunchBox, then click \"Repair frontends\"."), strings.Join(m, ", "))
		default:
			ajouter(true, "frontends", "%s", tr("LaunchBox configuré.", "LaunchBox set up."))
		}
	}
	if d := cfg.Frontends["esde"]; d != "" {
		fs, fr := filepath.Join(d, "custom_systems", "es_systems.xml"), filepath.Join(d, "custom_systems", "es_find_rules.xml")
		s, _ := os.ReadFile(fs)
		r, _ := os.ReadFile(fr)
		if ns, nr, _ := esde(string(s), string(r), lanceur, emus); ns != string(s) || nr != string(r) {
			ajouter(false, "frontends", "%s", tr("ES-DE : pas encore configuré. Fermez ES-DE, puis cliquez sur « Réparer les frontends ».",
				"ES-DE: not set up yet. Close ES-DE, then click \"Repair frontends\"."))
		} else {
			ajouter(true, "frontends", "%s", tr("ES-DE configuré.", "ES-DE set up."))
		}
	}

	if cfg.CartePreparee != "" {
		ajouter(true, "carte", tr("Carte SD de la 3DS préparée le %s.", "3DS SD card prepared on %s."), cfg.CartePreparee)
	} else {
		ajouter(false, "carte", "%s", tr("Carte SD de la 3DS pas encore préparée depuis ce PC : mettez-la dans le PC, puis cliquez sur « Préparer la carte SD de la 3DS ».",
			"3DS SD card not prepared from this PC yet: put it in the PC, then click \"Prepare the 3DS SD card\"."))
	}

	if cfg.iconeHorloge() && trayDisponible {
		if trayActif(filepath.Dir(config)) {
			ajouter(true, "frontends", "%s", tr("Icône près de l'horloge en marche.", "Icon near the clock running."))
		} else {
			ajouter(false, "frontends", "%s", tr("L'icône près de l'horloge ne tourne pas. Cliquez sur « Enregistrer » : elle démarre.",
				"The icon near the clock is not running. Click \"Save\": it starts."))
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
			ajouter(false, "frontends", "%s %s", tr("Recherche de mise à jour impossible.", "Update check failed."), erreurGitHub(err))
		case plusRecente(version, r.Tag):
			v := strings.TrimPrefix(r.Tag, "v")
			maj = map[string]string{"Version": v, "Page": r.Page}
			ajouter(false, "frontends", tr("Version %s disponible (vous avez la %s) : cliquez sur « Mettre à jour » en haut de la page.",
				"Version %s available (you have %s): click \"Update\" at the top of the page."), v, version)
		default:
			ajouter(true, "frontends", tr("3dssavesync %s est à jour.", "3dssavesync %s is up to date."), version)
		}
	}
	return l, maj
}
