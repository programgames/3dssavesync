package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// tests/archive3ds/cases.json: the shared cases of the 3dssavesync archive (ADR 0028). Each case lists files (data as
// hex) and the SHA-256 of the archive; the 3DS and Android tests read the same file. UPDATE_ARCHIVE=1 go test rewrites
// the Hashes from this implementation.
type archiveCase struct {
	Name  string `json:"name"`
	Files []struct {
		Path string `json:"path"`
		Hex  string `json:"hex"`
	} `json:"files"`
	SHA256 string `json:"sha256"`
	Error  bool   `json:"error,omitempty"`
}

func readArchiveCases(t *testing.T) ([]archiveCase, string) {
	path := filepath.Join("..", "tests", "archive3ds", "cases.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []archiveCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	return cases, path
}

func TestArchiveSharedCases(t *testing.T) {
	cases, path := readArchiveCases(t)
	update := os.Getenv("UPDATE_ARCHIVE") == "1"
	for i, c := range cases {
		var files []ArchiveFile
		for _, f := range c.Files {
			d, err := hex.DecodeString(f.Hex)
			if err != nil {
				t.Fatal(c.Name, err)
			}
			files = append(files, ArchiveFile{f.Path, d})
		}
		a, err := PackArchive(files)
		if c.Error {
			if err == nil {
				t.Errorf("%s: refused expected", c.Name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		sum := sha256.Sum256(a)
		got := hex.EncodeToString(sum[:])
		if update {
			cases[i].SHA256 = got
		} else if got != c.SHA256 {
			t.Errorf("%s: SHA-256 %s, expected %s", c.Name, got, c.SHA256)
		}
		back, err := UnpackArchive(a)
		if err != nil || len(back) != len(files) {
			t.Fatalf("%s: read back: %v", c.Name, err)
		}
		// a standard tar reader sees the same files: the archive opens in 7-Zip or tar
		r := tar.NewReader(bytes.NewReader(a))
		n := 0
		for {
			h, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: archive/tar: %v", c.Name, err)
			}
			d, _ := io.ReadAll(r)
			if h.Name != back[n].Path || !bytes.Equal(d, back[n].Data) {
				t.Errorf("%s: archive/tar reads %s differently", c.Name, h.Name)
			}
			n++
		}
	}
	if update {
		b, _ := json.MarshalIndent(cases, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestArchiveRefusesDamage(t *testing.T) {
	a, _ := PackArchive([]ArchiveFile{{"main", []byte("abc")}, {"sub/x", make([]byte, 600)}})
	bad := map[string][]byte{
		"cut":           a[:len(a)-1],
		"extra":         append(append([]byte(nil), a...), 0),
		"checksum":      func() []byte { b := append([]byte(nil), a...); b[0] = 'M'; return b }(),
		"directory":     func() []byte { b, _ := PackArchive([]ArchiveFile{{"d", nil}}); b[156] = '5'; return b }(),
		"empty":         nil,
		"no end blocks": a[:len(a)-2*tarBlock],
	}
	for name, b := range bad {
		if _, err := UnpackArchive(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := UnpackArchive(func() []byte { b, _ := PackArchive(nil); return b }()); err != nil {
		t.Errorf("empty archive (no files): %v", err)
	}
}
