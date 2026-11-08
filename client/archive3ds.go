package main

// 3dssavesync archive (ADR 0028): the files of a 3DS save archive as one deterministic ustar, so the 3DS, the PC and
// Android give the same bytes, hence the same Hash, for the same files. The 3DS (3ds/source/archive.c) and Android
// (core/Archive3ds.kt) follow the same rules; tests/archive3ds/ holds the shared cases.

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ArchiveFile is one file of a save archive: its path from the archive root ("main", "dir/file") and its bytes.
type ArchiveFile struct {
	Path string
	Data []byte
}

const tarBlock = 512

var errArchive = errors.New("3dssavesync archive: damaged")

// validArchivePath: relative, '/' separators, no empty, "." or ".." part, at most 100 bytes (the ustar name field).
func validArchivePath(p string) bool {
	if p == "" || len(p) > 100 || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.ContainsRune(p, 0) {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// octal writes v as n-1 octal digits then NUL in field.
func octal(field []byte, v int64) {
	s := strconv.FormatInt(v, 8)
	copy(field, strings.Repeat("0", len(field)-1-len(s))+s)
	field[len(field)-1] = 0
}

// PackArchive returns the archive of files, sorted by path. An invalid path or a duplicate is an error (the game is
// then Not synced, with the reason).
func PackArchive(files []ArchiveFile) ([]byte, error) {
	sorted := append([]ArchiveFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var out bytes.Buffer
	for i, f := range sorted {
		if !validArchivePath(f.Path) {
			return nil, fmt.Errorf("3DS save file %q: path not supported (100 bytes at most)", f.Path)
		}
		if i > 0 && sorted[i-1].Path == f.Path {
			return nil, fmt.Errorf("3DS save file %q: twice", f.Path)
		}
		h := make([]byte, tarBlock)
		copy(h[0:100], f.Path)
		octal(h[100:108], 0o644)
		octal(h[108:116], 0)
		octal(h[116:124], 0)
		octal(h[124:136], int64(len(f.Data)))
		octal(h[136:148], 0)
		h[156] = '0'
		copy(h[257:263], "ustar\x00")
		copy(h[263:265], "00")
		octal(h[329:337], 0)
		octal(h[337:345], 0)
		copy(h[148:156], "        ")
		sum := 0
		for _, b := range h {
			sum += int(b)
		}
		octal(h[148:155], int64(sum)) // 6 digits and NUL
		h[155] = ' '
		out.Write(h)
		out.Write(f.Data)
		if pad := len(f.Data) % tarBlock; pad != 0 {
			out.Write(make([]byte, tarBlock-pad))
		}
	}
	out.Write(make([]byte, 2*tarBlock))
	return out.Bytes(), nil
}

// UnpackArchive reads an archive written by PackArchive. Anything else (other entry types, unsorted or duplicate
// paths, a bad checksum, data past the end) is refused: a damaged version is never written to a save.
func UnpackArchive(data []byte) ([]ArchiveFile, error) {
	var files []ArchiveFile
	for off := 0; ; {
		if off+tarBlock > len(data) {
			return nil, errArchive
		}
		h := data[off : off+tarBlock]
		if bytes.Equal(h, make([]byte, tarBlock)) {
			if len(data) != off+2*tarBlock || !bytes.Equal(data[off+tarBlock:], make([]byte, tarBlock)) {
				return nil, errArchive
			}
			return files, nil
		}
		name := string(bytes.TrimRight(h[0:100], "\x00"))
		size, err := strconv.ParseInt(strings.TrimRight(string(h[124:135]), "\x00"), 8, 64)
		if err != nil || size < 0 || h[156] != '0' || string(h[257:263]) != "ustar\x00" || h[345] != 0 || !validArchivePath(name) {
			return nil, errArchive
		}
		check := make([]byte, tarBlock)
		copy(check, h)
		copy(check[148:156], "        ")
		sum := 0
		for _, b := range check {
			sum += int(b)
		}
		want, err := strconv.ParseInt(strings.TrimRight(string(h[148:155]), "\x00"), 8, 64)
		if err != nil || int64(sum) != want {
			return nil, errArchive
		}
		if len(files) > 0 && files[len(files)-1].Path >= name {
			return nil, errArchive
		}
		start := off + tarBlock
		end := start + int(size)
		if end > len(data) {
			return nil, errArchive
		}
		files = append(files, ArchiveFile{Path: name, Data: append([]byte(nil), data[start:end]...)})
		off = end
		if pad := int(size) % tarBlock; pad != 0 {
			off += tarBlock - pad
		}
	}
}
