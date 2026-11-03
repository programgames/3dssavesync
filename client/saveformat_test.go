package main

import (
	"bytes"
	"testing"
)

// encode(decode(x)) == x: a Save file written back with its own SRAM is the same file, byte for byte, in every
// format (Clock trailer, MBC2, DeSmuME .dsv, gpSP and Beetle GBA .srm, plain .sav, 3DS archive file).
func TestSaveFormatRoundTrip(t *testing.T) {
	withTrailer := func(sram []byte, n int) []byte { return append(append([]byte{}, sram...), random(n, 99)...) }
	sameboy := random(512, 5)
	for i := range sameboy {
		sameboy[i] |= 0xF0 // SameBoy, VBA-M, BGB: high nibble at 0xF
	}
	srmFile := func(core string, sram []byte) []byte { o, _ := srm(core, sram); return o }
	for _, c := range []struct {
		name string
		game Game
		raw  []byte
		srv  *Version // Current version, for the .srm size
		sram int      // expected SRAM size
	}{
		{"DS .sav", Game{System: "DS", File: "a.sav"}, random(512<<10, 1), nil, 512 << 10},
		{"GB with mGBA trailer", Game{System: "GB", File: "a.sav"}, withTrailer(random(32<<10, 2), 48), nil, 32 << 10},
		{"GB with VBA-M trailer", Game{System: "GB", File: "a.sav"}, withTrailer(random(32<<10, 2), 44), nil, 32 << 10},
		{"GBA with trailer", Game{System: "GBA", File: "a.sav"}, withTrailer(random(128<<10, 3), 16), nil, 128 << 10},
		{"GBA EEPROM", Game{System: "GBA", File: "a.sav"}, random(8<<10, 4), nil, 8 << 10},
		{"MBC2 mGBA", Game{System: "GB", File: "a.sav"}, random(256, 5), nil, 512},
		{"MBC2 SameBoy", Game{System: "GB", File: "a.sav"}, sameboy, nil, 512},
		{"DeSmuME", Game{System: "DS", File: "a.DSV"}, makeDSV(random(512<<10, 6), 0x40001, 6, 3), nil, 512 << 10},
		{"gpSP", Game{System: "GBA", File: "a.srm", Core: "gpSP"}, srmFile("gpSP", random(64<<10, 7)), &Version{Size: 64 << 10}, 64 << 10},
		{"Beetle GBA EEPROM", Game{System: "GBA", File: "a.srm", Core: "Beetle GBA"}, srmFile("Beetle GBA", random(8<<10, 8)), &Version{Size: 8 << 10}, 8 << 10},
		{"3DS", Game{System: "3DS", File: "main"}, random(0x80000, 9), nil, 0x80000},
	} {
		sram, err := decode(&c.game, c.raw, c.srv)
		if err != nil || len(sram) != c.sram {
			t.Errorf("%s: decode gives %d bytes, %d expected (%v)", c.name, len(sram), c.sram, err)
			continue
		}
		if raw, err := encode(&c.game, sram, c.raw); err != nil || !bytes.Equal(raw, c.raw) {
			t.Errorf("%s: encode(decode(x)) != x (%d bytes, %d expected, %v)", c.name, len(raw), len(c.raw), err)
		}
	}
}

// encode without a file: the emulator's own shape (no trailer, MBC2 at the emulator size, new .dsv footer).
func TestEncodeNewFile(t *testing.T) {
	mbc2 := make([]byte, 512)
	mbc2[0], mbc2[1] = 1, 2
	for _, c := range []struct {
		name string
		game Game
		sram []byte
		want []byte
	}{
		{"GB", Game{System: "GB", File: "a.sav"}, []byte("0123456789"), []byte("0123456789")},
		{"MBC2 mGBA", Game{System: "GB", File: "a.sav", MBC2: 256}, mbc2, append([]byte{0x21}, make([]byte, 255)...)},
		{"MBC2 SameBoy", Game{System: "GB", File: "a.sav"}, mbc2, append([]byte{0xF1, 0xF2}, bytes.Repeat([]byte{0xF0}, 510)...)},
		{"DeSmuME", Game{System: "DS", File: "a.dsv"}, make([]byte, 512), makeDSV(make([]byte, 512), 512, 1, 1)},
	} {
		if raw, err := encode(&c.game, c.sram, nil); err != nil || !bytes.Equal(raw, c.want) {
			t.Errorf("%s: %d bytes %x…, expected %d bytes %x… (%v)", c.name, len(raw), raw[:min(len(raw), 4)], len(c.want), c.want[:4], err)
		}
	}
}
