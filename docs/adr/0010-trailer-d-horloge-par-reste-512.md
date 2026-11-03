# GB/GBA Clock trailer: recognized by the remainder modulo 512

ADR 0008 computes the SRAM as `size - (size mod 1024)`. For a 512-byte SRAM (GBA EEPROM 4 Kbit, GB MBC2 in SameBoy, VBA-M, BGB), the rule gives 0: the code then keeps the whole file, Clock trailer included. A 4 Kbit EEPROM with the mGBA Trailer (528 bytes) therefore does not have the same Hash as the same SRAM without Trailer (512 bytes) on the other machine. Conversely, a 1536-byte file without a Trailer would be cut to 1024. Ticket #6, `docs/notes/research-v3-emulateurs.md` §8.1 (French).

All known GB/GBA SRAMs are multiples of 512 bytes. Known Trailers are 16 bytes (mGBA GBA), 44 bytes (old VBA format) and 48 bytes (mGBA, VBA-M, BGB, GB MBC3+TIMER). Rule, the same on the 3DS (`sram_taille`, `3ds/source/cle.c`) and on the PC (`readLocal`, `client/sync.go`):

- `r = size mod 512`;
- if `r` is 16, 44 or 48 and `size > r`, the SRAM is `size - r`;
- otherwise the SRAM is the whole file.

The rest of ADR 0008 does not change: Hash, size and Upload cover the SRAM only, and each machine keeps its own Trailer.

## Rejected options

- **Keep `mod 1024`, with a special case for 512**: two rules instead of one, and the 1536 case stays wrong.
- **Read the SRAM size in the ROM header**: exact for GB, impossible for GBA (no size field); needs the ROM at each read.

## Consequences

- No existing Save changes Hash for Pokémon sizes: GB 32 KiB (+ 0, 44 or 48 bytes), GBA 128 KiB (+ 0 or 16 bytes) give the same SRAM as before.
- These change SRAM: 512-byte files + Trailer (before: whole file) and files ≥ 1 KiB whose remainder is not a Trailer (before: cut at the KiB). Pokémon games are not in this case.
- GB MBC2 and GBA EEPROM stay out of scope for other reasons (sizes differ by emulator, open_agb_firm byte order): §8.2 and summary of `docs/notes/research-v3-emulateurs.md`.
- The v2 PowerShell Launcher (`pc/sync.ps1`, removed in v3.7.0) kept the old rule.
