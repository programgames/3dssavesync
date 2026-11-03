package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAtomic: the target gets the new content or keeps the old one; no temporary file is ever left behind.
func TestWriteAtomicLeavesNoTempFile(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "sous", "a.sav")
	only := func(names ...string) {
		t.Helper()
		var got []string
		filepath.WalkDir(d, func(p string, e os.DirEntry, _ error) error {
			if !e.IsDir() {
				r, _ := filepath.Rel(d, p)
				got = append(got, filepath.ToSlash(r))
			}
			return nil
		})
		if strings.Join(got, ",") != strings.Join(names, ",") {
			t.Errorf("files: %v, expected %v", got, names)
		}
	}
	if err := writeAtomic(f, []byte("v1")); err != nil || string(read(f)) != "v1" {
		t.Fatalf("new file: %v %q", err, read(f))
	}
	write(t, f+".3dssavesync-tmp", []byte("leftover of a hard stop"))
	if err := writeAtomic(f, []byte("v2")); err != nil || string(read(f)) != "v2" {
		t.Fatalf("replace: %v %q", err, read(f))
	}
	only("sous/a.sav")

	// The target is a non-empty folder: the rename fails, the folder stays, the temporary file goes.
	busy := filepath.Join(d, "dossier")
	write(t, filepath.Join(busy, "x"), []byte("x"))
	if err := writeAtomic(busy, []byte("v3")); err == nil {
		t.Fatal("rename over a folder accepted")
	}
	if string(read(filepath.Join(busy, "x"))) != "x" {
		t.Error("folder changed")
	}
	only("dossier/x", "sous/a.sav")
}
