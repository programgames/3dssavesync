package main

// Restore (GLOSSARY, ADR 0004, ADR 0016): an old Version of the server History becomes the Current version again,
// through a new conditional Upload. No local file is written and no Base changes: each device then gets this
// Version through its normal Download, with its Backup copy.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HistoryVersion is a History Version, as GET /saves/<key>/history describes it (server.py, server.go).
type HistoryVersion struct {
	Version
	File string `json:"version"`
	Date string `json:"date"`
}

// HistoryLine is one Version for the page and the console. N: the server's <seq>, never renumbered.
type HistoryLine struct {
	N        int
	File     string `json:"Fichier"`
	Hash     string
	Date     string
	Client   string
	Size     string `json:"Taille"`
	PlayInfo string `json:"Partie"`
	Current  bool   `json:"Courante"`
}

type HistoryGame struct {
	Key  string `json:"Cle"`
	Name string `json:"Nom"`
}

const (
	maxVersions   = 30
	restoreClient = "restauration" // X-Client of the Upload: the History shows who put this Version back
)

var reVersion = regexp.MustCompile(`^[0-9]{6}-[0-9]{8}T[0-9]{6}Z-[a-z0-9]{1,16}-[0-9a-f]{64}\.sav$`)

// serverGames returns the server's games, with the known name (3DS titles, ROM found by romNames, or name given to the
// server by a Client: ADR 0018).
func (s *Session) serverGames(names map[string]string) ([]HistoryGame, error) {
	st, o, err := s.api("GET", "/saves", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, &httpError{"GET /saves", st}
	}
	var l []Version
	if err := json.Unmarshal(o, &l); err != nil {
		return nil, err
	}
	games := []HistoryGame{}
	for _, v := range l {
		name := names[v.Code]
		if strings.HasPrefix(v.Code, "3DS-") {
			name = titles3DS[v.Code[4:]]
		}
		if name == "" {
			name = v.Name
		}
		games = append(games, HistoryGame{v.Code, name})
	}
	sort.SliceStable(games, func(a, b int) bool { return games[a].Name+games[a].Key < games[b].Name+games[b].Key })
	return games, nil
}

// romNames returns Key -> ROM name, from the ROM folders of the configured emulators.
// ponytail: a full walk, once per opened page; add a disk cache if thousands of ROMs make it slow.
func romNames(cfg *Config) map[string]string {
	m := map[string]string{}
	for _, e := range cfg.Emulators {
		for _, root := range e.Roms {
			filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if r, err := readRom(path); err == nil && r.Key != "" && m[r.Key] == "" {
						m[r.Key] = r.Name
					}
				}
				return nil
			})
		}
	}
	return m
}

// historyLines returns the last maxVersions Versions of a Game, the newest (the current one) first.
// Pokémon: play time and trainer of each Version, read in full.
// ponytail: up to 30 reads of 512 KiB per list; read on demand if a slow remote server makes it painful.
func (s *Session) historyLines(key string) ([]HistoryLine, error) {
	if !validKey(key) {
		return nil, fmt.Errorf(tr("Clé de jeu inconnue : %s. « 3dssavesync restore » affiche les clés des jeux du serveur.",
			"Unknown game key: %s. \"3dssavesync restore\" lists the keys of the games on the server."), key)
	}
	st, o, err := s.api("GET", "/saves/"+key+"/history", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, &httpError{"GET /saves/" + key + "/history", st}
	}
	var l []HistoryVersion
	if err := json.Unmarshal(o, &l); err != nil {
		return nil, err
	}
	slices.Reverse(l)
	if len(l) > maxVersions {
		l = l[:maxVersions]
	}
	lines := []HistoryLine{}
	for i, v := range l {
		n, _ := strconv.Atoi(strings.SplitN(v.File, "-", 2)[0])
		x := HistoryLine{n, v.File, v.Hash, localDate(v.Date), v.Client, formatSize(v.Size), "", i == 0}
		if _, g67 := pokemonFormats[int(v.Size)]; g67 || v.Size == 0x8000 || v.Size == 0x20000 || v.Size == 0x80000 {
			if st, o, err := s.api("GET", "/saves/"+key+"/history/"+v.File, nil, nil); err == nil && st == 200 {
				x.PlayInfo, _ = pokemonPlayInfo(o)
			}
		}
		lines = append(lines, x)
	}
	return lines, nil
}

// localDate converts a server date (UTC, server clock) to local time.
func localDate(d string) string {
	t, err := time.Parse("20060102T150405Z", d)
	if err != nil {
		return d
	}
	return t.Local().Format(tr("02/01/2006 15:04", "2006-01-02 15:04"))
}

func formatSize(n int64) string {
	if n >= 1024 {
		return fmt.Sprintf(tr("%d Kio", "%d KiB"), (n+512)/1024)
	}
	return fmt.Sprintf(tr("%d octets", "%d bytes"), n)
}

// restore makes the Version file the Current version again, only if the current one is still current (the Hash
// the user saw): an Upload that arrived meanwhile is refused, never overwritten. already: nothing to do, same content.
func (s *Session) restore(key, file, current string) (already bool, err error) {
	defer func() {
		if err != nil {
			s.log("restauration " + key + " " + file + " : " + err.Error())
		}
	}()
	if !validKey(key) || !reVersion.MatchString(file) {
		return false, fmt.Errorf(tr("Version introuvable : %s %s", "Version not found: %s %s"), key, file)
	}
	if hashOf(file) == current {
		return true, nil
	}
	st, o, err := s.api("GET", "/saves/"+key+"/history/"+file, nil, nil)
	if err != nil {
		return false, err
	}
	if st != 200 {
		return false, &httpError{"GET /saves/" + key + "/history/" + file, st}
	}
	if sha256Hex(o) != hashOf(file) {
		return false, errors.New(tr("La version reçue du serveur est incomplète, rien n'a été restauré. Réessayez.",
			"The version from the server arrived incomplete, so nothing was restored. Try again."))
	}
	st, _, err = s.api("PUT", "/saves/"+key, o, map[string]string{"If-Match": `"` + current + `"`})
	if err != nil {
		return false, err
	}
	if st == 412 {
		return false, errors.New(tr("Cette sauvegarde vient de changer sur le serveur (un appareil l'a envoyée entre-temps). Rien n'a été restauré : rouvrez la liste des versions et choisissez de nouveau.",
			"This save just changed on the server (a device uploaded it meanwhile). Nothing was restored. Open the version list again and choose again."))
	}
	if st != 200 {
		return false, &httpError{"PUT /saves/" + key, st}
	}
	s.log("restauration " + key + " : " + file + " redevient la Version courante (avant : " + current + ")")
	return false, nil
}

func restoredText() string {
	return tr("Version restaurée. C'est maintenant la version actuelle du serveur, et chaque appareil la recevra à sa prochaine synchronisation.",
		"Version restored. It's now the current version on the server, and each device gets it at its next sync.")
}

func alreadyText() string {
	return tr("Cette version est identique à la version actuelle. Il n'y a rien à restaurer.",
		"This version is the same as the current one. There's nothing to restore.")
}

// restoreSession returns the Restore Session: the configuration, and the Client name "restauration".
func restoreSession(config string) (*Session, error) {
	s, err := newSession(config)
	if s != nil {
		s.profile = restoreClient
	}
	return s, err
}

// cmdRestoreConsole runs in a console (the Windows exe has none, like repair) and shows the error there, no window.
func cmdRestoreConsole(config string, args []string, yes bool) int {
	openConsole()
	q := &Questions{in: bufio.NewReader(os.Stdin), out: os.Stdout}
	code, err := cmdRestore(config, args, yes, q)
	if err != nil {
		q.say("%s%s", tr("Erreur : ", "Error: "), errorText(err))
	}
	if cleanConsole {
		q.say("\n%s", tr("Appuyez sur Entrée pour fermer.", "Press Enter to close."))
		q.line()
	}
	return code
}

// cmdRestore: 3dssavesync restore [--yes] [<key> [<n>]]. Without a key, the server's games; without n, the Versions;
// otherwise the Restore of Version n, after confirmation. --yes may also come after the key (--oui: older name).
func cmdRestore(config string, args []string, yes bool, q *Questions) (int, error) {
	var rest []string
	for _, a := range args {
		if a == "--yes" || a == "-yes" || a == "--oui" || a == "-oui" {
			yes = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) > 2 {
		return 2, errors.New(tr("restore attend une clé de jeu, puis un numéro de version", "restore needs a game key, then a version number"))
	}
	s, err := restoreSession(config)
	if err != nil {
		return 1, err
	}
	if len(rest) == 0 {
		games, err := s.serverGames(nil)
		if err != nil {
			return 1, err
		}
		for _, j := range games {
			q.say("%-22s %s", j.Key, j.Name)
		}
		q.say(tr("Voir les versions d'un jeu : 3dssavesync restore <clé>", "See the versions of a game: 3dssavesync restore <key>"))
		return 0, nil
	}
	key := strings.ToUpper(rest[0])
	lines, err := s.historyLines(key)
	if err != nil {
		return 1, err
	}
	if len(lines) == 0 {
		return 1, fmt.Errorf(tr("Aucune version de %s sur le serveur. « 3dssavesync restore » affiche les jeux du serveur.",
			"No versions of %s on the server. \"3dssavesync restore\" lists the games on the server."), key)
	}
	line := func(l HistoryLine) string {
		t := fmt.Sprintf("%4d  %s  %-12s %8s", l.N, l.Date, l.Client, l.Size)
		if l.PlayInfo != "" {
			t += "  " + l.PlayInfo
		}
		if l.Current {
			t += tr("  (actuelle)", "  (current)")
		}
		return t
	}
	if len(rest) == 1 {
		for _, l := range lines {
			q.say("%s", line(l))
		}
		q.say(tr("Restaurer une version : 3dssavesync restore %s <n°>", "Restore a version: 3dssavesync restore %s <no.>"), key)
		return 0, nil
	}
	n, _ := strconv.Atoi(rest[1])
	i := slices.IndexFunc(lines, func(l HistoryLine) bool { return l.N == n })
	if i < 0 {
		return 1, fmt.Errorf(tr("Pas de version n° %s parmi les %d dernières de %s. « 3dssavesync restore %s » affiche la liste.",
			"No version %s among the last %d of %s. \"3dssavesync restore %s\" shows the list."), rest[1], maxVersions, key, key)
	}
	if lines[i].Hash == lines[0].Hash {
		q.say("%s", alreadyText())
		return 0, nil
	}
	q.say("%s\n%s", line(lines[i]), tr("Cette version deviendra la version actuelle sur le serveur. Aucune version n'est supprimée : la version actuelle reste dans l'historique.\n"+
		"Chaque appareil (3DS, PC) la recevra à sa prochaine synchronisation, après une copie de sécurité de sa sauvegarde.\n"+
		"Synchronisez d'abord vos autres appareils. Sinon, une partie pas encore envoyée pourrait remplacer cette version ou provoquer un conflit.",
		"This version will become the current version on the server. No version is deleted: the current one stays in the history.\n"+
			"Each device (3DS, PC) gets it at its next sync, after backing up its save.\n"+
			"Sync your other devices first. Otherwise a game not uploaded yet could replace this version or cause a conflict."))
	if !yes && !q.yes(tr("Restaurer cette version ?", "Restore this version?"), false) {
		q.say(tr("Rien n'a été modifié.", "Nothing was changed."))
		return 0, nil
	}
	already, err := s.restore(key, lines[i].File, lines[0].Hash)
	if err != nil {
		return 1, err
	}
	if already {
		q.say("%s", alreadyText())
	} else {
		q.say("%s", restoredText())
	}
	return 0, nil
}

// webGames serves the setup page: games, Versions, Restore.
func (a *WebSetup) webGames() map[string]any {
	s, err := restoreSession(a.config)
	if err != nil {
		return map[string]any{"Erreur": errorText(err)}
	}
	a.namesRead.Do(func() { a.names = romNames(s.cfg) }) // outside the page lock: two calls can arrive together
	games, err := s.serverGames(a.names)
	if err != nil {
		return map[string]any{"Erreur": errorText(err)}
	}
	return map[string]any{"Jeux": games}
}

// versionsWeb, restoreWeb: without the page lock (up to 30 network reads); ServeHTTP releases it before.
func (a *WebSetup) versionsWeb(key string) map[string]any {
	s, err := restoreSession(a.config)
	if err == nil {
		var l []HistoryLine
		if l, err = s.historyLines(key); err == nil {
			return map[string]any{"Versions": l}
		}
	}
	return map[string]any{"Erreur": errorText(err)}
}

func (a *WebSetup) restoreWeb(key, file, current string) map[string]any {
	s, err := restoreSession(a.config)
	if err != nil {
		return map[string]any{"Ok": false, "Message": errorText(err)}
	}
	already, err := s.restore(key, file, current)
	switch {
	case err != nil:
		return map[string]any{"Ok": false, "Message": errorText(err)}
	case already:
		return map[string]any{"Ok": true, "Message": alreadyText()}
	}
	return map[string]any{"Ok": true, "Message": restoredText()}
}
