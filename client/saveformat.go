package main

// Save formats: an emulator's Save file <-> the SRAM that is hashed, compared and synced. All the per-format rules
// sit behind decode and encode: Clock trailer (ADR 0008, 0010), MBC2 (mbc2.go, ADR 0012), DeSmuME .dsv (dsv.go),
// gpSP and Beetle GBA .srm (srm.go, ADR 0011). A plain .sav and a 3DS archive file are their own SRAM.

import (
	"path/filepath"
	"strings"
)

// trailer returns the length of the Clock trailer after the SRAM of a GB or GBA file: 16, 44 or 48 bytes, 0 if none.
func trailer(system System, raw []byte) int {
	r := len(raw) % 512
	if (system == "GB" || system == "GBA") && len(raw) > r && (r == 16 || r == 44 || r == 48) {
		return r
	}
	return 0
}

func isDSV(file string) bool { return strings.EqualFold(filepath.Ext(file), ".dsv") }

// decode returns the SRAM of a Save file. srv: Current version or nil; it gives the SRAM size of a fixed-size .srm.
func decode(j *Game, raw []byte, srv *Version) ([]byte, error) {
	switch {
	case fixedSRM[j.Core] > 0:
		return sramSRM(j.Core, raw, srv)
	case isDSV(j.File):
		return sramDSV(raw)
	}
	sram := raw[:len(raw)-trailer(j.System, raw)]
	if isMBC2(j.System, len(sram)) {
		return normalizeMBC2(sram), nil
	}
	return sram, nil
}

// encode returns the Save file to write for a received SRAM. old: the current file, or nil. Its Clock trailer stays
// (it belongs to this PC), its MBC2 size and .dsv footer too; without it, the shape the emulator writes (Game.MBC2).
func encode(j *Game, sram, old []byte) ([]byte, error) {
	switch {
	case fixedSRM[j.Core] > 0:
		return srm(j.Core, sram)
	case isMBC2(j.System, len(sram)):
		n := j.MBC2
		if isMBC2(j.System, len(old)) {
			n = len(old)
		} else if n == 0 {
			n = 512
		}
		return mbc2File(normalizeMBC2(sram), n), nil
	case isDSV(j.File):
		return dsv(sram, old)
	}
	return append(append([]byte{}, sram...), old[len(old)-trailer(j.System, old):]...), nil
}
