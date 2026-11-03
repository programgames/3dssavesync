package main

// DeSmuME : un .dsv = SRAM (taille de la puce) + pied de page de 122 octets (docs/research-v3-emulateurs.md §2.4).
// Pied : texte de 82 octets, 6 u32 LE (taille écrite, taille remplie, type, taille d'adresse, taille mémoire,
// version 0), cookie de 16 octets. Source : TASEmulators/desmume src/mc.cpp (BackupDevice::ensure, readFooter).

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	texteDSV  = "|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:"
	cookieDSV = "|-DESMUME SAVE-|"
	piedDSV   = len(texteDSV) + 6*4 + len(cookieDSV) // 122
)

// Puces de DeSmuME (save_types, dans l'ordre) : taille, taille d'adresse. Type = index + 1.
var pucesDSV = []struct{ taille, adresse uint32 }{
	{512, 1}, {8 << 10, 2}, {64 << 10, 2}, {32 << 10, 2}, {256 << 10, 3}, {512 << 10, 3}, {1 << 20, 3},
	{2 << 20, 3}, {4 << 20, 3}, {8 << 20, 3}, {16 << 20, 3}, {32 << 20, 3}, {64 << 20, 3},
}

// sramDSV : la SRAM d'un .dsv, sans le pied. Taille = taille remplie du pied, qui est aussi tout ce qui précède
// le pied (DeSmuME lit ainsi le fichier). La taille écrite, elle, reste souvent en deçà de la puce : pas utilisée.
// Fichier vide : SRAM vide (DeSmuME crée un .dsv vide au lancement d'un jeu sans sauvegarde).
func sramDSV(brut []byte) ([]byte, error) {
	if len(brut) == 0 {
		return brut, nil
	}
	n := len(brut) - piedDSV
	if n < 0 || string(brut[len(brut)-len(cookieDSV):]) != cookieDSV || binary.LittleEndian.Uint32(brut[n+86:]) != uint32(n) ||
		binary.LittleEndian.Uint32(brut[n+102:]) != 0 {
		return nil, errors.New(tr("pied de page DeSmuME absent ou inconnu", "DeSmuME footer missing or unknown"))
	}
	return brut[:n], nil
}

// dsv : le .dsv à écrire pour une SRAM reçue. Même taille que le .dsv existant : son pied reste tel quel (type et
// taille d'adresse choisis par DeSmuME). Sinon un pied neuf, comme celui de DeSmuME à l'import d'un .sav brut.
func dsv(sram, ancien []byte) ([]byte, error) {
	if s, err := sramDSV(ancien); err == nil && len(ancien) > 0 && len(s) == len(sram) {
		return append(append([]byte{}, sram...), ancien[len(s):]...), nil
	}
	for i, p := range pucesDSV {
		if int(p.taille) == len(sram) {
			o := append(append([]byte{}, sram...), texteDSV...)
			for _, v := range []uint32{p.taille, p.taille, uint32(i + 1), p.adresse, p.taille, 0} {
				o = binary.LittleEndian.AppendUint32(o, v)
			}
			return append(o, cookieDSV...), nil
		}
	}
	return nil, fmt.Errorf(tr("taille de sauvegarde inconnue de DeSmuME : %d octets", "save size unknown to DeSmuME: %d bytes"), len(sram))
}
