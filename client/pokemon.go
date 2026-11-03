package main

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Temps de jeu et dresseur des Pokémon, de Rouge/Bleu à Ultra-Lune, pour comparer deux Versions au Conflit.
// Même logique que 3ds/source/pokemon.c ; positions et tables de PKHeX (docs/research-pokemon-temps-de-jeu.md).
// Le jeu est reconnu par la taille, puis par une somme de contrôle ou une signature.

var le = binary.LittleEndian

// Gen 1 et 2 (StringConverter1, StringConverter2 TableFRE), Gen 3 (G3_EN), Gen 4 (TableINT) : accents français.
var (
	charsG12 = map[int]rune{0x7F: ' ', 0xBA: 'à', 0xBB: 'è', 0xBC: 'é', 0xBD: 'ù'}
	charsG1  = map[int]rune{0xBE: 'À', 0xC6: 'È', 0xC7: 'É'}
	charsG2  = map[int]rune{0xBF: 'ç', 0xC6: 'ë', 0xC7: 'ï', 0xC8: 'â', 0xC9: 'ô', 0xCA: 'û', 0xCB: 'ê', 0xCC: 'î'}
	charsG3  = map[int]rune{0x00: ' ', 0x01: 'À', 0x04: 'Ç', 0x05: 'È', 0x06: 'É', 0x16: 'à', 0x19: 'ç', 0x1A: 'è', 0x1B: 'é',
		0x1C: 'ê', 0x1D: 'ë', 0x20: 'î', 0x21: 'ï', 0x24: 'ô', 0x26: 'ù', 0x28: 'û'}
	charsG4 = map[int]rune{0x1DE: ' ', 0x15F: 'À', 0x166: 'Ç', 0x167: 'È', 0x168: 'É', 0x17F: 'à', 0x181: 'â', 0x186: 'ç',
		0x187: 'è', 0x188: 'é', 0x189: 'ê', 0x18A: 'ë', 0x18D: 'î', 0x18E: 'ï', 0x193: 'ô', 0x198: 'ù', 0x19A: 'û'}
)

// Un caractère : plage de lettres ou de chiffres (début du code, première rune, longueur), sinon table, sinon '?'.
func char(c int, ranges [][3]int, tables ...map[int]rune) rune {
	for _, p := range ranges {
		if c >= p[0] && c < p[0]+p[2] {
			return rune(p[1] + c - p[0])
		}
	}
	for _, t := range tables {
		if r, ok := t[c]; ok {
			return r
		}
	}
	return '?'
}

var (
	rangesG12 = [][3]int{{0x80, 'A', 26}, {0xA0, 'a', 26}, {0xF6, '0', 10}}
	rangesG3  = [][3]int{{0xBB, 'A', 26}, {0xD5, 'a', 26}, {0xA1, '0', 10}}
	rangesG4  = [][3]int{{0x121, '0', 10}, {0x12B, 'A', 26}, {0x145, 'a', 26}}
)

// Nom en octets (Gen 1 à 3) : n caractères au plus, jusqu'au code de fin.
func name8(d []byte, n int, end byte, conv func(int) rune) string {
	var b strings.Builder
	for _, c := range d[:n] {
		if c == end {
			break
		}
		b.WriteRune(conv(int(c)))
	}
	return b.String()
}

// Nom en mots de 16 bits (Gen 4 : table propre ; Gen 5 à 7 : UTF-16), jusqu'à 0 ou 0xFFFF.
func name16(d []byte, n int, conv func(int) rune) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		c := int(le.Uint16(d[2*i:]))
		if c == 0 || c == 0xFFFF {
			break
		}
		b.WriteRune(conv(c))
	}
	return b.String()
}

func sum(d []byte) (s int) {
	for _, o := range d {
		s += int(o)
	}
	return
}

// Gen 1 et 2 (32 Kio) : reconnues par leur somme de contrôle (Gen 1, puis Or/Argent et Cristal, versions INT).
func gen12(d []byte) (h, m int, name string, ok bool) {
	if ^sum(d[0x2598:0x3523])&0xFF == int(d[0x3523]) {
		return int(d[0x2CED]), int(d[0x2CEF]), name8(d[0x2598:], 7, 0x50, func(c int) rune { return char(c, rangesG12, charsG12, charsG1) }), true
	}
	for _, g := range [][3]int{{0x2D68, 0x2D69, 0x2053}, {0x2B82, 0x2D0D, 0x2052}} { // fin de la somme, somme, heures
		if s := sum(d[0x2009 : g[0]+1]); s != 0 && s&0xFFFF == int(le.Uint16(d[g[1]:])) { // somme nulle : fichier vierge
			return int(binary.BigEndian.Uint16(d[g[2]:])), int(d[g[2]+2]), name8(d[0x200B:], 7, 0x50, func(c int) rune { return char(c, rangesG12, charsG12, charsG2) }), true
		}
	}
	return
}

// Gen 3 (128 Kio) : deux moitiés de 14 secteurs ; la moitié valide au plus grand compteur ; section 0 : nom et temps.
func gen3(d []byte) (h, m int, name string, ok bool) {
	s0, best := -1, uint32(0)
	for half := 0; half < 2; half++ {
		seenSet, zero := 0, -1
		for k := 0; k < 14; k++ {
			o := half*0xE000 + k*0x1000
			id := int(le.Uint16(d[o+0xFF4:]))
			if id >= 14 || seenSet&(1<<id) != 0 {
				break
			}
			seenSet |= 1 << id
			if id == 0 {
				zero = o
			}
		}
		if seenSet == 0x3FFF && (s0 < 0 || le.Uint32(d[zero+0xFFC:]) > best) {
			s0, best = zero, le.Uint32(d[zero+0xFFC:])
		}
	}
	if s0 < 0 {
		return
	}
	return int(le.Uint16(d[s0+0x0E:])), int(d[s0+0x10]), name8(d[s0:], 7, 0xFF, func(c int) rune { return char(c, rangesG3, charsG3) }), true
}

// Gen 4 (512 Kio) : pied du bloc général de la partition 1 (taille, signature) ; bloc actif par les compteurs.
func gen4(d []byte) (h, m int, name string, ok bool) {
	for _, j := range [][2]int{{0xC100, 0x64}, {0xCF2C, 0x68}, {0xF628, 0x64}} { // DP, Pt, HGSS : taille du bloc, dresseur
		g := j[0]
		p0, p1 := d[g-0x14:], d[0x40000+g-0x14:]
		if sig := le.Uint32(p1[0x0C:]); int(le.Uint32(p1[0x08:])) != g || sig != 0x20060623 && sig != 0x20070903 {
			continue
		}
		// ponytail : plus grand compteur majeur puis mineur ; PKHeX traite aussi 0xFFFFFFFE
		a0, a1 := le.Uint32(p0), le.Uint32(p1)
		b := 0
		if a0 == 0xFFFFFFFF || a1 != 0xFFFFFFFF && (a1 > a0 || a1 == a0 && le.Uint32(p1[4:]) > le.Uint32(p0[4:])) {
			b = 0x40000
		}
		t := b + j[1]
		return int(le.Uint16(d[t+0x22:])), int(d[t+0x24]), name16(d[t:], 8, func(c int) rune { return char(c, rangesG4, charsG4) }), true
	}
	return
}

func crc16CCITT(d []byte) uint16 {
	c := uint16(0xFFFF)
	for _, o := range d {
		c ^= uint16(o) << 8
		for k := 0; k < 8; k++ {
			if c&0x8000 != 0 {
				c = c<<1 ^ 0x1021
			} else {
				c <<= 1
			}
		}
	}
	return c
}

// Gen 5 (512 Kio) : bloc des sommes de contrôle valide (Noir/Blanc ou Noir 2/Blanc 2) ; données du dresseur à 0x19400.
func gen5(d []byte) (h, m int, name string, ok bool) {
	if crc16CCITT(d[0x23F00:0x23F00+0x8C]) != le.Uint16(d[0x23F9A:]) && crc16CCITT(d[0x25F00:0x25F00+0x94]) != le.Uint16(d[0x25FA2:]) {
		return
	}
	return int(le.Uint16(d[0x19424:])), int(d[0x19426]), name16(d[0x19404:], 8, func(c int) rune { return rune(c) }), true
}

// Gen 6 et 7 (main des jeux 3DS) : reconnus par la taille (SaveUtil.cs : SIZE_G6XY, SIZE_G6ORAS, SIZE_G7SM, SIZE_G7USUM).
var pokemonFormats = map[int][2]int{ // taille : position du temps de jeu, position du dresseur
	0x65600: {0x01800, 0x14048}, // X/Y
	0x76000: {0x01800, 0x14048}, // Rubis Oméga/Saphir Alpha
	0x6BE00: {0x40C00, 0x01238}, // Soleil/Lune
	0x6CC00: {0x41000, 0x01438}, // Ultra-Soleil/Ultra-Lune
}

func gen67(d []byte) (h, m int, name string, ok bool) {
	f, ok := pokemonFormats[len(d)]
	if !ok {
		return
	}
	return int(le.Uint16(d[f[0]:])), int(d[f[0]+2]), name16(d[f[1]:], 13, func(c int) rune { return rune(c) }), true
}

// « 152 h 30 · Sacha » pour une sauvegarde (SRAM seule) d'un Pokémon reconnu ; false sinon.
func pokemonPlayInfo(d []byte) (string, bool) {
	var read func([]byte) (int, int, string, bool)
	switch len(d) {
	case 0x8000:
		read = gen12
	case 0x20000:
		read = gen3
	case 0x80000:
		read = func(d []byte) (int, int, string, bool) {
			if h, m, n, ok := gen4(d); ok {
				return h, m, n, ok
			}
			return gen5(d)
		}
	default:
		read = gen67
	}
	h, m, name, ok := read(d)
	if !ok {
		return "", false
	}
	if name == "" {
		return fmt.Sprintf(tr("%d h %02d · partie non commencée", "%d h %02d · not started yet"), h, m), true
	}
	return fmt.Sprintf("%d h %02d · %s", h, m, name), true
}
