package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// MBC2 (ADR 0012): mGBA 256 B and SameBoy 512 B give the same server form; the Download writes at the size of the
// existing file, otherwise at the size of the emulator.
func TestMBC2(t *testing.T) {
	mgba := make([]byte, 256)
	mgba[0], mgba[255] = 0x21, 0xA5 // even address in the low nibble
	server := make([]byte, 512)
	server[0], server[1], server[510], server[511] = 1, 2, 5, 0xA
	sameboy := make([]byte, 512)
	for i := range sameboy {
		sameboy[i] = 0xF0 | server[i]
	}
	if !bytes.Equal(normalizeMBC2(mgba), server) || !bytes.Equal(normalizeMBC2(sameboy), server) {
		t.Fatal("normaliserMBC2 : forme du serveur fausse")
	}
	if !bytes.Equal(mbc2File(server, 256), mgba) || !bytes.Equal(mbc2File(server, 512), sameboy) {
		t.Fatal("fichierMBC2 : fichier faux")
	}
	if isMBC2("GB", 8192) || isMBC2("GBA", 512) || !isMBC2("GB", 256) {
		t.Fatal("estMBC2")
	}

	dir := t.TempDir()
	j := &Game{System: "GB", Key: "GB-KIRBY-0000", Name: "Kirby", File: filepath.Join(dir, "Kirby.sav")}
	write(t, j.File, sameboy)
	l, err := readLocal(j, nil)
	if err != nil || l.Hash != sha256Hex(server) || l.Size != 512 {
		t.Fatalf("lireLocal MBC2 512 o : err=%v", err)
	}
	other := make([]byte, 512)
	other[3] = 7
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(other) }))
	defer srv.Close()
	s := &Session{cfg: &Config{URL: srv.URL}, dir: dir, profile: "mgba", http: srv.Client()}
	if err := s.download(j, l, &Version{Hash: sha256Hex(other), Size: 512}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(j.File), mbc2File(other, 512)) {
		t.Fatal("Récupération : taille du fichier existant non gardée")
	}
	fresh := &Game{System: "GB", Key: "GB-KIRBY-0000", Name: "Neuf", File: filepath.Join(dir, "Neuf.sav")}
	if err := s.download(fresh, nil, &Version{Hash: sha256Hex(other), Size: 512}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(fresh.File), mbc2File(other, 256)) {
		t.Fatal("Récupération sans fichier : pas 256 octets chez mGBA")
	}
}
