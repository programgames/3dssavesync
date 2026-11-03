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

// makeDSV writes a .dsv like DeSmuME (mc.cpp BackupDevice::ensure): SRAM, text, 6 u32 LE, cookie.
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
	raw := makeDSV(sram, 0x40001, 6, 3) // written size < chip, like after a growth by ensure()
	if len(raw) != len(sram)+122 {
		t.Fatalf("pied de %d octets", len(raw)-len(sram))
	}

	// .dsv -> SRAM: the Hash covers the SRAM only.
	j := &Game{System: "DS", Key: "IPKF", Name: "Pokemon HG", File: filepath.Join(dir, "Pokemon HG.dsv")}
	write(t, j.File, raw)
	l, err := readLocal(j, nil)
	if err != nil || !bytes.Equal(l.SRAM, sram) || l.Hash != sha256Hex(sram) || l.Size != 512<<10 || l.Blank {
		t.Fatalf("lireLocal : err=%v taille=%d", err, len(l.SRAM))
	}

	// SRAM -> .dsv: same size, the existing footer stays as it is, identical file.
	if o, err := dsv(sram, raw); err != nil || !bytes.Equal(o, raw) {
		t.Fatalf(".dsv -> SRAM -> .dsv différent (err=%v)", err)
	}
	other := random(512<<10, 8)
	if o, _ := dsv(other, raw); !bytes.Equal(o[:len(other)], other) || !bytes.Equal(o[len(other):], raw[len(sram):]) {
		t.Fatal("nouvelle SRAM : le pied existant doit rester")
	}

	// No file, or another size: new footer, like DeSmuME when it imports a raw 8 KiB .sav.
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

	// Empty .dsv (game started with no save): empty SRAM, blank. Without a valid footer: refused.
	empty, broken := filepath.Join(dir, "vide.dsv"), filepath.Join(dir, "casse.dsv")
	write(t, empty, nil)
	if l, err := readLocal(&Game{System: "DS", File: empty}, nil); err != nil || len(l.SRAM) != 0 || !l.Blank {
		t.Fatalf(".dsv vide : err=%v", err)
	}
	fake := append([]byte{}, raw...)
	binary.LittleEndian.PutUint32(fake[len(sram)+86:], 1234) // wrong filled size
	for _, b := range [][]byte{sram, fake, raw[:100]} {
		write(t, broken, b)
		if _, err := readLocal(&Game{System: "DS", File: broken}, nil); err == nil {
			t.Fatal("pied absent ou faux accepté")
		}
		os.Remove(broken)
	}

	// Download: Backup copy of the .dsv, then received SRAM + existing footer.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(other) }))
	defer srv.Close()
	s := testClient(srv, dir, "desmume")
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
