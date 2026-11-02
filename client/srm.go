package main

// gpSP et Beetle GBA (cœurs RetroArch) : un .srm de taille fixe, quel que soit le type de sauvegarde du jeu
// (docs/research-v3-emulateurs.md §4.6, ADR 0011). gpSP : 128 Kio, SRAM au début (libretro/libretro.c
// retro_get_memory_size). Beetle GBA : 0x22000 octets, Flash et SRAM au début, EEPROM à 0x20000 (libretro.cpp
// libretro_save_buf, mednafen/gba/eeprom.cpp eepromData ; adjust_save_ram n'est jamais appelée).
// Le .srm ne dit pas la taille du type : la taille de référence est celle de la Version du serveur.

import (
	"bytes"
	"errors"
	"fmt"
)

var srmFixes = map[string]int{"gpSP": 0x20000, "Beetle GBA": 0x22000}

// Début de la SRAM de n octets dans le .srm : EEPROM (512 o, 8 Kio) à 0x20000 chez Beetle GBA, sinon 0.
func zoneSRM(coeur string, n int) int {
	if coeur == "Beetle GBA" && n <= 0x2000 {
		return 0x20000
	}
	return 0
}

// vierge : que des 0x00 ou que des 0xFF (aussi pour une tranche vide).
func vierge(o []byte) bool {
	for _, c := range o {
		if c != o[0] || c != 0 && c != 0xFF {
			return false
		}
	}
	return true
}

// sramSRM : la SRAM du .srm, de la taille de la Version du serveur. Refus si un octet hors de cette SRAM n'est pas
// vierge (autre type de sauvegarde : rien n'est coupé), ou sans Version (taille inconnue) ; un .srm tout vierge passe.
func sramSRM(coeur string, brut []byte, srv *Version) ([]byte, error) {
	if len(brut) != srmFixes[coeur] {
		return nil, fmt.Errorf(tr(".srm %s de %d octets, %d attendus", "%s .srm of %d bytes, %d expected"), coeur, len(brut), srmFixes[coeur])
	}
	if srv == nil {
		if vierge(brut) {
			return brut, nil
		}
		return nil, fmt.Errorf(tr("%s : taille de la sauvegarde inconnue tant que le serveur n'a pas ce jeu. Synchronise-le d'abord depuis mGBA ou la 3DS",
			"%s: save size unknown while the server does not have this game. Sync it first from mGBA or the 3DS"), coeur)
	}
	n := int(srv.Taille)
	d := zoneSRM(coeur, n)
	if n <= 0 || n > 0x20000 || !vierge(brut[:d]) || !vierge(brut[d+n:]) {
		return nil, fmt.Errorf(tr("%s : le .srm a des données hors des %d octets de la Version du serveur. Synchronisation refusée",
			"%s: the .srm has data outside the %d bytes of the server version. Sync refused"), coeur, n)
	}
	return brut[d : d+n], nil
}

// srm : le .srm à écrire pour une SRAM reçue : 0xFF partout (valeur d'origine des deux cœurs), SRAM à sa place.
func srm(coeur string, sram []byte) ([]byte, error) {
	if len(sram) == 0 || len(sram) > 0x20000 {
		return nil, errors.New(tr("taille de Sauvegarde GBA inconnue", "unknown GBA save size"))
	}
	o := bytes.Repeat([]byte{0xFF}, srmFixes[coeur])
	copy(o[zoneSRM(coeur, len(sram)):], sram)
	return o, nil
}
