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
func fabriquerDSV(sram []byte, ecrite, typ, adresse uint32) []byte {
	o := append(append([]byte{}, sram...), "|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:"...)
	for _, v := range []uint32{ecrite, uint32(len(sram)), typ, adresse, uint32(len(sram)), 0} {
		o = binary.LittleEndian.AppendUint32(o, v)
	}
	return append(o, "|-DESMUME SAVE-|"...)
}

func TestDSV(t *testing.T) {
	dir := t.TempDir()
	sram := aleatoire(512<<10, 7)
	brut := fabriquerDSV(sram, 0x40001, 6, 3) // taille écrite < puce, comme après une croissance par ensure()
	if len(brut) != len(sram)+122 {
		t.Fatalf("pied de %d octets", len(brut)-len(sram))
	}

	// .dsv -> SRAM : le Hash porte sur la SRAM seule.
	j := &Jeu{Systeme: "DS", Cle: "IPKF", Nom: "Pokemon HG", Fichier: filepath.Join(dir, "Pokemon HG.dsv")}
	ecrire(t, j.Fichier, brut)
	l, err := lireLocal(j, nil)
	if err != nil || !bytes.Equal(l.SRAM, sram) || l.Hash != hachage(sram) || l.Taille != 512<<10 || l.Vide {
		t.Fatalf("lireLocal : err=%v taille=%d", err, len(l.SRAM))
	}

	// SRAM -> .dsv : même taille, le pied existant reste tel quel, fichier identique.
	if o, err := dsv(sram, brut); err != nil || !bytes.Equal(o, brut) {
		t.Fatalf(".dsv -> SRAM -> .dsv différent (err=%v)", err)
	}
	autre := aleatoire(512<<10, 8)
	if o, _ := dsv(autre, brut); !bytes.Equal(o[:len(autre)], autre) || !bytes.Equal(o[len(autre):], brut[len(sram):]) {
		t.Fatal("nouvelle SRAM : le pied existant doit rester")
	}

	// Pas de fichier, ou autre taille : pied neuf, comme DeSmuME à l'import d'un .sav brut de 8 Kio.
	petite := aleatoire(8<<10, 9)
	for _, ancien := range [][]byte{nil, brut} {
		o, err := dsv(petite, ancien)
		if err != nil || !bytes.Equal(o, fabriquerDSV(petite, 8<<10, 2, 2)) {
			t.Fatalf("pied neuf invalide (err=%v)", err)
		}
		if s, err := sramDSV(o); err != nil || !bytes.Equal(s, petite) {
			t.Fatal("pied neuf illisible")
		}
	}
	if _, err := dsv(aleatoire(1000, 1), nil); err == nil {
		t.Fatal("taille hors des puces DeSmuME acceptée")
	}

	// .dsv vide (jeu lancé sans sauvegarde) : SRAM vide, vierge. Sans pied valide : refus.
	vide, casse := filepath.Join(dir, "vide.dsv"), filepath.Join(dir, "casse.dsv")
	ecrire(t, vide, nil)
	if l, err := lireLocal(&Jeu{Systeme: "DS", Fichier: vide}, nil); err != nil || len(l.SRAM) != 0 || !l.Vide {
		t.Fatalf(".dsv vide : err=%v", err)
	}
	faux := append([]byte{}, brut...)
	binary.LittleEndian.PutUint32(faux[len(sram)+86:], 1234) // taille remplie fausse
	for _, b := range [][]byte{sram, faux, brut[:100]} {
		ecrire(t, casse, b)
		if _, err := lireLocal(&Jeu{Systeme: "DS", Fichier: casse}, nil); err == nil {
			t.Fatal("pied absent ou faux accepté")
		}
		os.Remove(casse)
	}

	// Récupération : Copie de sécurité du .dsv, puis SRAM reçue + pied existant.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(autre) }))
	defer srv.Close()
	s := &Session{cfg: &Config{URL: srv.URL}, dossier: dir, profil: "desmume", http: srv.Client()}
	if err := s.recuperer(j, l, &Version{Hash: hachage(autre)}); err != nil {
		t.Fatal(err)
	}
	if o := lire(j.Fichier); !bytes.Equal(o, append(append([]byte{}, autre...), brut[len(sram):]...)) {
		t.Fatal("Récupération : .dsv écrit faux")
	}
	c, _ := filepath.Glob(filepath.Join(dir, "copies", "desmume", "Pokemon HG-*.dsv"))
	if len(c) != 1 || !bytes.Equal(lire(c[0]), brut) {
		t.Fatal("Récupération : pas de Copie de sécurité du .dsv")
	}
}
