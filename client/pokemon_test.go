package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// main synthétique : temps de jeu et dresseur aux positions données.
func mainFactice(taille, temps, dresseur, h, m int, nom string) []byte {
	d := make([]byte, taille)
	binary.LittleEndian.PutUint16(d[temps:], uint16(h))
	d[temps+2] = byte(m)
	for i, c := range utf16.Encode([]rune(nom)) {
		binary.LittleEndian.PutUint16(d[dresseur+2*i:], c)
	}
	return d
}

func TestPartiePokemon(t *testing.T) {
	cas := []struct {
		d       []byte
		attendu string
	}{
		{mainFactice(0x65600, 0x01800, 0x14048, 312, 7, "Julien"), "312 h 07 · Julien"},                  // X/Y
		{mainFactice(0x76000, 0x01800, 0x14048, 1, 59, "Éloise"), "1 h 59 · Éloise"},                     // Rubis Oméga/Saphir Alpha
		{mainFactice(0x6BE00, 0x40C00, 0x01238, 0, 0, ""), "0 h 00 · partie jamais commencée"},           // Soleil/Lune
		{mainFactice(0x6CC00, 0x41000, 0x01438, 999, 30, "ABCDEFGHIJKLMNO"), "999 h 30 · ABCDEFGHIJKLM"}, // 13 caractères au plus
	}
	for _, c := range cas {
		if r, ok := partiePokemon(c.d); !ok || r != c.attendu {
			t.Errorf("taille %#x : %q (%v), attendu %q", len(c.d), r, ok, c.attendu)
		}
	}
	if _, ok := partiePokemon(make([]byte, 1024)); ok {
		t.Error("taille inconnue : refus attendu")
	}
}

// Le Conflit d'un Pokémon 3DS montre le temps de jeu des deux Versions.
func TestConflitTempsDeJeu(t *testing.T) {
	reinitialiser()
	reponses = []string{""}
	pc := mainFactice(0x6BE00, 0x40C00, 0x01238, 152, 30, "Julien")
	srv := &Version{Hash: strings.Repeat("b", 64), Client: "azahar", Age: new(int64)}
	resoudreConflit(&Jeu{Nom: "Pokemon Lune", Systeme: "3DS"}, &Local{Version: Version{Hash: strings.Repeat("a", 64)}, Brut: pc, SRAM: pc},
		srv, mainFactice(0x6BE00, 0x40C00, 0x01238, 0, 0, ""), false)
	if len(questions) != 1 || !strings.Contains(questions[0], "152 h 30 · Julien") || !strings.Contains(questions[0], "0 h 00 · partie jamais commencée") {
		t.Errorf("temps de jeu des deux Versions attendu : %q", questions)
	}
}
