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
var formesCle = map[string]*regexp.Regexp{
	"DS":  code4,
	"GBA": regexp.MustCompile(`^GBA-[A-Z0-9]{4}$`),
	"GB":  regexp.MustCompile(`^GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}$`),
}

var exemplesCle = map[string]string{"DS": "0RAD", "GBA": "GBA-0RAD", "GB": "GB-MONHACK-0001"}

// cleAlias : la Clé choisie dans « alias » pour cette ROM (nom du fichier sans extension), sinon celle de l'en-tête.
// Un alias mal formé est une erreur : la ROM ne se synchronise pas sous la Clé de l'en-tête (celle du jeu d'origine).
func (c *Config) cleAlias(systeme, nom, cle string) (string, error) {
	a, ok := c.Alias[nom]
	if !ok || formesCle[systeme] == nil {
		return cle, nil
	}
	if a = strings.ToUpper(strings.TrimSpace(a)); !formesCle[systeme].MatchString(a) {
		return "", fmt.Errorf(tr("l'alias « %s » de %s est invalide (exemple pour un jeu %s : %s). Retirez-le dans « 3dssavesync réglages », section ROM hacks, puis ajoutez de nouveau la ROM.",
			"the alias \"%s\" of %s is not valid (example for a %s game: %s). Remove it in \"3dssavesync settings\", ROM hacks section, then add the ROM again."), c.Alias[nom], nom, systeme, exemplesCle[systeme])
	}
	return a, nil
}

// ROM de chaque Clé synchronisée sur ce PC, tous émulateurs ensemble (ADR 0014) : la Version courante du serveur est
// la même pour tous. Hors de bases/ : la Base d'un émulateur nommé « roms » serait bases/roms.json.
func (s *Session) cheminRoms() string { return filepath.Join(s.dossier, "roms.json") }

// romsRetenues : roms.json, complété par les anciens fichiers par émulateur (bases/roms/<profil>.json, lus seulement ;
// à Clé égale, le premier émulateur par ordre alphabétique). Ils sont repris dans roms.json à sa prochaine écriture.
func (s *Session) romsRetenues() (map[string]string, error) {
	m := map[string]string{}
	err := lireJSON(s.cheminRoms(), &m)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	anciens, _ := filepath.Glob(filepath.Join(s.dossier, "bases", "roms", "*.json"))
	for _, f := range anciens {
		a := map[string]string{}
		lireJSON(f, &a)
		for cle, rom := range a {
			if m[cle] == "" {
				m[cle] = rom
			}
		}
	}
	return m, err
}

// baseConnue : un émulateur de ce PC a une Base pour cette Clé (ou une Base illisible : dans le doute, oui).
func (s *Session) baseConnue(cle string) bool {
	fichiers, _ := filepath.Glob(filepath.Join(s.dossier, "bases", "*.json"))
	for _, f := range fichiers {
		m := map[string]string{}
		if lireJSON(f, &m) != nil || m[cle] != "" {
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
func (s *Session) romAutre(j *Jeu, rom string, question bool) string {
	if j.Cle == "" || rom == "" {
		return ""
	}
	aPart, sans := tr("Synchroniser à part (ROM hack)", "Sync separately (ROM hack)"), tr("Jouer sans synchroniser", "Play without sync")
	conseil := tr("Pour lui donner sa propre sauvegarde : lancez-le depuis votre frontend et choisissez « Synchroniser à part », ou ajoutez-le dans « 3dssavesync réglages », section ROM hacks.",
		"To give it its own save: start it from your frontend and choose \"Sync separately\", or add it in \"3dssavesync settings\", ROM hacks section.")
	choix := ""
	m, _ := s.romsRetenues()
	if a, err := os.Stat(m[j.Cle]); err == nil {
		if b, err := os.Stat(rom); err == nil && !os.SameFile(a, b) {
			s.journal(fmt.Sprintf("même Clé %s que %s : %s", j.Cle, m[j.Cle], rom))
			texte := fmt.Sprintf(tr("%s est vu comme le même jeu que %s, déjà synchronisé. C'est sans doute un ROM hack.",
				"%s is seen as the same game as %s, already synced. It is probably a ROM hack."), filepath.Base(rom), filepath.Base(m[j.Cle]))
			if question {
				choix = demander(texte+"\n\n"+tr("Synchroniser cette ROM à part, avec sa propre sauvegarde ?", "Sync this ROM separately, with its own save?"),
					[]string{aPart, sans})
			}
			if choix != aPart {
				return texte + " " + conseil
			}
		}
	} else if m[j.Cle] == "" && s.baseConnue(j.Cle) {
		if l, err := lireLocal(j, nil); err == nil && (l == nil || l.Vide) {
			s.journal(fmt.Sprintf("Clé %s déjà synchronisée, aucune ROM retenue, pas de sauvegarde : %s", j.Cle, rom))
			oui := tr("Synchroniser", "Sync")
			if question {
				choix = demander(fmt.Sprintf(tr("%s : cette ROM n'a pas de sauvegarde dans cet émulateur, et ce jeu a déjà été synchronisé sur ce PC, peut-être avec une autre ROM.\n\n"+
					"Même jeu : « Synchroniser » lui donne la sauvegarde du serveur.\nROM hack : « Synchroniser à part » lui donne sa propre sauvegarde.",
					"%s: this ROM has no save in this emulator, and this game was already synced on this PC, maybe with another ROM.\n\n"+
						"Same game: \"Sync\" gives it the server save.\nROM hack: \"Sync separately\" gives it its own save."), filepath.Base(rom)),
					[]string{oui, aPart, sans})
			}
			if choix != oui && choix != aPart {
				return fmt.Sprintf(tr("%s n'a pas de sauvegarde ici, et ce jeu a déjà été synchronisé sur ce PC, peut-être avec une autre ROM. Lancez-le depuis votre frontend pour choisir.",
					"%s has no save here, and this game was already synced on this PC, maybe with another ROM. Start it from your frontend to choose."), filepath.Base(rom))
			}
		}
	}
	if choix == aPart {
		cle, err := s.creerAlias(j.Systeme, strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom)))
		if err != nil {
			s.journal("alias impossible pour " + rom + " : " + err.Error())
			return fmt.Sprintf(tr("%s : synchronisation à part impossible (%v).", "%s: cannot sync separately (%v)."), filepath.Base(rom), err)
		}
		s.journal(fmt.Sprintf("alias créé : %s -> %s", rom, cle))
		notifier(fmt.Sprintf(tr("%s a maintenant sa propre sauvegarde. Pour y jouer aussi sur la 3DS : « 3dssavesync réglages », Préparer la carte SD.",
			"%s now has its own save. To play it on the 3DS too: \"3dssavesync settings\", Prepare the SD card."), filepath.Base(rom)))
		j.Cle = cle
	}
	if abs, err := filepath.Abs(rom); err == nil {
		rom = abs
	}
	if s.romDe == nil {
		s.romDe = map[string]string{}
	}
	s.romDe[j.Cle] = rom
	return ""
}

// creerAlias : l'alias de la ROM nom, créé s'il manque et écrit dans config.json (copie datée avant).
func (s *Session) creerAlias(systeme, nom string) (string, error) {
	var cfg Config
	if err := lireConfig(s.config, &cfg); err != nil { // relu : la page des réglages a pu l'écrire depuis
		return "", err
	}
	cle, err := ajouterAlias(&cfg, systeme, nom)
	if err == nil {
		err = ecrireConfigCopie(s.config, &cfg)
	}
	if err != nil {
		return "", err
	}
	s.cfg.Alias = cfg.Alias
	return cle, nil
}

// ecrireConfigCopie : copie datée de config.json, puis écriture vérifiée.
func ecrireConfigCopie(chemin string, cfg *Config) error {
	if err := copieAvant(chemin); err != nil {
		return err
	}
	return ecrireJSON(chemin, cfg, true)
}

// ajouterAlias : l'alias de la ROM nom dans cfg, créé s'il manque (ADR 0014). La Clé est tirée du nom : la même ROM
// reçoit la même Clé sur chaque PC ; une Clé déjà prise par un autre alias en donne une autre.
func ajouterAlias(cfg *Config, systeme, nom string) (string, error) {
	if formesCle[systeme] == nil {
		return "", fmt.Errorf(tr("les jeux %s n'ont pas d'alias", "%s games have no alias"), systeme)
	}
	if strings.TrimSpace(nom) == "" || strings.ContainsAny(nom, "=\r\n") {
		return "", fmt.Errorf(tr("nom de ROM invalide pour un alias : « %s »", "invalid ROM name for an alias: \"%s\""), nom)
	}
	if a := strings.ToUpper(cfg.Alias[nom]); formesCle[systeme].MatchString(a) {
		return a, nil
	}
	prises := map[string]bool{}
	for n, a := range cfg.Alias {
		if n != nom {
			prises[strings.ToUpper(a)] = true
		}
	}
	cle := nouvelleCle(systeme, nom, prises)
	if cfg.Alias == nil {
		cfg.Alias = map[string]string{}
	}
	cfg.Alias[nom] = cle
	return cle, nil
}

// nouvelleCle : une Clé de la forme du Système, qui commence par un chiffre (jamais un code de jeu commercial).
func nouvelleCle(systeme, nom string, prises map[string]bool) string {
	const signes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := 0; ; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", nom, i)))
		suite := string([]byte{signes[h[0]%36], signes[h[1]%36], signes[h[2]%36]})
		cle := map[string]string{"DS": "0" + suite, "GBA": "GBA-0" + suite, "GB": fmt.Sprintf("GB-ALIAS-%02X%02X", h[0], h[1])}[systeme]
		if !prises[cle] {
			return cle
		}
	}
}

// retenirRom : après une Base écrite, la ROM de cette Clé (si elle est connue et a changé).
func (s *Session) retenirRom(cle string) error {
	rom := s.romDe[cle]
	if rom == "" {
		return nil
	}
	m, err := s.romsRetenues()
	if err != nil {
		s.journal("ROM retenues illisibles, remplacées : " + err.Error())
	}
	if m[cle] == rom {
		return nil
	}
	m[cle] = rom
	return ecrireJSON(s.cheminRoms(), m, true)
}

// cleDeRom (run) : Clé de la ROM pour ce Client, alias compris. pourquoi non vide = jeu lancé sans Synchronisation.
func (s *Session) cleDeRom(systeme, cle, nom string) (string, string) {
	if systeme == "3DS" { // Azahar range la Sauvegarde par title ID : deux ROM d'un jeu 3DS ont la même
		return cle, ""
	}
	cle, err := s.cfg.cleAlias(systeme, nom, cle)
	if err != nil {
		return "", err.Error()
	}
	return cle, ""
}

// Envoi en attente (D5) : une Sauvegarde jouée que le serveur n'a pas reçue (réseau coupé). Reprise au début de chaque
// run ou launch, quel que soit le jeu, par la Synchronisation habituelle (jamais d'écrasement sans comparaison).
type enAttente struct {
	Profil string `json:"profil"`
	Jeu
}

func (s *Session) cheminAttente() string { return filepath.Join(s.dossier, "en-attente.json") }

// verrouAttente : un seul Lanceur à la fois relit et réécrit la liste (deux Lanceurs peuvent tourner ensemble).
// Un verrou de plus de 30 s vient d'un Lanceur arrêté : retiré. La fonction rendue retire le verrou.
func (s *Session) verrouAttente() func() {
	v := s.cheminAttente() + ".verrou"
	os.MkdirAll(s.dossier, 0o755)
	for essai := 0; essai < 50; essai++ {
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
	s.journal("en-attente.json : verrou impossible, liste modifiée sans verrou")
	return func() {}
}

// modifierAttente : relit la liste, la change, la réécrit (fichier temporaire vérifié puis renommé, ecrireJSON).
// Liste illisible : mise de côté (en-attente.json.illisible-<date>), jamais remplacée sans copie.
func (s *Session) modifierAttente(f func([]enAttente) []enAttente) error {
	defer s.verrouAttente()()
	var l []enAttente
	if err := lireJSON(s.cheminAttente(), &l); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cote := s.cheminAttente() + ".illisible-" + time.Now().Format("20060102-150405")
		if e := os.Rename(s.cheminAttente(), cote); e != nil {
			s.journal("en-attente.json illisible, et impossible à mettre de côté : " + err.Error() + " ; " + e.Error())
			return e
		}
		s.journal("en-attente.json illisible, mis de côté dans " + cote + " : " + err.Error())
		notifier(fmt.Sprintf(tr("La liste des envois en attente était illisible. Elle est gardée dans %s. Ses jeux partiront à leur prochain lancement.",
			"The list of pending uploads could not be read. It is kept in %s. Its games will be sent at their next launch."), cote))
		l = nil
	}
	err := ecrireJSON(s.cheminAttente(), f(l), true)
	if err != nil {
		s.journal("en-attente.json : " + err.Error())
	}
	return err
}

// mettreEnAttente : les jeux dont la Sauvegarde locale a changé depuis la Base (ceux à envoyer). Erreur : la liste
// n'a pas pu être écrite.
func (s *Session) mettreEnAttente(jeux []*Jeu) error {
	b, _ := s.bases()
	var neufs []enAttente
	for _, j := range jeux {
		l, err := lireLocal(j, nil)
		if err == nil && (l == nil || l.Vide || b[j.Cle] == l.Hash) {
			continue
		}
		s.journal(j.Cle + " : Envoi en attente")
		neufs = append(neufs, enAttente{s.profil, *j})
	}
	if len(neufs) == 0 {
		return nil
	}
	return s.modifierAttente(func(l []enAttente) []enAttente {
		for _, n := range neufs {
			l = append(retirer(l, n), n)
		}
		return l
	})
}

func retirer(l []enAttente, a enAttente) []enAttente {
	var r []enAttente
	for _, x := range l {
		if x.Profil != a.Profil || x.Cle != a.Cle {
			r = append(r, x)
		}
	}
	return r
}

// reprendre : les Envois en attente, avant la Synchronisation du jeu lancé. Un Conflit passe par la question habituelle
// (après la partie : garder le PC ou ne rien faire). Reste en attente : émulateur ouvert, ou réseau encore coupé.
// Rend la dernière erreur d'envoi (menu de l'icône, « Envoyer maintenant »).
func (s *Session) reprendre() error {
	var l []enAttente
	if lireJSON(s.cheminAttente(), &l) != nil || len(l) == 0 {
		return nil
	}
	profil, recupere, romDe := s.profil, s.recupere, s.romDe // romDe : la ROM lancée n'est pas celle d'un Envoi en attente
	s.romDe = nil
	defer func() { s.profil, s.recupere, s.romDe = profil, recupere, romDe }()
	var faits []enAttente
	var derniere error
	for _, a := range l {
		e := s.cfg.Emulateurs[a.Profil]
		if e == nil || !existe(a.Fichier) {
			s.journal(a.Cle + " : Envoi en attente abandonné (émulateur ou fichier disparu) : " + a.Fichier)
			faits = append(faits, a)
			continue
		}
		if instances(e.processus()) > 0 { // une partie en cours peut encore écrire ce fichier
			continue
		}
		s.profil = a.Profil
		j := a.Jeu
		_, err := s.syncJeu(&j, true)
		if err == nil {
			s.journal(a.Cle + " : Envoi en attente repris")
			faits = append(faits, a)
			continue
		}
		s.journal(a.Cle + " en attente : " + err.Error())
		derniere = err
		if fin(err) {
			break
		}
	}
	if len(faits) > 0 {
		s.modifierAttente(func(l []enAttente) []enAttente {
			for _, a := range faits {
				l = retirer(l, a)
			}
			return l
		})
	}
	return derniere
}
