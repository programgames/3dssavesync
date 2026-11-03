package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// extensions maps a ROM extension to its System (ADR 0007).
var extensions = map[string]string{
	".nds": "DS", ".gb": "GB", ".gbc": "GB", ".gba": "GBA", ".3ds": "3DS", ".cci": "3DS", ".cxi": "3DS",
}

// titles3DS lists the 8 synced 3DS Pokémon games (ADR 0009): one title ID per game, all regions.
var titles3DS = map[string]string{
	"0004000000055D00": "Pokemon X", "0004000000055E00": "Pokemon Y",
	"000400000011C400": "Pokemon Rubis Omega", "000400000011C500": "Pokemon Saphir Alpha",
	"0004000000164800": "Pokemon Soleil", "0004000000175E00": "Pokemon Lune",
	"00040000001B5000": "Pokemon Ultra-Soleil", "00040000001B5100": "Pokemon Ultra-Lune",
}

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
	System string `json:"Systeme"`
	Key    string `json:"Cle"`
	Name   string `json:"Nom"`
	File   string `json:"Fichier"`
	Core   string `json:"Coeur"` // RetroArch core library or "": gpSP and Beetle GBA have a .srm to convert (srm.go)
}

var code4 = regexp.MustCompile(`^[A-Z0-9]{4}$`)

// readRom returns the System, the Key and the possible save names of a ROM (file or .zip).
// Empty System = unknown extension; empty Key = out of scope (homebrew, unlisted 3DS game).
// name: the ROM or the zip (mGBA, RetroArch); internal: the file inside the zip (melonDS), "" outside a zip.
func readRom(rom string) (system, key, name, internal string, err error) {
	ext := strings.ToLower(filepath.Ext(rom))
	name = strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom))
	if ext != ".zip" && extensions[ext] == "" {
		return "", "", name, "", nil
	}
	header := make([]byte, 0x200)
	if ext == ".zip" {
		z, err := zip.OpenReader(rom)
		if err != nil {
			return "", "", name, "", err
		}
		defer z.Close()
		for _, f := range z.File {
			e := strings.ToLower(path.Ext(f.Name))
			if s := extensions[e]; s == "DS" || s == "GB" || s == "GBA" {
				r, err := f.Open()
				if err != nil {
					return "", "", name, "", err
				}
				io.ReadFull(r, header) // a ROM shorter than the header keeps zeros
				r.Close()
				ext = e
				internal = strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name))
				break
			}
		}
	} else {
		f, err := os.Open(rom)
		if err != nil {
			return "", "", name, "", err
		}
		io.ReadFull(f, header)
		f.Close()
	}
	system = extensions[ext]
	switch system {
	case "DS":
		if c := string(header[0x0C:0x10]); code4.MatchString(c) {
			key = c
		}
	case "GBA":
		if c := string(header[0xAC:0xB0]); code4.MatchString(c) {
			key = "GBA-" + c
		}
	case "GB":
		title := header[0x134:0x143]
		if i := bytes.IndexByte(title, 0); i >= 0 {
			title = title[:i]
		}
		clean := make([]byte, len(title))
		for i, c := range title { // byte by byte like key.c: bytes.ToUpper would change the length (byte >= 0x80)
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				c = '_'
			}
			clean[i] = c
		}
		if len(clean) > 0 {
			key = fmt.Sprintf("GB-%s-%02X%02X", clean, header[0x14E], header[0x14F])
		}
	case "3DS":
		id, err := titleID3DS(rom)
		if err != nil {
			return "", "", name, "", err
		}
		if t, ok := titles3DS[id]; ok {
			key, name = "3DS-"+id, t
		}
	}
	return system, key, name, internal, nil
}

// titleID3DS returns the title ID of a 3DS ROM: NCSD (.3ds/.cci) -> partition 0 at u32@0x120 × 0x200, or direct
// NCCH (.cxi); program_id at NCCH + 0x118.
func titleID3DS(rom string) (string, error) {
	f, err := os.Open(rom)
	if err != nil {
		return "", err
	}
	defer f.Close()
	o := make([]byte, 0x200)
	io.ReadFull(f, o)
	var ncch int64
	switch string(o[0x100:0x104]) {
	case "NCSD":
		ncch = int64(binary.LittleEndian.Uint32(o[0x120:])) * 0x200
	case "NCCH":
	default:
		return "", nil
	}
	id := make([]byte, 8)
	if _, err := f.ReadAt(id, ncch+0x118); err != nil {
		return "", nil
	}
	return fmt.Sprintf("%016X", binary.LittleEndian.Uint64(id)), nil
}

// savePath returns the save file of a Game in an emulator's folder. 3DS: folder = Azahar's sdmc.
func savePath(dir, system, key, name, ext string) string {
	if system == "3DS" {
		id, z := strings.ToLower(key[4:]), strings.Repeat("0", 32)
		return filepath.Join(dir, "Nintendo 3DS", z, z, "title", id[:8], id[8:], "data", "00000001", "main")
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

// readLocal reads the Local save. GB/GBA: Hash, size and Upload cover the SRAM only; the Clock trailer stays local
// (ADR 0008). srv: Current version of the server or nil; it gives the SRAM size of a gpSP or Beetle GBA .srm
// (ADR 0011).
func readLocal(j *Game, srv *Version) (*Local, error) {
	raw, err := os.ReadFile(j.File)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sram := raw
	r := len(raw) % 512 // Clock trailer: 16, 44 or 48 bytes after the SRAM (ADR 0010)
	if (j.System == "GB" || j.System == "GBA") && len(raw) > r && (r == 16 || r == 44 || r == 48) {
		sram = raw[:len(raw)-r]
	}
	if isMBC2(j.System, len(sram)) { // server form (mbc2.go)
		sram = normalizeMBC2(sram)
	}
	if strings.EqualFold(filepath.Ext(j.File), ".dsv") { // DeSmuME: footer removed (dsv.go)
		if sram, err = sramDSV(raw); err != nil {
			return nil, fmt.Errorf("%s : %w", j.File, err)
		}
	}
	if fixedSRM[j.Core] > 0 { // gpSP, Beetle GBA: fixed-size .srm (srm.go)
		if sram, err = sramSRM(j.Core, raw, srv); err != nil {
			return nil, fmt.Errorf("%s : %w", j.File, err)
		}
	}
	return &Local{Version{Hash: sha256Hex(sram), Size: int64(len(sram))}, raw, sram, j.System != "3DS" && blank(sram)}, nil
}

// State of a Game (docs/design.md §1, ADR 0001). nil = absent; empty base = never synced.
const (
	inSync     = "A_JOUR"
	toUpload   = "A_ENVOYER"
	toDownload = "A_RECUPERER"
	conflict   = "CONFLIT"
	force      = "FORCER"
	nothing    = "RIEN"
	cancel     = "ANNULER"
)

func state(local, server *Version, base string) string {
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

// Session is an emulator (Profile) and its configuration folder (log, Bases, Backup copies).
type Session struct {
	cfg        *Config
	config     string // path of config.json: an Alias created at launch is written there (ADR 0014)
	dir        string
	profile    string
	http       *http.Client
	downloaded bool              // a Version arrived from the server during this Session
	romOf      map[string]string // Key -> ROM of this launch, kept with the Base (ADR 0014)
	// GET /saves list of the current pass (read once per pass); read again after an Upload or a 412.
	list     []Version
	listErr  error
	listRead bool
}

func (s *Session) log(text string) {
	f, err := os.OpenFile(filepath.Join(s.dir, "3dssavesync.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02T15:04:05"), text)
		f.Close()
	}
}

func (s *Session) api(method, path string, body []byte, headers map[string]string) (int, []byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, strings.TrimRight(s.cfg.URL, "/")+path, r)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	req.Header.Set("X-Client", clientName(s.profile))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rep, err := s.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer rep.Body.Close()
	if rep.StatusCode == http.StatusUnauthorized {
		return 0, nil, &httpError{method + " " + path, 401}
	}
	buf, err := io.ReadAll(rep.Body)
	if err != nil { // cut-off response: the network is at fault, like a request with no response (classify)
		return 0, nil, &neturl.Error{Op: method, URL: path, Err: err}
	}
	return rep.StatusCode, buf, nil
}

// httpError is a server response other than the expected one. Error() keeps the technical detail, for the log.
type httpError struct {
	request string
	code    int
}

func (e *httpError) Error() string { return fmt.Sprintf("%s : %d", e.request, e.code) }

// helpPage is the help page for error messages, added to the error windows of the PC.
const helpPage = "https://github.com/programgames/3dssavesync/blob/main/docs/troubleshooting.md"

// classify returns a text for the user (no HTTP code, no Go text); global = the server or the network is at fault,
// so the other games of the pass would fail the same way. Unknown error: its own text, already written for the user.
func classify(err error) (text string, global bool) {
	var h *httpError
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var header tls.RecordHeaderError
	var network net.Error
	var ue *neturl.Error
	var errno syscall.Errno
	var path *fs.PathError
	errors.As(err, &errno)
	windows := runtime.GOOS == "windows"
	switch {
	case err == nil:
		return "", false
	case errors.As(err, &h) && h.code == 401:
		return tr("Le serveur refuse ce jeton. Recopiez le jeton exact du serveur.",
			"The server rejected this token. Copy the exact token from the server."), true
	case errors.As(err, &h) && h.code == 404:
		return tr("Cette adresse ne correspond pas à un serveur 3dssavesync. Vérifiez-la.",
			"This address isn't a 3dssavesync server. Check it."), true
	case errors.As(err, &h) && h.code == 413:
		return tr("sauvegarde trop grosse pour le serveur : augmentez la taille maximale acceptée par le serveur ou par son proxy",
			"save too large for the server: raise the size limit on the server or its proxy"), false
	case errors.As(err, &h) && h.code == 400:
		return tr("le serveur a refusé cette sauvegarde (vide, trop grosse ou envoi interrompu). Relancez le jeu",
			"the server rejected this save (empty, too large or cut off). Start the game again"), false
	case errors.As(err, &h) && h.code >= 500:
		return tr("Le serveur a rencontré une erreur. Réessayez plus tard. Si le problème continue, consultez le journal du serveur.",
			"The server ran into an error. Try again later. If it keeps happening, check the server log."), true
	case errors.As(err, &h):
		return tr("Le serveur a répondu de façon inattendue. Vérifiez son adresse.", "The server sent an unexpected response. Check its address."), true
	case errors.As(err, &cert) || errors.As(err, &header):
		return tr("Le certificat du serveur n'est pas valide. Vérifiez l'adresse (https://) et la date du PC.",
			"The server's certificate isn't valid. Check the address (https://) and the PC's date."), true
	case errors.As(err, &dns):
		return tr("Serveur introuvable. Vérifiez votre connexion Internet et l'adresse du serveur.",
			"Can't find the server. Check your Internet connection and the server address."), true
	case errors.Is(err, syscall.ECONNREFUSED) || windows && errno == 10061: // WSAECONNREFUSED
		return tr("Le serveur refuse la connexion. Il est peut-être arrêté, ou le port est incorrect : démarrez le serveur ou vérifiez l'adresse.",
			"The server refused the connection. It may be stopped, or the port is wrong. Start the server or check the address."), true
	case errors.As(err, &network) && network.Timeout(), errors.As(err, &ue):
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

// clientName returns the Client name for the server ([a-z0-9]{1,16}): two emulators = two Clients.
func clientName(profile string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(profile) {
		if (c >= 'a' && c <= 'z' || c >= '0' && c <= '9') && b.Len() < 16 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// serverVersion returns the Current version of a Game on the server, or nil. The list is read once per pass, and so is
// an error (network down: a single timeout for the whole pass).
func (s *Session) serverVersion(key string) (*Version, error) {
	if !s.listRead {
		s.list, s.listErr = s.readList()
		s.listRead = true
	}
	if s.listErr != nil {
		return nil, s.listErr
	}
	for i := range s.list {
		if s.list[i].Code == key {
			v := s.list[i]
			return &v, nil
		}
	}
	return nil, nil
}

func (s *Session) readList() ([]Version, error) {
	st, o, err := s.api("GET", "/saves", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, &httpError{"GET /saves", st}
	}
	var list []Version
	if err := json.Unmarshal(o, &list); err != nil { // Wi-Fi portal page, wrong address: unexpected response
		s.log("GET /saves illisible : " + err.Error())
		return nil, &httpError{"GET /saves", st}
	}
	return list, nil
}

// basePath returns the Bases file: one file per emulator, Key -> Hash of the last Sync.
func (s *Session) basePath() string { return filepath.Join(s.dir, "bases", s.profile+".json") }

func (s *Session) bases() (map[string]string, error) {
	m := map[string]string{}
	o, err := os.ReadFile(s.basePath())
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(bytes.TrimPrefix(o, []byte("\xef\xbb\xbf")), &m)
}

func (s *Session) setBase(key, hash string) error {
	m, err := s.bases()
	if err != nil {
		return err
	}
	m[key] = hash
	if err := writeJSON(s.basePath(), m, true); err != nil {
		return err
	}
	if err := s.keepRom(key); err != nil {
		s.log("ROM de " + key + " non retenue : " + err.Error())
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
	tmp := filepath.Join(dir, filepath.Base(path)+tmpSuffix)
	os.Remove(tmp) // leftover of a hard stop
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
func (s *Session) upload(j *Game, l *Local, srv *Version) (bool, error) {
	cond := map[string]string{"If-None-Match": "*"}
	if srv != nil {
		cond = map[string]string{"If-Match": `"` + srv.Hash + `"`}
	}
	st, _, err := s.api("PUT", "/saves/"+j.Key, l.SRAM, cond)
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
		s.setName(j)
	}
	s.listRead = false // the list no longer has the Current version of this Game
	return true, s.setBase(j.Key, l.Hash)
}

// copy makes a Backup copy of the Local save: <copies>/<emulator>/<name>-<date>.
func (s *Session) copy(j *Game, raw []byte) error {
	dir := filepath.Join(s.cfg.copiesDir(s.dir), s.profile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ext := filepath.Ext(j.File)
	base := j.Name + time.Now().Format("-20060102-150405")
	for i := 0; ; i++ {
		name := base + ext
		if i > 0 {
			name = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		err := writeVerified(filepath.Join(dir, name), raw)
		if !errors.Is(err, fs.ErrExist) || i == 9 {
			return err
		}
	}
}

// download is a Download: Backup copy, verified temporary file, then replacement (ADR 0004, docs/design.md §3).
func (s *Session) download(j *Game, l *Local, srv *Version) error {
	st, o, err := s.api("GET", "/saves/"+j.Key, nil, nil)
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
	dir, tmpDir := filepath.Dir(j.File), filepath.Dir(j.File)
	if j.System == "3DS" {
		// No folder = Azahar never saw this game. Create it only with a known .metadata (formats3DS) and never
		// over an existing .metadata; otherwise refuse (ADR 0009).
		if _, err := os.Stat(dir); err != nil {
			format, meta := formats3DS[strings.ToUpper(strings.TrimPrefix(j.Key, "3DS-"))], filepath.Join(filepath.Dir(dir), "00000001.metadata")
			if format == nil || exists(meta) {
				return errors.New(tr("ce jeu n'a encore jamais été lancé dans Azahar. Lancez-le une fois dans Azahar, sauvegardez dans le jeu et relancez-le ici",
					"this game has never been started in Azahar. Start it once in Azahar, save in the game, then start it here again"))
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
	content := o
	if l != nil {
		if err := s.copy(j, l.Raw); err != nil {
			return fmt.Errorf(tr("impossible de faire la copie de sécurité : %w", "couldn't make a backup: %w"), err)
		}
		if len(l.Raw) > len(l.SRAM) { // GB/GBA: put the Clock trailer of this PC back after the received SRAM
			content = append(append([]byte{}, o...), l.Raw[len(l.SRAM):]...)
		}
	}
	if strings.EqualFold(filepath.Ext(j.File), ".dsv") { // DeSmuME: received SRAM + footer (dsv.go)
		var old []byte
		if l != nil {
			old = l.Raw
		}
		if content, err = dsv(o, old); err != nil {
			return err
		}
	}
	if isMBC2(j.System, len(o)) { // MBC2: at the size of the existing file, otherwise at the emulator size (mbc2.go)
		n := s.mbc2Size(j)
		if l != nil && isMBC2(j.System, len(l.Raw)) {
			n = len(l.Raw)
		}
		content = mbc2File(normalizeMBC2(o), n)
	}
	if fixedSRM[j.Core] > 0 { // gpSP, Beetle GBA: received SRAM inside a fixed-size .srm (srm.go)
		if content, err = srm(j.Core, o); err != nil {
			return err
		}
	}
	if err := writeAtomicIn(tmpDir, j.File, content); err != nil {
		return err
	}
	if reread, err := os.ReadFile(j.File); err != nil || !bytes.Equal(reread, content) {
		return fmt.Errorf(tr("%s : le fichier remplacé ne correspond pas", "%s: the replaced file doesn't match"), j.File)
	}
	s.downloaded = true
	return s.setBase(j.Key, srv.Hash)
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
func resolveConflict(j *Game, l *Local, srv *Version, remote []byte, after bool) string {
	pc := tr("sauvegarde présente", "save found")
	if i, err := os.Stat(j.File); err == nil {
		pc = fmt.Sprintf(tr("modifiée le %s", "modified on %s"), i.ModTime().Format("02/01/2006 15:04"))
	}
	if p, ok := pokemonPlayInfo(l.SRAM); ok {
		pc += " · " + p
	}
	online := fmt.Sprintf(tr("envoyée par %s %s", "uploaded by %s %s"), srv.Client, formatAge(srv.Age))
	if p, ok := pokemonPlayInfo(remote); ok {
		online += " · " + p
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
		return toDownload
	case pc:
		return force
	case noPlay:
		return cancel
	case "":
		notify(fmt.Sprintf(tr("Conflit sur %s. Le jeu démarre avec la sauvegarde du PC, rien n'a été remplacé.",
			"Conflict on %s. The game starts with the PC save, nothing was replaced."), j.Name))
	}
	return nothing
}

// setName gives the game name to the server, for the History (ADR 0018). A failure does not block the Sync: it is
// logged, and the name is marked as known so that this session does not try again.
func (s *Session) setName(j *Game) {
	if st, _, err := s.api("PUT", "/saves/"+j.Key+"/name", []byte(j.Name), nil); err != nil || st != 204 {
		s.log(fmt.Sprintf("nom de %s non envoyé : %d %v", j.Key, st, err))
	}
	for i := range s.list {
		if s.list[i].Code == j.Key {
			s.list[i].Name = j.Name
		}
	}
}

// syncGame runs one Sync pass. cancelled = the user does not want to play.
func (s *Session) syncGame(j *Game, after bool) (cancelled bool, err error) {
	for attempt := 0; attempt < 3; attempt++ { // a 412 starts a new pass with the new Current version
		if attempt > 0 {
			s.listRead = false
		}
		srv, err := s.serverVersion(j.Key)
		if err != nil {
			return false, err
		}
		if srv != nil && srv.Name == "" && j.Name != "" {
			s.setName(j)
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
		b, err := s.bases()
		if err != nil {
			return false, err
		}
		action := state(seen, srv, b[j.Key])
		if action == conflict {
			var remote []byte // Pokémon: read the server Version to compare the play times
			if _, ok := pokemonPlayInfo(l.SRAM); ok {
				if st, o, err := s.api("GET", "/saves/"+j.Key, nil, nil); err == nil && st == 200 {
					remote = o
				}
			}
			action = resolveConflict(j, l, srv, remote, after)
		}
		switch action {
		case inSync:
			return false, s.setBase(j.Key, l.Hash)
		case toUpload, force:
			if ok, err := s.upload(j, l, srv); ok || err != nil {
				return false, err
			}
		case toDownload:
			// Content other than the list: a Version arrived after the list was read (long pass), so read again.
			if err := s.download(j, l, srv); !errors.As(err, new(damagedError)) || attempt == 2 {
				return false, err
			}
		case cancel:
			return true, nil
		default:
			return false, nil
		}
	}
	return false, errors.New(tr("un autre appareil envoie cette sauvegarde en même temps. Relancez le jeu dans un moment",
		"another device is uploading this save at the same time. Start the game again in a moment"))
}
