package main

// Carte SD de la 3DS préparée depuis le PC : config.ini du homebrew avec l'adresse du serveur et le jeton, pour que la
// 3DS se connecte sans rien taper, et cias/3dssavesync.cia de la même version que ce programme (ADR 0023), à
// installer avec FBI. Seuls ces fichiers sont écrits ; les anciens sont copiés avant.

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

// embarque/ : 3dssavesync.cia, copié là par le job release de la CI avant go build ; seulement LISEZMOI.txt ailleurs.
//
//go:embed embarque
var embedded embed.FS

// Le .cia inclus dans ce programme (nil : build local ou de test). Remplacé par les tests.
var embeddedCia, _ = fs.ReadFile(embedded, "embarque/3dssavesync.cia")

// Place libre sur le disque d'un dossier, en octets. Remplacé par les tests.
var freeSpace = systemFreeSpace

// Dernière version de FBI, pour installer un .cia sur la 3DS (Steveice10/FBI n'évolue plus).
const fbiPage = "https://github.com/nh-server/FBI-NH/releases/latest"

// Adresse du serveur pour la 3DS, et les autres possibles. Serveur local : l'adresse du PC sur le réseau
// (127.0.0.1 de config.json ne mène pas au PC depuis la 3DS).
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

// Racines qui ressemblent à une carte SD de 3DS : un dossier « Nintendo 3DS ».
func cards3DS(roots []string) []string {
	var l []string
	for _, r := range roots {
		if exists(filepath.Join(r, "Nintendo 3DS")) {
			l = append(l, r)
		}
	}
	return l
}

// Remplacé par les tests : jamais de vraie carte SD.
var detectCards = func() []string { return cards3DS(removableRoots()) }

// config.ini du homebrew (main.c, cle_config) : url et jeton en tête, les autres lignes (roms, saves…) gardées.
func configIni(old, url, token string) string {
	t := "url=" + url + "\njeton=" + token + "\n"
	for _, l := range strings.Split(strings.ReplaceAll(old, "\r\n", "\n"), "\n") {
		if l != "" && !strings.HasPrefix(l, "url=") && !strings.HasPrefix(l, "jeton=") {
			t += l + "\n"
		}
	}
	return t
}

// Écrit <racine>/3ds/3dssavesync/config.ini : copie datée de l'ancien, fichier temporaire vérifié, renommage, relecture.
// Rend le chemin écrit.
func prepareCard(root, url, token string, alias map[string]string) (string, error) {
	if url == "" || token == "" {
		return "", errors.New(tr("Enregistrez d'abord le serveur.", "Save the server first."))
	}
	if len(url) > 255 || len(token) > 255 || strings.ContainsAny(url+token, "\r\n") { // tailles de cfg dans main.c
		return "", errors.New(tr("L'adresse ou le jeton est trop long pour la 3DS.", "The address or token is too long for the 3DS."))
	}
	root, err := cardRoot(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "3ds", "3dssavesync", "config.ini")
	if old := filepath.Join(root, "3ds", "nds-save-sync", "config.ini"); !exists(path) && exists(old) {
		path = old // dossier d'avant la v3 : le homebrew le reprend au démarrage, avec base.ini (migrer_dossier)
	}
	o, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := writeCard(path, []byte(configIni(string(o), url, token))); err != nil {
		return "", err
	}
	if len(alias) > 0 { // ROM hacks : mêmes Clés sur la 3DS que sur ce PC (ADR 0014)
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

// racineCarte : la racine choisie (guillemets et espaces retirés, « E: » -> « E:\ »), si c'est une carte SD de 3DS.
func cardRoot(root string) (string, error) {
	root = strings.Trim(strings.TrimSpace(root), `"`)
	if root == "" {
		return "", errors.New(tr("Choisissez la carte SD de la 3DS.", "Choose the 3DS SD card."))
	}
	if len(root) == 2 && root[1] == ':' {
		root += `\` // « E: » seul = dossier courant du lecteur E
	}
	if !filepath.IsAbs(root) || !exists(filepath.Join(root, "Nintendo 3DS")) {
		return "", fmt.Errorf(tr("%s n'est pas une carte SD de 3DS (il manque le dossier « Nintendo 3DS »). Choisissez la racine de la carte.",
			"%s isn't a 3DS SD card (there's no \"Nintendo 3DS\" folder). Choose the root of the card."), root)
	}
	return root, nil
}

// versionInstallee : la version qui a tourné en dernier sur la 3DS (installee= de maj.ini, écrit par main.c), ou "".
func installedVersion(root string) string {
	o, _ := os.ReadFile(filepath.Join(root, "3ds", "3dssavesync", "maj.ini"))
	for _, l := range strings.Split(string(o), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "installee="); ok {
			return v
		}
	}
	return ""
}

// copierCia : <racine>/cias/3dssavesync.cia, le .cia inclus dans ce programme (même version), sauf si la 3DS a déjà
// une version plus récente (mise à jour depuis la 3DS). Rend les lignes à afficher : fichier écrit ou déjà là, .cia
// absent de ce programme (où le télécharger), FBI absent de la carte (pas une erreur).
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
			needed := uint64(len(cia)+len(old)) + 1<<20 // fichier temporaire, copie datée de l'ancien, 1 Mo de marge
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

// preparerEtNoter : config.ini, puis 3dssavesync.cia, puis la date dans config.json (liste « Tout est prêt »). Rend
// ok et les lignes à afficher ; un échec dit quoi faire, puis le détail technique.
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
	if newest(version, installedVersion(root)) { // .cia pas copié : rien à installer
		return true, append(lines, tr("Remettez la carte dans la 3DS et lancez 3dssavesync : il se connecte tout seul.",
			"Put the card back in the 3DS and start 3dssavesync. It connects by itself."))
	}
	return true, append(lines, tr("Remettez la carte dans la 3DS. Dans FBI, ouvrez SD, cias, 3dssavesync.cia et choisissez « Install and delete CIA ». Lancez ensuite 3dssavesync depuis le menu HOME : il se connecte tout seul.",
		"Put the card back in the 3DS. In FBI, open SD, cias, 3dssavesync.cia and choose \"Install and delete CIA\". Then start 3dssavesync from the HOME Menu. It connects by itself."))
}

// erreurCarte : ce qui s'est passé et quoi faire, puis le détail technique sur sa ligne.
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

// protegee : écriture refusée par un support verrouillé (ERROR_WRITE_PROTECT sous Windows, EROFS ailleurs).
func protected(err error) bool {
	var e syscall.Errno
	return errors.As(err, &e) && (runtime.GOOS == "windows" && e == 19 || runtime.GOOS != "windows" && e == syscall.EROFS)
}

// ecrireCarte : rien si le contenu est déjà là ; sinon copie datée, fichier temporaire vérifié, renommage, relecture.
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

// aliasIni : alias.ini de la 3DS (« nom de la ROM=Clé ») avec les alias de ce PC ; les autres lignes restent.
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

// Assistant en console : carte SD de la 3DS, après le serveur.
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
