package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSRM(t *testing.T) {
	ff := func(n int) []byte { return bytes.Repeat([]byte{0xFF}, n) }
	for _, c := range []struct {
		core       string
		size, zone int
	}{
		{"gpSP", 128 << 10, 0}, {"gpSP", 64 << 10, 0}, {"gpSP", 32 << 10, 0}, {"gpSP", 8 << 10, 0}, {"gpSP", 512, 0},
		{"Beetle GBA", 128 << 10, 0}, {"Beetle GBA", 64 << 10, 0}, {"Beetle GBA", 32 << 10, 0},
		{"Beetle GBA", 8 << 10, 0x20000}, {"Beetle GBA", 512, 0x20000},
	} {
		// SRAM -> .srm -> SRAM : même Hash qu'avec mGBA ou la 3DS (SRAM seule).
		sram := random(c.size, int64(c.size))
		o, err := srm(c.core, sram)
		expected := append(append(ff(c.zone), sram...), ff(fixedSRM[c.core]-c.zone-c.size)...)
		if err != nil || !bytes.Equal(o, expected) {
			t.Fatalf("%s %d : .srm écrit faux (err=%v)", c.core, c.size, err)
		}
		srv := &Version{Size: int64(c.size)}
		if s, err := sramSRM(c.core, o, srv); err != nil || !bytes.Equal(s, sram) {
			t.Fatalf("%s %d : SRAM relue fausse (err=%v)", c.core, c.size, err)
		}
		// Octets hors de la SRAM à 0x00 (gpSP ne remplace par 0xFF que la taille du type) : acceptés.
		zero := bytes.Repeat([]byte{0}, len(o))
		copy(zero[c.zone:], sram)
		if s, err := sramSRM(c.core, zero, srv); err != nil || !bytes.Equal(s, sram) {
			t.Fatalf("%s %d : reste à 0x00 refusé (err=%v)", c.core, c.size, err)
		}
		// Une donnée hors de la taille du serveur (autre type de sauvegarde) : refus, rien n'est coupé.
		for _, i := range []int{c.zone + c.size, len(o) - 1, 0} {
			if i >= len(o) || i >= c.zone && i < c.zone+c.size {
				continue
			}
			broken := append([]byte{}, o...)
			broken[i] = 0x12
			if _, err := sramSRM(c.core, broken, srv); err == nil {
				t.Fatalf("%s %d : donnée à 0x%X hors de la SRAM acceptée", c.core, c.size, i)
			}
		}
		// Sans Version du serveur : taille inconnue, refus ; un .srm tout vierge passe (Sauvegarde vierge).
		if _, err := sramSRM(c.core, o, nil); err == nil {
			t.Fatalf("%s %d : .srm sans Version du serveur accepté", c.core, c.size)
		}
	}
	for _, c := range []string{"gpSP", "Beetle GBA"} {
		if s, err := sramSRM(c, ff(fixedSRM[c]), nil); err != nil || !blank(s) {
			t.Fatalf("%s : .srm vierge sans Version refusé (err=%v)", c, err)
		}
		if _, err := sramSRM(c, ff(fixedSRM[c]-1), &Version{Size: 512}); err == nil {
			t.Fatalf("%s : .srm de taille inattendue accepté", c)
		}
		if _, err := srm(c, random(0x20001, 1)); err == nil {
			t.Fatalf("%s : SRAM de plus de 128 Kio acceptée", c)
		}
	}

	// Lanceur : lireLocal coupe à la taille du serveur ; la Récupération copie le .srm, puis écrit le .srm complet.
	dir := t.TempDir()
	sram, other := random(128<<10, 3), random(128<<10, 4)
	j := &Game{System: "GBA", Key: "GBA-BPEF", Name: "Pokemon Emeraude", File: filepath.Join(dir, "Pokemon Emeraude.srm"), Core: "gpSP"}
	raw, _ := srm("gpSP", sram)
	write(t, j.File, raw)
	l, err := readLocal(j, &Version{Size: 128 << 10})
	if err != nil || l.Hash != sha256Hex(sram) || l.Size != 128<<10 || l.Blank {
		t.Fatalf("lireLocal gpSP : err=%v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(other) }))
	defer srv.Close()
	s := &Session{cfg: &Config{URL: srv.URL}, dir: dir, profile: "retroarch", http: srv.Client()}
	if err := s.download(j, l, &Version{Hash: sha256Hex(other), Size: 128 << 10}); err != nil {
		t.Fatal(err)
	}
	if o, _ := srm("gpSP", other); !bytes.Equal(read(j.File), o) {
		t.Fatal("Récupération : .srm écrit faux")
	}
	c, _ := filepath.Glob(filepath.Join(dir, "copies", "retroarch", "Pokemon Emeraude-*.srm"))
	if len(c) != 1 || !bytes.Equal(read(c[0]), raw) {
		t.Fatal("Récupération : pas de Copie de sécurité du .srm")
	}
}
