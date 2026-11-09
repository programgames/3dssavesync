package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// formats3DS holds the Save archive format of a 3DS game: Azahar's 00000001.metadata (ArchiveFormatInfo:
// total_size, directory count, file count as u32 LE, duplicate_data, 3 padding bytes). With it, the Launcher creates
// the folder of a game never started in Azahar; without it, it refuses: a game without .metadata can reformat its Save.
// Read on real installs: Moon, Ultra Sun (2026-11-01). The other Pokémon games: same engine, same value assumed;
// Pokémon X checked by the test on a copy of Azahar (issue #9).
var formats3DS = func() map[string][]byte {
	pokemon := []byte{0, 0, 4, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0} // 0x40000 bytes, 1 directory, 1 file, duplicated
	m := map[string][]byte{}
	for id := range titles3DS {
		m[id] = pokemon
	}
	return m
}()

// Game is a Key and its matching save file in an emulator.
type Game struct {
	System System `json:"Systeme"`
	Key    string `json:"Cle"`
	Name   string `json:"Nom"`
	File   string `json:"Fichier"`
	Core   string `json:"Coeur"` // RetroArch core library or "": gpSP and Beetle GBA have a .srm to convert (srm.go)
	MBC2   int    `json:"-"`     // size of a new MBC2 file for this emulator (Config.mbc2Size); 0 = 512
}

// savePath returns the save file of a Game in an emulator's folder. 3DS: folder = Azahar's sdmc; the file is /main
// of the save archive for a 3DS- Key, the save archive folder itself for a 3DSA- Key (ADR 0028).
func savePath(dir string, system System, key, name, ext string) string {
	if system == "3DS" {
		id, z := strings.ToLower(key[strings.IndexByte(key, '-')+1:]), strings.Repeat("0", 32)
		archive := filepath.Join(dir, "Nintendo 3DS", z, z, "title", id[:8], id[8:], "data", "00000001")
		if strings.HasPrefix(key, "3DSA-") {
			return archive
		}
		return filepath.Join(archive, "main")
	}
	if ext == "" {
		ext = ".sav"
	}
	return filepath.Join(dir, name+ext)
}

func sha256Hex(o []byte) string {
	h := sha256.Sum256(o)
	return hex.EncodeToString(h[:])
}

// Version is a Save as the server or the PC sees it (Hash and size of the SRAM).
type Version struct {
	Code   string `json:"code"`
	Hash   string `json:"hash"`
	Size   int64  `json:"taille"`
	Client string `json:"client"`
	Age    *int64 `json:"age"`
	Name   string `json:"nom"` // game name given by a Client (ADR 0018); empty on an older server
}

type Local struct {
	Version
	Raw   []byte
	SRAM  []byte
	Blank bool // blank SRAM (only 0x00 or only 0xFF), written before any play
}

// readLocal reads the Local save: Hash, size and Upload cover its SRAM only (decode). srv: Current version of the
// server or nil.
func readLocal(j *Game, srv *Version) (*Local, error) {
	if isArchive(j) { // the archive of the whole folder; a folder with no file counts as a Blank save
		files, err := readArchiveDir(j.File)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		data, err := PackArchive(files)
		if err != nil {
			return nil, fmt.Errorf("%s : %w", j.File, err)
		}
		return &Local{Version{Hash: sha256Hex(data), Size: int64(len(data))}, data, data, len(files) == 0}, nil
	}
	raw, err := os.ReadFile(j.File)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sram, err := decode(j, raw, srv)
	if err != nil {
		return nil, fmt.Errorf("%s : %w", j.File, err)
	}
	return &Local{Version{Hash: sha256Hex(sram), Size: int64(len(sram))}, raw, sram, j.System != "3DS" && blank(sram)}, nil
}

// State of a Game (docs/design.md §1, ADR 0001), named like tests/cas_etat.csv.
type State string

const (
	inSync     State = "A_JOUR"
	toUpload   State = "A_ENVOYER"
	toDownload State = "A_RECUPERER"
	conflict   State = "CONFLIT"
)

// state compares Local save, Current version and Base. nil = absent; empty base = never synced.
func state(local, server *Version, base string) State {
	switch {
	case local != nil && server != nil && local.Hash == server.Hash:
		return inSync
	case server == nil:
		return toUpload
	case local == nil:
		return toDownload
	case local.Size != server.Size:
		return conflict
	case local.Hash == base:
		return toDownload
	case server.Hash == base:
		return toUpload
	}
	return conflict
}

// Session is one run of the program: the configuration, its folder (log, Bases, Backup copies) and the server.
type Session struct {
	cfg    *Config
	config string // path of config.json: an Alias created at launch is written there (ADR 0014)
	dir    string
	*remote
}

// Client is an emulator of this PC on the server: its Bases, its Backup copies, the ROMs kept for its Keys. A launch
// has one; each pending Upload resumes with its own (resume).
type Client struct {
	*Session
	name       string            // config.json entry; X-Client on the server (clientName)
	romOf      map[string]string // Key -> ROM of this launch, kept with the Base (ADR 0014)
	downloaded bool              // a Version arrived from the server for this Client
	batch      *syncCount        // Sync all: outcomes counted, and a Conflict is skipped (no window)
}

// syncCount counts the outcomes of a Sync all. conflicts: names of the games left in Conflict.
type syncCount struct {
	uploaded, downloaded, inSync int
	conflicts                    []string
}

func (s *Session) client(name string) *Client { return &Client{Session: s, name: name} }

func (s *Session) log(text string) { logTo(s.dir, text) }

// logTo adds a dated line to the log of a configuration folder.
func logTo(dir, text string) {
	f, err := os.OpenFile(filepath.Join(dir, "3dssavesync.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02T15:04:05"), text)
		f.Close()
	}
}

func (c *Client) api(method, path string, body []byte, headers map[string]string) (int, []byte, error) {
	return c.request(c.name, method, path, body, headers)
}

// helpPage is the help page for error messages, added to the error windows of the PC.
const helpPage = "https://github.com/programgames/3dssavesync/blob/main/docs/troubleshooting.md"

// netCause is why a request failed: the same for the sync server and GitHub (gitHubError).
type netCause int

const (
	notNetwork     netCause = iota // a file or another error
	httpStatus                     // the server answered another HTTP status
	badCertificate                 // TLS: certificate refused, or not a TLS server
	noHost                         // DNS: name not found
	refused                        // connection refused: server stopped or wrong port
	noResponse                     // timeout, cut connection, or any other transport failure
)

// netFailure returns the cause of a failed request, and the HTTP status for httpStatus.
func netFailure(err error) (netCause, int) {
	var h *httpError
	var cert *tls.CertificateVerificationError
	var header tls.RecordHeaderError
	var dns *net.DNSError
	var errno syscall.Errno
	var network net.Error
	var ue *neturl.Error
	errors.As(err, &errno)
	switch {
	case errors.As(err, &h):
		return httpStatus, h.code
	case errors.As(err, &cert) || errors.As(err, &header):
		return badCertificate, 0
	case errors.As(err, &dns):
		return noHost, 0
	case errors.Is(err, syscall.ECONNREFUSED) || runtime.GOOS == "windows" && errno == 10061: // WSAECONNREFUSED
		return refused, 0
	case errors.As(err, &network) && network.Timeout(), errors.As(err, &ue):
		return noResponse, 0
	}
	return notNetwork, 0
}

// classify returns a text for the user (no HTTP code, no Go text); global = the server or the network is at fault,
// so the other games of the pass would fail the same way. Unknown error: its own text, already written for the user.
func classify(err error) (text string, global bool) {
	var errno syscall.Errno
	var path *fs.PathError
	errors.As(err, &errno)
	windows := runtime.GOOS == "windows"
	cause, code := netFailure(err)
	switch {
	case err == nil:
		return "", false
	case cause == httpStatus && code == 401:
		return tr("Le serveur refuse ce jeton. Recopiez le jeton exact du serveur.",
			"The server rejected this token. Copy the exact token from the server."), true
	case cause == httpStatus && code == 404:
		return tr("Cette adresse ne correspond pas à un serveur 3dssavesync. Vérifiez-la.",
			"This address isn't a 3dssavesync server. Check it."), true
	case cause == httpStatus && code == 413:
		return tr("sauvegarde trop grosse pour le serveur : augmentez la taille maximale acceptée par le serveur ou par son proxy",
			"save too large for the server: raise the size limit on the server or its proxy"), false
	case cause == httpStatus && code == 400:
		return tr("le serveur a refusé cette sauvegarde (vide, trop grosse ou envoi interrompu). Relancez le jeu",
			"the server rejected this save (empty, too large or cut off). Start the game again"), false
	case cause == httpStatus && code >= 500:
		return tr("Le serveur a rencontré une erreur. Réessayez plus tard. Si le problème continue, consultez le journal du serveur.",
			"The server ran into an error. Try again later. If it keeps happening, check the server log."), true
	case cause == httpStatus:
		return tr("Le serveur a répondu de façon inattendue. Vérifiez son adresse.", "The server sent an unexpected response. Check its address."), true
	case cause == badCertificate:
		return tr("Le certificat du serveur n'est pas valide. Vérifiez l'adresse (https://) et la date du PC.",
			"The server's certificate isn't valid. Check the address (https://) and the PC's date."), true
	case cause == noHost:
		return tr("Serveur introuvable. Vérifiez votre connexion Internet et l'adresse du serveur.",
			"Can't find the server. Check your Internet connection and the server address."), true
	case cause == refused:
		return tr("Le serveur refuse la connexion. Il est peut-être arrêté, ou le port est incorrect : démarrez le serveur ou vérifiez l'adresse.",
			"The server refused the connection. It may be stopped, or the port is wrong. Start the server or check the address."), true
	case cause == noResponse:
		return tr("Le serveur ne répond pas. Vérifiez votre connexion Internet et l'adresse du serveur.", "The server isn't responding. Check your Internet connection and the server address."), true
	case windows && (errno == 32 || errno == 33): // ERROR_SHARING_VIOLATION, ERROR_LOCK_VIOLATION
		return tr("un autre programme utilise ce fichier. Fermez l'émulateur et relancez le jeu",
			"another program is using this file. Close the emulator and start the game again"), false
	case errors.Is(err, syscall.ENOSPC) || windows && (errno == 112 || errno == 39): // ERROR_DISK_FULL, ERROR_HANDLE_DISK_FULL
		return tr("disque plein. Libérez de la place et relancez le jeu", "disk full. Free up some space and start the game again"), false
	case errors.Is(err, fs.ErrPermission) && errors.As(err, &path):
		return fmt.Sprintf(tr("accès refusé à %s. Vérifiez les autorisations de ce dossier", "access denied to %s. Check this folder's permissions"), path.Path), false
	case errors.Is(err, fs.ErrPermission):
		return tr("accès refusé. Vérifiez les autorisations du dossier", "access denied. Check the folder's permissions"), false
	}
	return err.Error(), false
}

// errorText returns the error as the user reads it ("" without error).
func errorText(err error) string {
	t, _ := classify(err)
	return t
}

// basePath returns the Bases file: one file per emulator, Key -> Hash of the last Sync.
func (c *Client) basePath() string { return filepath.Join(c.dir, "bases", c.name+".json") }

func (c *Client) bases() (map[string]string, error) {
	m := map[string]string{}
	o, err := os.ReadFile(c.basePath())
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(bytes.TrimPrefix(o, []byte("\xef\xbb\xbf")), &m)
}

func (c *Client) setBase(key, hash string) error {
	m, err := c.bases()
	if err != nil {
		return err
	}
	m[key] = hash
	if err := writeJSON(c.basePath(), m, true); err != nil {
		return err
	}
	if err := c.keepRom(key); err != nil {
		c.log("ROM de " + key + " non retenue : " + err.Error())
	}
	return nil
}

// writeJSON writes a verified temporary file, then replaces the target (replace=false: never overwrites).
func writeJSON(path string, v any, replace bool) error {
	o, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if replace {
		return writeAtomic(path, o)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeVerified(path, o)
}

// tmpSuffix ends the temporary file of every atomic write.
const tmpSuffix = ".3dssavesync-tmp"

// writeAtomic replaces path (folders created) with buf: verified temporary file next to it, then rename. On failure
// path stays as it was and no temporary file is left.
func writeAtomic(path string, buf []byte) error { return writeAtomicIn(filepath.Dir(path), path, buf) }

// writeAtomicIn is writeAtomic with the temporary file in dir (a 3DS Save archive must hold only its own files).
func writeAtomicIn(dir, path string, buf []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	os.Remove(filepath.Join(dir, filepath.Base(path)+tmpSuffix)) // leftover of a hard stop (fixed name of older versions)
	// A temporary name of its own: two Launchers of one emulator can write the same Base file at once.
	// ponytail: a hard stop leaves its random-named file behind; sweep old ones if they ever pile up.
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*"+tmpSuffix)
	if err != nil {
		return err
	}
	tmp := f.Name()
	f.Close()
	os.Remove(tmp) // writeVerified creates the file itself (O_EXCL)
	if err := writeVerified(tmp, buf); err != nil {
		return err
	}
	if err := rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// writeVerified creates the file (never overwrites), writes it to disk, then reads it back.
func writeVerified(path string, buf []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(buf)
	if e := f.Sync(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil {
		if reread, e := os.ReadFile(path); e != nil {
			err = e
		} else if !bytes.Equal(reread, buf) {
			err = fmt.Errorf(tr("%s : le fichier écrit ne correspond pas", "%s: the written file doesn't match"), path)
		}
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// upload is a conditional Upload: If-Match = Current version seen by this Client. false on 412.
func (c *Client) upload(j *Game, l *Local, srv *Version) (bool, error) {
	cond := map[string]string{"If-None-Match": "*"}
	if srv != nil {
		cond = map[string]string{"If-Match": `"` + srv.Hash + `"`}
	}
	st, _, err := c.api("PUT", "/saves/"+j.Key, l.SRAM, cond)
	if err != nil {
		return false, err
	}
	if st == 412 {
		return false, nil
	}
	if st != 200 {
		return false, &httpError{"PUT /saves/" + j.Key, st}
	}
	if srv == nil && j.Name != "" { // first Upload of this game: the server does not have its name yet (ADR 0018)
		c.setName(c.name, j)
	}
	c.listRead = false // the list no longer has the Current version of this Game
	return true, c.setBase(j.Key, l.Hash)
}

// copy makes a Backup copy of the Local save, <copies>/<emulator>/<name>-<date>, and returns its path. A 3DSA
// archive is kept as a .tar.
func (c *Client) copy(j *Game, raw []byte) (string, error) {
	dir := filepath.Join(c.cfg.copiesDir(c.dir), c.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(j.File)
	if isArchive(j) {
		ext = ".tar"
	}
	base := j.Name + time.Now().Format("-20060102-150405")
	for i := 0; ; i++ {
		name := base + ext
		if i > 0 {
			name = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		path := filepath.Join(dir, name)
		err := writeVerified(path, raw)
		if !errors.Is(err, fs.ErrExist) || i == 9 {
			return path, err
		}
	}
}

// readArchiveDir returns the files of a 3DS save archive folder, paths relative with '/' (ADR 0028). Only regular
// files: anything else is an error, so a folder that the archive can't hold is never synced.
func readArchiveDir(dir string) ([]ArchiveFile, error) {
	var files []ArchiveFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf(tr("%s : pas un fichier ordinaire", "%s: not a regular file"), path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		files = append(files, ArchiveFile{filepath.ToSlash(rel), data})
		return err
	})
	return files, err
}

// errNeverStarted: a 3DS game whose Azahar folder does not exist and is not created (ADR 0009, 0028).
var errNeverStarted = errors.New(tr("ce jeu n'a encore jamais été lancé dans Azahar. Lancez-le une fois dans Azahar, sauvegardez dans le jeu et relancez-le ici",
	"this game has never been started in Azahar. Start it once in Azahar, save in the game, then start it here again"))

// downloadArchive writes a 3DSA Version into the Azahar folder of the game (ADR 0028): a damaged archive is refused;
// the folder must exist (never created, 00000001.metadata never written); Backup copy (.tar, read back); the files
// absent from the Version are moved next to the Backup copy, never deleted; each file of the Version goes through a
// verified temporary file; then the folder is read back and compared with the Version.
func (c *Client) downloadArchive(j *Game, l *Local, srv *Version, o []byte) error {
	files, err := UnpackArchive(o)
	for _, f := range files {
		if !filepath.IsLocal(filepath.FromSlash(f.Path)) { // e.g. "a:b" on Windows
			err = errArchive
		}
	}
	if err != nil {
		return fmt.Errorf(tr("la version du serveur est endommagée, votre sauvegarde n'a pas été remplacée. Restaurez une version précédente dans l'Historique\n%w",
			"the server version is damaged, so your save wasn't replaced. Restore an earlier version in the History\n%w"), err)
	}
	if i, err := os.Stat(j.File); err != nil || !i.IsDir() {
		return errNeverStarted
	}
	// The folder as it is now: it must still be the Local save that the State compared.
	now, err := readArchiveDir(j.File)
	if err != nil {
		return err
	}
	raw, err := PackArchive(now)
	if err != nil {
		return err
	}
	if l == nil || sha256Hex(raw) != l.Hash {
		return errors.New(tr("la sauvegarde a changé pendant la synchronisation, elle n'a pas été remplacée. Fermez Azahar et relancez le jeu",
			"the save changed during the sync, so it wasn't replaced. Close Azahar and start the game again"))
	}
	backup, err := c.copy(j, raw)
	if err != nil {
		return fmt.Errorf(tr("impossible de faire la copie de sécurité : %w", "couldn't make a backup: %w"), err)
	}
	want := map[string]bool{}
	for _, f := range files {
		want[f.Path] = true
	}
	moved := strings.TrimSuffix(backup, ".tar") // <name>-<date>/ next to <name>-<date>.tar
	for _, f := range now {
		if want[f.Path] {
			continue
		}
		from, to := filepath.Join(j.File, filepath.FromSlash(f.Path)), filepath.Join(moved, filepath.FromSlash(f.Path))
		data, err := os.ReadFile(from)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(to), 0o755)
		}
		if err == nil {
			err = writeVerified(to, data)
		}
		if err == nil {
			err = os.Remove(from)
		}
		if err != nil {
			return err
		}
	}
	for _, f := range files {
		path := filepath.Join(j.File, filepath.FromSlash(f.Path))
		if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, f.Data) {
			continue
		}
		if err := writeAtomicIn(filepath.Dir(j.File), path, f.Data); err != nil { // temporary file outside the archive
			return err
		}
	}
	back, err := readArchiveDir(j.File)
	if err == nil {
		raw, err = PackArchive(back)
	}
	if err != nil || !bytes.Equal(raw, o) {
		return fmt.Errorf(tr("%s : le dossier écrit ne correspond pas à la version du serveur. Copie de sécurité : %s",
			"%s: the written folder doesn't match the server version. Backup: %s"), j.File, backup)
	}
	c.downloaded = true
	return c.setBase(j.Key, srv.Hash)
}

// download is a Download: Backup copy, verified temporary file, then replacement (ADR 0004, docs/design.md §3).
func (c *Client) download(j *Game, l *Local, srv *Version) error {
	st, o, err := c.api("GET", "/saves/"+j.Key, nil, nil)
	if err != nil {
		return err
	}
	if st != 200 {
		return &httpError{"GET /saves/" + j.Key, st}
	}
	if sha256Hex(o) != srv.Hash {
		return damagedError(tr("la sauvegarde reçue du serveur est incomplète, la vôtre n'a pas été remplacée. Relancez le jeu",
			"the save from the server arrived incomplete, so yours wasn't replaced. Start the game again"))
	}
	if isArchive(j) {
		return c.downloadArchive(j, l, srv, o)
	}
	dir, tmpDir := filepath.Dir(j.File), filepath.Dir(j.File)
	if j.System == "3DS" {
		// No folder = Azahar never saw this game. Create it only with a known .metadata (formats3DS) and never
		// over an existing .metadata; otherwise refuse (ADR 0009).
		if _, err := os.Stat(dir); err != nil {
			format, meta := formats3DS[strings.ToUpper(strings.TrimPrefix(j.Key, "3DS-"))], filepath.Join(filepath.Dir(dir), "00000001.metadata")
			if format == nil || exists(meta) {
				return errNeverStarted
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := writeVerified(meta, format); err != nil {
				os.Remove(dir)
				return err
			}
		}
		tmpDir = filepath.Dir(dir) // the archive must hold only main
	}
	var old []byte
	if l != nil {
		if _, err := c.copy(j, l.Raw); err != nil {
			return fmt.Errorf(tr("impossible de faire la copie de sécurité : %w", "couldn't make a backup: %w"), err)
		}
		old = l.Raw
	}
	content, err := encode(j, o, old)
	if err != nil {
		return err
	}
	if err := writeAtomicIn(tmpDir, j.File, content); err != nil {
		return err
	}
	if reread, err := os.ReadFile(j.File); err != nil || !bytes.Equal(reread, content) {
		return fmt.Errorf(tr("%s : le fichier remplacé ne correspond pas", "%s: the replaced file doesn't match"), j.File)
	}
	c.downloaded = true
	return c.setBase(j.Key, srv.Hash)
}

// damagedError: received content whose Hash is not the one of the list.
type damagedError string

func (e damagedError) Error() string { return string(e) }

// formatAge returns "2 h ago" from the age in seconds given by the server.
func formatAge(age *int64) string {
	switch {
	case age == nil:
		return ""
	case *age < 60:
		return tr("à l'instant", "just now")
	case *age < 3600:
		return fmt.Sprintf(tr("il y a %d min", "%d min ago"), *age/60)
	case *age < 86400:
		return fmt.Sprintf(tr("il y a %d h", "%d h ago"), *age/3600)
	}
	return fmt.Sprintf(tr("il y a %d j", "%d d ago"), *age/86400)
}

// resolveConflict returns the choice of the user on a Conflict. If no window can open, nothing is replaced.
// remote: content of the server Version if it was read (Pokémon), to show its play time.
func resolveConflict(j *Game, l *Local, srv *Version, remote []byte, after bool) Choice {
	pc := tr("sauvegarde présente", "save found")
	var mod time.Time // the save file, or the newest file of a 3DSA folder
	filepath.WalkDir(j.File, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if i, err := d.Info(); err == nil && i.ModTime().After(mod) {
				mod = i.ModTime()
			}
		}
		return nil
	})
	if !mod.IsZero() {
		pc = fmt.Sprintf(tr("modifiée le %s", "modified on %s"), mod.Format("02/01/2006 15:04"))
	}
	online := fmt.Sprintf(tr("envoyée par %s %s", "uploaded by %s %s"), srv.Client, formatAge(srv.Age))
	if !isArchive(j) { // the Pokémon play time reads a raw /main, never an archive
		if p, ok := pokemonPlayInfo(l.SRAM); ok {
			pc += " · " + p
		}
		if p, ok := pokemonPlayInfo(remote); ok {
			online += " · " + p
		}
	}
	text := fmt.Sprintf(tr("Conflit sur %s\n\n"+
		"La sauvegarde a changé sur ce PC et sur le serveur depuis la dernière synchronisation. Laquelle garder ?\n\n"+
		"PC : %s\nServeur : %s\n\n"+
		"Rien n'est perdu : l'autre version est gardée.",
		"Conflict on %s\n\n"+
			"The save changed on this PC and on the server since the last sync. Which one do you want to keep?\n\n"+
			"PC: %s\nServer: %s\n\n"+
			"Nothing is lost: the other version is kept."),
		j.Name, pc, online)
	server, pc := tr("Prendre celle du serveur", "Use server save"), tr("Garder celle du PC", "Keep PC save")
	noPlay := tr("Ne pas jouer", "Don't play")
	var choice string
	if after {
		choice = ask(text, []string{pc, tr("Ne rien faire", "Do nothing")})
	} else {
		choice = ask(text, []string{server, pc, noPlay})
	}
	switch choice {
	case server:
		return takeServer
	case pc:
		return keepPC
	case noPlay:
		return dontPlay
	case "":
		notify(fmt.Sprintf(tr("Conflit sur %s. Le jeu démarre avec la sauvegarde du PC, rien n'a été remplacé.",
			"Conflict on %s. The game starts with the PC save, nothing was replaced."), j.Name))
	}
	return keepNothing
}

// Choice is the answer of the user to a Conflict.
type Choice int

const (
	keepNothing Choice = iota // "Do nothing", or no window: nothing is replaced
	takeServer
	keepPC
	dontPlay
)

// syncGame runs one Sync pass. cancelled = the user does not want to play.
func (c *Client) syncGame(j *Game, after bool) (cancelled bool, err error) {
	for attempt := 0; attempt < 3; attempt++ { // a 412 starts a new pass with the new Current version
		if attempt > 0 {
			c.listRead = false
		}
		srv, err := c.serverVersion(c.name, j.Key)
		if err != nil {
			return false, err
		}
		if srv != nil && srv.Name == "" && j.Name != "" {
			c.setName(c.name, j)
		}
		l, err := readLocal(j, srv)
		if err != nil {
			return false, err
		}
		// A blank Save counts as absent; the file is still copied before it is overwritten (download).
		var seen *Version
		if l != nil && (!l.Blank || srv != nil && l.Hash == srv.Hash) {
			seen = &l.Version
		}
		if seen == nil && srv == nil {
			return false, nil
		}
		b, err := c.bases()
		if err != nil {
			return false, err
		}
		st := state(seen, srv, b[j.Key])
		if st == conflict && c.batch != nil { // nobody waits in front of a game: the next start of the game asks
			c.batch.conflicts = append(c.batch.conflicts, j.Name)
			return false, nil
		}
		if st == conflict {
			var remote []byte // Pokémon: read the server Version to compare the play times
			if _, ok := pokemonPlayInfo(l.SRAM); ok && !isArchive(j) {
				if st, o, err := c.api("GET", "/saves/"+j.Key, nil, nil); err == nil && st == 200 {
					remote = o
				}
			}
			switch resolveConflict(j, l, srv, remote, after) {
			case takeServer:
				st = toDownload
			case keepPC:
				st = toUpload
			case dontPlay:
				return true, nil
			default:
				return false, nil
			}
		}
		switch st {
		case inSync:
			if c.batch != nil {
				c.batch.inSync++
			}
			return false, c.setBase(j.Key, l.Hash)
		case toUpload:
			if ok, err := c.upload(j, l, srv); ok || err != nil {
				if err == nil && c.batch != nil {
					c.batch.uploaded++
				}
				return false, err
			}
		case toDownload:
			// Content other than the list: a Version arrived after the list was read (long pass), so read again.
			if err := c.download(j, l, srv); !errors.As(err, new(damagedError)) || attempt == 2 {
				if err == nil && c.batch != nil {
					c.batch.downloaded++
				}
				return false, err
			}
		}
	}
	return false, errors.New(tr("un autre appareil envoie cette sauvegarde en même temps. Relancez le jeu dans un moment",
		"another device is uploading this save at the same time. Start the game again in a moment"))
}
