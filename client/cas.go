package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
		return "", fmt.Errorf(tr("l'alias « %s » de %s est invalide. Exemple pour un jeu %s : %s. Corrigez-le dans config.json.",
			"the alias \"%s\" of %s is not valid. Example for a %s game: %s. Fix it in config.json."), c.Alias[nom], nom, systeme, exemplesCle[systeme])
	}
	return a, nil
}

// ROM de chaque Clé synchronisée par ce Client, à côté de sa Base (ADR 0014). Absent (avant le lot D) : aucune ROM retenue.
func (s *Session) cheminRoms() string {
	return filepath.Join(s.dossier, "bases", "roms", s.profil+".json")
}

// romAutre : texte du refus si une autre ROM, qui existe encore, est déjà synchronisée sous cette Clé (ROM hack qui
// garde la Clé du jeu d'origine). Sinon "" et cette ROM est retenue pour la prochaine Base (fixerBase).
// Une ROM retenue qui n'existe plus (renommée, déplacée, extraite dans un dossier temporaire) laisse la place.
func (s *Session) romAutre(cle, rom string) string {
	if cle == "" || rom == "" {
		return ""
	}
	m := map[string]string{}
	lireJSON(s.cheminRoms(), &m)
	if a, err := os.Stat(m[cle]); err == nil {
		if b, err := os.Stat(rom); err == nil && !os.SameFile(a, b) {
			s.journal(fmt.Sprintf("même Clé %s que %s : %s", cle, m[cle], rom))
			return fmt.Sprintf(tr("%s est vu comme le même jeu que %s, déjà synchronisé (ROM hack ?). Pour le synchroniser à part, ajoutez un alias dans config.json.",
				"%s is seen as the same game as %s, already synced (ROM hack?). To sync it separately, add an alias in config.json."),
				filepath.Base(rom), filepath.Base(m[cle]))
		}
	}
	if abs, err := filepath.Abs(rom); err == nil {
		rom = abs
	}
	if s.romDe == nil {
		s.romDe = map[string]string{}
	}
	s.romDe[cle] = rom
	return ""
}

// retenirRom : après une Base écrite, la ROM de cette Clé (si elle est connue et a changé).
func (s *Session) retenirRom(cle string) error {
	rom := s.romDe[cle]
	if rom == "" {
		return nil
	}
	m := map[string]string{}
	if err := lireJSON(s.cheminRoms(), &m); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.journal("ROM retenues illisibles, remplacées : " + err.Error())
	}
	if m[cle] == rom {
		return nil
	}
	m[cle] = rom
	return ecrireJSON(s.cheminRoms(), m, true)
}

// cleDeRom (run) : Clé de la ROM pour ce Client, alias compris. pourquoi non vide = jeu lancé sans Synchronisation.
func (s *Session) cleDeRom(systeme, cle, nom, rom string) (string, string) {
	if systeme == "3DS" { // Azahar range la Sauvegarde par title ID : deux ROM d'un jeu 3DS ont la même
		return cle, ""
	}
	cle, err := s.cfg.cleAlias(systeme, nom, cle)
	if err != nil {
		return "", err.Error()
	}
	if pourquoi := s.romAutre(cle, rom); pourquoi != "" {
		return "", pourquoi
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

// modifierAttente : relit la liste, la change, la réécrit (deux Lanceurs peuvent tourner ensemble).
// ponytail: pas de verrou de fichier ; deux écritures à la même milliseconde peuvent perdre un Envoi en attente, qui
// partira alors au prochain lancement de son jeu (comportement d'avant le lot D).
func (s *Session) modifierAttente(f func([]enAttente) []enAttente) {
	var l []enAttente
	lireJSON(s.cheminAttente(), &l)
	if err := ecrireJSON(s.cheminAttente(), f(l), true); err != nil {
		s.journal("en-attente.json : " + err.Error())
	}
}

// mettreEnAttente : les jeux dont la Sauvegarde locale a changé depuis la Base (ceux à envoyer).
func (s *Session) mettreEnAttente(jeux []*Jeu) {
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
		return
	}
	s.modifierAttente(func(l []enAttente) []enAttente {
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
func (s *Session) reprendre() {
	var l []enAttente
	if lireJSON(s.cheminAttente(), &l) != nil || len(l) == 0 {
		return
	}
	profil, recupere, romDe := s.profil, s.recupere, s.romDe // romDe : la ROM lancée n'est pas celle d'un Envoi en attente
	s.romDe = nil
	defer func() { s.profil, s.recupere, s.romDe = profil, recupere, romDe }()
	var faits []enAttente
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
}
