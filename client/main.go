// 3dssavesync : synchronise les sauvegardes d'un émulateur avec le serveur, avant et après la partie.
//
//	3dssavesync run [--emulator <nom>] <rom>             un jeu (frontends)
//	3dssavesync run [--emulator <nom>] --rom <rom> -- <commande…>  enveloppe une commande quelconque
//	3dssavesync launch <nom>                             tous les jeux d'un émulateur, puis l'émulateur
//	3dssavesync migrate-v2 <dossier de sync.ps1>         reprend sync.config.json et base.json
//	3dssavesync setup (ou sans argument)                 assistant d'installation
//	3dssavesync repair                                   refait la configuration des frontends
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
	Lanceur    string                `json:"lanceur,omitempty"`   // exe installé, vers lequel pointent les frontends
	Frontends  map[string]string     `json:"frontends,omitempty"` // launchbox, esde -> dossier (réparation)
}

type Emulateur struct {
	Profil      string            `json:"profil,omitempty"` // profil intégré (profils.go) ; défaut : le nom de l'entrée
	Exe         string            `json:"exe"`
	Sauvegardes map[string]string `json:"sauvegardes,omitempty"` // Système -> dossier (3DS : sdmc) ; défaut : réglage de l'émulateur
	Roms        map[string]string `json:"roms,omitempty"`        // Système -> dossier des ROM (commande launch)
	Coeurs      map[string]string `json:"coeurs,omitempty"`      // RetroArch : Système -> fichier du cœur
	Processus   string            `json:"processus,omitempty"`   // défaut : nom de l'exécutable
	// Profil personnalisé : arguments ({rom}, {sauvegardes}), extension (défaut .sav), nom du fichier interne d'une archive.
	Args       []string `json:"args,omitempty"`
	Extension  string   `json:"extension,omitempty"`
	NomInterne bool     `json:"nom_interne,omitempty"`
}

func (c *Config) dossierCopies(dossierConfig string) string {
	if c.Copies != "" {
		return c.Copies
	}
	return filepath.Join(dossierConfig, "copies")
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

	if len(args) == 0 { // double-clic sur l'exe : l'assistant
		args = []string{"setup"}
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
			err = errors.New(tr("launch : un nom d'émulateur", "launch: one emulator name"))
			break
		}
		code, err = commandeLaunch(*config, fl.Arg(0))
	case "setup":
		code, err = commandeSetup(*config)
	case "repair":
		code, err = commandeRepair(*config)
	case "migrate-v2":
		if fl.NArg() != 1 {
			err = errors.New(tr("migrate-v2 : le dossier de sync.ps1", "migrate-v2: the sync.ps1 folder"))
			break
		}
		err = migrerV2(fl.Arg(0), *config)
	default:
		err = fmt.Errorf(tr("commande inconnue : %s", "unknown command: %s"), args[0])
	}
	if err != nil {
		(&Session{dossier: dossier}).journal("erreur : " + err.Error() + " (" + strings.Join(args, " ") + ")")
		fmt.Fprintln(os.Stderr, err)
		demander(tr("Erreur de 3dssavesync : ", "3dssavesync error: ")+err.Error(), []string{"OK"})
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
	gere := func(nom string) bool {
		e := c.Emulateurs[nom]
		if e == nil {
			return false
		}
		p, err := e.profil(nom)
		return err == nil && e.gere(p, systeme)
	}
	if demande != "" {
		if !gere(demande) {
			return "", fmt.Errorf(tr("l'émulateur %s ne gère pas le Système %s", "emulator %s does not handle the %s system"), demande, systeme)
		}
		return demande, nil
	}
	var noms []string
	for nom := range c.Emulateurs {
		if gere(nom) {
			noms = append(noms, nom)
		}
	}
	sort.Strings(noms)
	switch len(noms) {
	case 0:
		return "", fmt.Errorf(tr("aucun émulateur configuré pour %s", "no emulator configured for %s"), systeme)
	case 1:
		return noms[0], nil
	}
	return "", fmt.Errorf(tr("plusieurs émulateurs pour %s (%s) : précise --emulator", "several emulators for %s (%s): use --emulator"), systeme, strings.Join(noms, ", "))
}

func commandeRun(config, demande, rom string, reste []string) (int, error) {
	commande := reste
	if rom == "" {
		if len(reste) != 1 {
			return 2, errors.New(tr("run : une ROM, ou --rom <rom> -- <commande>", "run: one ROM, or --rom <rom> -- <command>"))
		}
		rom, commande = reste[0], nil
	} else if len(reste) == 0 {
		return 2, errors.New(tr("run --rom : la commande à lancer manque après --", "run --rom: the command to run is missing after --"))
	}
	s, err := nouvelleSession(config)
	if err != nil {
		return 1, err
	}
	systeme, cle, nom, interne, err := lireRom(rom)
	s.journal(fmt.Sprintf("lancement %s %s : %s", systeme, cle, rom))
	if err != nil {
		return 1, err
	}
	if systeme == "" {
		return 1, fmt.Errorf(tr("ROM non reconnue (extension) : %s", "unknown ROM (extension): %s"), rom)
	}
	if s.profil, err = s.cfg.emulateurPour(systeme, demande); err != nil {
		return 1, err
	}
	e := s.cfg.Emulateurs[s.profil]
	p, err := e.profil(s.profil)
	if err != nil {
		return 1, err
	}
	if s.refus(e) {
		return 1, nil
	}
	dossier := e.dossier(p, systeme, rom)
	if commande == nil {
		commande = append([]string{e.Exe}, p.Args(e, systeme, rom, dossier)...)
	}
	var jeux []*Jeu
	switch {
	case cle == "": // homebrew, ou 3DS hors des 8 Pokémon
		s.journal("sans synchronisation : Clé illisible ou jeu hors périmètre")
	case dossier == "":
		return 1, fmt.Errorf(tr("dossier des sauvegardes %s de %s introuvable : précise-le dans sauvegardes", "%s save folder of %s not found: set it in sauvegardes"), systeme, s.profil)
	default:
		if p.NomInterne && interne != "" {
			nom = interne
		}
		jeux = []*Jeu{{systeme, cle, nom, fichierSauvegarde(dossier, systeme, cle, nom, p.Ext)}}
	}
	return s.jouer(jeux, commande, conseil(e, p))
}

// Réglage qui recharge un état au lancement : il peut remplacer une Version récupérée par une partie plus ancienne.
func conseil(e *Emulateur, p Profil) string {
	if p.Autoload == nil {
		return ""
	}
	return p.Autoload(e)
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
		return 1, fmt.Errorf(tr("émulateur absent de la configuration : %s", "emulator missing from the configuration: %s"), nom)
	}
	p, err := e.profil(nom)
	if err != nil {
		return 1, err
	}
	if s.refus(e) {
		return 1, nil
	}
	// mGBA sans jeu : un seul savegamePath possible, donc seulement si GB et GBA partagent le dossier.
	var dossier string
	for i, sys := range e.systemes(p) {
		if d := e.dossier(p, sys, ""); i > 0 && d != dossier {
			dossier = ""
			break
		} else {
			dossier = d
		}
	}
	return s.jouer(s.jeuxDe(e, p), append([]string{e.Exe}, p.Args(e, "", "", dossier)...), conseil(e, p))
}

// L'émulateur tourne déjà : synchroniser maintenant écraserait ou enverrait une partie en cours.
func (s *Session) refus(e *Emulateur) bool {
	nom := e.processus()
	if !emulateurActif(nom) {
		return false
	}
	s.journal("refus : " + nom + " tourne déjà")
	notifier(nom + tr(" tourne déjà. Ferme-le avant de lancer un jeu.", " is already running. Close it before starting a game."))
	return true
}

// Jeux d'un émulateur pour launch : les ROM de ses dossiers roms, et les Pokémon 3DS déjà vus par Azahar.
func (s *Session) jeuxDe(e *Emulateur, p Profil) []*Jeu {
	var jeux []*Jeu
	vus := map[string]bool{}
	for _, systeme := range e.systemes(p) {
		if systeme == "3DS" {
			dossier := e.dossier(p, systeme, "")
			for id, titre := range titres3DS {
				cle := "3DS-" + id
				j := &Jeu{systeme, cle, titre, fichierSauvegarde(dossier, systeme, cle, titre, "")}
				if dossier != "" && existe(filepath.Dir(j.Fichier)) { // jamais lancé dans Azahar : ignoré
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
			sys, cle, nom, interne, err := lireRom(chemin)
			if err == nil && sys == systeme && cle != "" && !vus[cle] {
				vus[cle] = true
				if p.NomInterne && interne != "" {
					nom = interne
				}
				jeux = append(jeux, &Jeu{sys, cle, nom, fichierSauvegarde(e.dossier(p, sys, chemin), sys, cle, nom, p.Ext)})
			}
			return nil
		})
	}
	sort.Slice(jeux, func(a, b int) bool { return jeux[a].Nom < jeux[b].Nom })
	return jeux
}

// Synchronisation avant, partie, Synchronisation après.
// ponytail: une requête GET /saves par Jeu et par passe ; une seule liste par passe si launch devient lent.
func (s *Session) jouer(jeux []*Jeu, commande []string, conseil string) (int, error) {
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
		non := tr("Non", "No")
		r := demander(tr("Synchronisation impossible", "Sync failed")+"\n\n"+strings.Join(echecs, "\n")+"\n\n"+
			tr("Jouer avec la Sauvegarde locale ?", "Play with the local save?"), []string{tr("Oui", "Yes"), non})
		if r == non {
			return 0, nil
		}
	}
	if conseil != "" && s.recupere {
		pasJouer := tr("Ne pas jouer", "Don't play")
		r := demander(tr("Une Sauvegarde vient d'arriver du serveur.", "A save just came from the server.")+"\n\n"+conseil+"\n\n"+
			tr("Sinon, une sauvegarde en jeu peut la remplacer par une partie plus ancienne.", "Otherwise, an in-game save can replace it with an older game."),
			[]string{tr("Jouer quand même", "Play anyway"), pasJouer})
		if r == pasJouer {
			return 0, nil
		}
		if r == "" { // pas de fenêtre : la partie a lieu, l'avertissement reste visible
			notifier(conseil)
		}
	} else if conseil != "" {
		s.journal("avertissement : " + conseil)
	}
	if err := lancer(commande); err != nil {
		return 1, fmt.Errorf(tr("lancement de %s : %w", "cannot start %s: %w"), commande[0], err)
	}
	echecs = nil
	for _, j := range jeux {
		if _, err := s.syncJeu(j, true); err != nil {
			s.journal(j.Cle + " après : " + err.Error())
			echecs = append(echecs, j.Nom+" : "+err.Error())
		}
	}
	if len(echecs) > 0 {
		notifier(tr("Envoi impossible (", "Upload failed (") + strings.Join(echecs, " ; ") + tr(").\nLa Sauvegarde reste sur le PC ; elle partira au prochain lancement.", ").\nThe save stays on the PC; it will be sent at the next launch."))
	}
	return 0, nil
}

// migrerV2 : sync.config.json et base.json de sync.ps1 -> config.json et bases/ de 3dssavesync.
// Rien n'est jamais remplacé ; l'installation v2 n'est pas modifiée.
func migrerV2(depuis, config string) error {
	dossier := filepath.Dir(config)
	for _, f := range []string{config, filepath.Join(dossier, "bases")} {
		if _, err := os.Stat(f); err == nil {
			return fmt.Errorf(tr("%s existe déjà : rien n'est remplacé", "%s already exists: nothing is replaced"), f)
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
