# GB/GBC Virtual Console games: key read through PxiFS0, one more Local save

A Game Boy or Game Boy Color game bought on the 3DS eShop (Virtual Console) keeps its save in its own save archive, as `/sav.dat`. To sync it like the cartridge ROM of the same game, the 3DS app needs the game's Key, which comes from the ROM header (ADR 0007), and the ROM is inside the title's RomFS.

## Decision

- **Which titles.** Every SD title whose product code (`AM_GetTitleProductCode`) has a game code starting with `R` (Game Boy) or `Q` (Game Boy Color), with `CTR-N-` or `CTR-P-` in front. In 3dsdb, all 271 titles with such a code are GB/GBC Virtual Console games. A real 3DS reports French Pokémon Red and Yellow with `CTR-P-`, while 3dsdb lists `CTR-N-`, so both prefixes are kept.
- **The Key comes from the ROM itself.** The VC ROM is the cartridge ROM (the emulator applies its patch at run time), so its Key is the cartridge's: on a real 3DS, Pokémon Crystal (French) reads `GB-PM_CRYSTAL-F2E2`, the same Key as the mGBA forwarder of the cartridge ROM. A Key is never guessed: no read, no sync.
- **How the ROM is read.** `fs:USER` never gives the RomFS of an SD application (`0xD9004676` on every path, whatever the FS access bits; Luma3DS already grants all of them). So the app uses Luma3DS's `svcControlService` to get a session to `PxiFS0` (the FS to Process9 interface, as Checkpoint does for raw saves), opens archive `0x2345678A` and reads the RomFS with the Title Access file path that `fs` itself sends: 0x24 bytes, a mask that allows all content types, and the title's NCCH seed from `fs:USER` GetSeed (the Pokémon VC titles are seeded; without the seed the first read fails with `0xD9004587`). Only the first 0x200 bytes of the file in `rom/` are read.
- **One more Local save, synced on its own.** A VC game and a ROM or forwarder with the same Key are two Local saves of one game, like two devices: each has its own Base (`VC-<title ID>` in `base.ini` for the VC game), and the server links them. Sync all uploads first, reads the server again, then downloads, so that what one uploads the other gets in the same pass. Two VC titles with one Key are a Duplicate, as two ROMs.
- **Writing `/sav.dat`.** Received SRAM + the footer of the old `sav.dat` (16 bytes for Gold, Silver and Crystal, the VC's clock; none for Gen 1), same size only, then commit, removal of the secure value, and a read back. Backup copy first, as for every Download.
- **A VC game never started gets its first save.** It has no save archive at all. With **A** on the game (never in Sync all), the app formats its archive like the VC Pokémon archives read on a real 3DS (1 directory, 4 files, duplicate data; `FormatSaveData` takes no size, FS reads it from the title), as save-data-copy-tool does for games never started, then creates `/sav.dat` = the server SRAM (its size from ROM byte `0x149`) + a zero 16-byte footer for a clock cartridge (`0x0F`, `0x10`), commits, removes the secure value and reads it back. An archive that opens is never formatted (that would erase its save); an archive without `sav.dat`, MBC2 and an unknown RAM size are left alone. Checked on a real 3DS: Pokémon Blue (Gen 1) and Gold (Gen 2, zero clock footer) load the created save.
- **Restore points.** A title whose archive holds anything but `sav.dat` (and `SecureValue`), such as `rsm*.dat`, is not synced: the emulator could bring an older game back over the synced save.

## Rejected options

- **A table of Keys per title ID**: it would have to cover about 280 titles, and every checksum needs a dumped ROM; one wrong entry syncs the wrong game.
- **One active copy per game, chosen with X** (as the mGBA forwarder and the ROM side already work): a VC save can't be emptied like a file, so the inactive copy would keep an old game you could start by mistake.
- **More FS access bits in the RSF**: the refusal is in `fs` for SD content, not an access bit.

## Consequences

- Reading the Key needs Luma3DS. Without it (or if the read fails), the game is listed Not synced with the step that failed, and nothing is written.
- Playing the VC game and the forwarder of the same game without a sync in between gives a Conflict, as with two devices. Nothing is lost: you choose which one to keep.
- The in-game clock of Gold, Silver and Crystal can jump between the VC and another copy: each keeps its own clock data (ADR 0008). A created save starts with a zero clock footer, so the time of day may need setting again.
- Sync all does not create saves: a VC game you never played stays as it is until you press A on it.
