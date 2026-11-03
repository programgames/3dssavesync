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
var iconICO []byte

// Demande d'arrêt (désinstallation, réglages enregistrés) : ce fichier dans le dossier des réglages, lu toutes les 2 s.
const stopFile = "arret-tray"

// Entrée « 3dssavesync » du démarrage automatique : tray, ou serve sans l'icône.
const startupName = "3dssavesync"

// Adaptateur remplacé par les tests : jamais le vrai dossier Démarrage.
var removeAutostart = removeAutostartEntry

// Icône affichée, sauf "icone_horloge": false.
func (c *Config) trayIcon() bool { return c.TrayIcon == nil || *c.TrayIcon }

// Élément du menu de l'icône. Texte vide : séparateur ; Action vide : ligne d'information, grisée.
type MenuItem struct {
	Text   string
	Action string // reglages, envoyer, quitter
}

// menuTray : le menu d'après l'état. attente : nombre d'Envois en attente ; serveur : ligne du serveur local ("" : aucun) ;
// maj : version plus récente sur GitHub ("" : aucune), mise à jour depuis les réglages.
func trayMenuItems(pending int, server, update string) []MenuItem {
	m := []MenuItem{{tr("Ouvrir les réglages", "Open settings"), "reglages"}}
	if update != "" {
		m = append(m, MenuItem{fmt.Sprintf(tr("Mettre à jour vers la version %s", "Update to version %s"), update), "reglages"})
	}
	if pending > 0 {
		m = append(m, MenuItem{fmt.Sprintf(tr("Envoyer maintenant (%d en attente)", "Upload now (%d pending)"), pending), "envoyer"})
	}
	if server != "" {
		m = append(m, MenuItem{server, ""})
	}
	return append(m, MenuItem{"3dssavesync " + version, ""}, MenuItem{}, MenuItem{tr("Quitter", "Quit"), "quitter"})
}

// ligneServeur : "" sans serveur local ; sinon l'adresse à entrer sur la 3DS, ou la première phrase de l'erreur.
func serverLine(cfg *Config, err error) string {
	if cfg.LocalServer == nil {
		return ""
	}
	if err != nil {
		t, _, _ := strings.Cut(errorText(err), ". ")
		return tr("Serveur local : arrêté (", "Local server: stopped (") + t + ")"
	}
	url, _ := address3DS(cfg)
	if url == "" {
		url = fmt.Sprintf("port %d", cfg.LocalServer.Port)
	}
	return tr("Serveur local : en marche (", "Local server: running (") + url + ")"
}

// Nombre d'Envois en attente (en-attente.json illisible : 0, la reprise habituelle le mettra de côté).
func pendingCount(dir string) int {
	var l []pendingEntry
	readJSON((&Session{dir: dir}).pendingPath(), &l)
	return len(l)
}

// envoyerMaintenant : les Envois en attente (même passe qu'au lancement d'un jeu) ; rend le texte de la notification.
func uploadNow(config string) string {
	s, err := newSession(config)
	if err != nil {
		return tr("Envoi impossible. ", "Upload failed. ") + errorText(err)
	}
	before := pendingCount(s.dir)
	err = s.resume()
	rest := pendingCount(s.dir)
	if rest == 0 {
		return fmt.Sprintf(tr("Sauvegardes envoyées : %d", "Saves uploaded: %d"), before)
	}
	text := fmt.Sprintf(tr("Sauvegardes encore en attente : %d. ", "Saves still pending: %d. "), rest)
	if err == nil { // jeu sauté : son émulateur est ouvert
		return text + tr("Fermez l'émulateur et réessayez.", "Close the emulator and try again.")
	}
	return text + errorText(err) + "\n" + tr("Détail : ", "Detail: ") + err.Error()
}

// avecConfig : args, avec --config quand ce n'est pas le fichier par défaut.
func withConfig(args []string, config string) []string {
	if config != defaultConfig() {
		args = append(args, "--config", config)
	}
	return args
}

// choixDemarrage : le programme lancé à l'ouverture de session (nil : aucun). Icône affichée : tray, qui fait aussi
// tourner le serveur local ; sinon serve pour le serveur local seul.
func startupChoice(cfg *Config, config string) []string {
	switch {
	case cfg.trayIcon() && trayAvailable:
		return withConfig([]string{"tray"}, config)
	case cfg.LocalServer != nil:
		return withConfig([]string{"serve"}, config)
	}
	return nil
}

// demarrageAutomatique : une seule entrée « 3dssavesync » (args nil : aucune), et plus celles « 3dssavesync serveur »
// d'avant la v3.7 (le serveur démarrerait deux fois). L'icône en marche s'arrête : elle redémarre avec les nouveaux réglages.
func setupAutostart(q *Questions, launcher string, args []string, dir string) {
	if err := stopTray(dir); err != nil {
		q.say("%s", err.Error())
	}
	if args != nil {
		f, err := autostart(startupName, launcher, args)
		if err != nil { // l'ancienne entrée reste : mieux qu'aucune
			q.say("%s %s", tr("Impossible d'activer le démarrage automatique :", "Couldn't turn on autostart:"), errorText(err))
			return
		}
		q.say("%s %s", tr("Démarrage automatique activé :", "Autostart on:"), f)
	}
	if err := removeAutostart(args != nil); err != nil {
		q.say("%s %s", tr("Impossible de retirer l'ancien démarrage automatique :", "Couldn't remove the old autostart:"), errorText(err))
	}
}

// retirerDemarrageAuto : les entrées « 3dssavesync serveur » d'avant la v3.7, et « 3dssavesync » sauf si garder.
// Windows : raccourcis de Démarrage à la Corbeille. Linux et macOS : demarrageAuto écrit toujours le même fichier.
func removeAutostartEntry(keep bool) error {
	if runtime.GOOS != "windows" {
		f := xdgConfig("autostart", "3dssavesync-serveur.desktop")
		if runtime.GOOS == "darwin" {
			f = home("Library", "LaunchAgents", "com.3dssavesync.serveur.plist")
		}
		if keep {
			return nil
		}
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	names := "3dssavesync serveur|3dssavesync server"
	if !keep {
		names += "|" + startupName
	}
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Add-Type -AssemblyName Microsoft.VisualBasic; $d=[Environment]::GetFolderPath('Startup'); foreach ($n in $env:R_NOMS.Split('|')) { `+
			`$f=Join-Path $d ($n+'.lnk'); if (Test-Path -LiteralPath $f) { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($f,'OnlyErrorDialogs','SendToRecycleBin') } }`)
	c.Env = append(os.Environ(), "R_NOMS="+names)
	if o, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%w : %s", err, strings.TrimSpace(string(o)))
	}
	return nil
}

// arreterTray : demande l'arrêt de l'icône en marche et attend la fin de son programme (son exe peut alors aller à la
// Corbeille, son port se libère). Rien à faire si elle ne tourne pas.
func stopTray(dir string) error {
	if !trayActive(dir) {
		return nil
	}
	f := filepath.Join(dir, stopFile)
	err := os.WriteFile(f, nil, 0o644)
	for i := 0; err == nil && i < 30 && trayActive(dir); i++ {
		time.Sleep(200 * time.Millisecond)
	}
	if err == nil && !trayActive(dir) {
		return nil
	}
	if err == nil {
		err = errors.New(f + tr(" : toujours actif après 6 s", ": still running after 6 s"))
	}
	return fmt.Errorf("%s\n%s%v", tr("L'icône près de l'horloge ne s'arrête pas. Faites un clic droit dessus et choisissez Quitter.",
		"The icon near the clock won't stop. Right-click it and choose Quit."), tr("Détail : ", "Detail: "), err)
}

// surveillerArret : appelle quitter quand le fichier d'arrêt apparaît (il est retiré), jusqu'à la fin.
func watchStop(dir string, quit func(), end <-chan struct{}) {
	f := filepath.Join(dir, stopFile)
	for {
		select {
		case <-end:
			return
		case <-time.After(2 * time.Second):
			if exists(f) {
				os.Remove(f)
				quit()
				return
			}
		}
	}
}

// imageIco : l'image d'un .ico la plus petite d'au moins taille pixels, sinon la plus grande. En-tête de 6 octets
// (nombre d'images à l'octet 4), puis 16 octets par image : largeur (0 = 256), taille (octet 8) et début (octet 12).
func imageIco(ico []byte, size int) []byte {
	var choice []byte
	bestOne := 0
	for i := 0; len(ico) >= 6 && i < int(binary.LittleEndian.Uint16(ico[4:])) && 22+16*i <= len(ico); i++ {
		e := ico[6+16*i:]
		l, n, start := int(e[0]), uint64(binary.LittleEndian.Uint32(e[8:])), uint64(binary.LittleEndian.Uint32(e[12:]))
		if l == 0 {
			l = 256
		}
		if start+n > uint64(len(ico)) {
			continue
		}
		if choice == nil || bestOne < size && l > bestOne || l >= size && l < bestOne {
			choice, bestOne = ico[start:start+n], l
		}
	}
	return choice
}

// 3dssavesync tray : l'icône, et le serveur local s'il est configuré. Une deuxième icône s'arrête sans rien dire.
func cmdTray(config string) (int, error) {
	dir := filepath.Dir(config)
	release := trayLock(dir)
	if release == nil {
		return 0, nil
	}
	defer release()
	os.Remove(filepath.Join(dir, stopFile)) // reste d'une demande faite quand l'icône ne tournait pas
	s := &Session{dir: dir}
	var cfg Config
	readJSON(config, &cfg) // pas encore configuré : seulement « Ouvrir les réglages » et « Quitter »
	var server struct {
		sync.Mutex
		err error
	}
	if cfg.LocalServer != nil {
		go func() {
			_, err := cmdServe(config) // rend la main seulement sur une erreur, ou s'il tourne déjà ailleurs
			if err != nil {
				s.log("tray : serveur local : " + err.Error())
				notify(tr("Le serveur local ne fonctionne pas : ", "The local server isn't running: ") + errorText(err))
				server.Lock()
				server.err = err
				server.Unlock()
			}
		}()
	}
	end := make(chan struct{})
	var one sync.Once
	quit := func() { one.Do(func() { close(end) }) }
	go watchStop(dir, quit, end)
	var update struct {
		sync.Mutex
		version string
	}
	if version != "dev" && cfg.checkUpdate() { // au plus une recherche par jour (derniereRelease) ; échec : rien dans le menu
		go func() {
			for {
				if r, err := latestRelease(dir, false); err == nil && newest(version, r.Tag) {
					update.Lock()
					update.version = strings.TrimPrefix(r.Tag, "v")
					update.Unlock()
				}
				select {
				case <-end:
					return
				case <-time.After(6 * time.Hour):
				}
			}
		}()
	}
	var upload sync.Mutex
	choose := func(action string) {
		switch action {
		case "reglages":
			if err := openDetached(executable(), withConfig([]string{"setup"}, config)); err != nil {
				notify(tr("Impossible d'ouvrir les réglages. Utilisez le raccourci « 3dssavesync réglages ».", "Couldn't open the settings. Use the \"3dssavesync settings\" shortcut.") +
					"\n" + tr("Détail : ", "Detail: ") + err.Error())
			}
		case "envoyer":
			if upload.TryLock() { // deux clics : un seul envoi
				defer upload.Unlock()
				notify(uploadNow(config))
			}
		case "quitter":
			quit()
		}
	}
	menu := func() []MenuItem {
		server.Lock()
		defer server.Unlock()
		update.Lock()
		defer update.Unlock()
		return trayMenuItems(pendingCount(dir), serverLine(&cfg, server.err), update.version)

	}
	s.log("tray : icône près de l'horloge")
	err := showIcon(menu, choose, end)
	if err == nil {
		return 0, nil
	}
	s.log("tray : " + err.Error())
	if cfg.LocalServer == nil {
		return 1, err
	}
	notify(err.Error() + "\n" + tr("Le serveur local reste en marche.", "The local server is still running."))
	<-end
	return 0, nil
}
