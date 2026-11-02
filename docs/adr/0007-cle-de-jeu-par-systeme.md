# The Key of a Game depends on its System and is read in the ROM

The Server stores each Game under a Key. With GB, GBA and 3DS, the 4-character DS Game code is no longer enough. Each Client computes the Key from the ROM header, never from the file name (names differ between the 3DS and the PC, ADR 0005).

| System | Key | Source |
|---|---|---|
| DS | `ADAF` | Game code, offset `0x0C` (unchanged: the existing History stays valid) |
| GBA | `GBA-BPEF` | game code, offset `0xAC` |
| GB / GBC | `GB-POKEMON_RED-91E6` | title `0x134`–`0x142` (uppercase, other character → `_`, stop at the first `0x00`) + global checksum `0x14E` (big-endian) |
| 3DS | `3DS-00040000001B5000` | title ID (16 hex) |

The Server accepts exactly these four forms, nothing else (no `/`, no `.`).

## Consequences

- Two revisions of the same GB game (different global checksum) are two Games. Two GBA revisions (same game code) are one Game: their saves are compatible.
- A GB game without a visible ROM (mGBA forwarder that hides the ROM in the CIA) has no readable Key: a ROM with the same name on the SD card is needed to read it. Out of v2 scope.
