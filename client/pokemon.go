package main

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Temps de jeu et dresseur des Pokémon 3DS, pour comparer deux Versions au Conflit (même table que 3ds/source/pokemon.c).
// Positions de PKHeX (SaveBlockAccessor6XY, 6AO, 7SM, 7USUM ; PlayTime6 ; MyStatus6/7), choisies par la taille de main.
var formatsPokemon = map[int][2]int{ // taille : position du temps de jeu, position du dresseur
	0x65600: {0x01800, 0x14048}, // X/Y
	0x76000: {0x01800, 0x14048}, // Rubis Oméga/Saphir Alpha
	0x6BE00: {0x40C00, 0x01238}, // Soleil/Lune
	0x6CC00: {0x41000, 0x01438}, // Ultra-Soleil/Ultra-Lune
}

// « 152 h 30 · Julien » ; false si ce n'est pas un Pokémon 3DS reconnu.
func partiePokemon(d []byte) (string, bool) {
	f, ok := formatsPokemon[len(d)]
	if !ok {
		return "", false
	}
	h, m := binary.LittleEndian.Uint16(d[f[0]:]), d[f[0]+2]
	nom := make([]uint16, 13)
	for i := range nom {
		nom[i] = binary.LittleEndian.Uint16(d[f[1]+2*i:])
	}
	dresseur, _, _ := strings.Cut(string(utf16.Decode(nom)), "\x00")
	if dresseur == "" {
		return fmt.Sprintf(tr("%d h %02d · partie jamais commencée", "%d h %02d · game never started"), h, m), true
	}
	return fmt.Sprintf("%d h %02d · %s", h, m, dresseur), true
}
