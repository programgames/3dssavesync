// 3dssavesync : synchronise les sauvegardes d'un émulateur avec le serveur, avant et après la partie.
//
//	3dssavesync run [--emulator <nom>] <rom>             un jeu (frontends)
//	3dssavesync run [--emulator <nom>] --rom <rom> -- <commande…>  enveloppe une commande quelconque
//	3dssavesync launch <nom>                             tous les jeux d'un émulateur, puis l'émulateur
//	3dssavesync migrate-v2 <dossier de sync.ps1>         reprend sync.config.json et base.json
//
// Option commune : --config <fichier> (défaut : <dossier de configuration>/3dssavesync/config.json).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	URL        string                `json:"url"`
	Jeton      string                `json:"jeton"`
	Copies     string                `json:"copies,omitempty"` // défaut : <dossier de la configuration>/copies
	Emulateurs map[string]*Emulateur `json:"emulateurs"`
}

type Emulateur struct {
	Profil      string            `json:"profil,omitempty"` // profil intégré ; défaut : le nom de l'entrée
	Exe         string            `json:"exe"`
	Sauvegardes map[string]string `json:"sauvegardes"`    // Système -> dossier (3DS : sdmc d'Azahar)
	Roms        map[string]string `json:"roms,omitempty"` // Système -> dossier des ROM (commande launch)
}

func (c *Config) dossierCopies(dossierConfig string) string {
	if c.Copies != "" {
		return c.Copies
	}
	return filepath.Join(dossierConfig, "copies")
}

// Profil intégré d'un émulateur (la phase 2 en ajoute d'autres).
type Profil struct {
	Processus string                                 // nom du processus à surveiller
	Args      func(rom, sauvegardes string) []string // rom vide : émulateur sans jeu (launch)
}

var profils = map[string]Profil{
	"melonds": {"melonDS", func(rom, _ string) []string { return seul(rom) }},
	"mgba": {"mGBA", func(rom, sauvegardes string) []string {
		if sauvegardes == "" {
			return seul(rom)
		}
		return append([]string{"-C", "savegamePath=" + sauvegardes}, seul(rom)...) // jamais --config (mGBA quitte)
	}},
	"azahar": {"azahar", func(rom, _ string) []string {
		if rom == "" {
			return nil
		}
		return []string{"-f", rom}
	}},
}

func seul(rom string) []string {
	if rom == "" {
		return nil
	}
	return []string{rom}
}

func (e *Emulateur) profil(nom string) (Profil, error) {
	p := e.Profil
	if p == "" {
		p = nom
	}
	if pr, ok := profils[p]; ok {
		return pr, nil
	}
	return Profil{}, fmt.Errorf("profil d'émulateur inconnu : %s", p)
}

// Adaptateurs remplacés par les tests.
var (
	demander       = demanderSysteme // texte, boutons -> bouton choisi ; "" si aucune fenêtre possible
	notifier       = notifierSysteme
	lancer         = lancerCommande
	emulateurActif = processusActif
	transport      = http.DefaultTransport
)

// Attend la fin de la commande. Seul l'échec du démarrage compte : après un plantage de l'émulateur,
// l'envoi doit quand même partir.
func lancerCommande(commande []string) error {
	c := exec.Command(commande[0], commande[1:]...)
	if err := c.Start(); err != nil {
		return err
	}
	c.Wait()
	return nil
}

func main() { os.Exit(principal(os.Args[1:])) }

func principal(args []string) int {
	// Un frontend qui arrête le programme (Lutris, Steam) ne doit pas couper l'envoi :
	// on attend la fin de l'émulateur, puis on envoie. SIGKILL reste imparable.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage : 3dssavesync run|launch|migrate-v2 … (voir docs)")
		return 2
	}
	defaut := "config.json"
	if d, err := os.UserConfigDir(); err == nil {
		defaut = filepath.Join(d, "3dssavesync", "config.json")
	}
	fl := flag.NewFlagSet(args[0], flag.ContinueOnError)
	config := fl.String("config", defaut, "fichier de configuration")
	emulateur := fl.String("emulator", "", "émulateur de la configuration")
	rom := fl.String("rom", "", "ROM, avec une commande après --")
	if err := fl.Parse(args[1:]); err != nil {
		return 2
	}
	dossier := filepath.Dir(*config)
	var code int
	var err error
	switch args[0] {
	case "run":
		code, err = commandeRun(*config, *emulateur, *rom, fl.Args())
	case "launch":
		if fl.NArg() != 1 {
			err = errors.New("launch : un nom d'émulateur")
			break
		}
		code, err = commandeLaunch(*config, fl.Arg(0))
	case "migrate-v2":
		if fl.NArg() != 1 {
			err = errors.New("migrate-v2 : le dossier de sync.ps1")
			break
		}
		err = migrerV2(fl.Arg(0), *config)
	default:
		err = fmt.Errorf("commande inconnue : %s", args[0])
	}
	if err != nil {
		(&Session{dossier: dossier}).journal("erreur : " + err.Error() + " (" + strings.Join(args, " ") + ")")
		fmt.Fprintln(os.Stderr, err)
		demander("Erreur de 3dssavesync : "+err.Error(), []string{"OK"})
		return 1
	}
	return code
}

func nouvelleSession(chemin string) (*Session, error) {
	var cfg Config
	if err := lireJSON(chemin, &cfg); err != nil {
		return nil, err
	}
	return &Session{cfg: &cfg, dossier: filepath.Dir(chemin), http: &http.Client{Timeout: 30 * time.Second, Transport: transport}}, nil
}

// Émulateur de la configuration qui gère ce Système : celui demandé, ou le seul possible.
func (c *Config) emulateurPour(systeme, demande string) (string, error) {
	if demande != "" {
		if e := c.Emulateurs[demande]; e == nil || e.Sauvegardes[systeme] == "" {
			return "", fmt.Errorf("l'émulateur %s n'a pas de dossier de sauvegardes pour %s", demande, systeme)
		}
		return demande, nil
	}
	var noms []string
	for nom, e := range c.Emulateurs {
		if e.Sauvegardes[systeme] != "" {
			noms = append(noms, nom)
		}
	}
	sort.Strings(noms)
	switch len(noms) {
	case 0:
		return "", fmt.Errorf("aucun émulateur configuré pour %s", systeme)
	case 1:
		return noms[0], nil
	}
	return "", fmt.Errorf("plusieurs émulateurs pour %s (%s) : précise --emulator", systeme, strings.Join(noms, ", "))
}

func commandeRun(config, demande, rom string, reste []string) (int, error) {
	commande := reste
	if rom == "" {
		if len(reste) != 1 {
			return 2, errors.New("run : une ROM, ou --rom <rom> -- <commande>")
		}
		rom, commande = reste[0], nil
	} else if len(reste) == 0 {
		return 2, errors.New("run --rom : la commande à lancer manque après --")
	}
	s, err := nouvelleSession(config)
	if err != nil {
		return 1, err
	}
	systeme, cle, nom, err := lireRom(rom)
	s.journal(fmt.Sprintf("lancement %s %s : %s", systeme, cle, rom))
	if err != nil {
		return 1, err
	}
	if systeme == "" {
		return 1, fmt.Errorf("ROM non reconnue (extension) : %s", rom)
	}
	if s.profil, err = s.cfg.emulateurPour(systeme, demande); err != nil {
		return 1, err
	}
	e := s.cfg.Emulateurs[s.profil]
	p, err := e.profil(s.profil)
	if err != nil {
		return 1, err
	}
	if s.refus(p) {
		return 1, nil
	}
	if commande == nil {
		commande = append([]string{e.Exe}, p.Args(rom, e.Sauvegardes[systeme])...)
	}
	var jeux []*Jeu
	if cle == "" { // homebrew, ou 3DS hors des 8 Pokémon
		s.journal("sans synchronisation : Clé illisible ou jeu hors périmètre")
	} else {
		jeux = []*Jeu{{systeme, cle, nom, fichierSauvegarde(e.Sauvegardes[systeme], systeme, cle, nom)}}
	}
	return s.jouer(jeux, commande)
}

func commandeLaunch(config, nom string) (int, error) {
	s, err := nouvelleSession(config)
	if err != nil {
		return 1, err
	}
	s.profil = nom
	s.journal("lancement de " + nom)
	e := s.cfg.Emulateurs[nom]
	if e == nil {
		return 1, fmt.Errorf("émulateur absent de la configuration : %s", nom)
	}
	p, err := e.profil(nom)
	if err != nil {
		return 1, err
	}
	if s.refus(p) {
		return 1, nil
	}
	// mGBA sans jeu : un seul savegamePath possible, donc seulement si GB et GBA partagent le dossier.
	var dossier string
	for _, d := range e.Sauvegardes {
		if dossier != "" && d != dossier {
			dossier = ""
			break
		}
		dossier = d
	}
	return s.jouer(s.jeuxDe(e), append([]string{e.Exe}, p.Args("", dossier)...))
}

// L'émulateur tourne déjà : synchroniser maintenant écraserait ou enverrait une partie en cours.
func (s *Session) refus(p Profil) bool {
	if !emulateurActif(p.Processus) {
		return false
	}
	s.journal("refus : " + p.Processus + " tourne déjà")
	notifier(p.Processus + " tourne déjà. Ferme-le avant de lancer un jeu.")
	return true
}

// Jeux d'un émulateur pour launch : les ROM de ses dossiers roms, et les Pokémon 3DS déjà vus par Azahar.
func (s *Session) jeuxDe(e *Emulateur) []*Jeu {
	var jeux []*Jeu
	vus := map[string]bool{}
	for systeme, dossier := range e.Sauvegardes {
		if systeme == "3DS" {
			for id, titre := range titres3DS {
				cle := "3DS-" + id
				j := &Jeu{systeme, cle, titre, fichierSauvegarde(dossier, systeme, cle, titre)}
				if _, err := os.Stat(filepath.Dir(j.Fichier)); err == nil { // jamais lancé dans Azahar : ignoré
					jeux = append(jeux, j)
				}
			}
			continue
		}
		racine := e.Roms[systeme]
		if racine == "" {
			s.journal("pas de dossier roms pour " + systeme + " : ses jeux ne sont pas synchronisés")
			continue
		}
		filepath.WalkDir(racine, func(chemin string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			sys, cle, nom, err := lireRom(chemin)
			if err == nil && sys == systeme && cle != "" && !vus[cle] {
				vus[cle] = true
				jeux = append(jeux, &Jeu{sys, cle, nom, fichierSauvegarde(dossier, sys, cle, nom)})
			}
			return nil
		})
	}
	sort.Slice(jeux, func(a, b int) bool { return jeux[a].Nom < jeux[b].Nom })
	return jeux
}

// Synchronisation avant, partie, Synchronisation après.
// ponytail: une requête GET /saves par Jeu et par passe ; une seule liste par passe si launch devient lent.
func (s *Session) jouer(jeux []*Jeu, commande []string) (int, error) {
	var echecs []string
	for _, j := range jeux {
		annule, err := s.syncJeu(j, false)
		if annule {
			return 0, nil
		}
		if err != nil {
			s.journal(j.Cle + " avant : " + err.Error())
			echecs = append(echecs, j.Nom+" : "+err.Error())
		}
	}
	if len(echecs) > 0 {
		r := demander("Synchronisation impossible\n\n"+strings.Join(echecs, "\n")+"\n\nJouer avec la Sauvegarde locale ?", []string{"Oui", "Non"})
		if r == "Non" {
			return 0, nil
		}
	}
	if err := lancer(commande); err != nil {
		return 1, fmt.Errorf("lancement de %s : %w", commande[0], err)
	}
	echecs = nil
	for _, j := range jeux {
		if _, err := s.syncJeu(j, true); err != nil {
			s.journal(j.Cle + " après : " + err.Error())
			echecs = append(echecs, j.Nom+" : "+err.Error())
		}
	}
	if len(echecs) > 0 {
		notifier("Envoi impossible (" + strings.Join(echecs, " ; ") + ").\nLa Sauvegarde reste sur le PC ; elle partira au prochain lancement.")
	}
	return 0, nil
}

// migrerV2 : sync.config.json et base.json de sync.ps1 -> config.json et bases/ de 3dssavesync.
// Rien n'est jamais remplacé ; l'installation v2 n'est pas modifiée.
func migrerV2(depuis, config string) error {
	dossier := filepath.Dir(config)
	for _, f := range []string{config, filepath.Join(dossier, "bases")} {
		if _, err := os.Stat(f); err == nil {
			return fmt.Errorf("%s existe déjà : rien n'est remplacé", f)
		}
	}
	var v2 map[string]string
	if err := lireJSON(filepath.Join(depuis, "sync.config.json"), &v2); err != nil {
		return err
	}
	cfg := Config{URL: v2["url"], Jeton: v2["jeton"], Emulateurs: map[string]*Emulateur{}}
	ajouter := func(nom, exe string, sauvegardes map[string]string) {
		for s, d := range sauvegardes {
			if d == "" {
				delete(sauvegardes, s)
			}
		}
		if exe != "" && len(sauvegardes) > 0 {
			cfg.Emulateurs[nom] = &Emulateur{Exe: exe, Sauvegardes: sauvegardes}
		}
	}
	ajouter("melonds", v2["melonds"], map[string]string{"DS": v2["sauvegardes"]})
	ajouter("mgba", v2["mgba"], map[string]string{"GB": v2["sauvegardes_gb"], "GBA": v2["sauvegardes_gba"]})
	ajouter("azahar", v2["azahar"], map[string]string{"3DS": v2["azahar_sdmc"]})
	if v2["sauvegardes"] != "" {
		cfg.Copies = filepath.Join(filepath.Dir(v2["sauvegardes"]), "3dssavesync-copies")
	}
	base := map[string]string{}
	if err := lireJSON(filepath.Join(depuis, "base.json"), &base); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	bases := map[string]map[string]string{}
	for cle, h := range base {
		e := "melonds"
		if strings.HasPrefix(cle, "GB") {
			e = "mgba"
		} else if strings.HasPrefix(cle, "3DS-") {
			e = "azahar"
		}
		if bases[e] == nil {
			bases[e] = map[string]string{}
		}
		bases[e][cle] = h
	}
	if err := ecrireJSON(config, cfg, false); err != nil {
		return err
	}
	for e, m := range bases {
		if err := ecrireJSON(filepath.Join(dossier, "bases", e+".json"), m, false); err != nil {
			return err
		}
	}
	fmt.Printf("Configuration écrite : %s (%d émulateurs, %d Bases)\n", config, len(cfg.Emulateurs), len(base))
	return nil
}

func lireJSON(chemin string, v any) error {
	o, err := os.ReadFile(chemin)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes.TrimPrefix(o, []byte("\xef\xbb\xbf")), v); err != nil {
		return fmt.Errorf("%s : %w", chemin, err)
	}
	return nil
}
