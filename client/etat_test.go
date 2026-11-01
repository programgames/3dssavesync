package main

import (
	"encoding/csv"
	"os"
	"strconv"
	"testing"
)

// Même table que le C (3ds/test/test_etat.c) et sync.ps1 : tests/cas_etat.csv, "-" = absent.
func TestEtat(t *testing.T) {
	f, err := os.Open("../tests/cas_etat.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = ';'
	lignes, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	obs := func(hash, taille string) *Version {
		if hash == "-" {
			return nil
		}
		n, _ := strconv.ParseInt(taille, 10, 64)
		return &Version{Hash: hash, Taille: n}
	}
	for _, c := range lignes[1:] {
		base := c[5]
		if base == "-" {
			base = ""
		}
		if got := etat(obs(c[1], c[2]), obs(c[3], c[4]), base); got != c[6] {
			t.Errorf("%s : attendu %s, obtenu %s", c[0], c[6], got)
		}
	}
}
