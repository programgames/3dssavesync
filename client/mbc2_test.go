package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// MBC2 (ADR 0012) : mGBA 256 o et SameBoy 512 o donnent la même forme du serveur ; la Récupération écrit à la taille
// du fichier existant, sinon à celle de l'émulateur.
func TestMBC2(t *testing.T) {
	mgba := make([]byte, 256)
	mgba[0], mgba[255] = 0x21, 0xA5 // adresse paire dans le quartet bas
	serveur := make([]byte, 512)
	serveur[0], serveur[1], serveur[510], serveur[511] = 1, 2, 5, 0xA
	sameboy := make([]byte, 512)
	for i := range sameboy {
		sameboy[i] = 0xF0 | serveur[i]
	}
	if !bytes.Equal(normaliserMBC2(mgba), serveur) || !bytes.Equal(normaliserMBC2(sameboy), serveur) {
		t.Fatal("normaliserMBC2 : forme du serveur fausse")
	}
	if !bytes.Equal(fichierMBC2(serveur, 256), mgba) || !bytes.Equal(fichierMBC2(serveur, 512), sameboy) {
		t.Fatal("fichierMBC2 : fichier faux")
	}
	if estMBC2("GB", 8192) || estMBC2("GBA", 512) || !estMBC2("GB", 256) {
		t.Fatal("estMBC2")
	}

	dir := t.TempDir()
	j := &Jeu{Systeme: "GB", Cle: "GB-KIRBY-0000", Nom: "Kirby", Fichier: filepath.Join(dir, "Kirby.sav")}
	ecrire(t, j.Fichier, sameboy)
	l, err := lireLocal(j, nil)
	if err != nil || l.Hash != hachage(serveur) || l.Taille != 512 {
		t.Fatalf("lireLocal MBC2 512 o : err=%v", err)
	}
	autre := make([]byte, 512)
	autre[3] = 7
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(autre) }))
	defer srv.Close()
	s := &Session{cfg: &Config{URL: srv.URL}, dossier: dir, profil: "mgba", http: srv.Client()}
	if err := s.recuperer(j, l, &Version{Hash: hachage(autre), Taille: 512}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lire(j.Fichier), fichierMBC2(autre, 512)) {
		t.Fatal("Récupération : taille du fichier existant non gardée")
	}
	neuf := &Jeu{Systeme: "GB", Cle: "GB-KIRBY-0000", Nom: "Neuf", Fichier: filepath.Join(dir, "Neuf.sav")}
	if err := s.recuperer(neuf, nil, &Version{Hash: hachage(autre), Taille: 512}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lire(neuf.Fichier), fichierMBC2(autre, 256)) {
		t.Fatal("Récupération sans fichier : pas 256 octets chez mGBA")
	}
}
