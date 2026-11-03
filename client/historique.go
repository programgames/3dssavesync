package main

// Restauration (GLOSSARY, ADR 0004, ADR 0016) : une ancienne Version de l'Historique du serveur redevient la Version
// courante, par un nouvel Envoi conditionnel. Aucun fichier local n'est écrit et aucune Base ne change : chaque
// appareil reçoit ensuite cette Version par sa Récupération normale, avec sa Copie de sécurité.

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

// Version de l'Historique, telle que GET /saves/<clé>/history la décrit (server.py, serveur.go).
type VersionHisto struct {
	Version
	Fichier string `json:"version"`
	Date    string `json:"date"`
}

// Une Version pour la page et la console. N : le <seq> du serveur, jamais renuméroté.
type LigneHisto struct {
	N                                           int
	Fichier, Hash, Date, Client, Taille, Partie string
	Courante                                    bool
}

type JeuHisto struct{ Cle, Nom string }

const (
	versionsMax    = 30
	clientRestaure = "restauration" // X-Client de l'Envoi : l'Historique montre qui a remis cette Version
)

var reVersion = regexp.MustCompile(`^[0-9]{6}-[0-9]{8}T[0-9]{6}Z-[a-z0-9]{1,16}-[0-9a-f]{64}\.sav$`)

func cleValide(cle string) bool { return reCode.MatchString("/saves/" + cle) }

// jeuxServeur : les jeux du serveur, avec le nom connu (titres 3DS, ROM trouvée par nomsRoms, ou nom donné au serveur
// par un Client : ADR 0018).
func (s *Session) jeuxServeur(noms map[string]string) ([]JeuHisto, error) {
	st, o, err := s.api("GET", "/saves", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, &erreurHTTP{"GET /saves", st}
	}
	var l []Version
	if err := json.Unmarshal(o, &l); err != nil {
		return nil, err
	}
	jeux := []JeuHisto{}
	for _, v := range l {
		nom := noms[v.Code]
		if strings.HasPrefix(v.Code, "3DS-") {
			nom = titres3DS[v.Code[4:]]
		}
		if nom == "" {
			nom = v.Nom
		}
		jeux = append(jeux, JeuHisto{v.Code, nom})
	}
	sort.SliceStable(jeux, func(a, b int) bool { return jeux[a].Nom+jeux[a].Cle < jeux[b].Nom+jeux[b].Cle })
	return jeux, nil
}

// nomsRoms : Clé -> nom de la ROM, dans les dossiers des ROM des émulateurs configurés.
// ponytail: un parcours complet, une seule fois par page ouverte ; un cache sur le disque si des milliers de ROM le rendent lent.
func nomsRoms(cfg *Config) map[string]string {
	m := map[string]string{}
	for _, e := range cfg.Emulateurs {
		for _, racine := range e.Roms {
			filepath.WalkDir(racine, func(chemin string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if _, cle, nom, _, err := lireRom(chemin); err == nil && cle != "" && m[cle] == "" {
						m[cle] = nom
					}
				}
				return nil
			})
		}
	}
	return m
}

// lignesHisto : les versionsMax dernières Versions d'un Jeu, la plus récente (la courante) d'abord.
// Pokémon : temps de jeu et dresseur de chaque Version, lue en entier.
// ponytail: jusqu'à 30 lectures de 512 Kio par liste ; à lire à la demande si un serveur distant lent le rend pénible.
func (s *Session) lignesHisto(cle string) ([]LigneHisto, error) {
	if !cleValide(cle) {
		return nil, fmt.Errorf(tr("clé de jeu invalide : %s. « 3dssavesync restore » affiche les clés des jeux du serveur.",
			"invalid game key: %s. \"3dssavesync restore\" shows the keys of the server games."), cle)
	}
	st, o, err := s.api("GET", "/saves/"+cle+"/history", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, &erreurHTTP{"GET /saves/" + cle + "/history", st}
	}
	var l []VersionHisto
	if err := json.Unmarshal(o, &l); err != nil {
		return nil, err
	}
	slices.Reverse(l)
	if len(l) > versionsMax {
		l = l[:versionsMax]
	}
	lignes := []LigneHisto{}
	for i, v := range l {
		n, _ := strconv.Atoi(strings.SplitN(v.Fichier, "-", 2)[0])
		x := LigneHisto{n, v.Fichier, v.Hash, dateLocale(v.Date), v.Client, formatTaille(v.Taille), "", i == 0}
		if _, g67 := formatsPokemon[int(v.Taille)]; g67 || v.Taille == 0x8000 || v.Taille == 0x20000 || v.Taille == 0x80000 {
			if st, o, err := s.api("GET", "/saves/"+cle+"/history/"+v.Fichier, nil, nil); err == nil && st == 200 {
				x.Partie, _ = partiePokemon(o)
			}
		}
		lignes = append(lignes, x)
	}
	return lignes, nil
}

// Date du serveur (UTC, horloge du serveur) en heure locale.
func dateLocale(d string) string {
	t, err := time.Parse("20060102T150405Z", d)
	if err != nil {
		return d
	}
	return t.Local().Format(tr("02/01/2006 15:04", "2006-01-02 15:04"))
}

func formatTaille(n int64) string {
	if n >= 1024 {
		return fmt.Sprintf(tr("%d Kio", "%d KiB"), (n+512)/1024)
	}
	return fmt.Sprintf(tr("%d octets", "%d bytes"), n)
}

// restaurer : la Version fichier redevient la Version courante, seulement si la courante est encore courant (le Hash
// vu par l'utilisateur) : un Envoi arrivé entre-temps est refusé, jamais écrasé. deja : rien à faire, même contenu.
func (s *Session) restaurer(cle, fichier, courant string) (deja bool, err error) {
	defer func() {
		if err != nil {
			s.journal("restauration " + cle + " " + fichier + " : " + err.Error())
		}
	}()
	if !cleValide(cle) || !reVersion.MatchString(fichier) {
		return false, fmt.Errorf(tr("version inconnue : %s %s", "unknown version: %s %s"), cle, fichier)
	}
	if hashDe(fichier) == courant {
		return true, nil
	}
	st, o, err := s.api("GET", "/saves/"+cle+"/history/"+fichier, nil, nil)
	if err != nil {
		return false, err
	}
	if st != 200 {
		return false, &erreurHTTP{"GET /saves/" + cle + "/history/" + fichier, st}
	}
	if hachage(o) != hashDe(fichier) {
		return false, errors.New(tr("la version reçue du serveur est abîmée (transfert incomplet) : rien n'est restauré. Réessayez.",
			"the version received from the server is damaged (incomplete transfer): nothing is restored. Try again."))
	}
	st, _, err = s.api("PUT", "/saves/"+cle, o, map[string]string{"If-Match": `"` + courant + `"`})
	if err != nil {
		return false, err
	}
	if st == 412 {
		return false, errors.New(tr("la sauvegarde a changé sur le serveur entre-temps (un appareil vient de l'envoyer) : rien n'est restauré. Affichez de nouveau la liste des versions, puis choisissez.",
			"the save changed on the server in the meantime (a device just uploaded it): nothing is restored. Show the version list again, then choose."))
	}
	if st != 200 {
		return false, &erreurHTTP{"PUT /saves/" + cle, st}
	}
	s.journal("restauration " + cle + " : " + fichier + " redevient la Version courante (avant : " + courant + ")")
	return false, nil
}

func texteRestaure() string {
	return tr("Version restaurée : elle est maintenant la version actuelle sur le serveur. Chaque appareil la reçoit à sa prochaine synchronisation.",
		"Version restored: it is now the current version on the server. Each device gets it at its next sync.")
}

func texteDeja() string {
	return tr("Cette version a le même contenu que la version actuelle : rien n'est changé.",
		"This version has the same content as the current version: nothing is changed.")
}

// Session de la Restauration : la configuration, et le nom de Client « restauration ».
func sessionRestauration(config string) (*Session, error) {
	s, err := nouvelleSession(config)
	if s != nil {
		s.profil = clientRestaure
	}
	return s, err
}

// Dans une console (l'exe Windows n'en a pas, comme repair) ; l'erreur y est affichée, sans fenêtre.
func commandeRestoreConsole(config string, args []string, oui bool) int {
	ouvrirConsole()
	q := &Questions{in: bufio.NewReader(os.Stdin), out: os.Stdout}
	code, err := commandeRestore(config, args, oui, q)
	if err != nil {
		q.dire("%s%s", tr("Erreur : ", "Error: "), texteErreur(err))
	}
	if consolePropre {
		q.dire("\n%s", tr("Appuyez sur Entrée pour fermer.", "Press Enter to close."))
		q.ligne()
	}
	return code
}

// 3dssavesync restore [--oui] [<clé> [<n°>]] : sans clé, les jeux du serveur ; sans n°, les Versions ; sinon la
// Restauration de la Version n°, après confirmation.
func commandeRestore(config string, args []string, oui bool, q *Questions) (int, error) {
	var reste []string
	for _, a := range args {
		if a == "--oui" || a == "-oui" {
			oui = true
		} else {
			reste = append(reste, a)
		}
	}
	if len(reste) > 2 {
		return 2, errors.New(tr("restore : une clé de jeu, puis le numéro de la version", "restore: one game key, then the version number"))
	}
	s, err := sessionRestauration(config)
	if err != nil {
		return 1, err
	}
	if len(reste) == 0 {
		jeux, err := s.jeuxServeur(nil)
		if err != nil {
			return 1, err
		}
		for _, j := range jeux {
			q.dire("%-22s %s", j.Cle, j.Nom)
		}
		q.dire(tr("Versions d'un jeu : 3dssavesync restore <clé>", "Versions of a game: 3dssavesync restore <key>"))
		return 0, nil
	}
	cle := strings.ToUpper(reste[0])
	lignes, err := s.lignesHisto(cle)
	if err != nil {
		return 1, err
	}
	if len(lignes) == 0 {
		return 1, fmt.Errorf(tr("aucune version de %s sur le serveur. « 3dssavesync restore » affiche les jeux du serveur.",
			"no version of %s on the server. \"3dssavesync restore\" shows the server games."), cle)
	}
	ligne := func(l LigneHisto) string {
		t := fmt.Sprintf("%4d  %s  %-12s %8s", l.N, l.Date, l.Client, l.Taille)
		if l.Partie != "" {
			t += "  " + l.Partie
		}
		if l.Courante {
			t += tr("  (actuelle)", "  (current)")
		}
		return t
	}
	if len(reste) == 1 {
		for _, l := range lignes {
			q.dire("%s", ligne(l))
		}
		q.dire(tr("Restaurer une version : 3dssavesync restore %s <n°>", "Restore a version: 3dssavesync restore %s <no.>"), cle)
		return 0, nil
	}
	n, _ := strconv.Atoi(reste[1])
	i := slices.IndexFunc(lignes, func(l LigneHisto) bool { return l.N == n })
	if i < 0 {
		return 1, fmt.Errorf(tr("pas de version n° %s parmi les %d dernières de %s. « 3dssavesync restore %s » affiche la liste.",
			"no version no. %s among the last %d of %s. \"3dssavesync restore %s\" shows the list."), reste[1], versionsMax, cle, cle)
	}
	if lignes[i].Hash == lignes[0].Hash {
		q.dire("%s", texteDeja())
		return 0, nil
	}
	q.dire("%s\n%s", ligne(lignes[i]), tr("Cette version devient la version actuelle sur le serveur. Aucune version n'est supprimée : la version actuelle reste dans l'historique.\n"+
		"Chaque appareil (3DS, PC) reçoit ensuite cette version à sa prochaine synchronisation, et fait d'abord une copie de sécurité de sa sauvegarde.\n"+
		"Synchronisez d'abord les autres appareils : une partie pas encore envoyée peut sinon remplacer cette version, ou afficher un conflit.",
		"This version becomes the current version on the server. No version is deleted: the current version stays in the history.\n"+
			"Each device (3DS, PC) then gets this version at its next sync, and first makes a backup copy of its save.\n"+
			"Sync the other devices first: otherwise a game not yet uploaded can replace this version, or show a conflict."))
	if !oui && !q.oui(tr("Restaurer cette version ?", "Restore this version?"), false) {
		q.dire(tr("Rien n'est changé.", "Nothing is changed."))
		return 0, nil
	}
	deja, err := s.restaurer(cle, lignes[i].Fichier, lignes[0].Hash)
	if err != nil {
		return 1, err
	}
	if deja {
		q.dire("%s", texteDeja())
	} else {
		q.dire("%s", texteRestaure())
	}
	return 0, nil
}

// Page de l'assistant : jeux, Versions, Restauration.
func (a *AssistantWeb) jeuxWeb() map[string]any {
	s, err := sessionRestauration(a.config)
	if err != nil {
		return map[string]any{"Erreur": texteErreur(err)}
	}
	a.nomsLus.Do(func() { a.noms = nomsRoms(s.cfg) }) // hors du verrou de la page : deux appels peuvent arriver ensemble
	jeux, err := s.jeuxServeur(a.noms)
	if err != nil {
		return map[string]any{"Erreur": texteErreur(err)}
	}
	return map[string]any{"Jeux": jeux}
}

// versionsWeb, restaurerWeb : sans le verrou de la page (jusqu'à 30 lectures sur le réseau), ServeHTTP le rend avant.
func (a *AssistantWeb) versionsWeb(cle string) map[string]any {
	s, err := sessionRestauration(a.config)
	if err == nil {
		var l []LigneHisto
		if l, err = s.lignesHisto(cle); err == nil {
			return map[string]any{"Versions": l}
		}
	}
	return map[string]any{"Erreur": texteErreur(err)}
}

func (a *AssistantWeb) restaurerWeb(cle, fichier, courant string) map[string]any {
	s, err := sessionRestauration(a.config)
	if err != nil {
		return map[string]any{"Ok": false, "Message": texteErreur(err)}
	}
	deja, err := s.restaurer(cle, fichier, courant)
	switch {
	case err != nil:
		return map[string]any{"Ok": false, "Message": texteErreur(err)}
	case deja:
		return map[string]any{"Ok": true, "Message": texteDeja()}
	}
	return map[string]any{"Ok": true, "Message": texteRestaure()}
}
