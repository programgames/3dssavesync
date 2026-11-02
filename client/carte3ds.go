package main

// Carte SD de la 3DS préparée depuis le PC : config.ini du homebrew avec l'adresse du serveur et le jeton, pour que la
// 3DS se connecte sans rien taper. Seul ce fichier est écrit ; l'ancien est copié avant.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
func preparerCarte(racine, url, jeton string) (string, error) {
	if url == "" || jeton == "" {
		return "", errors.New(tr("Enregistrez d'abord le serveur.", "Save the server first."))
	}
	if len(url) > 255 || len(jeton) > 255 || strings.ContainsAny(url+jeton, "\r\n") { // tailles de cfg dans main.c
		return "", errors.New(tr("Adresse ou jeton trop long pour la 3DS.", "Address or token too long for the 3DS."))
	}
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
	chemin := filepath.Join(racine, "3ds", "3dssavesync", "config.ini")
	if ancien := filepath.Join(racine, "3ds", "nds-save-sync", "config.ini"); !existe(chemin) && existe(ancien) {
		chemin = ancien // dossier d'avant la v3 : le homebrew le reprend au démarrage, avec base.ini (migrer_dossier)
	}
	o, err := os.ReadFile(chemin)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	contenu := []byte(configIni(string(o), url, jeton))
	if bytes.Equal(o, contenu) {
		return chemin, nil
	}
	if err := remplacerFichier(chemin, contenu); err != nil {
		return "", err
	}
	if relu, err := os.ReadFile(chemin); err != nil || !bytes.Equal(relu, contenu) {
		return "", fmt.Errorf(tr("%s relu différent. Vérifiez la carte SD, puis recommencez.", "%s read back differently. Check the SD card, then try again."), chemin)
	}
	return chemin, nil
}

// Assistant en console : carte SD de la 3DS, après le serveur.
func etapeCarteSD(q *Questions, cfg *Config) {
	url, _ := adresse3DS(cfg)
	if url == "" {
		return
	}
	q.dire("\n== %s ==", tr("Carte SD de la 3DS", "3DS SD card"))
	q.dire("%s", tr("Mettez la carte SD de la 3DS dans le PC : son config.ini reçoit l'adresse et le jeton.",
		"Put the 3DS SD card in the PC: its config.ini gets the address and the token."))
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
	if f, err := preparerCarte(r, url, cfg.Jeton); err != nil {
		q.dire("%s", texteErreur(err))
	} else {
		q.dire("%s %s", tr("Carte SD prête :", "SD card ready:"), f)
	}
}
