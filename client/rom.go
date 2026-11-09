package main

// ROMs and Keys (ADR 0007): the System of a ROM, its Key, and the one definition of a valid Key, read by the ROM
// reader, the Aliases (ADR 0014), the History and the local server.

import (
	"archive/zip"
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// System is the original console of a Game (GB covers GB and GBC).
type System string

const (
	DS   System = "DS"
	GB   System = "GB"
	GBA  System = "GBA"
	N3DS System = "3DS"
)

// keyShapes holds the Key shape of each System, as the server accepts it.
var keyShapes = map[System]*regexp.Regexp{}

// keyPattern matches any Key (a capture group, for the routes of the local server).
var keyPattern = func() string {
	shapes := []struct {
		system System
		shape  string
	}{{DS, `[A-Z0-9]{4}`}, {GBA, `GBA-[A-Z0-9]{4}`}, {GB, `GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}`}, {N3DS, `(?:3DSA?-[0-9A-F]{16}|3DSX-[0-9A-F]{8})`}}
	var all []string
	for _, s := range shapes {
		keyShapes[s.system] = regexp.MustCompile(`^` + s.shape + `$`)
		all = append(all, s.shape)
	}
	return `(` + strings.Join(all, "|") + `)`
}()

var reKey = regexp.MustCompile(`^` + keyPattern + `$`)

// validKey reports a Key of any System.
func validKey(key string) bool { return reKey.MatchString(key) }

// extensions maps a ROM extension to its System.
var extensions = map[string]System{
	".nds": DS, ".gb": GB, ".gbc": GB, ".gba": GBA, ".3ds": N3DS, ".cci": N3DS, ".cxi": N3DS,
}

// titles3DS lists the 8 3DS Pokémon games (ADR 0009): one title ID per game, all regions. They keep the Key
// 3DS-<title ID> and the raw bytes of /main; every other 3DS game is a 3DSA-<title ID> archive (ADR 0028).
var titles3DS = map[string]string{
	"0004000000055D00": "Pokemon X", "0004000000055E00": "Pokemon Y",
	"000400000011C400": "Pokemon Rubis Omega", "000400000011C500": "Pokemon Saphir Alpha",
	"0004000000164800": "Pokemon Soleil", "0004000000175E00": "Pokemon Lune",
	"00040000001B5000": "Pokemon Ultra-Soleil", "00040000001B5100": "Pokemon Ultra-Lune",
}

// ignored3DS: games whose save is tied to one console, never synced (ADR 0028, "titres-ignores" on the 3DS).
var ignored3DS = map[string]bool{"00040000001A0300": true, "00040000001A0400": true, "00040000001A0500": true, "00040000001BB800": true}

// ignoredExtdata: extdata never synced (ADR 0031, "extdata-ignorees" on the 3DS): NetPass's SpotPass downloads and the
// extdata of the ignored3DS games. Key = extdata ID.
var ignoredExtdata = func() map[string]bool {
	m := map[string]bool{extdataID("000400000F657400"): true}
	for id := range ignored3DS {
		m[extdataID(id)] = true
	}
	return m
}()

// extdataID returns the extdata ID of a 3DS title ID as Azahar devices map it: titleLow >> 8, 8 upper-case hex digits
// (ADR 0031). "" for a title ID that is not 16 hex digits.
func extdataID(titleID string) string {
	low, err := strconv.ParseUint(titleID[min(8, len(titleID)):], 16, 32)
	if len(titleID) != 16 || err != nil {
		return ""
	}
	return fmt.Sprintf("%08X", low>>8)
}

// names3DS holds the names given in config.json (titres_3ds) to 3DS games: title ID -> name (newSession).
var names3DS = map[string]string{}

// key3DS returns the Key of a 3DS title ID: 3DS-<id> for the 8 Pokémon games, 3DSA-<id> for any other application
// (title ID high word 00040000) outside ignored3DS, "" otherwise (ADR 0028).
func key3DS(id string) string {
	switch {
	case titles3DS[id] != "":
		return "3DS-" + id
	case len(id) == 16 && strings.HasPrefix(id, "00040000") && !ignored3DS[id]:
		return "3DSA-" + id
	}
	return ""
}

// isArchive: a 3DSA Game (its whole Azahar save folder, ADR 0028) or a 3DSX extdata (the files under its user/ folder,
// ADR 0031), whose Local save is the 3dssavesync archive of the folder j.File.
func isArchive(j *Game) bool {
	return strings.HasPrefix(j.Key, "3DSA-") || strings.HasPrefix(j.Key, "3DSX-")
}

// Rom is what a ROM tells: its System and Key, and the possible save names.
// Empty System = unknown extension; empty Key = out of scope (homebrew, ignored 3DS game).
// Name: the ROM or the zip (mGBA, RetroArch); Internal: the file inside the zip (melonDS), "" outside a zip.
type Rom struct {
	System         System
	Key            string
	Name, Internal string
}

// readRom reads the header of a ROM (file or .zip). On error, Name is still set.
func readRom(rom string) (Rom, error) {
	ext := strings.ToLower(filepath.Ext(rom))
	r := Rom{Name: strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom))}
	if ext != ".zip" && extensions[ext] == "" {
		return r, nil
	}
	header := make([]byte, 0x200)
	if ext == ".zip" {
		z, err := zip.OpenReader(rom)
		if err != nil {
			return r, err
		}
		defer z.Close()
		for _, f := range z.File {
			e := strings.ToLower(path.Ext(f.Name))
			if s := extensions[e]; s == DS || s == GB || s == GBA {
				o, err := f.Open()
				if err != nil {
					return r, err
				}
				io.ReadFull(o, header) // a ROM shorter than the header keeps zeros
				o.Close()
				ext = e
				r.Internal = strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name))
				break
			}
		}
	} else {
		f, err := os.Open(rom)
		if err != nil {
			return r, err
		}
		io.ReadFull(f, header)
		f.Close()
	}
	system := extensions[ext]
	key := ""
	switch system {
	case DS:
		key = string(header[0x0C:0x10])
	case GBA:
		key = "GBA-" + string(header[0xAC:0xB0])
	case GB:
		title := header[0x134:0x143]
		if i := bytes.IndexByte(title, 0); i >= 0 {
			title = title[:i]
		}
		clean := make([]byte, len(title))
		for i, c := range title { // byte by byte like key.c: bytes.ToUpper would change the length (byte >= 0x80)
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				c = '_'
			}
			clean[i] = c
		}
		key = fmt.Sprintf("GB-%s-%02X%02X", clean, header[0x14E], header[0x14F])
	case N3DS:
		id, err := titleID3DS(rom)
		if err != nil {
			return Rom{Name: r.Name}, err
		}
		key = key3DS(id)
		if t := cmp.Or(titles3DS[id], names3DS[id]); t != "" && key != "" {
			r.Name = t
		}
	}
	r.System = system
	if key != "" && keyShapes[system].MatchString(key) {
		r.Key = key
	}
	return r, nil
}

// titleID3DS returns the title ID of a 3DS ROM: NCSD (.3ds/.cci) -> partition 0 at u32@0x120 × 0x200, or direct
// NCCH (.cxi); program_id at NCCH + 0x118.
func titleID3DS(rom string) (string, error) {
	f, err := os.Open(rom)
	if err != nil {
		return "", err
	}
	defer f.Close()
	o := make([]byte, 0x200)
	io.ReadFull(f, o)
	var ncch int64
	switch string(o[0x100:0x104]) {
	case "NCSD":
		ncch = int64(binary.LittleEndian.Uint32(o[0x120:])) * 0x200
	case "NCCH":
	default:
		return "", nil
	}
	id := make([]byte, 8)
	if _, err := f.ReadAt(id, ncch+0x118); err != nil {
		return "", nil
	}
	return fmt.Sprintf("%016X", binary.LittleEndian.Uint64(id)), nil
}
