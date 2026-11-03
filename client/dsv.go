package main

// DeSmuME: a .dsv = SRAM (chip size) + 122-byte footer (docs/notes/research-v3-emulateurs.md §2.4).
// Footer: 82-byte text, 6 u32 LE (written size, filled size, type, address size, memory size,
// version 0), 16-byte cookie. Source: TASEmulators/desmume src/mc.cpp (BackupDevice::ensure, readFooter).

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	dsvText   = "|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:"
	dsvCookie = "|-DESMUME SAVE-|"
	dsvFooter = len(dsvText) + 6*4 + len(dsvCookie) // 122
)

// dsvChips lists the DeSmuME chips (save_types, in order): size, address size. Type = index + 1.
var dsvChips = []struct{ size, address uint32 }{
	{512, 1}, {8 << 10, 2}, {64 << 10, 2}, {32 << 10, 2}, {256 << 10, 3}, {512 << 10, 3}, {1 << 20, 3},
	{2 << 20, 3}, {4 << 20, 3}, {8 << 20, 3}, {16 << 20, 3}, {32 << 20, 3}, {64 << 20, 3},
}

// sramDSV returns the SRAM of a .dsv, without the footer. Size = filled size of the footer, which is also all that
// comes before the footer (DeSmuME reads the file this way). The written size often stays below the chip: not used.
// Empty file: empty SRAM (DeSmuME creates an empty .dsv when it starts a game with no save).
func sramDSV(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	n := len(raw) - dsvFooter
	if n < 0 || string(raw[len(raw)-len(dsvCookie):]) != dsvCookie || binary.LittleEndian.Uint32(raw[n+86:]) != uint32(n) ||
		binary.LittleEndian.Uint32(raw[n+102:]) != 0 {
		return nil, errors.New(tr("fin de fichier DeSmuME absente ou inconnue", "DeSmuME footer missing or unknown"))
	}
	return raw[:n], nil
}

// dsv returns the .dsv to write for a received SRAM. Same size as the existing .dsv: its footer stays as it is (type
// and address size chosen by DeSmuME). Otherwise a new footer, like the one DeSmuME writes when it imports a raw .sav.
func dsv(sram, old []byte) ([]byte, error) {
	if s, err := sramDSV(old); err == nil && len(old) > 0 && len(s) == len(sram) {
		return append(append([]byte{}, sram...), old[len(s):]...), nil
	}
	for i, p := range dsvChips {
		if int(p.size) == len(sram) {
			o := append(append([]byte{}, sram...), dsvText...)
			for _, v := range []uint32{p.size, p.size, uint32(i + 1), p.address, p.size, 0} {
				o = binary.LittleEndian.AppendUint32(o, v)
			}
			return append(o, dsvCookie...), nil
		}
	}
	return nil, fmt.Errorf(tr("DeSmuME ne connaît pas cette taille de sauvegarde : %d octets", "DeSmuME doesn't know this save size: %d bytes"), len(sram))
}
