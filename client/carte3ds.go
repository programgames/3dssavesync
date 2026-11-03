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
var embarque embed.FS

// Le .cia inclus dans ce programme (nil : build local ou de test). Remplacé par les tests.
var ciaEmbarque, _ = fs.ReadFile(embarque, "embarque/3dssavesync.cia")

// Place libre sur le disque d'un dossier, en octets. Remplacé par les tests.
var espaceLibre = espaceLibreSysteme

// Dernière version de FBI, pour installer un .cia sur la 3DS (Steveice10/FBI n'évolue plus).
const pageFBI = "https://github.com/nh-server/FBI-NH/releases/latest"

// Adresse du serveur pour la 3DS, et les autres possibles. Serveur local : l'adresse du PC sur le réseau
// (127.0.0.1 de config.json ne mène pas au PC depuis la 3DS).
func adresse3DS(cfg *Config) (string, []string) {
	if cfg.ServeurLocal == nil {
		return cfg.URL, nil
	}
	var l []string
	for _, ip := range adressesLocales() {
		l = append(l, fmt.Sprintf("http://%s:%d", ip, cfg.ServeurLocal.Port))
	}
	if len(l) == 0 {
		return "", nil
	}
	return l[0], l[1:]
}

// Racines qui ressemblent à une carte SD de 3DS : un dossier « Nintendo 3DS ».
func cartes3DS(racines []string) []string {
	var l []string
	for _, r := range racines {
		if existe(filepath.Join(r, "Nintendo 3DS")) {
			l = append(l, r)
		}
	}
	return l
}

// Remplacé par les tests : jamais de vraie carte SD.
var detecterCartes = func() []string { return cartes3DS(racinesAmovibles()) }

// config.ini du homebrew (main.c, cle_config) : url et jeton en tête, les autres lignes (roms, saves…) gardées.
func configIni(ancien, url, jeton string) string {
	t := "url=" + url + "\njeton=" + jeton + "\n"
	for _, l := range strings.Split(strings.ReplaceAll(ancien, "\r\n", "\n"), "\n") {
		if l != "" && !strings.HasPrefix(l, "url=") && !strings.HasPrefix(l, "jeton=") {
			t += l + "\n"
		}
	}
	return t
}

// Écrit <racine>/3ds/3dssavesync/config.ini : copie datée de l'ancien, fichier temporaire vérifié, renommage, relecture.
// Rend le chemin écrit.
func preparerCarte(racine, url, jeton string, alias map[string]string) (string, error) {
	if url == "" || jeton == "" {
		return "", errors.New(tr("Enregistrez d'abord le serveur.", "Save the server first."))
	}
	if len(url) > 255 || len(jeton) > 255 || strings.ContainsAny(url+jeton, "\r\n") { // tailles de cfg dans main.c
		return "", errors.New(tr("L'adresse ou le jeton est trop long pour la 3DS.", "The address or token is too long for the 3DS."))
	}
	racine, err := racineCarte(racine)
	if err != nil {
		return "", err
	}
	chemin := filepath.Join(racine, "3ds", "3dssavesync", "config.ini")
	if ancien := filepath.Join(racine, "3ds", "nds-save-sync", "config.ini"); !existe(chemin) && existe(ancien) {
		chemin = ancien // dossier d'avant la v3 : le homebrew le reprend au démarrage, avec base.ini (migrer_dossier)
	}
	o, err := os.ReadFile(chemin)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := ecrireCarte(chemin, []byte(configIni(string(o), url, jeton))); err != nil {
		return "", err
	}
	if len(alias) > 0 { // ROM hacks : mêmes Clés sur la 3DS que sur ce PC (ADR 0014)
		f := filepath.Join(filepath.Dir(chemin), "alias.ini")
		a, err := os.ReadFile(f)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err := ecrireCarte(f, []byte(aliasIni(string(a), alias))); err != nil {
			return "", err
		}
	}
	return chemin, nil
}

// racineCarte : la racine choisie (guillemets et espaces retirés, « E: » -> « E:\ »), si c'est une carte SD de 3DS.
func racineCarte(racine string) (string, error) {
	racine = strings.Trim(strings.TrimSpace(racine), `"`)
	if racine == "" {
		return "", errors.New(tr("Choisissez la carte SD de la 3DS.", "Choose the 3DS SD card."))
	}
	if len(racine) == 2 && racine[1] == ':' {
		racine += `\` // « E: » seul = dossier courant du lecteur E
	}
	if !filepath.IsAbs(racine) || !existe(filepath.Join(racine, "Nintendo 3DS")) {
		return "", fmt.Errorf(tr("%s n'est pas une carte SD de 3DS (il manque le dossier « Nintendo 3DS »). Choisissez la racine de la carte.",
			"%s isn't a 3DS SD card (there's no \"Nintendo 3DS\" folder). Choose the root of the card."), racine)
	}
	return racine, nil
}

// versionInstallee : la version qui a tourné en dernier sur la 3DS (installee= de maj.ini, écrit par main.c), ou "".
func versionInstallee(racine string) string {
	o, _ := os.ReadFile(filepath.Join(racine, "3ds", "3dssavesync", "maj.ini"))
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
func copierCia(racine string, cia []byte) ([]string, error) {
	var lignes []string
	chemin := filepath.Join(racine, "cias", "3dssavesync.cia")
	if v := versionInstallee(racine); plusRecente(version, v) {
		return []string{fmt.Sprintf(tr("La 3DS a déjà une version plus récente de 3dssavesync (%s). Inutile d'installer la version %s.",
			"The 3DS already has a newer 3dssavesync (%s). There's no need to install version %s."), v, version)}, nil
	}
	if len(cia) == 0 {
		lignes = append(lignes, fmt.Sprintf(tr("Ce programme ne contient pas 3dssavesync.cia (version %s). Téléchargez-le sur %s et copiez-le dans le dossier cias de la carte SD.",
			"This program doesn't include 3dssavesync.cia (version %s). Download it from %s and copy it to the cias folder on the SD card."), version, pageVersion()))
	} else {
		ancien, err := os.ReadFile(chemin)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if bytes.Equal(ancien, cia) {
			lignes = append(lignes, fmt.Sprintf(tr("3dssavesync.cia %s déjà sur la carte : ", "3dssavesync.cia %s is already on the card: "), version)+chemin)
		} else {
			besoin := uint64(len(cia)+len(ancien)) + 1<<20 // fichier temporaire, copie datée de l'ancien, 1 Mo de marge
			if libre, err := espaceLibre(racine); err == nil && libre < besoin {
				return nil, fmt.Errorf(tr("Pas assez de place sur la carte SD (%d Mo libres, %d Mo nécessaires). Libérez de la place et réessayez.\nDétail : %s",
					"Not enough space on the SD card (%d MB free, %d MB needed). Free up some space and try again.\nDetail: %s"), libre>>20, (besoin+1<<20-1)>>20, racine)
			}
			if err := ecrireCarte(chemin, cia); err != nil {
				return nil, err
			}
			lignes = append(lignes, fmt.Sprintf(tr("3dssavesync.cia %s copié : ", "3dssavesync.cia %s copied: "), version)+chemin)
		}
	}
	if !existe(filepath.Join(racine, "3ds", "FBI.3dsx")) && !existe(filepath.Join(racine, "3ds", "FBI")) {
		lignes = append(lignes, fmt.Sprintf(tr("FBI n'est pas sur la carte. S'il est déjà installé sur le menu HOME, ignorez ce message. Sinon, téléchargez FBI.3dsx sur %s, copiez-le dans le dossier 3ds de la carte et ouvrez-le depuis le Homebrew Launcher.",
			"FBI isn't on the card. If it's already installed on the HOME Menu, ignore this message. Otherwise, download FBI.3dsx from %s, copy it to the 3ds folder on the card and open it from the Homebrew Launcher."), pageFBI))
	}
	return lignes, nil
}

// preparerEtNoter : config.ini, puis 3dssavesync.cia, puis la date dans config.json (liste « Tout est prêt »). Rend
// ok et les lignes à afficher ; un échec dit quoi faire, puis le détail technique.
func preparerEtNoter(config, racine string) (bool, []string) {
	var cfg Config
	if err := lireConfig(config, &cfg); err != nil {
		return false, []string{texteErreur(err)}
	}
	url, _ := adresse3DS(&cfg)
	f, err := preparerCarte(racine, url, cfg.Jeton, cfg.Alias)
	if err != nil {
		return false, []string{erreurCarte(err)}
	}
	lignes := []string{tr("config.ini écrit (adresse et jeton) : ", "config.ini written (address and token): ") + f}
	racine, _ = racineCarte(racine)
	l, err := copierCia(racine, ciaEmbarque)
	lignes = append(lignes, l...)
	if err != nil {
		return false, append(lignes, erreurCarte(err))
	}
	cfg.CartePreparee = time.Now().Format("2006-01-02 15:04")
	if err := ecrireConfigCopie(config, &cfg); err != nil {
		lignes = append(lignes, tr("La date de préparation n'a pas pu être notée : ", "Couldn't record the preparation date: ")+erreurCarte(err))
	}
	if plusRecente(version, versionInstallee(racine)) { // .cia pas copié : rien à installer
		return true, append(lignes, tr("Remettez la carte dans la 3DS et lancez 3dssavesync : il se connecte tout seul.",
			"Put the card back in the 3DS and start 3dssavesync. It connects by itself."))
	}
	return true, append(lignes, tr("Remettez la carte dans la 3DS. Dans FBI, ouvrez SD, cias, 3dssavesync.cia et choisissez « Install and delete CIA ». Lancez ensuite 3dssavesync depuis le menu HOME : il se connecte tout seul.",
		"Put the card back in the 3DS. In FBI, open SD, cias, 3dssavesync.cia and choose \"Install and delete CIA\". Then start 3dssavesync from the HOME Menu. It connects by itself."))
}

// erreurCarte : ce qui s'est passé et quoi faire, puis le détail technique sur sa ligne.
func erreurCarte(err error) string {
	t := texteErreur(err)
	if protegee(err) {
		t = tr("La carte SD est verrouillée en écriture. Retirez-la, poussez vers le haut le petit loquet sur le côté (ou sur l'adaptateur microSD), remettez-la et réessayez.",
			"The SD card is write-protected. Take it out, slide the small lock switch on its side (or on the microSD adapter) up, put it back and try again.")
	}
	if t != err.Error() {
		t += "\n" + tr("Détail : ", "Detail: ") + err.Error()
	}
	return t
}

// protegee : écriture refusée par un support verrouillé (ERROR_WRITE_PROTECT sous Windows, EROFS ailleurs).
func protegee(err error) bool {
	var e syscall.Errno
	return errors.As(err, &e) && (runtime.GOOS == "windows" && e == 19 || runtime.GOOS != "windows" && e == syscall.EROFS)
}

// ecrireCarte : rien si le contenu est déjà là ; sinon copie datée, fichier temporaire vérifié, renommage, relecture.
func ecrireCarte(chemin string, contenu []byte) error {
	if o, err := os.ReadFile(chemin); err == nil && bytes.Equal(o, contenu) {
		return nil
	}
	if err := remplacerFichier(chemin, contenu); err != nil {
		return err
	}
	if relu, err := os.ReadFile(chemin); err != nil || !bytes.Equal(relu, contenu) {
		return fmt.Errorf(tr("%s ne s'est pas écrit correctement. Vérifiez la carte SD et réessayez.", "%s wasn't written correctly. Check the SD card and try again."), chemin)
	}
	return nil
}

// aliasIni : alias.ini de la 3DS (« nom de la ROM=Clé ») avec les alias de ce PC ; les autres lignes restent.
func aliasIni(ancien string, alias map[string]string) string {
	t := ""
	for _, l := range strings.Split(strings.ReplaceAll(ancien, "\r\n", "\n"), "\n") {
		if nom, _, ok := strings.Cut(l, "="); l != "" && (!ok || alias[nom] == "") {
			t += l + "\n"
		}
	}
	noms := make([]string, 0, len(alias))
	for n := range alias {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	for _, n := range noms {
		t += n + "=" + alias[n] + "\n"
	}
	return t
}

// Assistant en console : carte SD de la 3DS, après le serveur.
func etapeCarteSD(q *Questions, cfg *Config, config string) {
	url, _ := adresse3DS(cfg)
	if url == "" {
		return
	}
	q.dire("\n== %s ==", tr("Carte SD de la 3DS", "3DS SD card"))
	q.dire("%s", tr("Mettez la carte SD de la 3DS dans le PC. 3dssavesync y écrit l'adresse et le jeton, et copie l'application 3DS dans cias.",
		"Put the 3DS SD card in the PC. 3dssavesync writes the address and token to it, and copies the 3DS app to cias."))
	cartes := detecterCartes()
	for i, c := range cartes {
		q.dire("  %d. %s", i+1, c)
	}
	r := q.texte(tr("Numéro ou dossier de la carte (Entrée pour passer)", "Card number or folder (Enter to skip)"), "")
	if i, err := strconv.Atoi(r); err == nil && i >= 1 && i <= len(cartes) {
		r = cartes[i-1]
	}
	if r == "" {
		return
	}
	_, lignes := preparerEtNoter(config, r)
	for _, l := range lignes {
		q.dire("- %s", l)
	}
}
