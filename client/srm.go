package main

// gpSP and Beetle GBA (RetroArch cores): a fixed-size .srm, whatever the save type of the game
// (docs/notes/research-v3-emulateurs.md §4.6, ADR 0011). gpSP: 128 KiB, SRAM at the start (libretro/libretro.c
// retro_get_memory_size). Beetle GBA: 0x22000 bytes, Flash and SRAM at the start, EEPROM at 0x20000 (libretro.cpp
// libretro_save_buf, mednafen/gba/eeprom.cpp eepromData; adjust_save_ram is never called).
// The .srm does not give the size of the type: the reference size is the size of the server Version.

import (
	"bytes"
	"errors"
	"fmt"
)

var fixedSRM = map[string]int{"gpSP": 0x20000, "Beetle GBA": 0x22000}

// srmZone returns the start of the n-byte SRAM in the .srm: EEPROM (512 B, 8 KiB) at 0x20000 in Beetle GBA, else 0.
func srmZone(core string, n int) int {
	if core == "Beetle GBA" && n <= 0x2000 {
		return 0x20000
	}
	return 0
}

// blank reports only 0x00 or only 0xFF (also true for an empty slice).
func blank(o []byte) bool {
	for _, c := range o {
		if c != o[0] || c != 0 && c != 0xFF {
			return false
		}
	}
	return true
}

// sramSRM returns the SRAM of the .srm, at the size of the server Version. Refused if a byte outside this SRAM is not
// blank (other save type: nothing is cut), or with no Version (unknown size); a fully blank .srm passes.
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

// srm returns the .srm to write for a received SRAM: 0xFF everywhere (initial value of both cores), SRAM in its place.
func srm(core string, sram []byte) ([]byte, error) {
	if len(sram) == 0 || len(sram) > 0x20000 {
		return nil, errors.New(tr("taille de sauvegarde GBA inconnue", "unknown GBA save size"))
	}
	o := bytes.Repeat([]byte{0xFF}, fixedSRM[core])
	copy(o[srmZone(core, len(sram)):], sram)
	return o, nil
}
