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

var fixedSRM = map[string]int{"gpSP": 0x20000, "Beetle GBA": 0x22000}

// Début de la SRAM de n octets dans le .srm : EEPROM (512 o, 8 Kio) à 0x20000 chez Beetle GBA, sinon 0.
func srmZone(core string, n int) int {
	if core == "Beetle GBA" && n <= 0x2000 {
		return 0x20000
	}
	return 0
}

// vierge : que des 0x00 ou que des 0xFF (aussi pour une tranche vide).
func blank(o []byte) bool {
	for _, c := range o {
		if c != o[0] || c != 0 && c != 0xFF {
			return false
		}
	}
	return true
}

// sramSRM : la SRAM du .srm, de la taille de la Version du serveur. Refus si un octet hors de cette SRAM n'est pas
// vierge (autre type de sauvegarde : rien n'est coupé), ou sans Version (taille inconnue) ; un .srm tout vierge passe.
func sramSRM(core string, raw []byte, srv *Version) ([]byte, error) {
	if len(raw) != fixedSRM[core] {
		return nil, fmt.Errorf(tr("taille du .srm %s inattendue : %d octets au lieu de %d", "unexpected %s .srm size: %d bytes instead of %d"), core, len(raw), fixedSRM[core])
	}
	if srv == nil {
		if blank(raw) {
			return raw, nil
		}
		return nil, fmt.Errorf(tr("%s : synchronisez d'abord ce jeu depuis mGBA ou la 3DS, pour que le serveur connaisse la taille de sa sauvegarde",
			"%s: sync this game from mGBA or the 3DS first, so the server knows its save size"), core)
	}
	n := int(srv.Size)
	d := srmZone(core, n)
	if n <= 0 || n > 0x20000 || !blank(raw[:d]) || !blank(raw[d+n:]) {
		return nil, fmt.Errorf(tr("%s : synchronisation arrêtée, le .srm contient des données au-delà des %d octets de la version du serveur",
			"%s: sync stopped, the .srm has data beyond the %d bytes of the server version"), core, n)
	}
	return raw[d : d+n], nil
}

// srm : le .srm à écrire pour une SRAM reçue : 0xFF partout (valeur d'origine des deux cœurs), SRAM à sa place.
func srm(core string, sram []byte) ([]byte, error) {
	if len(sram) == 0 || len(sram) > 0x20000 {
		return nil, errors.New(tr("taille de sauvegarde GBA inconnue", "unknown GBA save size"))
	}
	o := bytes.Repeat([]byte{0xFF}, fixedSRM[core])
	copy(o[srmZone(core, len(sram)):], sram)
	return o, nil
}
