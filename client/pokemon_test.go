package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// Une sauvegarde synthétique par génération (sommes de contrôle et signatures valides), comme 3ds/test/test_pokemon.c.
// Vérifié aussi, hors dépôt, sur de vraies sauvegardes de chaque génération.
func TestPartiePokemon(t *testing.T) {
	be := binary.BigEndian
	cas := map[string]struct {
		d       []byte
		attendu string
	}{}

	g1 := make([]byte, 0x8000) // J, inconnu, é ; 57 h 32
	copy(g1[0x2598:], []byte{0x89, 0xE0, 0xBC, 0x50})
	g1[0x2CED], g1[0x2CEF] = 57, 32
	g1[0x3523] = byte(^somme(g1[0x2598:0x3523]))
	cas["Gen 1"] = struct {
		d       []byte
		attendu string
	}{g1, "57 h 32 · J?é"}

	g2 := make([]byte, 0x8000) // Cristal : Jean ; heures big-endian
	copy(g2[0x200B:], []byte{0x89, 0xA4, 0xA0, 0xAD, 0x50})
	be.PutUint16(g2[0x2052:], 295)
	g2[0x2054] = 10
	le.PutUint16(g2[0x2D0D:], uint16(somme(g2[0x2009:0x2B83])))
	cas["Gen 2 Cristal"] = struct {
		d       []byte
		attendu string
	}{g2, "295 h 10 · Jean"}

	g3 := make([]byte, 0x20000) // deux moitiés valides, secteurs décalés ; la moitié 1 a le plus grand compteur
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
	cas["Gen 3"] = struct {
		d       []byte
		attendu string
	}{g3, "576 h 48 · Jé"}

	g4 := make([]byte, 0x80000) // Platine : partition 1 plus récente
	le.PutUint32(g4[0x40000+0xCF2C-0x0C:], 0xCF2C)
	le.PutUint32(g4[0x40000+0xCF2C-0x08:], 0x20060623)
	le.PutUint32(g4[0xCF2C-0x14:], 1)
	le.PutUint32(g4[0x40000+0xCF2C-0x14:], 2)
	for i, c := range []uint16{0x134, 0x188, 0xFFFF} {
		le.PutUint16(g4[0x40000+0x68+2*i:], c)
	}
	le.PutUint16(g4[0x40000+0x68+0x22:], 35)
	g4[0x40000+0x68+0x24] = 7
	cas["Gen 4 Platine"] = struct {
		d       []byte
		attendu string
	}{g4, "35 h 07 · Jé"}

	g5 := make([]byte, 0x80000) // Noir 2/Blanc 2 : CRC16-CCITT du bloc 0x25F00
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
	cas["Gen 5 Noir 2/Blanc 2"] = struct {
		d       []byte
		attendu string
	}{g5, "15 h 41 · Jé"}

	g7 := make([]byte, 0x6CC00) // Ultra : 13 caractères au plus
	le.PutUint16(g7[0x41000:], 999)
	g7[0x41002] = 30
	for i, c := range utf16.Encode([]rune("ABCDEFGHIJKLMNO")) {
		le.PutUint16(g7[0x1438+2*i:], c)
	}
	cas["Ultra"] = struct {
		d       []byte
		attendu string
	}{g7, "999 h 30 · ABCDEFGHIJKLM"}
	cas["Soleil/Lune jamais commencé"] = struct {
		d       []byte
		attendu string
	}{make([]byte, 0x6BE00), "0 h 00 · partie jamais commencée"}

	for nom, c := range cas {
		if r, ok := partiePokemon(c.d); !ok || r != c.attendu {
			t.Errorf("%s : %q (%v), attendu %q", nom, r, ok, c.attendu)
		}
	}
	for _, taille := range []int{0x8000, 0x20000, 0x80000, 1024} { // vierges ou taille inconnue : refusés
		if r, ok := partiePokemon(make([]byte, taille)); ok {
			t.Errorf("%d octets vierges reconnus à tort : %q", taille, r)
		}
	}
}

// main synthétique de Soleil/Lune : temps de jeu et dresseur.
func mainSoleilLune(h, m int, nom string) []byte {
	d := make([]byte, 0x6BE00)
	le.PutUint16(d[0x40C00:], uint16(h))
	d[0x40C02] = byte(m)
	for i, c := range utf16.Encode([]rune(nom)) {
		le.PutUint16(d[0x1238+2*i:], c)
	}
	return d
}

// Le Conflit d'un Pokémon montre le temps de jeu des deux Versions.
func TestConflitTempsDeJeu(t *testing.T) {
	reinitialiser()
	reponses = []string{""}
	pc := mainSoleilLune(152, 30, "Julien")
	srv := &Version{Hash: strings.Repeat("b", 64), Client: "azahar", Age: new(int64)}
	resoudreConflit(&Jeu{Nom: "Pokemon Lune", Systeme: "3DS"}, &Local{Version: Version{Hash: strings.Repeat("a", 64)}, Brut: pc, SRAM: pc},
		srv, mainSoleilLune(0, 0, ""), false)
	if len(questions) != 1 || !strings.Contains(questions[0], "152 h 30 · Julien") || !strings.Contains(questions[0], "0 h 00 · partie jamais commencée") {
		t.Errorf("temps de jeu des deux Versions attendu : %q", questions)
	}
}
