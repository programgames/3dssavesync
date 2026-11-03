package main

// Mise à jour du programme PC depuis GitHub (ADR 0023) : recherchée au plus une fois par jour, installée seulement par
// le bouton « Mettre à jour » de la page. SHA256SUMS de la même release ; l'exe d'avant reste à côté, renommé.

import (
	"crypto/sha256"
	"crypto/tls"
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
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Version de ce programme : -ldflags "-X main.version=3.7.0" dans le job release. « dev » ne propose jamais de mise à jour.
var version = "dev"

// Adaptateurs remplacés par les tests : jamais GitHub, jamais le vrai exe.
var (
	adresseRelease = "https://api.github.com/repos/programgames/3dssavesync/releases/latest"
	renommer       = os.Rename
)

// pageVersion : la page GitHub de cette version (« dev » : la dernière).
func pageVersion() string {
	if version == "dev" {
		return pageReleases
	}
	return "https://github.com/programgames/3dssavesync/releases/tag/v" + version
}

// Recherche des mises à jour, sauf "verifier_maj": false.
func (c *Config) verifierMaj() bool { return c.VerifierMaj == nil || *c.VerifierMaj }

type Release struct {
	Tag    string `json:"tag_name"`
	Page   string `json:"html_url"`
	Assets []struct {
		Nom    string `json:"name"`
		URL    string `json:"browser_download_url"`
		Taille int64  `json:"size"`
	} `json:"assets"`
}

// "v3.10.2-rc.1" -> {3, 10, 2}, pre = true (comme maj.c du homebrew).
func lireVersion(s string) (n [3]int, pre, ok bool) {
	s, suite, pre := strings.Cut(strings.TrimPrefix(s, "v"), "-")
	p := strings.Split(s, ".")
	if len(p) != 3 || pre && suite == "" {
		return n, false, false
	}
	for i := range p {
		v, err := strconv.Atoi(p[i])
		if err != nil || v < 0 || p[i][0] == '+' {
			return n, false, false
		}
		n[i] = v
	}
	return n, pre, true
}

// plusRecente : distante après courante ; une version illisible (dev) n'est jamais plus ancienne.
func plusRecente(courante, distante string) bool {
	a, pa, oka := lireVersion(courante)
	b, pb, okb := lireVersion(distante)
	if !oka || !okb {
		return false
	}
	if c := slices.Compare(a[:], b[:]); c != 0 {
		return c < 0
	}
	return pa && !pb
}

func telecharger(url string, max int64) ([]byte, error) {
	r, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", "3dssavesync/"+version)
	rep, err := (&http.Client{Timeout: 5 * time.Minute, Transport: transport}).Do(r)
	if err != nil {
		return nil, err
	}
	defer rep.Body.Close()
	if rep.StatusCode != 200 {
		return nil, &erreurHTTP{"GET " + url, rep.StatusCode}
	}
	o, err := io.ReadAll(io.LimitReader(rep.Body, max+1))
	if err == nil && int64(len(o)) > max {
		err = fmt.Errorf("GET %s : %w (%d)", url, errTropGrand, max)
	}
	return o, err
}

// Échecs de GitHub reconnus par erreurGitHub.
var (
	errTropGrand = errors.New("réponse trop grande")
	errIllisible = errors.New("réponse illisible")
)

const pageReleases = "https://github.com/programgames/3dssavesync/releases/latest"

// erreurGitHub : la recherche ou le téléchargement a échoué ; ce qui s'est passé et quoi faire, puis le détail.
func erreurGitHub(err error) string {
	var h *erreurHTTP
	var cert *tls.CertificateVerificationError
	var dns *net.DNSError
	var reseau net.Error
	var ue *neturl.Error
	var t string
	switch {
	case errors.As(err, &h) && (h.code == 403 || h.code == 429):
		t = tr("GitHub limite pour l'instant les demandes venant de votre connexion. Réessayez dans une heure.",
			"GitHub is limiting requests from your connection for now. Try again in an hour.")
	case errors.As(err, &h) && h.code == 404:
		t = fmt.Sprintf(tr("Il manque un fichier dans cette version sur GitHub, la mise à jour n'a pas été faite. Téléchargez-la vous-même sur %s.",
			"A file is missing from this version on GitHub, so the update wasn't installed. Download it yourself from %s."), pageReleases)
	case errors.As(err, &h):
		t = tr("GitHub rencontre un problème. Réessayez plus tard.",
			"GitHub is having problems. Try again later.")
	case errors.Is(err, errTropGrand):
		t = fmt.Sprintf(tr("Le fichier de GitHub est plus gros que prévu, la mise à jour n'a pas été faite. Téléchargez la version vous-même sur %s.",
			"The file from GitHub is bigger than expected, so the update wasn't installed. Download the version yourself from %s."), pageReleases)
	case errors.Is(err, errIllisible):
		t = fmt.Sprintf(tr("GitHub a envoyé une réponse illisible. Réessayez plus tard. Si le problème continue, téléchargez la version vous-même sur %s.",
			"GitHub sent a response that couldn't be read. Try again later. If it keeps happening, download the version yourself from %s."), pageReleases)
	case errors.As(err, &cert):
		t = tr("Impossible d'établir une connexion sécurisée avec GitHub. Vérifiez la date et l'heure du PC et réessayez.",
			"Couldn't make a secure connection to GitHub. Check the PC's date and time and try again.")
	case errors.As(err, &dns), errors.As(err, &reseau), errors.As(err, &ue):
		t = tr("GitHub ne répond pas. Vérifiez votre connexion Internet et réessayez.",
			"GitHub isn't responding. Check your Internet connection and try again.")
	default:
		t = tr("Impossible de joindre GitHub. Réessayez plus tard.", "Couldn't reach GitHub. Try again later.")
	}
	return t + "\n" + tr("Détail : ", "Detail: ") + err.Error()
}

// verrouMaj : maj-pc.verrou, créé seul (O_EXCL) dans le dossier des réglages : une mise à jour à la fois (deux onglets,
// deux fenêtres de réglages). Un verrou de plus de 15 minutes vient d'un programme arrêté en route : il est remplacé.
// Rend la fonction qui le retire.
func verrouMaj(dossier string) (func(), error) {
	f := filepath.Join(dossier, "maj-pc.verrou")
	if i, err := os.Stat(f); err == nil && time.Since(i.ModTime()) > 15*time.Minute {
		os.Remove(f)
	}
	os.MkdirAll(dossier, 0o755)
	v, err := os.OpenFile(f, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("%s\n%s%s", tr("Une mise à jour est déjà en cours dans un autre onglet ou une autre fenêtre. Attendez qu'elle se termine, puis rechargez cette page.",
			"An update is already running in another tab or window. Wait for it to finish, then reload this page."), tr("Détail : ", "Detail: "), f)
	}
	if err != nil {
		return nil, fmt.Errorf("%s\n%s%v", tr("Impossible de mettre à jour : 3dssavesync ne peut pas écrire dans le dossier des réglages. Vérifiez les autorisations de ce dossier et réessayez.",
			"Couldn't update: 3dssavesync can't write to the settings folder. Check this folder's permissions and try again."), tr("Détail : ", "Detail: "), err)
	}
	v.Close()
	return func() { os.Remove(f) }, nil
}

// Dernière recherche, dans le dossier des réglages : le jour et la release vue.
type RechercheMaj struct {
	Jour    string
	Release Release
}

// derniereRelease : la dernière release de GitHub, lue au plus une fois par jour (maj-pc.json), sauf forcer.
func derniereRelease(dossier string, forcer bool) (*Release, error) {
	f := filepath.Join(dossier, "maj-pc.json")
	jour := time.Now().Format("2006-01-02")
	var vue RechercheMaj
	if !forcer && lireJSON(f, &vue) == nil && vue.Jour == jour && vue.Release.Tag != "" {
		return &vue.Release, nil
	}
	o, err := telecharger(adresseRelease, 4<<20)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(o, &r); err != nil || r.Tag == "" {
		return nil, fmt.Errorf("GET %s : %w (%v, tag_name %q)", adresseRelease, errIllisible, err, r.Tag)
	}
	ecrireJSON(f, RechercheMaj{jour, r}, true) // pas écrit (deux recherches en même temps) : une recherche de plus, sans gêne
	return &r, nil
}

// Nom du fichier de la release pour ce système : 3dssavesync-<os>-<arch>[.exe].
func nomFichierPC(goos, goarch string) string {
	if goos == "windows" {
		return "3dssavesync-" + goos + "-" + goarch + ".exe"
	}
	return "3dssavesync-" + goos + "-" + goarch
}

// sommeValide : la ligne « <sha256>  <nom> » de SHA256SUMS correspond à o.
func sommeValide(sommes, nom string, o []byte) bool {
	h := sha256.Sum256(o)
	for _, l := range strings.Split(sommes, "\n") {
		if c := strings.Fields(l); len(c) == 2 && strings.TrimPrefix(c[1], "*") == nom {
			return strings.EqualFold(c[0], hex.EncodeToString(h[:]))
		}
	}
	return false
}

// mettreAJour : télécharge le programme de ce système et SHA256SUMS, vérifie la taille, la somme et la place libre,
// puis remplace exe (remplacerExe). Tout échec laisse exe tel quel. Rend le chemin de l'exe d'avant, gardé.
func mettreAJour(r *Release, exe, goos, goarch string) (string, error) {
	nom := nomFichierPC(goos, goarch)
	var url, sommes string
	var taille int64
	for _, a := range r.Assets {
		switch a.Nom {
		case nom:
			url, taille = a.URL, a.Taille
		case "SHA256SUMS":
			sommes = a.URL
		}
	}
	if url == "" || sommes == "" {
		return "", fmt.Errorf(tr("La version %s sur GitHub est incomplète (fichier %s ou SHA256SUMS manquant). La mise à jour n'a pas été faite. Téléchargez-la vous-même sur %s.",
			"Version %s on GitHub is incomplete (%s or SHA256SUMS is missing). The update wasn't installed. Download it yourself from %s."), r.Tag, nom, r.Page)
	}
	s, err := telecharger(sommes, 1<<20)
	if err != nil {
		return "", fmt.Errorf("%s", erreurGitHub(err))
	}
	// Déjà fait (autre fenêtre, onglet resté ouvert) : l'exe sur le disque est celui de la release.
	if o, err := os.ReadFile(exe); err == nil && sommeValide(string(s), nom, o) {
		return "", fmt.Errorf(tr("La version %s est déjà installée, depuis une autre fenêtre. Fermez cet onglet et rouvrez le raccourci « 3dssavesync réglages ».\nDétail : %s",
			"Version %s is already installed from another window. Close this tab and open the \"3dssavesync settings\" shortcut again.\nDetail: %s"), strings.TrimPrefix(r.Tag, "v"), exe)
	}
	o, err := telecharger(url, 256<<20)
	if err != nil {
		return "", fmt.Errorf("%s", erreurGitHub(err))
	}
	if int64(len(o)) != taille || !sommeValide(string(s), nom, o) {
		return "", fmt.Errorf(tr("Le fichier téléchargé est abîmé, la mise à jour n'a pas été faite. Réessayez plus tard.\nDétail : %s, %d octets, %d attendus",
			"The downloaded file is damaged, so the update wasn't installed. Try again later.\nDetail: %s, %d bytes, %d expected"), nom, len(o), taille)
	}
	if libre, err := espaceLibre(filepath.Dir(exe)); err == nil && libre < uint64(len(o))+1<<20 {
		return "", fmt.Errorf(tr("Pas assez de place sur le disque pour la mise à jour (%d Mo libres, %d Mo nécessaires). Libérez de la place et réessayez.\nDétail : %s",
			"Not enough disk space for the update (%d MB free, %d MB needed). Free up some space and try again.\nDetail: %s"), libre>>20, len(o)>>20+1, filepath.Dir(exe))
	}
	return remplacerExe(exe, o, version)
}

// remplacerExe : le nouvel exe en fichier temporaire vérifié ; exe renommé 3dssavesync-<ancienne>[.exe] (Windows
// accepte de renommer un exe en marche) ; le temporaire prend sa place. Un échec remet l'ancien exe en place.
func remplacerExe(exe string, neuf []byte, ancienne string) (string, error) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = filepath.Ext(exe)
	}
	base := strings.TrimSuffix(exe, ext)
	copie := base + "-" + ancienne + ext
	for i := 1; existe(copie); i++ {
		copie = fmt.Sprintf("%s-%s-%d%s", base, ancienne, i, ext)
	}
	tmp := exe + ".3dssavesync-tmp"
	os.Remove(tmp)
	echec := func(err error) error {
		return fmt.Errorf("%s\n%s%v", tr("La mise à jour n'a pas pu remplacer le programme, l'ancien reste en place. Fermez les autres fenêtres de 3dssavesync et réessayez.",
			"The update couldn't replace the program, so the old one is still in place. Close the other 3dssavesync windows and try again."), tr("Détail : ", "Detail: "), err)
	}
	if err := ecrireVerifie(tmp, neuf); err != nil {
		return "", echec(err)
	}
	os.Chmod(tmp, 0o755)
	if err := renommer(exe, copie); err != nil {
		os.Remove(tmp)
		return "", echec(err)
	}
	if err := renommer(tmp, exe); err != nil {
		os.Remove(tmp)
		if e := renommer(copie, exe); e != nil {
			return "", fmt.Errorf(tr("La mise à jour a échoué et l'ancien programme n'a pas pu être remis en place. Pour le récupérer, renommez %s en %s.\nDétail : %v ; %v",
				"The update failed and the old program couldn't be put back. To get it back, rename %s to %s.\nDetail: %v; %v"), copie, filepath.Base(exe), err, e)
		}
		return "", echec(err)
	}
	return copie, nil
}

// mettreAJourWeb : bouton « Mettre à jour » de la page. Puis le nouvel exe redémarre l'icône (si elle tournait) et
// les réglages ; ce programme s'arrête.
func (a *AssistantWeb) mettreAJourWeb() map[string]any {
	dossier := filepath.Dir(a.config)
	exe := executable()
	liberer, err := verrouMaj(dossier)
	if err != nil {
		return map[string]any{"Ok": false, "Message": err.Error()}
	}
	defer liberer()
	r, err := derniereRelease(dossier, true)
	if err != nil {
		return map[string]any{"Ok": false, "Message": erreurGitHub(err)}
	}
	if exe == "" || !plusRecente(version, r.Tag) {
		return map[string]any{"Ok": false, "Message": fmt.Sprintf(tr("Vous avez déjà la dernière version (%s).", "You already have the latest version (%s)."), version)}
	}
	copie, err := mettreAJour(r, exe, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return map[string]any{"Ok": false, "Message": err.Error()}
	}
	(&Session{dossier: dossier}).journal("mise à jour : " + version + " -> " + r.Tag + ", ancien exe : " + copie)
	lignes := []string{fmt.Sprintf(tr("Mise à jour vers %s terminée. Les réglages se rouvrent dans un nouvel onglet, vous pouvez fermer celui-ci.",
		"Updated to %s. The settings page reopens in a new tab, so you can close this one."), strings.TrimPrefix(r.Tag, "v")),
		tr("Ancienne version conservée : ", "Previous version kept: ") + copie}
	if trayActif(dossier) {
		if err := arreterTray(dossier); err != nil {
			lignes = append(lignes, err.Error())
		} else if err := lancerServeur(exe, avecConfig([]string{"tray"}, a.config)); err != nil {
			lignes = append(lignes, tr("L'icône près de l'horloge n'a pas pu redémarrer. Elle reviendra à votre prochaine ouverture de session.", "The icon near the clock couldn't restart. It'll come back the next time you sign in.")+
				"\n"+tr("Détail : ", "Detail: ")+err.Error())
		}
	}
	if err := ouvrirDetache(exe, avecConfig([]string{"setup"}, a.config)); err != nil {
		lignes[0] = fmt.Sprintf(tr("Mise à jour vers %s terminée. Les réglages n'ont pas pu se rouvrir : ouvrez le raccourci « 3dssavesync réglages ».",
			"Updated to %s. The settings page couldn't reopen. Open the \"3dssavesync settings\" shortcut."), strings.TrimPrefix(r.Tag, "v")) +
			"\n" + tr("Détail : ", "Detail: ") + err.Error()
	}
	a.finir()
	return map[string]any{"Ok": true, "Message": strings.Join(lignes, "\n")}
}
