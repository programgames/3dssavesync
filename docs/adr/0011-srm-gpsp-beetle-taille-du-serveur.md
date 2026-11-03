# gpSP and Beetle GBA: the SRAM size comes from the server Version

The RetroArch cores gpSP and Beetle GBA write a fixed-size `.srm`, whatever the save type of the game (ticket #4, `docs/research-v3-emulateurs.md` §4.6, French):

- gpSP: 131072 bytes (128 KiB). The SRAM is at the start; the rest keeps its original value (0xFF, or 0x00 if the loaded file was all 0x00).
- Beetle GBA: 139264 bytes (0x22000). Flash and SRAM at the start, EEPROM at 0x20000. The function that guesses the size (`adjust_save_ram`) exists but is never called: the size does not change.

The Hash must cover the SRAM only, with the same size as with mGBA or the 3DS (ADR 0008 and 0010). The `.srm` does not tell this size. The content is not enough: a Pokémon Emerald game that has written only one slot so far leaves the second Flash bank at 0xFF, and looks like a 64 KiB Flash.

Rule:

- **Upload**: the size is that of the server Current version. The Launcher takes these bytes (at 0x20000 for an EEPROM in Beetle GBA). If a single byte outside this zone is not blank (all 0xFF or all 0x00), it refuses: nothing is cut.
- **No Version on the server**: size unknown, the Launcher refuses with a message ("sync this game first from mGBA or the 3DS"). An all-blank `.srm` counts as absent, like any Blank save.
- **Download**: Backup copy of the `.srm`, then a fixed-size `.srm`, 0xFF everywhere and the received SRAM in its place.

## Rejected options

- **Guess the size from the content** (last non-blank byte): wrong for a half-written 128 KiB Flash. False Conflict (different sizes), or worse, a 64 KiB Version uploaded for a 128 KiB game.
- **Read the type in the ROM** (Nintendo SDK strings `FLASH1M_V`, `SRAM_V`, `EEPROM_V`): reads the whole ROM at each sync, strings sometimes wrong (gpSP keeps a database of fixes), and a 512-byte or 8 KiB EEPROM carries the same string.
- **Upload the whole `.srm`**: a Hash different from mGBA and the 3DS for each game, so Conflicts everywhere.

## Consequences

- No loss possible: the Launcher only cuts blank bytes, and always makes a Backup copy before writing.
- A first Upload from gpSP or Beetle GBA is impossible: the game must already be on the server.
- Beetle GBA with the `gba_use_mednafen_save_method` option (no `.srm`): not supported.
