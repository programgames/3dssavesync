package main

import (
	"encoding/csv"
	"os"
	"strconv"
	"testing"
)

// Même table que le C (3ds/test/test_etat.c) et sync.ps1 : tests/cas_etat.csv, "-" = absent.
func TestState(t *testing.T) {
	f, err := os.Open("../tests/cas_etat.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = ';'
	lines, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	obs := func(hash, size string) *Version {
		if hash == "-" {
			return nil
		}
		n, _ := strconv.ParseInt(size, 10, 64)
		return &Version{Hash: hash, Size: n}
	}
	for _, c := range lines[1:] {
		base := c[5]
		if base == "-" {
			base = ""
		}
		if got := state(obs(c[1], c[2]), obs(c[3], c[4]), base); got != c[6] {
			t.Errorf("%s : attendu %s, obtenu %s", c[0], c[6], got)
		}
	}
}
