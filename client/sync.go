package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Systèmes (ADR 0007) : extension de ROM -> Système.
var extensions = map[string]string{
	".nds": "DS", ".gb": "GB", ".gbc": "GB", ".gba": "GBA", ".3ds": "3DS", ".cci": "3DS", ".cxi": "3DS",
}

// Les 8 Pokémon 3DS synchronisés (ADR 0009) : un title ID par jeu, toutes régions.
var titres3DS = map[string]string{
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
	for id := range titres3DS {
		m[id] = pokemon
	}
	return m
}()

// Jeu : une Clé et le fichier de sauvegarde qui lui correspond chez un émulateur.
type Jeu struct {
	Systeme, Cle, Nom string
	Fichier           string
	Coeur             string // bibliothèque du cœur RetroArch, "" sinon : gpSP et Beetle GBA ont un .srm à convertir (srm.go)
}

var code4 = regexp.MustCompile(`^[A-Z0-9]{4}$`)

// lireRom : Système, Clé et noms possibles de la sauvegarde d'une ROM (fichier ou .zip).
// Système vide = extension inconnue ; Clé vide = hors périmètre (homebrew, 3DS non listé).
// nom : la ROM ou le zip (mGBA, RetroArch) ; interne : le fichier dans le zip (melonDS), "" hors zip.
func lireRom(rom string) (systeme, cle, nom, interne string, err error) {
	ext := strings.ToLower(filepath.Ext(rom))
	nom = strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom))
	if ext != ".zip" && extensions[ext] == "" {
		return "", "", nom, "", nil
	}
	entete := make([]byte, 0x200)
	if ext == ".zip" {
		z, err := zip.OpenReader(rom)
		if err != nil {
			return "", "", nom, "", err
		}
		defer z.Close()
		for _, f := range z.File {
			e := strings.ToLower(path.Ext(f.Name))
			if s := extensions[e]; s == "DS" || s == "GB" || s == "GBA" {
				r, err := f.Open()
				if err != nil {
					return "", "", nom, "", err
				}
				io.ReadFull(r, entete) // une ROM plus courte que l'en-tête garde des zéros
				r.Close()
				ext = e
				interne = strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name))
				break
			}
		}
	} else {
		f, err := os.Open(rom)
		if err != nil {
			return "", "", nom, "", err
		}
		io.ReadFull(f, entete)
		f.Close()
	}
	systeme = extensions[ext]
	switch systeme {
	case "DS":
		if c := string(entete[0x0C:0x10]); code4.MatchString(c) {
			cle = c
		}
	case "GBA":
		if c := string(entete[0xAC:0xB0]); code4.MatchString(c) {
			cle = "GBA-" + c
		}
	case "GB":
		titre := entete[0x134:0x143]
		if i := bytes.IndexByte(titre, 0); i >= 0 {
			titre = titre[:i]
		}
		propre := make([]byte, len(titre))
		for i, c := range titre { // octet par octet comme cle.c : bytes.ToUpper changerait la longueur (octet >= 0x80)
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				c = '_'
			}
			propre[i] = c
		}
		if len(propre) > 0 {
			cle = fmt.Sprintf("GB-%s-%02X%02X", propre, entete[0x14E], entete[0x14F])
		}
	case "3DS":
		id, err := titleID3DS(rom)
		if err != nil {
			return "", "", nom, "", err
		}
		if t, ok := titres3DS[id]; ok {
			cle, nom = "3DS-"+id, t
		}
	}
	return systeme, cle, nom, interne, nil
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
func fichierSauvegarde(dossier, systeme, cle, nom, ext string) string {
	if systeme == "3DS" {
		id, z := strings.ToLower(cle[4:]), strings.Repeat("0", 32)
		return filepath.Join(dossier, "Nintendo 3DS", z, z, "title", id[:8], id[8:], "data", "00000001", "main")
	}
	if ext == "" {
		ext = ".sav"
	}
	return filepath.Join(dossier, nom+ext)
}

func hachage(o []byte) string {
	h := sha256.Sum256(o)
	return hex.EncodeToString(h[:])
}

// Version : une Sauvegarde vue du serveur ou du PC (Hash et taille de la SRAM).
type Version struct {
	Code   string `json:"code"`
	Hash   string `json:"hash"`
	Taille int64  `json:"taille"`
	Client string `json:"client"`
	Age    *int64 `json:"age"`
}

type Local struct {
	Version
	Brut, SRAM []byte
	Vide       bool // SRAM vierge (que des 0x00 ou que des 0xFF), écrite avant toute partie
}

// GB/GBA : Hash, taille et envoi portent sur la SRAM seule ; le Trailer d'horloge reste local (ADR 0008).
// srv : Version courante du serveur ou nil ; elle donne la taille de la SRAM d'un .srm gpSP ou Beetle GBA (ADR 0011).
func lireLocal(j *Jeu, srv *Version) (*Local, error) {
	brut, err := os.ReadFile(j.Fichier)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sram := brut
	r := len(brut) % 512 // Trailer d'horloge : 16, 44 ou 48 octets après la SRAM (ADR 0010)
	if (j.Systeme == "GB" || j.Systeme == "GBA") && len(brut) > r && (r == 16 || r == 44 || r == 48) {
		sram = brut[:len(brut)-r]
	}
	if estMBC2(j.Systeme, len(sram)) { // forme du serveur (mbc2.go)
		sram = normaliserMBC2(sram)
	}
	if strings.EqualFold(filepath.Ext(j.Fichier), ".dsv") { // DeSmuME : pied de page retiré (dsv.go)
		if sram, err = sramDSV(brut); err != nil {
			return nil, fmt.Errorf("%s : %w", j.Fichier, err)
		}
	}
	if srmFixes[j.Coeur] > 0 { // gpSP, Beetle GBA : .srm de taille fixe (srm.go)
		if sram, err = sramSRM(j.Coeur, brut, srv); err != nil {
			return nil, fmt.Errorf("%s : %w", j.Fichier, err)
		}
	}
	return &Local{Version{Hash: hachage(sram), Taille: int64(len(sram))}, brut, sram, j.Systeme != "3DS" && vierge(sram)}, nil
}

// État d'un Jeu (docs/design.md §1, ADR 0001). nil = absent ; base vide = jamais synchronisé.
const (
	aJour      = "A_JOUR"
	aEnvoyer   = "A_ENVOYER"
	aRecuperer = "A_RECUPERER"
	conflit    = "CONFLIT"
	forcer     = "FORCER"
	rien       = "RIEN"
	annuler    = "ANNULER"
)

func etat(local, serveur *Version, base string) string {
	switch {
	case local != nil && serveur != nil && local.Hash == serveur.Hash:
		return aJour
	case serveur == nil:
		return aEnvoyer
	case local == nil:
		return aRecuperer
	case local.Taille != serveur.Taille:
		return conflit
	case local.Hash == base:
		return aRecuperer
	case serveur.Hash == base:
		return aEnvoyer
	}
	return conflit
}

// Session : un émulateur (profil) et son dossier de configuration (journal, Bases, Copies de sécurité).
type Session struct {
	cfg      *Config
	dossier  string
	profil   string
	http     *http.Client
	recupere bool // une Version est arrivée du serveur pendant cette Session
}

func (s *Session) journal(texte string) {
	f, err := os.OpenFile(filepath.Join(s.dossier, "3dssavesync.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02T15:04:05"), texte)
		f.Close()
	}
}

func (s *Session) api(methode, chemin string, corps []byte, entetes map[string]string) (int, []byte, error) {
	var r io.Reader
	if corps != nil {
		r = bytes.NewReader(corps)
	}
	req, err := http.NewRequest(methode, strings.TrimRight(s.cfg.URL, "/")+chemin, r)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.Jeton)
	req.Header.Set("X-Client", nomClient(s.profil))
	for k, v := range entetes {
		req.Header.Set(k, v)
	}
	rep, err := s.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer rep.Body.Close()
	if rep.StatusCode == http.StatusUnauthorized {
		return 0, nil, errors.New(tr("jeton refusé par le serveur (401)", "token refused by the server (401)"))
	}
	octets, err := io.ReadAll(rep.Body)
	return rep.StatusCode, octets, err
}

// Nom de Client pour le serveur ([a-z0-9]{1,16}) : deux émulateurs = deux Clients (Q17).
func nomClient(profil string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(profil) {
		if (c >= 'a' && c <= 'z' || c >= '0' && c <= '9') && b.Len() < 16 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Version courante d'un Jeu sur le serveur, ou nil.
func (s *Session) versionServeur(cle string) (*Version, error) {
	st, o, err := s.api("GET", "/saves", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, fmt.Errorf("GET /saves : %d", st)
	}
	var liste []Version
	if err := json.Unmarshal(o, &liste); err != nil {
		return nil, err
	}
	for i := range liste {
		if liste[i].Code == cle {
			return &liste[i], nil
		}
	}
	return nil, nil
}

// Bases : un fichier par émulateur (Q17), Clé -> Hash de la dernière Synchronisation.
func (s *Session) cheminBase() string { return filepath.Join(s.dossier, "bases", s.profil+".json") }

func (s *Session) bases() (map[string]string, error) {
	m := map[string]string{}
	o, err := os.ReadFile(s.cheminBase())
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(bytes.TrimPrefix(o, []byte("\xef\xbb\xbf")), &m)
}

func (s *Session) fixerBase(cle, hash string) error {
	m, err := s.bases()
	if err != nil {
		return err
	}
	m[cle] = hash
	return ecrireJSON(s.cheminBase(), m, true)
}

// ecrireJSON : fichier temporaire vérifié puis remplacement (remplacer=false : jamais d'écrasement).
func ecrireJSON(chemin string, v any, remplacer bool) error {
	o, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(chemin), 0o755); err != nil {
		return err
	}
	if !remplacer {
		return ecrireVerifie(chemin, o)
	}
	tmp := chemin + ".tmp"
	os.Remove(tmp)
	if err := ecrireVerifie(tmp, o); err != nil {
		return err
	}
	return os.Rename(tmp, chemin)
}

// ecrireVerifie : crée le fichier (jamais d'écrasement), l'écrit sur le disque, puis le relit.
func ecrireVerifie(chemin string, octets []byte) error {
	f, err := os.OpenFile(chemin, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(octets)
	if e := f.Sync(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil {
		if relu, e := os.ReadFile(chemin); e != nil {
			err = e
		} else if !bytes.Equal(relu, octets) {
			err = fmt.Errorf(tr("%s : relecture différente", "%s: read-back differs"), chemin)
		}
	}
	if err != nil {
		os.Remove(chemin)
	}
	return err
}

// Envoi conditionnel : If-Match = Version courante vue par ce Client. false sur 412.
func (s *Session) envoyer(j *Jeu, l *Local, srv *Version) (bool, error) {
	cond := map[string]string{"If-None-Match": "*"}
	if srv != nil {
		cond = map[string]string{"If-Match": `"` + srv.Hash + `"`}
	}
	st, _, err := s.api("PUT", "/saves/"+j.Cle, l.SRAM, cond)
	if err != nil {
		return false, err
	}
	if st == 412 {
		return false, nil
	}
	if st != 200 {
		return false, fmt.Errorf("PUT : %d", st)
	}
	return true, s.fixerBase(j.Cle, l.Hash)
}

// Copie de sécurité de la Sauvegarde locale : <copies>/<émulateur>/<nom>-<date>.
func (s *Session) copier(j *Jeu, brut []byte) error {
	dossier := filepath.Join(s.cfg.dossierCopies(s.dossier), s.profil)
	if err := os.MkdirAll(dossier, 0o755); err != nil {
		return err
	}
	ext := filepath.Ext(j.Fichier)
	base := j.Nom + time.Now().Format("-20060102-150405")
	for i := 0; ; i++ {
		nom := base + ext
		if i > 0 {
			nom = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		err := ecrireVerifie(filepath.Join(dossier, nom), brut)
		if !errors.Is(err, fs.ErrExist) || i == 9 {
			return err
		}
	}
}

// Récupération : Copie de sécurité, fichier temporaire vérifié, puis remplacement (ADR 0004, design §3).
func (s *Session) recuperer(j *Jeu, l *Local, srv *Version) error {
	st, o, err := s.api("GET", "/saves/"+j.Cle, nil, nil)
	if err != nil {
		return err
	}
	if st != 200 {
		return fmt.Errorf("GET /saves/%s : %d", j.Cle, st)
	}
	if hachage(o) != srv.Hash {
		return errors.New(tr("Version reçue corrompue (Hash différent)", "received version is corrupt (hash differs)"))
	}
	dossier := filepath.Dir(j.Fichier)
	tmp := j.Fichier + ".3dssavesync-tmp"
	if j.Systeme == "3DS" {
		// Pas de dossier = Azahar n'a jamais vu ce jeu. Créé seulement avec un .metadata connu (formats3DS) et jamais
		// par-dessus un .metadata existant ; sinon refus (ADR 0009).
		if _, err := os.Stat(dossier); err != nil {
			format, meta := formats3DS[strings.ToUpper(strings.TrimPrefix(j.Cle, "3DS-"))], filepath.Join(filepath.Dir(dossier), "00000001.metadata")
			if format == nil || existe(meta) {
				return errors.New(tr("jeu jamais lancé dans Azahar. Lance-le une fois et sauvegarde en jeu, puis relance-le ici", "game never started in Azahar. Start it once and save in game, then start it here again"))
			}
			if err := os.MkdirAll(dossier, 0o755); err != nil {
				return err
			}
			if err := ecrireVerifie(meta, format); err != nil {
				os.Remove(dossier)
				return err
			}
		}
		tmp = filepath.Join(filepath.Dir(dossier), "main.3dssavesync-tmp") // l'archive ne doit contenir que main
	} else if err := os.MkdirAll(dossier, 0o755); err != nil {
		return err
	}
	contenu := o
	if l != nil {
		if err := s.copier(j, l.Brut); err != nil {
			return fmt.Errorf(tr("Copie de sécurité impossible : %w", "backup copy failed: %w"), err)
		}
		if len(l.Brut) > len(l.SRAM) { // GB/GBA : on remet le Trailer d'horloge de ce PC après la SRAM reçue
			contenu = append(append([]byte{}, o...), l.Brut[len(l.SRAM):]...)
		}
	}
	if strings.EqualFold(filepath.Ext(j.Fichier), ".dsv") { // DeSmuME : SRAM reçue + pied de page (dsv.go)
		var ancien []byte
		if l != nil {
			ancien = l.Brut
		}
		if contenu, err = dsv(o, ancien); err != nil {
			return err
		}
	}
	if estMBC2(j.Systeme, len(o)) { // MBC2 : à la taille du fichier existant, sinon à celle de l'émulateur (mbc2.go)
		n := s.tailleMBC2(j)
		if l != nil && estMBC2(j.Systeme, len(l.Brut)) {
			n = len(l.Brut)
		}
		contenu = fichierMBC2(normaliserMBC2(o), n)
	}
	if srmFixes[j.Coeur] > 0 { // gpSP, Beetle GBA : SRAM reçue dans un .srm de taille fixe (srm.go)
		if contenu, err = srm(j.Coeur, o); err != nil {
			return err
		}
	}
	os.Remove(tmp) // reste d'un arrêt brutal
	if err := ecrireVerifie(tmp, contenu); err != nil {
		return err
	}
	if err := os.Rename(tmp, j.Fichier); err != nil {
		os.Remove(tmp)
		return err
	}
	if relu, err := os.ReadFile(j.Fichier); err != nil || !bytes.Equal(relu, contenu) {
		return fmt.Errorf(tr("%s : relecture différente après remplacement", "%s: read-back differs after replacement"), j.Fichier)
	}
	s.recupere = true
	return s.fixerBase(j.Cle, srv.Hash)
}

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
	return fmt.Sprintf(tr("il y a %d j", "%d days ago"), *age/86400)
}

// Choix de l'utilisateur sur un Conflit (Q5). Sans fenêtre possible (Q16) : rien n'est remplacé.
// distant : contenu de la Version du serveur s'il a été lu (Pokémon 3DS), pour montrer son temps de jeu.
func resoudreConflit(j *Jeu, l *Local, srv *Version, distant []byte, apres bool) string {
	pc := tr("sauvegarde présente", "save present")
	if i, err := os.Stat(j.Fichier); err == nil {
		pc = fmt.Sprintf(tr("modifiée le %s", "modified on %s"), i.ModTime().Format("02/01/2006 15:04"))
	}
	if p, ok := partiePokemon(l.SRAM); ok {
		pc += " · " + p
	}
	enLigne := fmt.Sprintf(tr("envoyée par %s %s", "sent by %s %s"), srv.Client, formatAge(srv.Age))
	if p, ok := partiePokemon(distant); ok {
		enLigne += " · " + p
	}
	texte := fmt.Sprintf(tr("Conflit : %s\n\n"+
		"La Sauvegarde du PC et celle du serveur ont toutes deux changé depuis la dernière Synchronisation.\n\n"+
		"PC : %s\nServeur : %s\n\n"+
		"L'autre version reste dans l'Historique du serveur.",
		"Conflict: %s\n\n"+
			"The PC save and the server save have both changed since the last sync.\n\n"+
			"PC: %s\nServer: %s\n\n"+
			"The other version stays in the server history."),
		j.Nom, pc, enLigne)
	serveur, pc := tr("Prendre le serveur", "Take the server"), tr("Garder le PC", "Keep the PC")
	pasJouer := tr("Ne pas jouer", "Don't play")
	var choix string
	if apres {
		choix = demander(texte, []string{pc, tr("Ne rien faire", "Do nothing")})
	} else {
		choix = demander(texte, []string{serveur, pc, pasJouer})
	}
	switch choix {
	case serveur:
		return aRecuperer
	case pc:
		return forcer
	case pasJouer:
		return annuler
	case "":
		notifier(fmt.Sprintf(tr("Conflit sur %s : rien n'est remplacé. La partie utilise la Sauvegarde du PC.",
			"Conflict on %s: nothing is replaced. The game uses the PC save."), j.Nom))
	}
	return rien
}

// Une passe de Synchronisation. annule = l'utilisateur ne veut pas jouer.
func (s *Session) syncJeu(j *Jeu, apres bool) (annule bool, err error) {
	for essai := 0; essai < 3; essai++ { // un 412 relance une passe avec la nouvelle Version courante
		srv, err := s.versionServeur(j.Cle)
		if err != nil {
			return false, err
		}
		l, err := lireLocal(j, srv)
		if err != nil {
			return false, err
		}
		// Une Sauvegarde vierge compte comme absente ; le fichier reste copié avant écrasement (recuperer).
		var vu *Version
		if l != nil && (!l.Vide || srv != nil && l.Hash == srv.Hash) {
			vu = &l.Version
		}
		if vu == nil && srv == nil {
			return false, nil
		}
		b, err := s.bases()
		if err != nil {
			return false, err
		}
		action := etat(vu, srv, b[j.Cle])
		if action == conflit {
			var distant []byte // Pokémon : la Version du serveur est lue pour comparer les temps de jeu
			if _, ok := partiePokemon(l.SRAM); ok {
				if st, o, err := s.api("GET", "/saves/"+j.Cle, nil, nil); err == nil && st == 200 {
					distant = o
				}
			}
			action = resoudreConflit(j, l, srv, distant, apres)
		}
		switch action {
		case aJour:
			return false, s.fixerBase(j.Cle, l.Hash)
		case aEnvoyer, forcer:
			if ok, err := s.envoyer(j, l, srv); ok || err != nil {
				return false, err
			}
		case aRecuperer:
			return false, s.recuperer(j, l, srv)
		case annuler:
			return true, nil
		default:
			return false, nil
		}
	}
	return false, errors.New(tr("le serveur change sans cesse (412 répétés)", "the server keeps changing (repeated 412)"))
}
