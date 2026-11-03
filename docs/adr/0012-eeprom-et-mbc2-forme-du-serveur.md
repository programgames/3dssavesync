# GBA EEPROM and GB MBC2: one single form on the server

Two save types have a different file depending on the emulator, for the same data (tickets #7 and #8, `docs/notes/research-eeprom-mbc2.md`, French). The Hash covers the SRAM (ADR 0008): without conversion, the 3DS and the PC see two versions, so a Conflict, and a file copied as is gives an unreadable game.

- **GBA EEPROM** (512 bytes or 8 KiB): open_agb_firm writes the bytes in hardware order; mGBA reverses each 8-byte block.
- **GB MBC2** (512 4-bit values): mGBA writes 256 bytes, two values per byte (even address in the low nibble); SameBoy, VBA-M and BGB 512 bytes, one value per byte; GameYob (TWiLight Menu++) 8 KiB, the first 512 bytes used.

Rule:

- **EEPROM**: the server keeps the mGBA order. The 3DS reverses each 8-byte block for a 512-byte or 8 KiB GBA save that is not an mGBA forwarder save (so open_agb_firm), at Upload and at Download. On GBA, these two sizes only come from an EEPROM. The PC converts nothing.
- **MBC2**: the server keeps 512 bytes, one value per byte, high nibble at 0.
  - 3DS: MBC2 game recognized by byte `0x147` of the ROM (0x05 or 0x06), because 8 KiB is also the size of other GB games. At Download: size of the existing file; otherwise 256 bytes for an mGBA forwarder and 8 KiB for TWiLight Menu++.
  - PC: a 256- or 512-byte GB file can only be MBC2. At Download: size of the existing file; otherwise 256 bytes in mGBA (app or RetroArch core) and 512 bytes (high nibble at 0xF, read by VBA-M) elsewhere.
- **Backup copies**: always the bytes of the file, without conversion.

## Rejected options

- **Keep the file of each emulator on the server**: one Hash per emulator, so Conflicts at each machine change.
- **open_agb_firm form for the EEPROM**: each PC emulator would need a conversion; mGBA is the default emulator of the PC and of the forwarders.
- **mGBA form (256 bytes) for MBC2**: the 512-byte form is the simplest to read and write for the three other formats.

## Consequences

- An EEPROM uploaded by an earlier version from open_agb_firm is on the server in the wrong order. With this version, the 3DS computes another Hash for the same file: it offers an Upload, in the right order. Do not download this game on the PC before this Upload.
- An 8 KiB MBC2 file uploaded by an earlier version stays as is on the PC (8 KiB is not recognized as MBC2 there). The 3DS converts it at Download.
- The internal EEPROM format of other PC emulators (VBA-M, SkyEmu, NanoBoyAdvance) is not verified: mGBA order assumed.
