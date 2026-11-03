package main

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// .dsv écrit comme DeSmuME (mc.cpp BackupDevice::ensure) : SRAM, texte, 6 u32 LE, cookie.
func makeDSV(sram []byte, written, typ, address uint32) []byte {
	o := append(append([]byte{}, sram...), "|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:"...)
	for _, v := range []uint32{written, uint32(len(sram)), typ, address, uint32(len(sram)), 0} {
		o = binary.LittleEndian.AppendUint32(o, v)
	}
	return append(o, "|-DESMUME SAVE-|"...)
}

func TestDSV(t *testing.T) {
	dir := t.TempDir()
	sram := random(512<<10, 7)
	raw := makeDSV(sram, 0x40001, 6, 3) // taille écrite < puce, comme après une croissance par ensure()
	if len(raw) != len(sram)+122 {
		t.Fatalf("pied de %d octets", len(raw)-len(sram))
	}

	// .dsv -> SRAM : le Hash porte sur la SRAM seule.
	j := &Game{System: "DS", Key: "IPKF", Name: "Pokemon HG", File: filepath.Join(dir, "Pokemon HG.dsv")}
	write(t, j.File, raw)
	l, err := readLocal(j, nil)
	if err != nil || !bytes.Equal(l.SRAM, sram) || l.Hash != sha256Hex(sram) || l.Size != 512<<10 || l.Blank {
		t.Fatalf("lireLocal : err=%v taille=%d", err, len(l.SRAM))
	}

	// SRAM -> .dsv : même taille, le pied existant reste tel quel, fichier identique.
	if o, err := dsv(sram, raw); err != nil || !bytes.Equal(o, raw) {
		t.Fatalf(".dsv -> SRAM -> .dsv différent (err=%v)", err)
	}
	other := random(512<<10, 8)
	if o, _ := dsv(other, raw); !bytes.Equal(o[:len(other)], other) || !bytes.Equal(o[len(other):], raw[len(sram):]) {
		t.Fatal("nouvelle SRAM : le pied existant doit rester")
	}

	// Pas de fichier, ou autre taille : pied neuf, comme DeSmuME à l'import d'un .sav brut de 8 Kio.
	small := random(8<<10, 9)
	for _, old := range [][]byte{nil, raw} {
		o, err := dsv(small, old)
		if err != nil || !bytes.Equal(o, makeDSV(small, 8<<10, 2, 2)) {
			t.Fatalf("pied neuf invalide (err=%v)", err)
		}
		if s, err := sramDSV(o); err != nil || !bytes.Equal(s, small) {
			t.Fatal("pied neuf illisible")
		}
	}
	if _, err := dsv(random(1000, 1), nil); err == nil {
		t.Fatal("taille hors des puces DeSmuME acceptée")
	}

	// .dsv vide (jeu lancé sans sauvegarde) : SRAM vide, vierge. Sans pied valide : refus.
	empty, broken := filepath.Join(dir, "vide.dsv"), filepath.Join(dir, "casse.dsv")
	write(t, empty, nil)
	if l, err := readLocal(&Game{System: "DS", File: empty}, nil); err != nil || len(l.SRAM) != 0 || !l.Blank {
		t.Fatalf(".dsv vide : err=%v", err)
	}
	fake := append([]byte{}, raw...)
	binary.LittleEndian.PutUint32(fake[len(sram)+86:], 1234) // taille remplie fausse
	for _, b := range [][]byte{sram, fake, raw[:100]} {
		write(t, broken, b)
		if _, err := readLocal(&Game{System: "DS", File: broken}, nil); err == nil {
			t.Fatal("pied absent ou faux accepté")
		}
		os.Remove(broken)
	}

	// Récupération : Copie de sécurité du .dsv, puis SRAM reçue + pied existant.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(other) }))
	defer srv.Close()
	s := &Session{cfg: &Config{URL: srv.URL}, dir: dir, profile: "desmume", http: srv.Client()}
	if err := s.download(j, l, &Version{Hash: sha256Hex(other)}); err != nil {
		t.Fatal(err)
	}
	if o := read(j.File); !bytes.Equal(o, append(append([]byte{}, other...), raw[len(sram):]...)) {
		t.Fatal("Récupération : .dsv écrit faux")
	}
	c, _ := filepath.Glob(filepath.Join(dir, "copies", "desmume", "Pokemon HG-*.dsv"))
	if len(c) != 1 || !bytes.Equal(read(c[0]), raw) {
		t.Fatal("Récupération : pas de Copie de sécurité du .dsv")
	}
}
