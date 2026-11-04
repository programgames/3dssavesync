package main

// 3DS SD card prepared from the PC: the homebrew's config.ini with the Server address and token, so that the 3DS
// connects without typing anything, and cias/3dssavesync.cia of the same version as this program (ADR 0023), to
// install with FBI. Only these files are written; the old ones are copied first.

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// embarque/ holds 3dssavesync.cia, copied there by the CI release job before go build; elsewhere only LISEZMOI.txt.
//
//go:embed embarque
var embedded embed.FS

// embeddedCia is the .cia inside this program (nil: local or test build). Tests replace it.
var embeddedCia, _ = fs.ReadFile(embedded, "embarque/3dssavesync.cia")

// freeSpace returns the free space on a folder's disk, in bytes. Tests replace it.
var freeSpace = systemFreeSpace

// fbiPage is the latest FBI release, to install a .cia on the 3DS (Steveice10/FBI is no longer maintained).
const fbiPage = "https://github.com/nh-server/FBI-NH/releases/latest"

// address3DS returns the Server address for the 3DS, and the other possible ones. Local server: the PC's network
// address (the 127.0.0.1 of config.json doesn't lead to the PC from the 3DS).
func address3DS(cfg *Config) (string, []string) {
	if cfg.LocalServer == nil {
		return cfg.URL, nil
	}
	var l []string
	for _, ip := range localAddresses() {
		l = append(l, fmt.Sprintf("http://%s:%d", ip, cfg.LocalServer.Port))
	}
	if len(l) == 0 {
		return "", nil
	}
	return l[0], l[1:]
}

// cards3DS keeps the roots that look like a 3DS SD card: a "Nintendo 3DS" folder.
func cards3DS(roots []string) []string {
	var l []string
	for _, r := range roots {
		if exists(filepath.Join(r, "Nintendo 3DS")) {
			l = append(l, r)
		}
	}
	return l
}

// detectCards: tests replace it, never a real SD card.
var detectCards = func() []string { return cards3DS(removableRoots()) }

// configIni builds the homebrew's config.ini (main.c, cle_config): url and token first, other lines (roms…) kept.
func configIni(old, url, token string) string {
	t := "url=" + url + "\njeton=" + token + "\n"
	for _, l := range strings.Split(strings.ReplaceAll(old, "\r\n", "\n"), "\n") {
		if l != "" && !strings.HasPrefix(l, "url=") && !strings.HasPrefix(l, "jeton=") {
			t += l + "\n"
		}
	}
	return t
}

// prepareCard writes <root>/3ds/3dssavesync/config.ini: dated copy of the old one, verified temp file, rename, reread.
// Returns the written path.
func prepareCard(root, url, token string, alias map[string]string) (string, error) {
	if url == "" || token == "" {
		return "", errors.New(tr("Enregistrez d'abord le serveur.", "Save the server first."))
	}
	if len(url) > 255 || len(token) > 255 || strings.ContainsAny(url+token, "\r\n") { // sizes of cfg in main.c
		return "", errors.New(tr("L'adresse ou le jeton est trop long pour la 3DS.", "The address or token is too long for the 3DS."))
	}
	root, err := cardRoot(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "3ds", "3dssavesync", "config.ini")
	if old := filepath.Join(root, "3ds", "nds-save-sync", "config.ini"); !exists(path) && exists(old) {
		path = old // old nds-save-sync folder: the homebrew takes it over at start, with base.ini (migrer_dossier)
	}
	o, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := writeCard(path, []byte(configIni(string(o), url, token))); err != nil {
		return "", err
	}
	if len(alias) > 0 { // ROM hacks: same Keys on the 3DS as on this PC (ADR 0014)
		f := filepath.Join(filepath.Dir(path), "alias.ini")
		a, err := os.ReadFile(f)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err := writeCard(f, []byte(aliasIni(string(a), alias))); err != nil {
			return "", err
		}
	}
	return path, nil
}

// cardRoot returns the chosen root (quotes and spaces removed, "E:" -> "E:\"), if it is a 3DS SD card.
func cardRoot(root string) (string, error) {
	root = strings.Trim(strings.TrimSpace(root), `"`)
	if root == "" {
		return "", errors.New(tr("Choisissez la carte SD de la 3DS.", "Choose the 3DS SD card."))
	}
	if len(root) == 2 && root[1] == ':' {
		root += `\` // "E:" alone = current folder of drive E
	}
	if !filepath.IsAbs(root) || !exists(filepath.Join(root, "Nintendo 3DS")) {
		return "", fmt.Errorf(tr("%s n'est pas une carte SD de 3DS (il manque le dossier « Nintendo 3DS »). Choisissez la racine de la carte.",
			"%s isn't a 3DS SD card (there's no \"Nintendo 3DS\" folder). Choose the root of the card."), root)
	}
	return root, nil
}

// installedVersion returns the version that last ran on the 3DS (installee= of maj.ini, written by main.c), or "".
func installedVersion(root string) string {
	o, _ := os.ReadFile(filepath.Join(root, "3ds", "3dssavesync", "maj.ini"))
	for _, l := range strings.Split(string(o), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "installee="); ok {
			return v
		}
	}
	return ""
}

// copyCia writes <root>/cias/3dssavesync.cia, the .cia inside this program (same version), unless the 3DS already
// has a newer version (updated from the 3DS). Returns the lines to show: file written or already there, .cia missing
// from this program (where to download it), FBI missing from the card (not an error).
func copyCia(root string, cia []byte) ([]string, error) {
	var lines []string
	path := filepath.Join(root, "cias", "3dssavesync.cia")
	if v := installedVersion(root); newest(version, v) {
		return []string{fmt.Sprintf(tr("La 3DS a déjà une version plus récente de 3dssavesync (%s). Inutile d'installer la version %s.",
			"The 3DS already has a newer 3dssavesync (%s). There's no need to install version %s."), v, version)}, nil
	}
	if len(cia) == 0 {
		lines = append(lines, fmt.Sprintf(tr("Ce programme ne contient pas 3dssavesync.cia (version %s). Téléchargez-le sur %s et copiez-le dans le dossier cias de la carte SD.",
			"This program doesn't include 3dssavesync.cia (version %s). Download it from %s and copy it to the cias folder on the SD card."), version, versionPage()))
	} else {
		old, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if bytes.Equal(old, cia) {
			lines = append(lines, fmt.Sprintf(tr("3dssavesync.cia %s déjà sur la carte : ", "3dssavesync.cia %s is already on the card: "), version)+path)
		} else {
			needed := uint64(len(cia)+len(old)) + 1<<20 // temp file, dated copy of the old one, 1 MB margin
			if free, err := freeSpace(root); err == nil && free < needed {
				return nil, fmt.Errorf(tr("Pas assez de place sur la carte SD (%d Mo libres, %d Mo nécessaires). Libérez de la place et réessayez.\nDétail : %s",
					"Not enough space on the SD card (%d MB free, %d MB needed). Free up some space and try again.\nDetail: %s"), free>>20, (needed+1<<20-1)>>20, root)
			}
			if err := writeCard(path, cia); err != nil {
				return nil, err
			}
			lines = append(lines, fmt.Sprintf(tr("3dssavesync.cia %s copié : ", "3dssavesync.cia %s copied: "), version)+path)
		}
	}
	if !exists(filepath.Join(root, "3ds", "FBI.3dsx")) && !exists(filepath.Join(root, "3ds", "FBI")) {
		lines = append(lines, fmt.Sprintf(tr("FBI n'est pas sur la carte. S'il est déjà installé sur le menu HOME, ignorez ce message. Sinon, téléchargez FBI.3dsx sur %s, copiez-le dans le dossier 3ds de la carte et ouvrez-le depuis le Homebrew Launcher.",
			"FBI isn't on the card. If it's already installed on the HOME Menu, ignore this message. Otherwise, download FBI.3dsx from %s, copy it to the 3ds folder on the card and open it from the Homebrew Launcher."), fbiPage))
	}
	return lines, nil
}

// prepareAndRecord writes config.ini, then 3dssavesync.cia, then the date in config.json ("All set" list). Returns ok
// and the lines to show; a failure says what to do, then the technical detail.
func prepareAndRecord(config, root string) (bool, []string) {
	var cfg Config
	if err := readConfig(config, &cfg); err != nil {
		return false, []string{errorText(err)}
	}
	url, _ := address3DS(&cfg)
	f, err := prepareCard(root, url, cfg.Token, cfg.Alias)
	if err != nil {
		return false, []string{cardError(err)}
	}
	lines := []string{tr("config.ini écrit (adresse et jeton) : ", "config.ini written (address and token): ") + f}
	root, _ = cardRoot(root)
	l, err := copyCia(root, embeddedCia)
	lines = append(lines, l...)
	if err != nil {
		return false, append(lines, cardError(err))
	}
	cfg.PreparedCard = time.Now().Format("2006-01-02 15:04")
	if err := writeConfigCopy(config, &cfg); err != nil {
		lines = append(lines, tr("La date de préparation n'a pas pu être notée : ", "Couldn't record the preparation date: ")+cardError(err))
	}
	if newest(version, installedVersion(root)) { // .cia not copied: nothing to install
		return true, append(lines, tr("Remettez la carte dans la 3DS et lancez 3dssavesync : il se connecte tout seul.",
			"Put the card back in the 3DS and start 3dssavesync. It connects by itself."))
	}
	return true, append(lines, tr("Remettez la carte dans la 3DS. Dans FBI, ouvrez SD, cias, 3dssavesync.cia et choisissez « Install and delete CIA ». Lancez ensuite 3dssavesync depuis le menu HOME : il se connecte tout seul.",
		"Put the card back in the 3DS. In FBI, open SD, cias, 3dssavesync.cia and choose \"Install and delete CIA\". Then start 3dssavesync from the HOME Menu. It connects by itself."))
}

// cardError says what happened and what to do, then the technical detail on its own line.
func cardError(err error) string {
	t := errorText(err)
	if protected(err) {
		t = tr("La carte SD est verrouillée en écriture. Retirez-la, poussez vers le haut le petit loquet sur le côté (ou sur l'adaptateur microSD), remettez-la et réessayez.",
			"The SD card is write-protected. Take it out, slide the small lock switch on its side (or on the microSD adapter) up, put it back and try again.")
	}
	if t != err.Error() {
		t += "\n" + tr("Détail : ", "Detail: ") + err.Error()
	}
	return t
}

// protected: write refused by a locked medium (ERROR_WRITE_PROTECT on Windows, EROFS elsewhere).
func protected(err error) bool {
	var e syscall.Errno
	return errors.As(err, &e) && (runtime.GOOS == "windows" && e == 19 || runtime.GOOS != "windows" && e == syscall.EROFS)
}

// writeCard does nothing if the content is already there; else dated copy, verified temp file, rename, reread.
func writeCard(path string, content []byte) error {
	if o, err := os.ReadFile(path); err == nil && bytes.Equal(o, content) {
		return nil
	}
	if err := replaceFile(path, content); err != nil {
		return err
	}
	if reread, err := os.ReadFile(path); err != nil || !bytes.Equal(reread, content) {
		return fmt.Errorf(tr("%s ne s'est pas écrit correctement. Vérifiez la carte SD et réessayez.", "%s wasn't written correctly. Check the SD card and try again."), path)
	}
	return nil
}

// aliasIni builds the 3DS alias.ini ("ROM name=Key") with this PC's Aliases; the other lines stay.
func aliasIni(old string, alias map[string]string) string {
	t := ""
	for _, l := range strings.Split(strings.ReplaceAll(old, "\r\n", "\n"), "\n") {
		if name, _, ok := strings.Cut(l, "="); l != "" && (!ok || alias[name] == "") {
			t += l + "\n"
		}
	}
	names := make([]string, 0, len(alias))
	for n := range alias {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t += n + "=" + alias[n] + "\n"
	}
	return t
}

// sdCardStep is the console setup step for the 3DS SD card, after the Server.
func sdCardStep(q *Questions, cfg *Config, config string) {
	url, _ := address3DS(cfg)
	if url == "" {
		return
	}
	q.say("\n== %s ==", tr("Carte SD de la 3DS", "3DS SD card"))
	q.say("%s", tr("Mettez la carte SD de la 3DS dans le PC. 3dssavesync y écrit l'adresse et le jeton, et copie l'application 3DS dans cias.",
		"Put the 3DS SD card in the PC. 3dssavesync writes the address and token to it, and copies the 3DS app to cias."))
	cards := detectCards()
	for i, c := range cards {
		q.say("  %d. %s", i+1, c)
	}
	r := q.text(tr("Numéro ou dossier de la carte (Entrée pour passer)", "Card number or folder (Enter to skip)"), "")
	if i, err := strconv.Atoi(r); err == nil && i >= 1 && i <= len(cards) {
		r = cards[i-1]
	}
	if r == "" {
		return
	}
	_, lines := prepareAndRecord(config, r)
	for _, l := range lines {
		q.say("- %s", l)
	}
}
