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

// Systèmes (ADR 0007) : extension de ROM -> Système.
var extensions = map[string]string{
	".nds": "DS", ".gb": "GB", ".gbc": "GB", ".gba": "GBA", ".3ds": "3DS", ".cci": "3DS", ".cxi": "3DS",
}

// Les 8 Pokémon 3DS synchronisés (ADR 0009) : un title ID par jeu, toutes régions.
var titles3DS = map[string]string{
	"0004000000055D00": "Pokemon X", "0004000000055E00": "Pokemon Y",
	"000400000011C400": "Pokemon Rubis Omega", "000400000011C500": "Pokemon Saphir Alpha",
	"0004000000164800": "Pokemon Soleil", "0004000000175E00": "Pokemon Lune",
	"00040000001B5000": "Pokemon Ultra-Soleil", "00040000001B5100": "Pokemon Ultra-Lune",
}

// Format de l'archive de sauvegarde d'un jeu 3DS : le 00000001.metadata d'Azahar (ArchiveFormatInfo : total_size,
// nombre de répertoires, nombre de fichiers en u32 LE, duplicate_data, 3 octets de bourrage). Avec lui, le Lanceur crée
// le dossier d'un jeu jamais lancé dans Azahar ; sans lui, il refuse : un jeu sans .metadata peut reformater sa sauvegarde.
// Relevé sur de vraies installations : Lune, Ultra-Soleil (2026-11-01). Les autres Pokémon : même moteur, même valeur
// supposée ; Pokémon X vérifié par le test sur une copie d'Azahar (issue #9).
var formats3DS = func() map[string][]byte {
	pokemon := []byte{0, 0, 4, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0} // 0x40000 octets, 1 répertoire, 1 fichier, dupliqué
	m := map[string][]byte{}
	for id := range titles3DS {
		m[id] = pokemon
	}
	return m
}()

// Jeu : une Clé et le fichier de sauvegarde qui lui correspond chez un émulateur.
type Game struct {
	System string `json:"Systeme"`
	Key    string `json:"Cle"`
	Name   string `json:"Nom"`
	File   string `json:"Fichier"`
	Core   string `json:"Coeur"` // bibliothèque du cœur RetroArch, "" sinon : gpSP et Beetle GBA ont un .srm à convertir (srm.go)
}

var code4 = regexp.MustCompile(`^[A-Z0-9]{4}$`)

// lireRom : Système, Clé et noms possibles de la sauvegarde d'une ROM (fichier ou .zip).
// Système vide = extension inconnue ; Clé vide = hors périmètre (homebrew, 3DS non listé).
// nom : la ROM ou le zip (mGBA, RetroArch) ; interne : le fichier dans le zip (melonDS), "" hors zip.
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
				io.ReadFull(r, header) // une ROM plus courte que l'en-tête garde des zéros
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
		for i, c := range title { // octet par octet comme cle.c : bytes.ToUpper changerait la longueur (octet >= 0x80)
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

// Title ID d'une ROM 3DS : NCSD (.3ds/.cci) -> partition 0 à u32@0x120 × 0x200, ou NCCH direct (.cxi) ;
// program_id à NCCH + 0x118.
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

// Fichier de sauvegarde d'un Jeu dans le dossier d'un émulateur. 3DS : dossier = sdmc d'Azahar.
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

// Version : une Sauvegarde vue du serveur ou du PC (Hash et taille de la SRAM).
type Version struct {
	Code   string `json:"code"`
	Hash   string `json:"hash"`
	Size   int64  `json:"taille"`
	Client string `json:"client"`
	Age    *int64 `json:"age"`
	Name   string `json:"nom"` // nom du jeu donné par un Client (ADR 0018) ; vide sur un serveur plus ancien
}

type Local struct {
	Version
	Raw   []byte
	SRAM  []byte
	Blank bool // SRAM vierge (que des 0x00 ou que des 0xFF), écrite avant toute partie
}

// GB/GBA : Hash, taille et envoi portent sur la SRAM seule ; le Trailer d'horloge reste local (ADR 0008).
// srv : Version courante du serveur ou nil ; elle donne la taille de la SRAM d'un .srm gpSP ou Beetle GBA (ADR 0011).
func readLocal(j *Game, srv *Version) (*Local, error) {
	raw, err := os.ReadFile(j.File)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sram := raw
	r := len(raw) % 512 // Trailer d'horloge : 16, 44 ou 48 octets après la SRAM (ADR 0010)
	if (j.System == "GB" || j.System == "GBA") && len(raw) > r && (r == 16 || r == 44 || r == 48) {
		sram = raw[:len(raw)-r]
	}
	if isMBC2(j.System, len(sram)) { // forme du serveur (mbc2.go)
		sram = normalizeMBC2(sram)
	}
	if strings.EqualFold(filepath.Ext(j.File), ".dsv") { // DeSmuME : pied de page retiré (dsv.go)
		if sram, err = sramDSV(raw); err != nil {
			return nil, fmt.Errorf("%s : %w", j.File, err)
		}
	}
	if fixedSRM[j.Core] > 0 { // gpSP, Beetle GBA : .srm de taille fixe (srm.go)
		if sram, err = sramSRM(j.Core, raw, srv); err != nil {
			return nil, fmt.Errorf("%s : %w", j.File, err)
		}
	}
	return &Local{Version{Hash: sha256Hex(sram), Size: int64(len(sram))}, raw, sram, j.System != "3DS" && blank(sram)}, nil
}

// État d'un Jeu (docs/design.md §1, ADR 0001). nil = absent ; base vide = jamais synchronisé.
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

// Session : un émulateur (profil) et son dossier de configuration (journal, Bases, Copies de sécurité).
type Session struct {
	cfg        *Config
	config     string // chemin de config.json : un alias créé au lancement y est écrit (ADR 0014)
	dir        string
	profile    string
	http       *http.Client
	downloaded bool              // une Version est arrivée du serveur pendant cette Session
	romOf      map[string]string // Clé -> ROM de ce lancement, retenue avec la Base (ADR 0014)
	// Liste GET /saves de la passe en cours (une seule lecture par passe) ; relue après un Envoi ou un 412.
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
	if err != nil { // réponse coupée : le réseau est en cause, comme une requête sans réponse (classer)
		return 0, nil, &neturl.Error{Op: method, URL: path, Err: err}
	}
	return rep.StatusCode, buf, nil
}

// Réponse du serveur autre que celle attendue. Error() garde le détail technique, pour le journal.
type httpError struct {
	request string
	code    int
}

func (e *httpError) Error() string { return fmt.Sprintf("%s : %d", e.request, e.code) }

// Page d'aide des messages d'erreur, ajoutée aux fenêtres d'erreur du PC.
const helpPage = "https://github.com/programgames/3dssavesync/blob/main/docs/troubleshooting.md"

// classer : texte pour l'utilisateur (sans code HTTP ni texte de Go) ; global = le serveur ou le réseau est en cause,
// les autres jeux de la passe échoueraient pareil. Erreur inconnue : son texte, déjà écrit pour l'utilisateur.
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

// texteErreur : l'erreur telle que l'utilisateur la lit ("" sans erreur).
func errorText(err error) string {
	t, _ := classify(err)
	return t
}

// Nom de Client pour le serveur ([a-z0-9]{1,16}) : deux émulateurs = deux Clients (Q17).
func clientName(profile string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(profile) {
		if (c >= 'a' && c <= 'z' || c >= '0' && c <= '9') && b.Len() < 16 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Version courante d'un Jeu sur le serveur, ou nil. La liste est lue une fois par passe : une erreur aussi (réseau
// coupé : un seul délai d'attente pour toute la passe).
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
	if err := json.Unmarshal(o, &list); err != nil { // page d'un portail Wi-Fi, mauvaise adresse : réponse inattendue
		s.log("GET /saves illisible : " + err.Error())
		return nil, &httpError{"GET /saves", st}
	}
	return list, nil
}

// Bases : un fichier par émulateur (Q17), Clé -> Hash de la dernière Synchronisation.
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

// ecrireJSON : fichier temporaire vérifié puis remplacement (remplacer=false : jamais d'écrasement).
func writeJSON(path string, v any, replace bool) error {
	o, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if !replace {
		return writeVerified(path, o)
	}
	tmp := path + ".tmp"
	os.Remove(tmp)
	if err := writeVerified(tmp, o); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ecrireVerifie : crée le fichier (jamais d'écrasement), l'écrit sur le disque, puis le relit.
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

// Envoi conditionnel : If-Match = Version courante vue par ce Client. false sur 412.
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
	if srv == nil && j.Name != "" { // premier Envoi de ce jeu : le serveur n'a pas encore son nom (ADR 0018)
		s.setName(j)
	}
	s.listRead = false // la liste n'a plus la Version courante de ce Jeu
	return true, s.setBase(j.Key, l.Hash)
}

// Copie de sécurité de la Sauvegarde locale : <copies>/<émulateur>/<nom>-<date>.
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

// Récupération : Copie de sécurité, fichier temporaire vérifié, puis remplacement (ADR 0004, design §3).
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
	dir := filepath.Dir(j.File)
	tmp := j.File + ".3dssavesync-tmp"
	if j.System == "3DS" {
		// Pas de dossier = Azahar n'a jamais vu ce jeu. Créé seulement avec un .metadata connu (formats3DS) et jamais
		// par-dessus un .metadata existant ; sinon refus (ADR 0009).
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
		tmp = filepath.Join(filepath.Dir(dir), "main.3dssavesync-tmp") // l'archive ne doit contenir que main
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := o
	if l != nil {
		if err := s.copy(j, l.Raw); err != nil {
			return fmt.Errorf(tr("impossible de faire la copie de sécurité : %w", "couldn't make a backup: %w"), err)
		}
		if len(l.Raw) > len(l.SRAM) { // GB/GBA : on remet le Trailer d'horloge de ce PC après la SRAM reçue
			content = append(append([]byte{}, o...), l.Raw[len(l.SRAM):]...)
		}
	}
	if strings.EqualFold(filepath.Ext(j.File), ".dsv") { // DeSmuME : SRAM reçue + pied de page (dsv.go)
		var old []byte
		if l != nil {
			old = l.Raw
		}
		if content, err = dsv(o, old); err != nil {
			return err
		}
	}
	if isMBC2(j.System, len(o)) { // MBC2 : à la taille du fichier existant, sinon à celle de l'émulateur (mbc2.go)
		n := s.mbc2Size(j)
		if l != nil && isMBC2(j.System, len(l.Raw)) {
			n = len(l.Raw)
		}
		content = mbc2File(normalizeMBC2(o), n)
	}
	if fixedSRM[j.Core] > 0 { // gpSP, Beetle GBA : SRAM reçue dans un .srm de taille fixe (srm.go)
		if content, err = srm(j.Core, o); err != nil {
			return err
		}
	}
	os.Remove(tmp) // reste d'un arrêt brutal
	if err := writeVerified(tmp, content); err != nil {
		return err
	}
	if err := os.Rename(tmp, j.File); err != nil {
		os.Remove(tmp)
		return err
	}
	if reread, err := os.ReadFile(j.File); err != nil || !bytes.Equal(reread, content) {
		return fmt.Errorf(tr("%s : le fichier remplacé ne correspond pas", "%s: the replaced file doesn't match"), j.File)
	}
	s.downloaded = true
	return s.setBase(j.Key, srv.Hash)
}

// Contenu reçu dont le Hash n'est pas celui de la liste.
type damagedError string

func (e damagedError) Error() string { return string(e) }

// « il y a 2 h » à partir de l'âge en secondes donné par le serveur.
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

// Choix de l'utilisateur sur un Conflit (Q5). Sans fenêtre possible (Q16) : rien n'est remplacé.
// distant : contenu de la Version du serveur s'il a été lu (Pokémon 3DS), pour montrer son temps de jeu.
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

// nommer : donne au serveur le nom du jeu, pour l'Historique (ADR 0018). Un échec ne gêne pas la Synchronisation : il
// est noté, et le nom est noté comme connu pour ne pas réessayer pendant cette session.
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

// Une passe de Synchronisation. annule = l'utilisateur ne veut pas jouer.
func (s *Session) syncGame(j *Game, after bool) (cancelled bool, err error) {
	for attempt := 0; attempt < 3; attempt++ { // un 412 relance une passe avec la nouvelle Version courante
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
		// Une Sauvegarde vierge compte comme absente ; le fichier reste copié avant écrasement (recuperer).
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
			var remote []byte // Pokémon : la Version du serveur est lue pour comparer les temps de jeu
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
			// Contenu autre que la liste : une Version est arrivée depuis la lecture de la liste (passe longue), on relit.
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
