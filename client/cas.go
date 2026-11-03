package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Forme d'une Clé par Système (ADR 0007) : celle que le serveur accepte. Un alias doit l'avoir (ADR 0014).
var keyShapes = map[string]*regexp.Regexp{
	"DS":  code4,
	"GBA": regexp.MustCompile(`^GBA-[A-Z0-9]{4}$`),
	"GB":  regexp.MustCompile(`^GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}$`),
}

var keyExamples = map[string]string{"DS": "0RAD", "GBA": "GBA-0RAD", "GB": "GB-MONHACK-0001"}

// cleAlias : la Clé choisie dans « alias » pour cette ROM (nom du fichier sans extension), sinon celle de l'en-tête.
// Un alias mal formé est une erreur : la ROM ne se synchronise pas sous la Clé de l'en-tête (celle du jeu d'origine).
func (c *Config) aliasKey(system, name, key string) (string, error) {
	a, ok := c.Alias[name]
	if !ok || keyShapes[system] == nil {
		return key, nil
	}
	if a = strings.ToUpper(strings.TrimSpace(a)); !keyShapes[system].MatchString(a) {
		return "", fmt.Errorf(tr("L'alias « %s » de %s n'est pas valable (exemple pour un jeu %s : %s). Retirez-le dans les réglages, section ROM hacks, et ajoutez de nouveau la ROM.",
			"The alias \"%s\" of %s isn't valid (example for a %s game: %s). Remove it in the settings, ROM hacks section, and add the ROM again."), c.Alias[name], name, system, keyExamples[system])
	}
	return a, nil
}

// ROM de chaque Clé synchronisée sur ce PC, tous émulateurs ensemble (ADR 0014) : la Version courante du serveur est
// la même pour tous. Hors de bases/ : la Base d'un émulateur nommé « roms » serait bases/roms.json.
func (s *Session) romsPath() string { return filepath.Join(s.dir, "roms.json") }

// romsRetenues : roms.json, complété par les anciens fichiers par émulateur (bases/roms/<profil>.json, lus seulement ;
// à Clé égale, le premier émulateur par ordre alphabétique). Ils sont repris dans roms.json à sa prochaine écriture.
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

// baseConnue : un émulateur de ce PC a une Base pour cette Clé (ou une Base illisible : dans le doute, oui).
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

// romAutre : texte du refus si cette ROM ne doit pas se synchroniser sous la Clé de j, sinon "" et la ROM est retenue
// pour la prochaine Base (fixerBase). Refus :
//   - une autre ROM, qui existe encore, est déjà synchronisée sous cette Clé (ROM hack qui garde la Clé du jeu
//     d'origine). Une ROM retenue qui n'existe plus (renommée, déplacée, extraite dans un dossier temporaire) laisse la place ;
//   - aucune ROM retenue, mais une Base de cette Clé sur ce PC (synchronisée avant que les ROM soient retenues), et pas de
//     sauvegarde pour cette ROM : la Récupération donnerait à un éventuel ROM hack la partie du jeu d'origine, que son
//     prochain Envoi remplacerait.
//
// Avec question (run), l'utilisateur choisit : même jeu (second cas), « Synchroniser à part » (un alias est créé, j prend
// sa Clé), ou jouer sans synchroniser.
func (s *Session) romRefusal(j *Game, rom string, question bool) string {
	if j.Key == "" || rom == "" {
		return ""
	}
	apart, without := tr("Synchroniser à part", "Sync separately"), tr("Jouer sans synchroniser", "Play without syncing")
	hint := tr("Pour lui donner sa propre sauvegarde, lancez-le depuis votre frontend et choisissez « Synchroniser à part ». Vous pouvez aussi l'ajouter dans les réglages, section ROM hacks.",
		"To give it its own save, start it from your frontend and choose \"Sync separately\". You can also add it in the settings, ROM hacks section.")
	choice := ""
	m, _ := s.keptRoms()
	if a, err := os.Stat(m[j.Key]); err == nil {
		if b, err := os.Stat(rom); err == nil && !os.SameFile(a, b) {
			s.log(fmt.Sprintf("même Clé %s que %s : %s", j.Key, m[j.Key], rom))
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
	} else if m[j.Key] == "" && s.knownBase(j.Key) {
		if l, err := readLocal(j, nil); err == nil && (l == nil || l.Blank) {
			s.log(fmt.Sprintf("Clé %s déjà synchronisée, aucune ROM retenue, pas de sauvegarde : %s", j.Key, rom))
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
		key, err := s.createAlias(j.System, strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom)))
		if err != nil {
			s.log("alias impossible pour " + rom + " : " + err.Error())
			return fmt.Sprintf(tr("%s : impossible de le synchroniser à part (%v).", "%s: couldn't sync it separately (%v)."), filepath.Base(rom), err)
		}
		s.log(fmt.Sprintf("alias créé : %s -> %s", rom, key))
		notify(fmt.Sprintf(tr("%s a maintenant sa propre sauvegarde. Pour y jouer aussi sur la 3DS, préparez de nouveau la carte SD dans les réglages.",
			"%s now has its own save. To play it on the 3DS too, prepare the SD card again in the settings."), filepath.Base(rom)))
		j.Key = key
	}
	if abs, err := filepath.Abs(rom); err == nil {
		rom = abs
	}
	if s.romOf == nil {
		s.romOf = map[string]string{}
	}
	s.romOf[j.Key] = rom
	return ""
}

// creerAlias : l'alias de la ROM nom, créé s'il manque et écrit dans config.json (copie datée avant).
func (s *Session) createAlias(system, name string) (string, error) {
	var cfg Config
	if err := readConfig(s.config, &cfg); err != nil { // relu : la page des réglages a pu l'écrire depuis
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

// ecrireConfigCopie : copie datée de config.json, puis écriture vérifiée.
func writeConfigCopy(path string, cfg *Config) error {
	if err := copyBefore(path); err != nil {
		return err
	}
	return writeJSON(path, cfg, true)
}

// ajouterAlias : l'alias de la ROM nom dans cfg, créé s'il manque (ADR 0014). La Clé est tirée du nom : la même ROM
// reçoit la même Clé sur chaque PC ; une Clé déjà prise par un autre alias en donne une autre.
func addAlias(cfg *Config, system, name string) (string, error) {
	if keyShapes[system] == nil {
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

// nouvelleCle : une Clé de la forme du Système, qui commence par un chiffre (jamais un code de jeu commercial).
func newKey(system, name string, taken map[string]bool) string {
	const signs = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := 0; ; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", name, i)))
		suffix := string([]byte{signs[h[0]%36], signs[h[1]%36], signs[h[2]%36]})
		key := map[string]string{"DS": "0" + suffix, "GBA": "GBA-0" + suffix, "GB": fmt.Sprintf("GB-ALIAS-%02X%02X", h[0], h[1])}[system]
		if !taken[key] {
			return key
		}
	}
}

// retenirRom : après une Base écrite, la ROM de cette Clé (si elle est connue et a changé).
func (s *Session) keepRom(key string) error {
	rom := s.romOf[key]
	if rom == "" {
		return nil
	}
	m, err := s.keptRoms()
	if err != nil {
		s.log("ROM retenues illisibles, remplacées : " + err.Error())
	}
	if m[key] == rom {
		return nil
	}
	m[key] = rom
	return writeJSON(s.romsPath(), m, true)
}

// cleDeRom (run) : Clé de la ROM pour ce Client, alias compris. pourquoi non vide = jeu lancé sans Synchronisation.
func (s *Session) romKey(system, key, name string) (string, string) {
	if system == "3DS" { // Azahar range la Sauvegarde par title ID : deux ROM d'un jeu 3DS ont la même
		return key, ""
	}
	key, err := s.cfg.aliasKey(system, name, key)
	if err != nil {
		return "", err.Error()
	}
	return key, ""
}

// Envoi en attente (D5) : une Sauvegarde jouée que le serveur n'a pas reçue (réseau coupé). Reprise au début de chaque
// run ou launch, quel que soit le jeu, par la Synchronisation habituelle (jamais d'écrasement sans comparaison).
type pendingEntry struct {
	Profile string `json:"profil"`
	Game
}

func (s *Session) pendingPath() string { return filepath.Join(s.dir, "en-attente.json") }

// verrouAttente : un seul Lanceur à la fois relit et réécrit la liste (deux Lanceurs peuvent tourner ensemble).
// Un verrou de plus de 30 s vient d'un Lanceur arrêté : retiré. La fonction rendue retire le verrou.
func (s *Session) pendingLock() func() {
	v := s.pendingPath() + ".verrou"
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

// modifierAttente : relit la liste, la change, la réécrit (fichier temporaire vérifié puis renommé, ecrireJSON).
// Liste illisible : mise de côté (en-attente.json.illisible-<date>), jamais remplacée sans copie.
func (s *Session) editPending(f func([]pendingEntry) []pendingEntry) error {
	defer s.pendingLock()()
	var l []pendingEntry
	if err := readJSON(s.pendingPath(), &l); err != nil && !errors.Is(err, fs.ErrNotExist) {
		side := s.pendingPath() + ".illisible-" + time.Now().Format("20060102-150405")
		if e := os.Rename(s.pendingPath(), side); e != nil {
			s.log("en-attente.json illisible, et impossible à mettre de côté : " + err.Error() + " ; " + e.Error())
			return e
		}
		s.log("en-attente.json illisible, mis de côté dans " + side + " : " + err.Error())
		notify(fmt.Sprintf(tr("La liste des envois en attente était illisible. Elle a été mise de côté dans %s, et ses jeux seront envoyés à leur prochain lancement.",
			"The list of pending uploads couldn't be read. It was set aside in %s, and its games will be uploaded the next time you start them."), side))
		l = nil
	}
	err := writeJSON(s.pendingPath(), f(l), true)
	if err != nil {
		s.log("en-attente.json : " + err.Error())
	}
	return err
}

// mettreEnAttente : les jeux dont la Sauvegarde locale a changé depuis la Base (ceux à envoyer). Erreur : la liste
// n'a pas pu être écrite.
func (s *Session) addPending(games []*Game) error {
	b, _ := s.bases()
	var added []pendingEntry
	for _, j := range games {
		l, err := readLocal(j, nil)
		if err == nil && (l == nil || l.Blank || b[j.Key] == l.Hash) {
			continue
		}
		s.log(j.Key + " : Envoi en attente")
		added = append(added, pendingEntry{s.profile, *j})
	}
	if len(added) == 0 {
		return nil
	}
	return s.editPending(func(l []pendingEntry) []pendingEntry {
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

// reprendre : les Envois en attente, avant la Synchronisation du jeu lancé. Un Conflit passe par la question habituelle
// (après la partie : garder le PC ou ne rien faire). Reste en attente : émulateur ouvert, ou réseau encore coupé.
// Rend la dernière erreur d'envoi (menu de l'icône, « Envoyer maintenant »).
func (s *Session) resume() error {
	var l []pendingEntry
	if readJSON(s.pendingPath(), &l) != nil || len(l) == 0 {
		return nil
	}
	profile, downloaded, romOf := s.profile, s.downloaded, s.romOf // romDe : la ROM lancée n'est pas celle d'un Envoi en attente
	s.romOf = nil
	defer func() { s.profile, s.downloaded, s.romOf = profile, downloaded, romOf }()
	var done []pendingEntry
	var last error
	for _, a := range l {
		e := s.cfg.Emulators[a.Profile]
		if e == nil || !exists(a.File) {
			s.log(a.Key + " : Envoi en attente abandonné (émulateur ou fichier disparu) : " + a.File)
			done = append(done, a)
			continue
		}
		if instances(e.process()) > 0 { // une partie en cours peut encore écrire ce fichier
			continue
		}
		s.profile = a.Profile
		j := a.Game
		_, err := s.syncGame(&j, true)
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
