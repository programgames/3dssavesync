package main

// Icône près de l'horloge (ADR 0022) : « 3dssavesync tray », le seul programme résident. Son menu ouvre les réglages,
// envoie les Envois en attente et montre l'état du serveur local, qu'il fait tourner lui-même (à la place de serve).
// Windows : Win32 (tray_windows.go) ; Linux : StatusNotifierItem sur D-Bus (tray_linux.go) ; macOS : pas encore.

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed icone/icone.ico
var iconeIco []byte

// Demande d'arrêt (désinstallation, réglages enregistrés) : ce fichier dans le dossier des réglages, lu toutes les 2 s.
const fichierArret = "arret-tray"

// Entrée « 3dssavesync » du démarrage automatique : tray, ou serve sans l'icône.
const nomDemarrage = "3dssavesync"

// Adaptateur remplacé par les tests : jamais le vrai dossier Démarrage.
var retirerAutostart = retirerDemarrageAuto

// Icône affichée, sauf "icone_horloge": false.
func (c *Config) iconeHorloge() bool { return c.IconeHorloge == nil || *c.IconeHorloge }

// Élément du menu de l'icône. Texte vide : séparateur ; Action vide : ligne d'information, grisée.
type ElementMenu struct {
	Texte  string
	Action string // reglages, envoyer, quitter
}

// menuTray : le menu d'après l'état. attente : nombre d'Envois en attente ; serveur : ligne du serveur local ("" : aucun) ;
// maj : version plus récente sur GitHub ("" : aucune), mise à jour depuis les réglages.
func menuTray(attente int, serveur, maj string) []ElementMenu {
	m := []ElementMenu{{tr("Ouvrir les réglages", "Open settings"), "reglages"}}
	if maj != "" {
		m = append(m, ElementMenu{fmt.Sprintf(tr("Mettre à jour vers la version %s", "Update to version %s"), maj), "reglages"})
	}
	if attente > 0 {
		m = append(m, ElementMenu{fmt.Sprintf(tr("Envoyer maintenant (%d en attente)", "Upload now (%d pending)"), attente), "envoyer"})
	}
	if serveur != "" {
		m = append(m, ElementMenu{serveur, ""})
	}
	return append(m, ElementMenu{"3dssavesync " + version, ""}, ElementMenu{}, ElementMenu{tr("Quitter", "Quit"), "quitter"})
}

// ligneServeur : "" sans serveur local ; sinon l'adresse à entrer sur la 3DS, ou la première phrase de l'erreur.
func ligneServeur(cfg *Config, err error) string {
	if cfg.ServeurLocal == nil {
		return ""
	}
	if err != nil {
		t, _, _ := strings.Cut(texteErreur(err), ". ")
		return tr("Serveur local : arrêté (", "Local server: stopped (") + t + ")"
	}
	url, _ := adresse3DS(cfg)
	if url == "" {
		url = fmt.Sprintf("port %d", cfg.ServeurLocal.Port)
	}
	return tr("Serveur local : en marche (", "Local server: running (") + url + ")"
}

// Nombre d'Envois en attente (en-attente.json illisible : 0, la reprise habituelle le mettra de côté).
func nombreEnAttente(dossier string) int {
	var l []enAttente
	lireJSON((&Session{dossier: dossier}).cheminAttente(), &l)
	return len(l)
}

// envoyerMaintenant : les Envois en attente (même passe qu'au lancement d'un jeu) ; rend le texte de la notification.
func envoyerMaintenant(config string) string {
	s, err := nouvelleSession(config)
	if err != nil {
		return tr("Envoi impossible. ", "Upload failed. ") + texteErreur(err)
	}
	avant := nombreEnAttente(s.dossier)
	err = s.reprendre()
	reste := nombreEnAttente(s.dossier)
	if reste == 0 {
		return fmt.Sprintf(tr("Sauvegardes envoyées : %d", "Saves uploaded: %d"), avant)
	}
	texte := fmt.Sprintf(tr("Sauvegardes encore en attente : %d. ", "Saves still pending: %d. "), reste)
	if err == nil { // jeu sauté : son émulateur est ouvert
		return texte + tr("Fermez l'émulateur et réessayez.", "Close the emulator and try again.")
	}
	return texte + texteErreur(err) + "\n" + tr("Détail : ", "Detail: ") + err.Error()
}

// avecConfig : args, avec --config quand ce n'est pas le fichier par défaut.
func avecConfig(args []string, config string) []string {
	if config != configDefaut() {
		args = append(args, "--config", config)
	}
	return args
}

// choixDemarrage : le programme lancé à l'ouverture de session (nil : aucun). Icône affichée : tray, qui fait aussi
// tourner le serveur local ; sinon serve pour le serveur local seul.
func choixDemarrage(cfg *Config, config string) []string {
	switch {
	case cfg.iconeHorloge() && trayDisponible:
		return avecConfig([]string{"tray"}, config)
	case cfg.ServeurLocal != nil:
		return avecConfig([]string{"serve"}, config)
	}
	return nil
}

// demarrageAutomatique : une seule entrée « 3dssavesync » (args nil : aucune), et plus celles « 3dssavesync serveur »
// d'avant la v3.7 (le serveur démarrerait deux fois). L'icône en marche s'arrête : elle redémarre avec les nouveaux réglages.
func demarrageAutomatique(q *Questions, lanceur string, args []string, dossier string) {
	if err := arreterTray(dossier); err != nil {
		q.dire("%s", err.Error())
	}
	if args != nil {
		f, err := autostart(nomDemarrage, lanceur, args)
		if err != nil { // l'ancienne entrée reste : mieux qu'aucune
			q.dire("%s %s", tr("Impossible d'activer le démarrage automatique :", "Couldn't turn on autostart:"), texteErreur(err))
			return
		}
		q.dire("%s %s", tr("Démarrage automatique activé :", "Autostart on:"), f)
	}
	if err := retirerAutostart(args != nil); err != nil {
		q.dire("%s %s", tr("Impossible de retirer l'ancien démarrage automatique :", "Couldn't remove the old autostart:"), texteErreur(err))
	}
}

// retirerDemarrageAuto : les entrées « 3dssavesync serveur » d'avant la v3.7, et « 3dssavesync » sauf si garder.
// Windows : raccourcis de Démarrage à la Corbeille. Linux et macOS : demarrageAuto écrit toujours le même fichier.
func retirerDemarrageAuto(garder bool) error {
	if runtime.GOOS != "windows" {
		f := xdgConfig("autostart", "3dssavesync-serveur.desktop")
		if runtime.GOOS == "darwin" {
			f = maison("Library", "LaunchAgents", "com.3dssavesync.serveur.plist")
		}
		if garder {
			return nil
		}
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	noms := "3dssavesync serveur|3dssavesync server"
	if !garder {
		noms += "|" + nomDemarrage
	}
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Add-Type -AssemblyName Microsoft.VisualBasic; $d=[Environment]::GetFolderPath('Startup'); foreach ($n in $env:R_NOMS.Split('|')) { `+
			`$f=Join-Path $d ($n+'.lnk'); if (Test-Path -LiteralPath $f) { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($f,'OnlyErrorDialogs','SendToRecycleBin') } }`)
	c.Env = append(os.Environ(), "R_NOMS="+noms)
	if o, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%w : %s", err, strings.TrimSpace(string(o)))
	}
	return nil
}

// arreterTray : demande l'arrêt de l'icône en marche et attend la fin de son programme (son exe peut alors aller à la
// Corbeille, son port se libère). Rien à faire si elle ne tourne pas.
func arreterTray(dossier string) error {
	if !trayActif(dossier) {
		return nil
	}
	f := filepath.Join(dossier, fichierArret)
	err := os.WriteFile(f, nil, 0o644)
	for i := 0; err == nil && i < 30 && trayActif(dossier); i++ {
		time.Sleep(200 * time.Millisecond)
	}
	if err == nil && !trayActif(dossier) {
		return nil
	}
	if err == nil {
		err = errors.New(f + tr(" : toujours actif après 6 s", ": still running after 6 s"))
	}
	return fmt.Errorf("%s\n%s%v", tr("L'icône près de l'horloge ne s'arrête pas. Faites un clic droit dessus et choisissez Quitter.",
		"The icon near the clock won't stop. Right-click it and choose Quit."), tr("Détail : ", "Detail: "), err)
}

// surveillerArret : appelle quitter quand le fichier d'arrêt apparaît (il est retiré), jusqu'à la fin.
func surveillerArret(dossier string, quitter func(), fin <-chan struct{}) {
	f := filepath.Join(dossier, fichierArret)
	for {
		select {
		case <-fin:
			return
		case <-time.After(2 * time.Second):
			if existe(f) {
				os.Remove(f)
				quitter()
				return
			}
		}
	}
}

// imageIco : l'image d'un .ico la plus petite d'au moins taille pixels, sinon la plus grande. En-tête de 6 octets
// (nombre d'images à l'octet 4), puis 16 octets par image : largeur (0 = 256), taille (octet 8) et début (octet 12).
func imageIco(ico []byte, taille int) []byte {
	var choix []byte
	meilleure := 0
	for i := 0; len(ico) >= 6 && i < int(binary.LittleEndian.Uint16(ico[4:])) && 22+16*i <= len(ico); i++ {
		e := ico[6+16*i:]
		l, n, debut := int(e[0]), uint64(binary.LittleEndian.Uint32(e[8:])), uint64(binary.LittleEndian.Uint32(e[12:]))
		if l == 0 {
			l = 256
		}
		if debut+n > uint64(len(ico)) {
			continue
		}
		if choix == nil || meilleure < taille && l > meilleure || l >= taille && l < meilleure {
			choix, meilleure = ico[debut:debut+n], l
		}
	}
	return choix
}

// 3dssavesync tray : l'icône, et le serveur local s'il est configuré. Une deuxième icône s'arrête sans rien dire.
func commandeTray(config string) (int, error) {
	dossier := filepath.Dir(config)
	liberer := verrouTray(dossier)
	if liberer == nil {
		return 0, nil
	}
	defer liberer()
	os.Remove(filepath.Join(dossier, fichierArret)) // reste d'une demande faite quand l'icône ne tournait pas
	s := &Session{dossier: dossier}
	var cfg Config
	lireJSON(config, &cfg) // pas encore configuré : seulement « Ouvrir les réglages » et « Quitter »
	var serveur struct {
		sync.Mutex
		err error
	}
	if cfg.ServeurLocal != nil {
		go func() {
			_, err := commandeServe(config) // rend la main seulement sur une erreur, ou s'il tourne déjà ailleurs
			if err != nil {
				s.journal("tray : serveur local : " + err.Error())
				notifier(tr("Le serveur local ne fonctionne pas : ", "The local server isn't running: ") + texteErreur(err))
				serveur.Lock()
				serveur.err = err
				serveur.Unlock()
			}
		}()
	}
	fin := make(chan struct{})
	var une sync.Once
	quitter := func() { une.Do(func() { close(fin) }) }
	go surveillerArret(dossier, quitter, fin)
	var maj struct {
		sync.Mutex
		version string
	}
	if version != "dev" && cfg.verifierMaj() { // au plus une recherche par jour (derniereRelease) ; échec : rien dans le menu
		go func() {
			for {
				if r, err := derniereRelease(dossier, false); err == nil && plusRecente(version, r.Tag) {
					maj.Lock()
					maj.version = strings.TrimPrefix(r.Tag, "v")
					maj.Unlock()
				}
				select {
				case <-fin:
					return
				case <-time.After(6 * time.Hour):
				}
			}
		}()
	}
	var envoi sync.Mutex
	choisir := func(action string) {
		switch action {
		case "reglages":
			if err := ouvrirDetache(executable(), avecConfig([]string{"setup"}, config)); err != nil {
				notifier(tr("Impossible d'ouvrir les réglages. Utilisez le raccourci « 3dssavesync réglages ».", "Couldn't open the settings. Use the \"3dssavesync settings\" shortcut.") +
					"\n" + tr("Détail : ", "Detail: ") + err.Error())
			}
		case "envoyer":
			if envoi.TryLock() { // deux clics : un seul envoi
				defer envoi.Unlock()
				notifier(envoyerMaintenant(config))
			}
		case "quitter":
			quitter()
		}
	}
	menu := func() []ElementMenu {
		serveur.Lock()
		defer serveur.Unlock()
		maj.Lock()
		defer maj.Unlock()
		return menuTray(nombreEnAttente(dossier), ligneServeur(&cfg, serveur.err), maj.version)

	}
	s.journal("tray : icône près de l'horloge")
	err := afficherIcone(menu, choisir, fin)
	if err == nil {
		return 0, nil
	}
	s.journal("tray : " + err.Error())
	if cfg.ServeurLocal == nil {
		return 1, err
	}
	notifier(err.Error() + "\n" + tr("Le serveur local reste en marche.", "The local server is still running."))
	<-fin
	return 0, nil
}
