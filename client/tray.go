package main

// Tray icon (ADR 0022): "3dssavesync tray", the only resident program. Its menu opens the settings page, sends the
// pending Uploads and shows the state of the local Server, which it runs itself (in place of serve).
// Windows: Win32 (tray_windows.go); Linux: StatusNotifierItem on D-Bus (tray_linux.go); macOS: not yet.

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

// Stop request (uninstall, settings saved): this file in the settings folder, checked every 2 s.
const stopFile = "arret-tray"

// "3dssavesync" autostart entry: tray, or serve without the tray icon.
const startupName = "3dssavesync"

// Adapter that the tests replace: never the real Startup folder.
var removeAutostart = removeAutostartEntry

// trayIcon reports a shown tray icon, unless "icone_horloge": false.
func (c *Config) trayIcon() bool { return c.TrayIcon == nil || *c.TrayIcon }

// MenuItem is an item of the tray icon menu. Empty Text: separator; empty Action: greyed information line.
type MenuItem struct {
	Text   string
	Action string // "reglages", "synchro", "envoyer" or "quitter"
}

// trayMenuItems builds the menu from the state. pending: number of pending Uploads; server: local Server line
// ("": none); update: newer version on GitHub ("": none), updated from the settings page.
func trayMenuItems(pending int, server, update string) []MenuItem {
	m := []MenuItem{{tr("Ouvrir les réglages", "Open settings"), "reglages"}}
	if update != "" {
		m = append(m, MenuItem{fmt.Sprintf(tr("Mettre à jour vers la version %s", "Update to version %s"), update), "reglages"})
	}
	m = append(m, MenuItem{tr("Tout synchroniser", "Sync all"), "synchro"})
	if pending > 0 {
		m = append(m, MenuItem{fmt.Sprintf(tr("Envoyer maintenant (%d en attente)", "Upload now (%d pending)"), pending), "envoyer"})
	}
	if server != "" {
		m = append(m, MenuItem{server, ""})
	}
	return append(m, MenuItem{"3dssavesync " + version, ""}, MenuItem{}, MenuItem{tr("Quitter", "Quit"), "quitter"})
}

// serverLine returns "" with no local Server; else the address to enter on the 3DS, or the first sentence of the error.
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

// pendingCount returns the number of pending Uploads (unreadable en-attente.json: 0, the usual resume sets it aside).
func pendingCount(dir string) int {
	var l []pendingEntry
	readJSON(pendingPath(dir), &l)
	return len(l)
}

// uploadNow sends the pending Uploads (same pass as at game start); returns the notification text.
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
	if err == nil { // Game skipped: its emulator is open
		return text + tr("Fermez l'émulateur et réessayez.", "Close the emulator and try again.")
	}
	return text + errorText(err) + "\n" + tr("Détail : ", "Detail: ") + err.Error()
}

// withConfig returns args, with --config when config is not the default file.
func withConfig(args []string, config string) []string {
	if config != defaultConfig() {
		args = append(args, "--config", config)
	}
	return args
}

// startupChoice returns the program started at logon (nil: none). Tray icon shown: tray, which also runs the local
// Server; else serve for the local Server alone.
func startupChoice(cfg *Config, config string) []string {
	switch {
	case cfg.trayIcon() && trayAvailable:
		return withConfig([]string{"tray"}, config)
	case cfg.LocalServer != nil:
		return withConfig([]string{"serve"}, config)
	}
	return nil
}

// setupAutostart keeps one "3dssavesync" entry (nil args: none) and removes the old "3dssavesync serveur" entries
// (the Server would start twice). The running tray icon stops: it starts again with the new settings.
func setupAutostart(q *Questions, launcher string, args []string, dir string) {
	if err := stopTray(dir); err != nil {
		q.say("%s", err.Error())
	}
	if args != nil {
		f, err := autostart(startupName, launcher, args)
		if err != nil { // the old entry stays: better than none
			q.say("%s %s", tr("Impossible d'activer le démarrage automatique :", "Couldn't turn on autostart:"), errorText(err))
			return
		}
		q.say("%s %s", tr("Démarrage automatique activé :", "Autostart on:"), f)
	}
	if err := removeAutostart(args != nil); err != nil {
		q.say("%s %s", tr("Impossible de retirer l'ancien démarrage automatique :", "Couldn't remove the old autostart:"), errorText(err))
	}
}

// removeAutostartEntry removes the old "3dssavesync serveur" entries, and "3dssavesync" unless keep.
// Windows: Startup shortcuts to the Recycle Bin. Linux and macOS: addAutostart always writes the same file.
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

// stopTray asks the running tray icon to stop and waits for its program to end (its exe can then go to the
// Recycle Bin, its port is free). Nothing to do if it is not running.
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

// watchStop calls quit when the stop file appears (it is removed), until end.
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

// imageIco returns the smallest image of an .ico with at least size pixels, else the largest. 6-byte header
// (image count at byte 4), then 16 bytes per image: width (0 = 256), size (byte 8) and start (byte 12).
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

// cmdTray runs 3dssavesync tray: the tray icon, and the local Server if it is configured. A second icon stops silently.
func cmdTray(config string) (int, error) {
	dir := filepath.Dir(config)
	release := trayLock(dir)
	if release == nil {
		return 0, nil
	}
	defer release()
	os.Remove(filepath.Join(dir, stopFile)) // left from a request made when the tray icon was not running
	var cfg Config
	readJSON(config, &cfg) // not configured yet: only the open-settings and quit items
	var server struct {
		sync.Mutex
		err error
	}
	if cfg.LocalServer != nil {
		go func() {
			_, err := cmdServe(config) // returns only on an error, or if it already runs elsewhere
			if err != nil {
				logTo(dir, "tray : serveur local : "+err.Error())
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
	if version != "dev" && cfg.checkUpdate() { // at most one check per day (latestRelease); failure: nothing in the menu
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
			if upload.TryLock() { // two clicks: one Upload only
				defer upload.Unlock()
				notify(uploadNow(config))
			}
		case "synchro":
			if upload.TryLock() {
				defer upload.Unlock()
				notify(syncAll(config))
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
	logTo(dir, "tray : icône près de l'horloge")
	err := showIcon(menu, choose, end)
	if err == nil {
		return 0, nil
	}
	logTo(dir, "tray : "+err.Error())
	if cfg.LocalServer == nil {
		return 1, err
	}
	notify(err.Error() + "\n" + tr("Le serveur local reste en marche.", "The local server is still running."))
	<-end
	return 0, nil
}
