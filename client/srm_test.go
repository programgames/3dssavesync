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
		coeur        string
		taille, zone int
	}{
		{"gpSP", 128 << 10, 0}, {"gpSP", 64 << 10, 0}, {"gpSP", 32 << 10, 0}, {"gpSP", 8 << 10, 0}, {"gpSP", 512, 0},
		{"Beetle GBA", 128 << 10, 0}, {"Beetle GBA", 64 << 10, 0}, {"Beetle GBA", 32 << 10, 0},
		{"Beetle GBA", 8 << 10, 0x20000}, {"Beetle GBA", 512, 0x20000},
	} {
		// SRAM -> .srm -> SRAM : même Hash qu'avec mGBA ou la 3DS (SRAM seule).
		sram := aleatoire(c.taille, int64(c.taille))
		o, err := srm(c.coeur, sram)
		attendu := append(append(ff(c.zone), sram...), ff(srmFixes[c.coeur]-c.zone-c.taille)...)
		if err != nil || !bytes.Equal(o, attendu) {
			t.Fatalf("%s %d : .srm écrit faux (err=%v)", c.coeur, c.taille, err)
		}
		srv := &Version{Taille: int64(c.taille)}
		if s, err := sramSRM(c.coeur, o, srv); err != nil || !bytes.Equal(s, sram) {
			t.Fatalf("%s %d : SRAM relue fausse (err=%v)", c.coeur, c.taille, err)
		}
		// Octets hors de la SRAM à 0x00 (gpSP ne remplace par 0xFF que la taille du type) : acceptés.
		zero := bytes.Repeat([]byte{0}, len(o))
		copy(zero[c.zone:], sram)
		if s, err := sramSRM(c.coeur, zero, srv); err != nil || !bytes.Equal(s, sram) {
			t.Fatalf("%s %d : reste à 0x00 refusé (err=%v)", c.coeur, c.taille, err)
		}
		// Une donnée hors de la taille du serveur (autre type de sauvegarde) : refus, rien n'est coupé.
		for _, i := range []int{c.zone + c.taille, len(o) - 1, 0} {
			if i >= len(o) || i >= c.zone && i < c.zone+c.taille {
				continue
			}
			casse := append([]byte{}, o...)
			casse[i] = 0x12
			if _, err := sramSRM(c.coeur, casse, srv); err == nil {
				t.Fatalf("%s %d : donnée à 0x%X hors de la SRAM acceptée", c.coeur, c.taille, i)
			}
		}
		// Sans Version du serveur : taille inconnue, refus ; un .srm tout vierge passe (Sauvegarde vierge).
		if _, err := sramSRM(c.coeur, o, nil); err == nil {
			t.Fatalf("%s %d : .srm sans Version du serveur accepté", c.coeur, c.taille)
		}
	}
	for _, c := range []string{"gpSP", "Beetle GBA"} {
		if s, err := sramSRM(c, ff(srmFixes[c]), nil); err != nil || !vierge(s) {
			t.Fatalf("%s : .srm vierge sans Version refusé (err=%v)", c, err)
		}
		if _, err := sramSRM(c, ff(srmFixes[c]-1), &Version{Taille: 512}); err == nil {
			t.Fatalf("%s : .srm de taille inattendue accepté", c)
		}
		if _, err := srm(c, aleatoire(0x20001, 1)); err == nil {
			t.Fatalf("%s : SRAM de plus de 128 Kio acceptée", c)
		}
	}

	// Lanceur : lireLocal coupe à la taille du serveur ; la Récupération copie le .srm, puis écrit le .srm complet.
	dir := t.TempDir()
	sram, autre := aleatoire(128<<10, 3), aleatoire(128<<10, 4)
	j := &Jeu{Systeme: "GBA", Cle: "GBA-BPEF", Nom: "Pokemon Emeraude", Fichier: filepath.Join(dir, "Pokemon Emeraude.srm"), Coeur: "gpSP"}
	brut, _ := srm("gpSP", sram)
	ecrire(t, j.Fichier, brut)
	l, err := lireLocal(j, &Version{Taille: 128 << 10})
	if err != nil || l.Hash != hachage(sram) || l.Taille != 128<<10 || l.Vide {
		t.Fatalf("lireLocal gpSP : err=%v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(autre) }))
	defer srv.Close()
	s := &Session{cfg: &Config{URL: srv.URL}, dossier: dir, profil: "retroarch", http: srv.Client()}
	if err := s.recuperer(j, l, &Version{Hash: hachage(autre), Taille: 128 << 10}); err != nil {
		t.Fatal(err)
	}
	if o, _ := srm("gpSP", autre); !bytes.Equal(lire(j.Fichier), o) {
		t.Fatal("Récupération : .srm écrit faux")
	}
	c, _ := filepath.Glob(filepath.Join(dir, "copies", "retroarch", "Pokemon Emeraude-*.srm"))
	if len(c) != 1 || !bytes.Equal(lire(c[0]), brut) {
		t.Fatal("Récupération : pas de Copie de sécurité du .srm")
	}
}
