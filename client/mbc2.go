package main

// MBC2 (Game Boy): 512 4-bit values (docs/notes/research-eeprom-mbc2.md, ADR 0012). mGBA writes 256 bytes, two
// values per byte (even address in the low nibble); SameBoy, VBA-M and BGB write 512 bytes, one value per byte.
// Server form: 512 bytes, high nibble at 0. On PC, a GB file of 256 or 512 bytes can only be MBC2.

func isMBC2(system System, n int) bool { return system == "GB" && (n == 256 || n == 512) }

// normalizeMBC2 converts a file of 256 or 512 bytes to the server form.
func normalizeMBC2(f []byte) []byte {
	s := make([]byte, 512)
	for i := range s {
		if len(f) == 256 {
			s[i] = f[i/2] >> (i % 2 * 4) & 0x0F
		} else {
			s[i] = f[i] & 0x0F
		}
	}
	return s
}

// mbc2File converts the server form to a file of n bytes: 256 (mGBA) or 512 (high nibble at 0xF, read by SameBoy,
// VBA-M and BGB).
func mbc2File(s []byte, n int) []byte {
	f := make([]byte, n)
	for i, v := range s {
		if n == 256 {
			f[i/2] |= v & 0x0F << (i % 2 * 4)
		} else {
			f[i] = v&0x0F | 0xF0
		}
	}
	return f
}

// mbc2Size returns the size of a new MBC2 file for an emulator entry: 256 bytes for mGBA (app or RetroArch core),
// 512 elsewhere.
func (c *Config) mbc2Size(emulator, core string) int {
	if e := c.Emulators[emulator]; e != nil && e.Profile != "" {
		emulator = e.Profile
	}
	if emulator == "mgba" || core == "mGBA" {
		return 256
	}
	return 512
}
