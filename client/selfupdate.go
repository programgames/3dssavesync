package main

// PC program update from GitHub (ADR 0023): checked at most once a day, installed only by the page's "update"
// button. SHA256SUMS from the same release; the previous exe stays next to it, renamed.

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

// version of this program: -ldflags "-X main.version=3.7.0" in the release job. "dev" never offers an update.
var version = "dev"

// Adapters that the tests replace: never GitHub, never the real exe.
var (
	releaseURL = "https://api.github.com/repos/programgames/3dssavesync/releases/latest"
	rename     = os.Rename
)

// versionPage returns the GitHub page of this version ("dev": the latest).
func versionPage() string {
	if version == "dev" {
		return releasesPage
	}
	return "https://github.com/programgames/3dssavesync/releases/tag/v" + version
}

// checkUpdate: look for updates, unless "verifier_maj": false.
func (c *Config) checkUpdate() bool { return c.CheckUpdates == nil || *c.CheckUpdates }

type Release struct {
	Tag    string `json:"tag_name"`
	Page   string `json:"html_url"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// readVersion: "v3.10.2-rc.1" -> {3, 10, 2}, pre = true (like maj.c in the homebrew).
func readVersion(s string) (n [3]int, pre, ok bool) {
	s, suffix, pre := strings.Cut(strings.TrimPrefix(s, "v"), "-")
	p := strings.Split(s, ".")
	if len(p) != 3 || pre && suffix == "" {
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

// newest: remote comes after current; an unreadable version (dev) is never older.
func newest(current, remote string) bool {
	a, pa, oka := readVersion(current)
	b, pb, okb := readVersion(remote)
	if !oka || !okb {
		return false
	}
	if c := slices.Compare(a[:], b[:]); c != 0 {
		return c < 0
	}
	return pa && !pb
}

func fetch(url string, max int64) ([]byte, error) {
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
		return nil, &httpError{"GET " + url, rep.StatusCode}
	}
	o, err := io.ReadAll(io.LimitReader(rep.Body, max+1))
	if err == nil && int64(len(o)) > max {
		err = fmt.Errorf("GET %s : %w (%d)", url, errTooBig, max)
	}
	return o, err
}

// GitHub failures that gitHubError recognizes.
var (
	errTooBig     = errors.New("réponse trop grande")
	errUnreadable = errors.New("réponse illisible")
)

const releasesPage = "https://github.com/programgames/3dssavesync/releases/latest"

// gitHubError: the check or the download failed; says what happened and what to do, then the detail.
func gitHubError(err error) string {
	var h *httpError
	var cert *tls.CertificateVerificationError
	var dns *net.DNSError
	var network net.Error
	var ue *neturl.Error
	var t string
	switch {
	case errors.As(err, &h) && (h.code == 403 || h.code == 429):
		t = tr("GitHub limite pour l'instant les demandes venant de votre connexion. Réessayez dans une heure.",
			"GitHub is limiting requests from your connection for now. Try again in an hour.")
	case errors.As(err, &h) && h.code == 404:
		t = fmt.Sprintf(tr("Il manque un fichier dans cette version sur GitHub, la mise à jour n'a pas été faite. Téléchargez-la vous-même sur %s.",
			"A file is missing from this version on GitHub, so the update wasn't installed. Download it yourself from %s."), releasesPage)
	case errors.As(err, &h):
		t = tr("GitHub rencontre un problème. Réessayez plus tard.",
			"GitHub is having problems. Try again later.")
	case errors.Is(err, errTooBig):
		t = fmt.Sprintf(tr("Le fichier de GitHub est plus gros que prévu, la mise à jour n'a pas été faite. Téléchargez la version vous-même sur %s.",
			"The file from GitHub is bigger than expected, so the update wasn't installed. Download the version yourself from %s."), releasesPage)
	case errors.Is(err, errUnreadable):
		t = fmt.Sprintf(tr("GitHub a envoyé une réponse illisible. Réessayez plus tard. Si le problème continue, téléchargez la version vous-même sur %s.",
			"GitHub sent a response that couldn't be read. Try again later. If it keeps happening, download the version yourself from %s."), releasesPage)
	case errors.As(err, &cert):
		t = tr("Impossible d'établir une connexion sécurisée avec GitHub. Vérifiez la date et l'heure du PC et réessayez.",
			"Couldn't make a secure connection to GitHub. Check the PC's date and time and try again.")
	case errors.As(err, &dns), errors.As(err, &network), errors.As(err, &ue):
		t = tr("GitHub ne répond pas. Vérifiez votre connexion Internet et réessayez.",
			"GitHub isn't responding. Check your Internet connection and try again.")
	default:
		t = tr("Impossible de joindre GitHub. Réessayez plus tard.", "Couldn't reach GitHub. Try again later.")
	}
	return t + "\n" + tr("Détail : ", "Detail: ") + err.Error()
}

// updateLock: maj-pc.verrou, created exclusively (O_EXCL) in the settings folder: one update at a time (two tabs,
// two settings windows). A lock older than 15 minutes comes from a program stopped midway: it is replaced.
// Returns the function that removes it.
func updateLock(dir string) (func(), error) {
	f := filepath.Join(dir, "maj-pc.verrou")
	if i, err := os.Stat(f); err == nil && time.Since(i.ModTime()) > 15*time.Minute {
		os.Remove(f)
	}
	os.MkdirAll(dir, 0o755)
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

// UpdateCheck is the last check, in the settings folder: the day and the release seen.
type UpdateCheck struct {
	Day     string `json:"Jour"`
	Release Release
}

// latestRelease returns the latest GitHub release, read at most once a day (maj-pc.json), unless force.
func latestRelease(dir string, force bool) (*Release, error) {
	f := filepath.Join(dir, "maj-pc.json")
	day := time.Now().Format("2006-01-02")
	var cached UpdateCheck
	if !force && readJSON(f, &cached) == nil && cached.Day == day && cached.Release.Tag != "" {
		return &cached.Release, nil
	}
	o, err := fetch(releaseURL, 4<<20)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(o, &r); err != nil || r.Tag == "" {
		return nil, fmt.Errorf("GET %s : %w (%v, tag_name %q)", releaseURL, errUnreadable, err, r.Tag)
	}
	writeJSON(f, UpdateCheck{day, r}, true) // if not written (two checks at once): one more check, harmless
	return &r, nil
}

// pcFileName returns the release file name for this OS: 3dssavesync-<os>-<arch>[.exe].
func pcFileName(goos, goarch string) string {
	if goos == "windows" {
		return "3dssavesync-" + goos + "-" + goarch + ".exe"
	}
	return "3dssavesync-" + goos + "-" + goarch
}

// validSum: the "<sha256>  <name>" line of SHA256SUMS matches o.
func validSum(sums, name string, o []byte) bool {
	h := sha256.Sum256(o)
	for _, l := range strings.Split(sums, "\n") {
		if c := strings.Fields(l); len(c) == 2 && strings.TrimPrefix(c[1], "*") == name {
			return strings.EqualFold(c[0], hex.EncodeToString(h[:]))
		}
	}
	return false
}

// selfUpdate downloads the program for this OS and SHA256SUMS, checks the size, the sum and the free space,
// then replaces exe (replaceExe). Any failure leaves exe as is. Returns the path of the previous exe, kept.
func selfUpdate(r *Release, exe, goos, goarch string) (string, error) {
	name := pcFileName(goos, goarch)
	var url, sums string
	var size int64
	for _, a := range r.Assets {
		switch a.Name {
		case name:
			url, size = a.URL, a.Size
		case "SHA256SUMS":
			sums = a.URL
		}
	}
	if url == "" || sums == "" {
		return "", fmt.Errorf(tr("La version %s sur GitHub est incomplète (fichier %s ou SHA256SUMS manquant). La mise à jour n'a pas été faite. Téléchargez-la vous-même sur %s.",
			"Version %s on GitHub is incomplete (%s or SHA256SUMS is missing). The update wasn't installed. Download it yourself from %s."), r.Tag, name, r.Page)
	}
	s, err := fetch(sums, 1<<20)
	if err != nil {
		return "", fmt.Errorf("%s", gitHubError(err))
	}
	// Already done (other window, tab left open): the exe on disk is the release one.
	if o, err := os.ReadFile(exe); err == nil && validSum(string(s), name, o) {
		return "", fmt.Errorf(tr("La version %s est déjà installée, depuis une autre fenêtre. Fermez cet onglet et rouvrez le raccourci « 3dssavesync réglages ».\nDétail : %s",
			"Version %s is already installed from another window. Close this tab and open the \"3dssavesync settings\" shortcut again.\nDetail: %s"), strings.TrimPrefix(r.Tag, "v"), exe)
	}
	o, err := fetch(url, 256<<20)
	if err != nil {
		return "", fmt.Errorf("%s", gitHubError(err))
	}
	if int64(len(o)) != size || !validSum(string(s), name, o) {
		return "", fmt.Errorf(tr("Le fichier téléchargé est abîmé, la mise à jour n'a pas été faite. Réessayez plus tard.\nDétail : %s, %d octets, %d attendus",
			"The downloaded file is damaged, so the update wasn't installed. Try again later.\nDetail: %s, %d bytes, %d expected"), name, len(o), size)
	}
	if free, err := freeSpace(filepath.Dir(exe)); err == nil && free < uint64(len(o))+1<<20 {
		return "", fmt.Errorf(tr("Pas assez de place sur le disque pour la mise à jour (%d Mo libres, %d Mo nécessaires). Libérez de la place et réessayez.\nDétail : %s",
			"Not enough disk space for the update (%d MB free, %d MB needed). Free up some space and try again.\nDetail: %s"), free>>20, len(o)>>20+1, filepath.Dir(exe))
	}
	return replaceExe(exe, o, version)
}

// replaceExe writes the new exe to a verified temp file; renames exe to 3dssavesync-<previous>[.exe] (Windows
// allows renaming a running exe); the temp file takes its place. A failure puts the old exe back.
func replaceExe(exe string, fresh []byte, prev string) (string, error) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = filepath.Ext(exe)
	}
	base := strings.TrimSuffix(exe, ext)
	cp := base + "-" + prev + ext
	for i := 1; exists(cp); i++ {
		cp = fmt.Sprintf("%s-%s-%d%s", base, prev, i, ext)
	}
	tmp := exe + ".3dssavesync-tmp"
	os.Remove(tmp)
	fail := func(err error) error {
		return fmt.Errorf("%s\n%s%v", tr("La mise à jour n'a pas pu remplacer le programme, l'ancien reste en place. Fermez les autres fenêtres de 3dssavesync et réessayez.",
			"The update couldn't replace the program, so the old one is still in place. Close the other 3dssavesync windows and try again."), tr("Détail : ", "Detail: "), err)
	}
	if err := writeVerified(tmp, fresh); err != nil {
		return "", fail(err)
	}
	os.Chmod(tmp, 0o755)
	if err := rename(exe, cp); err != nil {
		os.Remove(tmp)
		return "", fail(err)
	}
	if err := rename(tmp, exe); err != nil {
		os.Remove(tmp)
		if e := rename(cp, exe); e != nil {
			return "", fmt.Errorf(tr("La mise à jour a échoué et l'ancien programme n'a pas pu être remis en place. Pour le récupérer, renommez %s en %s.\nDétail : %v ; %v",
				"The update failed and the old program couldn't be put back. To get it back, rename %s to %s.\nDetail: %v; %v"), cp, filepath.Base(exe), err, e)
		}
		return "", fail(err)
	}
	return cp, nil
}

// selfUpdateWeb: the page's "update" button. Then the new exe restarts the tray icon (if it was running) and
// the settings page; this program stops.
func (a *WebSetup) selfUpdateWeb() map[string]any {
	dir := filepath.Dir(a.config)
	exe := executable()
	release, err := updateLock(dir)
	if err != nil {
		return map[string]any{"Ok": false, "Message": err.Error()}
	}
	defer release()
	r, err := latestRelease(dir, true)
	if err != nil {
		return map[string]any{"Ok": false, "Message": gitHubError(err)}
	}
	if exe == "" || !newest(version, r.Tag) {
		return map[string]any{"Ok": false, "Message": fmt.Sprintf(tr("Vous avez déjà la dernière version (%s).", "You already have the latest version (%s)."), version)}
	}
	cp, err := selfUpdate(r, exe, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return map[string]any{"Ok": false, "Message": err.Error()}
	}
	(&Session{dir: dir}).log("mise à jour : " + version + " -> " + r.Tag + ", ancien exe : " + cp)
	lines := []string{fmt.Sprintf(tr("Mise à jour vers %s terminée. Les réglages se rouvrent dans un nouvel onglet, vous pouvez fermer celui-ci.",
		"Updated to %s. The settings page reopens in a new tab, so you can close this one."), strings.TrimPrefix(r.Tag, "v")),
		tr("Ancienne version conservée : ", "Previous version kept: ") + cp}
	if trayActive(dir) {
		if err := stopTray(dir); err != nil {
			lines = append(lines, err.Error())
		} else if err := launchServer(exe, withConfig([]string{"tray"}, a.config)); err != nil {
			lines = append(lines, tr("L'icône près de l'horloge n'a pas pu redémarrer. Elle reviendra à votre prochaine ouverture de session.", "The icon near the clock couldn't restart. It'll come back the next time you sign in.")+
				"\n"+tr("Détail : ", "Detail: ")+err.Error())
		}
	}
	if err := openDetached(exe, withConfig([]string{"setup"}, a.config)); err != nil {
		lines[0] = fmt.Sprintf(tr("Mise à jour vers %s terminée. Les réglages n'ont pas pu se rouvrir : ouvrez le raccourci « 3dssavesync réglages ».",
			"Updated to %s. The settings page couldn't reopen. Open the \"3dssavesync settings\" shortcut."), strings.TrimPrefix(r.Tag, "v")) +
			"\n" + tr("Détail : ", "Detail: ") + err.Error()
	}
	a.finish()
	return map[string]any{"Ok": true, "Message": strings.Join(lines, "\n")}
}
