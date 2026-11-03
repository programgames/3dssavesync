package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// One synthetic save per generation (valid checksums and signatures), like 3ds/test/test_pokemon.c.
// Also checked, outside the repo, on real saves of each generation.
func TestPokemonPlayInfo(t *testing.T) {
	be := binary.BigEndian
	tcase := map[string]struct {
		d        []byte
		expected string
	}{}

	g1 := make([]byte, 0x8000) // J, unknown, é; 57 h 32
	copy(g1[0x2598:], []byte{0x89, 0xE0, 0xBC, 0x50})
	g1[0x2CED], g1[0x2CEF] = 57, 32
	g1[0x3523] = byte(^sum(g1[0x2598:0x3523]))
	tcase["Gen 1"] = struct {
		d        []byte
		expected string
	}{g1, "57 h 32 · J?é"}

	g2 := make([]byte, 0x8000) // Crystal: Jean; big-endian hours
	copy(g2[0x200B:], []byte{0x89, 0xA4, 0xA0, 0xAD, 0x50})
	be.PutUint16(g2[0x2052:], 295)
	g2[0x2054] = 10
	le.PutUint16(g2[0x2D0D:], uint16(sum(g2[0x2009:0x2B83])))
	tcase["Gen 2 Cristal"] = struct {
		d        []byte
		expected string
	}{g2, "295 h 10 · Jean"}

	g3 := make([]byte, 0x20000) // two valid halves, shifted sectors; half 1 has the highest counter
	for k := 0; k < 28; k++ {
		m, id := k/14, (k%14+3+k/14)%14
		sec := g3[m*0xE000+(k%14)*0x1000:]
		le.PutUint16(sec[0xFF4:], uint16(id))
		le.PutUint32(sec[0xFFC:], uint32(4+5*m))
		if id == 0 {
			copy(sec, []byte{byte(0xBB + 9*m), 0x1B, 0xFF})
			le.PutUint16(sec[0x0E:], uint16(1+575*m))
			sec[0x10] = byte(2 + 46*m)
		}
	}
	tcase["Gen 3"] = struct {
		d        []byte
		expected string
	}{g3, "576 h 48 · Jé"}

	g4 := make([]byte, 0x80000) // Platinum: partition 1 is more recent
	le.PutUint32(g4[0x40000+0xCF2C-0x0C:], 0xCF2C)
	le.PutUint32(g4[0x40000+0xCF2C-0x08:], 0x20060623)
	le.PutUint32(g4[0xCF2C-0x14:], 1)
	le.PutUint32(g4[0x40000+0xCF2C-0x14:], 2)
	for i, c := range []uint16{0x134, 0x188, 0xFFFF} {
		le.PutUint16(g4[0x40000+0x68+2*i:], c)
	}
	le.PutUint16(g4[0x40000+0x68+0x22:], 35)
	g4[0x40000+0x68+0x24] = 7
	tcase["Gen 4 Platine"] = struct {
		d        []byte
		expected string
	}{g4, "35 h 07 · Jé"}

	g5 := make([]byte, 0x80000) // Black 2/White 2: CRC16-CCITT of block 0x25F00
	for i := 0; i < 0x94; i++ {
		g5[0x25F00+i] = byte(i * 7)
	}
	le.PutUint16(g5[0x25FA2:], crc16CCITT(g5[0x25F00:0x25F00+0x94]))
	for i, c := range utf16.Encode([]rune("Jé")) {
		le.PutUint16(g5[0x19404+2*i:], c)
	}
	le.PutUint16(g5[0x19404+4:], 0xFFFF)
	le.PutUint16(g5[0x19424:], 15)
	g5[0x19426] = 41
	tcase["Gen 5 Noir 2/Blanc 2"] = struct {
		d        []byte
		expected string
	}{g5, "15 h 41 · Jé"}

	g7 := make([]byte, 0x6CC00) // Ultra: 13 characters at most
	le.PutUint16(g7[0x41000:], 999)
	g7[0x41002] = 30
	for i, c := range utf16.Encode([]rune("ABCDEFGHIJKLMNO")) {
		le.PutUint16(g7[0x1438+2*i:], c)
	}
	tcase["Ultra"] = struct {
		d        []byte
		expected string
	}{g7, "999 h 30 · ABCDEFGHIJKLM"}
	tcase["Soleil/Lune jamais commencé"] = struct {
		d        []byte
		expected string
	}{make([]byte, 0x6BE00), "0 h 00 · partie non commencée"}

	for name, c := range tcase {
		if r, ok := pokemonPlayInfo(c.d); !ok || r != c.expected {
			t.Errorf("%s : %q (%v), attendu %q", name, r, ok, c.expected)
		}
	}
	for _, size := range []int{0x8000, 0x20000, 0x80000, 1024} { // blank or unknown size: refused
		if r, ok := pokemonPlayInfo(make([]byte, size)); ok {
			t.Errorf("%d octets vierges reconnus à tort : %q", size, r)
		}
	}
}

// mainSunMoon builds a synthetic Sun/Moon main: play time and trainer.
func mainSunMoon(h, m int, name string) []byte {
	d := make([]byte, 0x6BE00)
	le.PutUint16(d[0x40C00:], uint16(h))
	d[0x40C02] = byte(m)
	for i, c := range utf16.Encode([]rune(name)) {
		le.PutUint16(d[0x1238+2*i:], c)
	}
	return d
}

// A Conflict on a Pokémon game shows the play time of both Versions.
func TestConflictPlayTime(t *testing.T) {
	reset()
	responses = []string{""}
	pc := mainSunMoon(152, 30, "Sacha")
	srv := &Version{Hash: strings.Repeat("b", 64), Client: "azahar", Age: new(int64)}
	resolveConflict(&Game{Name: "Pokemon Lune", System: "3DS"}, &Local{Version: Version{Hash: strings.Repeat("a", 64)}, Raw: pc, SRAM: pc},
		srv, mainSunMoon(0, 0, ""), false)
	if len(questions) != 1 || !strings.Contains(questions[0], "152 h 30 · Sacha") || !strings.Contains(questions[0], "0 h 00 · partie non commencée") {
		t.Errorf("temps de jeu des deux Versions attendu : %q", questions)
	}
}
