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
		return "", errors.New(tr("Adresse ou jeton trop long pour la 3DS.", "Address or token too long for the 3DS."))
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
		return "", fmt.Errorf(tr("%s n'est pas une carte SD de 3DS : pas de dossier « Nintendo 3DS ». Choisissez la racine de la carte.",
			"%s is not a 3DS SD card: no \"Nintendo 3DS\" folder. Choose the root of the card."), racine)
	}
	return racine, nil
}

// copierCia : <racine>/cias/3dssavesync.cia, le .cia inclus dans ce programme. Rend les lignes à afficher : fichier
// écrit ou déjà là, .cia absent de ce programme (où le télécharger), FBI absent de la carte (pas une erreur).
func copierCia(racine string, cia []byte) ([]string, error) {
	var lignes []string
	chemin := filepath.Join(racine, "cias", "3dssavesync.cia")
	if len(cia) == 0 {
		lignes = append(lignes, fmt.Sprintf(tr("Ce programme ne contient pas 3dssavesync.cia (version %s). Téléchargez-le depuis %s, puis copiez-le dans le dossier cias de la carte SD.",
			"This program does not include 3dssavesync.cia (version %s). Download it from %s, then copy it to the cias folder of the SD card."), version, pageVersion()))
	} else {
		ancien, err := os.ReadFile(chemin)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if bytes.Equal(ancien, cia) {
			lignes = append(lignes, tr("3dssavesync.cia déjà sur la carte (même version) : ", "3dssavesync.cia already on the card (same version): ")+chemin)
		} else {
			besoin := uint64(len(cia)+len(ancien)) + 1<<20 // fichier temporaire, copie datée de l'ancien, 1 Mo de marge
			if libre, err := espaceLibre(racine); err == nil && libre < besoin {
				return nil, fmt.Errorf(tr("Pas assez de place sur la carte SD pour 3dssavesync.cia : %d Mo libres, %d Mo nécessaires. Libérez de la place sur la carte, puis recommencez.\nDétail : %s",
					"Not enough space on the SD card for 3dssavesync.cia: %d MB free, %d MB needed. Free some space on the card, then try again.\nDetail: %s"), libre>>20, (besoin+1<<20-1)>>20, racine)
			}
			if err := ecrireCarte(chemin, cia); err != nil {
				return nil, err
			}
			lignes = append(lignes, tr("3dssavesync.cia copié : ", "3dssavesync.cia copied: ")+chemin)
		}
	}
	if !existe(filepath.Join(racine, "3ds", "FBI.3dsx")) && !existe(filepath.Join(racine, "3ds", "FBI")) {
		lignes = append(lignes, fmt.Sprintf(tr("FBI n'est pas sur la carte (ni 3ds/FBI.3dsx, ni le dossier 3ds/FBI). Déjà installé dans le menu HOME : ignorez ce message. Sinon, téléchargez FBI.3dsx depuis %s, copiez-le dans le dossier 3ds de la carte, puis ouvrez-le depuis le Homebrew Launcher.",
			"FBI was not found on the card (no 3ds/FBI.3dsx, no 3ds/FBI folder). Already installed on the HOME menu: ignore this message. Otherwise, download FBI.3dsx from %s, copy it to the 3ds folder of the card, then open it from the Homebrew Launcher."), pageFBI))
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
		lignes = append(lignes, tr("Date de préparation non notée dans config.json : ", "Preparation date not noted in config.json: ")+erreurCarte(err))
	}
	return true, append(lignes, tr("Remettez la carte dans la 3DS. Sur la 3DS : FBI, SD, cias, 3dssavesync.cia, « Install and delete CIA ». Puis lancez 3dssavesync depuis le menu HOME : il se connecte sans rien taper.",
		"Put the card back in the 3DS. On the 3DS: FBI, SD, cias, 3dssavesync.cia, \"Install and delete CIA\". Then start 3dssavesync from the HOME menu: it connects without typing anything."))
}

// erreurCarte : ce qui s'est passé et quoi faire, puis le détail technique sur sa ligne.
func erreurCarte(err error) string {
	t := texteErreur(err)
	if protegee(err) {
		t = tr("La carte SD est verrouillée : rien ne peut y être écrit. Retirez-la, glissez vers le haut le petit interrupteur de verrouillage sur son côté (ou sur l'adaptateur microSD), remettez-la, puis recommencez.",
			"The SD card is locked: nothing can be written to it. Take it out, slide the small lock switch on its side (or on the microSD adapter) up, put it back, then try again.")
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
		return fmt.Errorf(tr("%s relu différent. Vérifiez la carte SD, puis recommencez.", "%s read back differently. Check the SD card, then try again."), chemin)
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
	q.dire("%s", tr("Mettez la carte SD de la 3DS dans le PC : son config.ini reçoit l'adresse et le jeton, et 3dssavesync.cia est copié dans cias.",
		"Put the 3DS SD card in the PC: its config.ini gets the address and the token, and 3dssavesync.cia is copied to cias."))
	cartes := detecterCartes()
	for i, c := range cartes {
		q.dire("  %d. %s", i+1, c)
	}
	r := q.texte(tr("Numéro ou dossier de la carte (Entrée = pas maintenant)", "Card number or folder (Enter = not now)"), "")
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
