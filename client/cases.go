package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// keyExamples: the Systems that have Aliases (a 3DS Save follows the title ID), with a Key for the help text.
var keyExamples = map[System]string{DS: "0RAD", GBA: "GBA-0RAD", GB: "GB-MONHACK-0001"}

// aliasKey returns the Key chosen in "alias" for this ROM (file name without extension), otherwise the header Key.
// A malformed Alias is an error: the ROM does not sync under the header Key (the one of the original game).
func (c *Config) aliasKey(system System, name, key string) (string, error) {
	a, ok := c.Alias[name]
	if !ok || keyExamples[system] == "" {
		return key, nil
	}
	if a = strings.ToUpper(strings.TrimSpace(a)); !keyShapes[system].MatchString(a) {
		return "", fmt.Errorf(tr("L'alias « %s » de %s n'est pas valable (exemple pour un jeu %s : %s). Retirez-le dans les réglages, section ROM hacks, et ajoutez de nouveau la ROM.",
			"The alias \"%s\" of %s isn't valid (example for a %s game: %s). Remove it in the settings, ROM hacks section, and add the ROM again."), c.Alias[name], name, system, keyExamples[system])
	}
	return a, nil
}

// romsPath returns the file that holds the ROM of each Key synced on this PC, all emulators together (ADR 0014): the
// Current version of the server is the same for all. Outside bases/: the Base of an emulator named "roms" would be
// bases/roms.json.
func (s *Session) romsPath() string { return filepath.Join(s.dir, "roms.json") }

// keptRoms returns roms.json, completed by the old per-emulator files (bases/roms/<profile>.json, read only; for the
// same Key, the first emulator in alphabetical order). The next write of roms.json takes them over.
func (s *Session) keptRoms() (map[string]string, error) {
	m := map[string]string{}
	err := readJSON(s.romsPath(), &m)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	olds, _ := filepath.Glob(filepath.Join(s.dir, "bases", "roms", "*.json"))
	for _, f := range olds {
		a := map[string]string{}
		readJSON(f, &a)
		for key, rom := range a {
			if m[key] == "" {
				m[key] = rom
			}
		}
	}
	return m, err
}

// knownBase reports whether an emulator of this PC has a Base for this Key (or an unreadable Base: when in doubt, yes).
func (s *Session) knownBase(key string) bool {
	files, _ := filepath.Glob(filepath.Join(s.dir, "bases", "*.json"))
	for _, f := range files {
		m := map[string]string{}
		if readJSON(f, &m) != nil || m[key] != "" {
			return true
		}
	}
	return false
}

// romRefusal returns the refusal text if this ROM must not sync under the Key of j, otherwise "" and the ROM is kept
// for the next Base (setBase). Refusals:
//   - another ROM, which still exists, is already synced under this Key (ROM hack that keeps the Key of the original
//     game). A kept ROM that no longer exists (renamed, moved, extracted to a temporary folder) gives way;
//   - no kept ROM, but a Base of this Key on this PC (synced before ROMs were kept), and no save for this ROM: the
//     Download would give a possible ROM hack the playthrough of the original game, and its next Upload would replace
//     it.
//
// With question (run), the user chooses: same game (second case), "Sync separately" (an Alias is created, j takes
// its Key), or play without syncing.
func (c *Client) romRefusal(j *Game, rom string, question bool) string {
	if j.Key == "" || rom == "" {
		return ""
	}
	apart, without := tr("Synchroniser à part", "Sync separately"), tr("Jouer sans synchroniser", "Play without syncing")
	hint := tr("Pour lui donner sa propre sauvegarde, lancez-le depuis votre frontend et choisissez « Synchroniser à part ». Vous pouvez aussi l'ajouter dans les réglages, section ROM hacks.",
		"To give it its own save, start it from your frontend and choose \"Sync separately\". You can also add it in the settings, ROM hacks section.")
	choice := ""
	m, _ := c.keptRoms()
	if a, err := os.Stat(m[j.Key]); err == nil {
		if b, err := os.Stat(rom); err == nil && !os.SameFile(a, b) {
			c.log(fmt.Sprintf("même Clé %s que %s : %s", j.Key, m[j.Key], rom))
			text := fmt.Sprintf(tr("%s ressemble à %s, déjà synchronisé. C'est sans doute un ROM hack.",
				"%s looks like %s, which is already synced. It's probably a ROM hack."), filepath.Base(rom), filepath.Base(m[j.Key]))
			if question {
				choice = ask(text+"\n\n"+tr("Lui donner sa propre sauvegarde ?", "Give it its own save?"),
					[]string{apart, without})
			}
			if choice != apart {
				return text + " " + hint
			}
		}
	} else if m[j.Key] == "" && c.knownBase(j.Key) {
		if l, err := readLocal(j, nil); err == nil && (l == nil || l.Blank) {
			c.log(fmt.Sprintf("Clé %s déjà synchronisée, aucune ROM retenue, pas de sauvegarde : %s", j.Key, rom))
			yes := tr("Synchroniser", "Sync")
			if question {
				choice = ask(fmt.Sprintf(tr("%s n'a pas encore de sauvegarde dans cet émulateur, mais ce jeu a déjà été synchronisé sur ce PC, peut-être avec une autre ROM.\n\n"+
					"C'est le même jeu ? « Synchroniser » récupère la sauvegarde du serveur.\nC'est un ROM hack ? « Synchroniser à part » lui donne sa propre sauvegarde.",
					"%s has no save in this emulator yet, but this game was already synced on this PC, maybe with another ROM.\n\n"+
						"Same game? \"Sync\" downloads the server save.\nROM hack? \"Sync separately\" gives it its own save."), filepath.Base(rom)),
					[]string{yes, apart, without})
			}
			if choice != yes && choice != apart {
				return fmt.Sprintf(tr("%s n'a pas encore de sauvegarde ici, mais ce jeu a déjà été synchronisé sur ce PC, peut-être avec une autre ROM. Lancez-le depuis votre frontend pour choisir.",
					"%s has no save here yet, but this game was already synced on this PC, maybe with another ROM. Start it from your frontend to choose."), filepath.Base(rom))
			}
		}
	}
	if choice == apart {
		key, err := c.createAlias(j.System, strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom)))
		if err != nil {
			c.log("alias impossible pour " + rom + " : " + err.Error())
			return fmt.Sprintf(tr("%s : impossible de le synchroniser à part (%v).", "%s: couldn't sync it separately (%v)."), filepath.Base(rom), err)
		}
		c.log(fmt.Sprintf("alias créé : %s -> %s", rom, key))
		notify(fmt.Sprintf(tr("%s a maintenant sa propre sauvegarde. Pour y jouer aussi sur la 3DS, préparez de nouveau la carte SD dans les réglages.",
			"%s now has its own save. To play it on the 3DS too, prepare the SD card again in the settings."), filepath.Base(rom)))
		j.Key = key
	}
	if abs, err := filepath.Abs(rom); err == nil {
		rom = abs
	}
	if c.romOf == nil {
		c.romOf = map[string]string{}
	}
	c.romOf[j.Key] = rom
	return ""
}

// createAlias returns the Alias of the ROM name, created if missing and written to config.json (dated copy first).
func (s *Session) createAlias(system System, name string) (string, error) {
	var cfg Config
	if err := readConfig(s.config, &cfg); err != nil { // read again: the settings page may have written it since
		return "", err
	}
	key, err := addAlias(&cfg, system, name)
	if err == nil {
		err = writeConfigCopy(s.config, &cfg)
	}
	if err != nil {
		return "", err
	}
	s.cfg.Alias = cfg.Alias
	return key, nil
}

// writeConfigCopy makes a dated copy of config.json, then a verified write.
func writeConfigCopy(path string, cfg *Config) error {
	if err := copyBefore(path); err != nil {
		return err
	}
	return writeJSON(path, cfg, true)
}

// addAlias returns the Alias of the ROM name in cfg, created if missing (ADR 0014). The Key comes from the name: the
// same ROM gets the same Key on each PC; a Key already taken by another Alias gives another one.
func addAlias(cfg *Config, system System, name string) (string, error) {
	if keyExamples[system] == "" {
		return "", fmt.Errorf(tr("Les jeux %s n'ont pas d'alias.", "%s games don't use aliases."), system)
	}
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "=\r\n") {
		return "", fmt.Errorf(tr("Ce nom de ROM ne peut pas servir d'alias : « %s »", "This ROM name can't be used as an alias: \"%s\""), name)
	}
	if a := strings.ToUpper(cfg.Alias[name]); keyShapes[system].MatchString(a) {
		return a, nil
	}
	taken := map[string]bool{}
	for n, a := range cfg.Alias {
		if n != name {
			taken[strings.ToUpper(a)] = true
		}
	}
	key := newKey(system, name, taken)
	if cfg.Alias == nil {
		cfg.Alias = map[string]string{}
	}
	cfg.Alias[name] = key
	return key, nil
}

// newKey returns a Key with the shape of the System that starts with a digit (never a commercial Game code).
func newKey(system System, name string, taken map[string]bool) string {
	const signs = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := 0; ; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", name, i)))
		suffix := string([]byte{signs[h[0]%36], signs[h[1]%36], signs[h[2]%36]})
		key := map[System]string{DS: "0" + suffix, GBA: "GBA-0" + suffix, GB: fmt.Sprintf("GB-ALIAS-%02X%02X", h[0], h[1])}[system]
		if !taken[key] {
			return key
		}
	}
}

// keepRom keeps the ROM of this Key after a Base is written (if the ROM is known and changed).
func (c *Client) keepRom(key string) error {
	rom := c.romOf[key]
	if rom == "" {
		return nil
	}
	m, err := c.keptRoms()
	if err != nil {
		c.log("ROM retenues illisibles, remplacées : " + err.Error())
	}
	if m[key] == rom {
		return nil
	}
	m[key] = rom
	return writeJSON(c.romsPath(), m, true)
}

// romKey (run) returns the Key of the ROM for this Client, Alias included, or a reason to start without Sync.
func (s *Session) romKey(system System, key, name string) (string, string) {
	if system == "3DS" { // Azahar stores the Save by title ID: two ROMs of a 3DS game share it
		return key, ""
	}
	key, err := s.cfg.aliasKey(system, name, key)
	if err != nil {
		return "", err.Error()
	}
	return key, ""
}

// pendingEntry is a pending Upload: a played Save that the server did not receive (network down). The usual Sync
// resumes it at the start of each run or launch, whatever the game (never an overwrite without comparison).
type pendingEntry struct {
	Profile string `json:"profil"`
	Game
}

// pendingPath returns the list of pending Uploads of a configuration folder.
func pendingPath(dir string) string { return filepath.Join(dir, "en-attente.json") }

// pendingLock lets one Launcher at a time read and rewrite the list (two Launchers can run together).
// A lock older than 30 s comes from a stopped Launcher: it is removed. The returned function removes the lock.
func (s *Session) pendingLock() func() {
	v := pendingPath(s.dir) + ".verrou"
	os.MkdirAll(s.dir, 0o755)
	for attempt := 0; attempt < 50; attempt++ {
		f, err := os.OpenFile(v, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return func() { os.Remove(v) }
		}
		if !errors.Is(err, fs.ErrExist) {
			break
		}
		if i, err := os.Stat(v); err == nil && time.Since(i.ModTime()) > 30*time.Second {
			os.Remove(v)
			continue
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.log("en-attente.json : verrou impossible, liste modifiée sans verrou")
	return func() {}
}

// editPending reads the list again, changes it and writes it back (verified temporary file then renamed, writeJSON).
// Unreadable list: set aside (en-attente.json.illisible-<date>), never replaced without a copy.
func (s *Session) editPending(f func([]pendingEntry) []pendingEntry) error {
	defer s.pendingLock()()
	var l []pendingEntry
	if err := readJSON(pendingPath(s.dir), &l); err != nil && !errors.Is(err, fs.ErrNotExist) {
		side := pendingPath(s.dir) + ".illisible-" + time.Now().Format("20060102-150405")
		if e := os.Rename(pendingPath(s.dir), side); e != nil {
			s.log("en-attente.json illisible, et impossible à mettre de côté : " + err.Error() + " ; " + e.Error())
			return e
		}
		s.log("en-attente.json illisible, mis de côté dans " + side + " : " + err.Error())
		notify(fmt.Sprintf(tr("La liste des envois en attente était illisible. Elle a été mise de côté dans %s, et ses jeux seront envoyés à leur prochain lancement.",
			"The list of pending uploads couldn't be read. It was set aside in %s, and its games will be uploaded the next time you start them."), side))
		l = nil
	}
	err := writeJSON(pendingPath(s.dir), f(l), true)
	if err != nil {
		s.log("en-attente.json : " + err.Error())
	}
	return err
}

// addPending marks as pending the games whose Local save changed since the Base (those To upload). Error: the list
// could not be written.
func (c *Client) addPending(games []*Game) error {
	b, _ := c.bases()
	var added []pendingEntry
	for _, j := range games {
		l, err := readLocal(j, nil)
		if err == nil && (l == nil || l.Blank || b[j.Key] == l.Hash) {
			continue
		}
		c.log(j.Key + " : Envoi en attente")
		added = append(added, pendingEntry{c.name, *j})
	}
	if len(added) == 0 {
		return nil
	}
	return c.editPending(func(l []pendingEntry) []pendingEntry {
		for _, n := range added {
			l = append(remove(l, n), n)
		}
		return l
	})
}

func remove(l []pendingEntry, a pendingEntry) []pendingEntry {
	var r []pendingEntry
	for _, x := range l {
		if x.Profile != a.Profile || x.Key != a.Key {
			r = append(r, x)
		}
	}
	return r
}

// resume runs the pending Uploads, before the Sync of the started game. A Conflict goes through the usual question
// (after play: keep the PC save or do nothing). Stays pending: emulator open, or network still down.
// Returns the last Upload error (tray icon menu, "Upload now").
func (s *Session) resume() error {
	var l []pendingEntry
	if readJSON(pendingPath(s.dir), &l) != nil || len(l) == 0 {
		return nil
	}
	var done []pendingEntry
	var last error
	for _, a := range l {
		e := s.cfg.Emulators[a.Profile]
		if e == nil || !exists(a.File) {
			s.log(a.Key + " : Envoi en attente abandonné (émulateur ou fichier disparu) : " + a.File)
			done = append(done, a)
			continue
		}
		if instances(e.process()) > 0 { // a running game can still write this file
			continue
		}
		j := a.Game
		j.MBC2 = s.cfg.mbc2Size(a.Profile, a.Core)       // not in en-attente.json
		_, err := s.client(a.Profile).syncGame(&j, true) // its own Client: no ROM kept, no warning for the started game
		if err == nil {
			s.log(a.Key + " : Envoi en attente repris")
			done = append(done, a)
			continue
		}
		s.log(a.Key + " en attente : " + err.Error())
		last = err
		if end(err) {
			break
		}
	}
	if len(done) > 0 {
		s.editPending(func(l []pendingEntry) []pendingEntry {
			for _, a := range done {
				l = remove(l, a)
			}
			return l
		})
	}
	return last
}
