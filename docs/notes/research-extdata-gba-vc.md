# Research A2 (extdata) and A3 (GBA Virtual Console) for 3dssavesync

Date: 2026-11-09. Read-only research for A2 (extdata) and A3 (GBA Virtual Console); A3 is set aside for now.
Builds on `docs/notes/research-virtual-console.md` §2 (same Checkpoint commit) and ADR 0012 (EEPROM order), ADR 0010 (clock Trailer), ADR 0028 (archives).

## Sources (pinned)

| Short | Source | Commit / date |
|---|---|---|
| CP | FlagBrew/Checkpoint `3ds/` — `https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/` | `740f221a`, 2026-10-13 |
| CPT | dwalker109/cloudpoint — `https://github.com/dwalker109/cloudpoint/blob/8652e73af1609999ec1957554905a312edd059cf/` | `8652e73a`, 2026-11-08 |
| JKSM | J-D-K/JKSM — `https://github.com/J-D-K/JKSM/blob/f883dff8fccbbb90851c3c6c2a16ea8bda250356/` | `f883dff8`, 2025-08-28 |
| AZ | azahar-emu/azahar — `https://github.com/azahar-emu/azahar/blob/6b4e7f2e694514fdf2f72dc20368609b7cd4912e/` | `6b4e7f2e`, 2026-11-06 |
| GM9 | d0k3/GodMode9 — `https://github.com/d0k3/GodMode9/blob/01aac721c4a3f34abb2eea71ca5a62e755c0918d/` | `01aac721` |
| N3DS | profi200/libn3ds (open_agb_firm's ARM9 code) — `https://github.com/profi200/libn3ds/blob/a3975be4d544a0a60406b9570e504092b43f1ea2/` | `a3975be4` |
| 3DB | 3dbrew.org pages, read 2026-11-09 with `action=raw`: `3DS_Virtual_Console`, `Filesystem_services`, `Flash_Filesystem`, `Extdata`, `DISA_and_DIFF`, `FS:CreateExtSaveData`, `FSPXI:CalcSavegameMAC`, `FIRM` | — |

"**inference**" marks a conclusion that no source states directly. "**NOT VERIFIED**" marks a point that needs a test on hardware.

---

# A3 — GBA Virtual Console

## A3.1 Where the save lives, and how a homebrew reads and writes it

### NAND, SD, and when each is written

- AGB_FIRM runs GBA VC titles (3DB `FIRM` §AGB_FIRM). "AGB_FIRM saves its active save memory to NAND on exit, this is then immediately picked up by NATIVE_FIRM on reboot by checking CFG_BOOTENV. From there, this is verified and copied out to SD" (3DB `3DS_Virtual_Console` §NAND Savegame).
- The NAND partition `agbsave` is 0x180 sectors (0x30000 bytes); it is empty until AGB_FIRM runs once and holds "the AGB_FIRM GBA savegame" (3DB `Flash_Filesystem`; GM9 `arm9/source/game/gba.h` `AGBSAVE_MAX_SIZE (0x000180 * 0x200)`, `arm9/source/utils/nandutil.c` `RebuildNandNcsdHeader`, partition 1 size `0x000180`).
- There is one `agbsave` partition for all GBA titles, so it can only hold the last played title. **inference**: on launch, the system loads the title's SD copy into NAND. 3dbrew documents only NAND → SD. Checkpoint's restore writes only the SD copy and states that this works ("Re-sign the slot for this console, or AGB_FIRM will reject the save on next launch", CP `source/gbasave.cpp` `GbaSave::restore`).
- SD copy: `title/<high>/<low>/data/00000001.sav` in the title's SD area; its size is `max(0x30000, 2 * (0x200 + saveSize))` (GM9 `nandutil.c` `InjectGbaVcSavegameBuffered`, `fno.fsize != max(AGBSAVE_MAX_SIZE, 2 * data_size)`).
- **When is it written?** Only at AGB_FIRM exit (NAND), then at the next NATIVE_FIRM boot (SD). Our homebrew runs under NATIVE_FIRM, so by then the SD copy is already the newest copy. There is no "uncommitted" state visible to a NATIVE_FIRM homebrew.
  - If the console loses power inside AGB_FIRM, nothing is saved (the save is written only on exit) (**inference** from "saves ... on exit").
  - If AGB_FIRM wrote NAND but the next boot is a cold boot (CFG_BOOTENV = 0), NATIVE_FIRM does not import, and NAND is newer than SD. Next launch of the game would load the older SD copy (**inference**, **NOT VERIFIED**). A homebrew write does not make this worse, but it cannot see it either, unless it reads the NAND copy (FSPXI archive `0x567890B3` "AGB Save Data", FSPXI only, 3DB `Filesystem_services`) and compares title ID + "times saved" (**NOT VERIFIED**; no tool does this).

### Container format (identical on NAND and SD except the CMAC)

Header 0x200 bytes, little-endian (3DB §NAND Savegame; same struct in CP `gbasave.cpp` `AgbSaveHeader`, GM9 `gba.h` `AgbSaveHeader`, CPT `cloudpoint_lib/src/agb_utils.rs`):

| Offset | Size | Field |
|---|---|---|
| 0x00 | 4 | magic `.SAV` |
| 0x04 | 0xC | 0xFF |
| 0x10 | 0x10 | AES-CMAC |
| 0x20 | 0x10 | 0xFF |
| 0x30 | 4 | always 1 (CMAC coverage starts here) |
| 0x34 | 4 | times saved |
| 0x38 | 8 | title ID |
| 0x40 | 0x10 | SD card CID of the console that made it ("verified on load") |
| 0x50 | 4 | save start, always 0x200 |
| 0x54 | 4 | save size: 512, 8 K, 32 K, 64 K or 128 K |
| 0x58 | 8 | 0xFF |
| 0x60, 0x64 | 4 + 4 | ARM7 registers |
| 0x68 | 0x198 | 0xFF |

- Two slots, each header + save: top at 0, bottom at `0x200 + saveSize`. Current slot = higher "times saved", top on a tie; if the top header is invalid, probe the bottom slot for each valid size (CP `gbasave.cpp` `locateCurrentSlot`; CPT `agb_utils.rs` `locate_current`, with fixtures `agb_0/1/2.bin` and tests).
- CMAC on NAND: AES-CMAC of SHA-256(`header[0x30..0x200]` + save), keyslot 0x24. CMAC on the SD copy: a CTR-SIGN / CTR-SAV0 stack with keyslot 0x30 (movable.sed keyY) (3DB §NAND Savegame on SD). A homebrew does not need the keys: `FSPXI_CalcSavegameMAC` picks the CMAC type from the file handle and applies the stacking (3DB `FSPXI:CalcSavegameMAC` §Usage).

### Calls (Checkpoint and cloudpoint, same sequence)

1. Session to `PxiFS0` with the Luma3DS-only `svcControlService(SERVICEOP_STEAL_CLIENT_SESSION, &h, "PxiFS0")`, SVC 0xB0 (CP `source/archive.cpp` `Archive::init`, `source/csvc.s`; CPT `cloudpoint_app/src/pxi.rs`, which logs "requires Luma3DS" and disables GBA when it fails).
2. `FSPXI_OpenArchive(h, &a, ARCHIVE_SAVEDATA_AND_CONTENT /*0x2345678A*/, {PATH_BINARY, 16, {low, high, mediatype, 1}})` (CP `archive.cpp` `Archive::rawSave`; CPT `cloudpoint_app/src/ctr_fs/fs_pxi/driver.rs` `FsPxiArchivePath`, mediatype SD). Not `FSUSER_`: FS-user access to this archive allows only RomFS / System Menu Data / seed checks / save transfer (3DB `Filesystem_services` §FS Limitations).
3. `FSPXI_OpenFile(h, &f, a, {PATH_BINARY, 20, {1, 1, 3, 0, 0}}, flags, 0)` — fixed path, the decrypted two-slot container (CP `source/fsstream.cpp` `pxi_path`). Title access type 1 = "Save data access (high-level, decrypted)" (3DB §Title Access Type).
4. Detection: a title is GBA VC when `ARCHIVE_USER_SAVEDATA` fails **and** this file opens (the archive itself opens even with no save) (CP `source/titleprobe.cpp` `probeCtr`; CP `archive.cpp` `SaveDataSource::accessible`). cloudpoint probes the PXI open per title (CPT `cloudpoint_app/src/title.rs` `lookup_gba_sync_item_for_title`).
5. Read: whole file, current slot, `saveSize` bytes after the header (CP `extractCurrentSave`; CPT `FsPxiArchive::open`). No valid slot → game never saved; Checkpoint returns `resNoSave` and asks to launch the game once.
6. Write (CP `GbaSave::restore`):
   - keep the console's own header (it carries this console's SD CID and title ID);
   - refuse when the new save size differs from the header's `saveSize` (`resSizeMismatch`);
   - write the save at `slot + 0x200` with `FS_WRITE_FLUSH` (CP `fsstream.cpp` `write`, PXI branch);
   - CMAC: SHA-256(`header[0x30..0x200]` + new save), then `FSPXI_CalcSavegameMAC(h, f, hash, 32, cmac, 16)`; call until two results agree, max 10 ("the first call after opening the file can return a stale result", CP `calcSlotCmac`; CPT `AgbContainer::calc_mac` fails if it never stabilises);
   - write the 16-byte CMAC at `slot + 0x10`.
   - cloudpoint does the same, but writes the body, then the whole 0x200 header with the new CMAC (CPT `fs_pxi/driver.rs` `FsPxiArchive::finalise`).
   - No `ControlArchive` commit and no secure value on this path (neither tool calls one).
7. GodMode9 writes the **other** slot with "times saved" + 1 and a fresh CMAC (GM9 `nandutil.c` `InjectGbaVcSavegameBuffered`, "inject next slot"). Checkpoint and cloudpoint overwrite the **current** slot in place.

## A3.2 Raw bytes vs. a PC `.sav`, and the Key

- Payload = the `saveSize` bytes after the current slot header. Sizes: 512 / 8 K (EEPROM), 32 K (SRAM), 64 K / 128 K (Flash) (CP `kValidSaveSizes`; GM9 `GBASAVE_SIZE`, from the footer save type).
- **EEPROM byte order**: GodMode9 reverses each 8-byte block of a 512 B / 8 K save when it dumps to `.sav`, and again when it injects ("byteswap for eeprom type saves", GM9 `nandutil.c` `DumpGbaVcSavegameBuffered` / `InjectGbaVcSavegameBuffered`). open_agb_firm writes the raw save memory with no swap (N3DS `source/arm9/drivers/lgy9.c` `LGY_backupGbaSave`), and its author says the emulator order is the swapped one (open_agb_firm issue #43, profi200: "EEPROM saves made by some emulators are incompatible because they have every 8 bytes block endian swapped"). Checkpoint and cloudpoint export raw, no swap. **inference**: AGBSAVE = hardware order = open_agb_firm order, so ADR 0012's rule applies unchanged: reverse 8-byte blocks for 512 B / 8 K at Upload and Download.
- **Size vs. mGBA**: Flash/SRAM sizes are equal; mGBA adds 16 RTC bytes for RTC games, which ADR 0010 already strips (see `research-virtual-console.md` §2.4). GodMode9 also accepts `size - 16` on inject (GM9 `GBASAVE_VALID(fno.fsize - 16)`).
- **ROM header / game code**:
  - The GBA ROM is at offset 0 of ExeFS `.code`, followed by a 0x360-byte footer; RomFS is unused (3DB §GBA VC). GM9's footer struct puts `rom_size` at +4, `save_type` at +8 and the `.CAA` magic 0x10 bytes before the end (GM9 `gba.h` `AgbVcFooter`); Azahar detects GBA VC by `.CAA` + `1` at `code.size() - 0x10` (AZ `src/core/loader/ncch.cpp` `IsGbaVirtualConsole`). So: game code = `.code[0xAC..0xB0]`, save type = footer +8.
  - GBA header check: fixed byte `0xB2 == 0x96` and the complement checksum at `0xBD` (GM9 `gba.h` `AgbHeader`, citing GBATEK).
  - Reading `.code` from a homebrew: FS-user forbids `.code` on 0x2345678A (3DB §FS Limitations), FSPXI allows all access types. Path, by analogy with Checkpoint's icon read (archive `{low, high, media, 0}`, file `{0, 0, 2, "icon"}`, CP `source/smdh.cpp` `loadSMDH`): FSPXI archive `{low, high, media, 0}`, file `{0 /*NCCH*/, 0 /*content index*/, 1 /*.code*/, ".cod", "e\0\0\0"}` (3DB §Title Access / NCCH Access Type). **NOT VERIFIED**: no tool found that reads a GBA VC `.code` this way. If the exheader marks `.code` as compressed, FSPXI returns compressed bytes; Azahar decompresses before its check, so it gives no answer. Validate with the 0x96 / checksum test and fall back to a user-set mapping. Only the first 0xC0 bytes are needed.

## A3.3 Azahar

- Azahar refuses GBA VC: `LOG_ERROR(Loader, "Encountered unsupported GBA Virtual Console code section.")`, `ResultStatus::ErrorGbaTitle` (AZ `src/core/loader/ncch.cpp`); Qt UI "GBA Virtual Console is not supported by Azahar." (AZ `src/citra_qt/citra_qt.cpp`); Android `ErrorLoaderErrorGBATitle` (AZ `src/android/.../NativeLibrary.kt`). Confirmed: no Azahar side. PC and Android sync GBA VC through the existing `GBA-<code>` `.sav` path (mGBA etc.).

## A3.4 Data-loss risks and how Checkpoint avoids them

| Risk | Checkpoint | Note |
|---|---|---|
| Wrong slot read/written | `locateCurrentSlot`, GM9 tie rule | Two tools agree; cloudpoint has tests |
| Size mismatch (VC footer save type ≠ PC save) | refuses (`resSizeMismatch`) | must become Not synced, not a pad/truncate (GM9 pads with 0xFF; do not copy that) |
| Never-saved container | refuses (`resNoSave`) | "start the game once", as ADR 0028 |
| Foreign console CID | keeps own header, re-signs CMAC | CID is "verified on load" (3DB) |
| Stale CMAC from first call | repeat until stable | cloudpoint errors out after 10 |
| Interrupted write | none: in-place overwrite of current slot | GM9 "other slot + times saved + 1" keeps the old slot valid (**inference**) |
| Backup | Checkpoint writes the bare save to SD before; cloudpoint backs up before replacing (CPT `README.md` FAQ) | ADR 0028 backup copy + read back |
| No Luma3DS | cloudpoint disables GBA support | homebrew must degrade the same way |

---

# A2 — Extra data (extdata)

## A2.1 Finding, reading and writing a title's extdata

- **Archive**: `FSUSER_OpenArchive(&a, ARCHIVE_EXTDATA /*0x6*/, {PATH_BINARY, 12, {MEDIATYPE_SD, extdataIdLow, 0}})` (CP `source/archive.cpp` `Archive::extdata`; CPT `cloudpoint_app/src/ctr_fs/fs_user/driver.rs` `FsUserArchivePath`). Application extdata is always on SD, high word 0, even for game-card titles (3DB `Extdata`: "ExtdataID-High is always 00000000 for SD").
- The mount shows the `/user` virtual folder only; `/boss` (SpotPass) and `/icon` are separate mounts (3DB `Extdata` §Virtual File System Structure). `/user/ExBanner/` may hold an extended banner (same section).
- **Extdata ID**:
  - Normal rule `titleLow >> 8` (CP `source/titlequirks.cpp` `TitleQuirks::extdataIdFor`; JKSM `JKSM/source/Data/ExtData.cpp` `ExtDataRedirect`; CPT `title.rs` `infer_extdata_sync_item_for_title`).
  - Real source: the exheader ("Regular apps can only mount SD extdata using the same extdataID which is stored in the CXI exheader", 3DB `Extdata`). cloudpoint asks AM first: `AM_GetTitleExtDataId(&id, MEDIATYPE_SD, titleId)`, then falls back to `low >> 8` (CPT `title.rs` `lookup_extdata_sync_item_for_title`, `ctr_get_ext_data_id_for_title`).
  - Hand tables disagree: Checkpoint maps `0x001B5100` → `0x1B50` "Pokémon Ultramoon" and `0x00175E00` → `0x1648` "Pokémon Moon"; JKSM maps `0x001B5100` → `0x1B50` "Pokemon Moon" and has no `0x00175E00` entry. Use AM, not a table.
- **Shared extdata**: several titles can use one ID: Pokémon X/Y `0x55D`, OR/AS `0x11C5`, Sun/Moon `0x1648`, Fire Emblem Fates editions `0x1794` / `0x1795` / `0x12DC` (CP, JKSM tables; 3DB `Extdata` §SD Extdata table). Region versions usually differ (e.g. Mario Kart 7: JPN `0x306`, USA `0x308`, EUR `0x307`, 3DB). So the Key must be per extdata ID, not per title ID.
- **Fixed file size**: "Extdata files do not support resizing after their initial creation, because each file is a separate, fixed-size DIFF container" (3DB `Extdata` §Quota File). Azahar copies this: `FixSizeDiskFile` refuses `SetSize` and writes past the end; zero-size create is prohibited (AZ `src/core/file_sys/archive_extsavedata.cpp`).
  - cloudpoint: other size → read, delete, `FSUSER_CreateFile(path, newSize)`, write back (CPT `fs_user.rs` `FsUserLeaf::set_len`, "Extdata requires recreating the file").
  - Checkpoint restore: delete every file and folder in the archive, then `FSUSER_CreateFile` each file at its backup size and write it (CP `source/io.cpp` `io::restore` → `deleteFolderRecursively`, `copyTree`; `source/fsstream.cpp` 4-argument `FSStream` creates the file at `size` when the open fails).
  - No `ControlArchive(COMMIT)` and no secure value for extdata (CP `io::restore` commits only `BackupKind::Save`; CPT `FsUserArchive::finalise` commits only `SyncItem::Savedata`).
- **Limits**: `FS:CreateExtSaveData` takes the number of directories, the number of files and a size limit (3DB `FS:CreateExtSaveData`); the VSXE tables are sized at creation. Quota.dat exists only for NAND shared extdata (3DB `Extdata`). **inference**: on SD, a restore with more files than the game created can fail; a same-game archive does not.
- **Files to skip / special cases**:
  - Files held open by another process fail with `0xC92044E6`; Checkpoint drops them from backup and restore (HOME Menu `/CacheD.dat`, issue #592) (CP `io.cpp` `RES_FS_IN_USE`, `io::dropInUse`). Not expected for a game's own extdata while our homebrew runs.
  - Desktop metadata (AppleDouble `._*` etc.) dropped before restore (CP `io::dropHostMetadata`, issue #577).
  - **Sparse files**: a read past the "real, allocated data" fails with `0xD900458B`; Checkpoint halves the read and then **stops the file there**, so its backup file can be shorter than `FSFILE_GetSize` (CP `io.cpp` `RES_FS_PAST_DATA`, `io::copyFile`). cloudpoint ignores the read error and keeps the full length (CPT `fs_user/driver.rs` `FsUserReader::read`, "advancing cursor and continuing anyway"; the unread bytes keep what the buffer held). **inference**: for a sync, keep the full `GetSize` length and fill unreadable bytes with zeros, so the recreated file has the size the game created.
  - Skip lists: cloudpoint skips NetPass `0x000400000F657400` ("uses extdata for SpotPass download") and the Super Mario Maker titles (CPT `cloudpoint_app/romfs/title_config.toml`).
- **Not created by us**: if the archive does not exist, the game was never started (or does not use extdata). Neither tool calls `FSUSER_CreateExtSaveData`; both need the game started once (CP `titleprobe.cpp`: extdata is listed only when `accessible()`).

## A2.2 Sizes, and progress vs. cache

- Documented sizes (3DB `Extdata` §SD Extdata): StreetPass Mii Plaza "11 mb big!"; Nintendo Video files of 50.7 MB each. No primary source found for Animal Crossing: New Leaf or Mario Kart 7 extdata sizes (**NOT FOUND**; measure on the owner's SD).
- Which games keep real progress in extdata: **NOT FOUND** in a primary source. Azahar's guide names only Mario vs. Donkey Kong: Tipping Stars (`extdata/00000000/000012C8/user`) and says "some games and system applications use extra data" (citra.azahar-emu.org, "Dumping Extra Data from a 3DS Console"). SpotPass content goes to `/boss`, not `/user` (3DB), so the `/user` mount already excludes the main cache type.

## A2.3 Azahar layout (PC and Android)

- Container: `<sdmc>/Nintendo 3DS/<SYSTEM_ID>/<SDCARD_ID>/extdata/` (AZ `archive_extsavedata.cpp` `GetExtDataContainerPath`; both IDs are 32 zeros in Azahar, as the guide path shows).
- Archive folder: `GetExtSaveDataPath` = `<container>/<high %08X>/<low %08X>/` — **upper-case hex** (e.g. `00000000/000012C8/`). `GetExtDataPathFromId` builds the same path in lower case (`%08x`). Look up case-insensitively.
- Inside the archive folder: `user/` (what the game mounts), `boss/`, `metadata` (16-byte `ArchiveFormatInfo`: total_size, number_directories, number_files, duplicate_data; AZ `src/core/file_sys/archive_backend.h`), `icon` (SMDH) (AZ `FormatAsExtData`). `Open` fails with "not found" when `user/` is missing.
- Mapping: 3DS extdata archive root `/` ↔ host `…/<low>/user/`, file bytes 1:1 (the guide: "put the files inside the `user` folder"). Never write `metadata`, `icon` or `boss/`.

## A2.4 Risks

- No atomicity across files: each extdata file is a DIFF container with its own DPFS pair, "designed for atomic writing" per file (3DB `DISA_and_DIFF` §DPFS tree), but there is no archive commit. A stop between files leaves a mix of old and new files. Checkpoint even deletes all files first. Mitigation: verified backup copy (ADR 0028) and a read back.
- Not covered by the save's secure value, and has none of its own (DIFF has no secure-value field; extdata CMAC is `CTR-EXT0` over extdata ID + device file IDs + DIFF header, 3DB `DISA_and_DIFF` §CTR-EXT0, computed by FS on write). **inference**: save and extdata can roll back independently; a game that cross-checks them may complain. Sync save and extdata of one game together.
- Delete + recreate of a file of another size is two steps: a stop between them loses that file (backup copy covers it).
- Sparse files (above) and size changes make the ADR 0028 size-check Conflict more frequent if Checkpoint-style shortened files reach the server.
- Large archives: transfer time and server space; the server stores every version (ADR 0028).

---

# Open questions for the owner

A3:
1. Does the owner have GBA VC titles (Ambassador or injects) on the 3DS? Which ones (title IDs, save types)?
2. Accept a Luma3DS-only feature (SVC 0xB0)?
3. Write the current slot in place (Checkpoint) or the other slot with times saved + 1 (GodMode9)?
4. Accept a hardware test of the FSPXI `.code` read; if it fails, accept a manual title ID → game code mapping?

A2:
1. Which games does the owner play that have extdata, and how large is each `extdata/00000000/<id>/` folder on the SD?
2. Extdata on by default, or off by default and on per game?
3. One Key per extdata ID (shared by X/Y, OR/AS, Sun/Moon) — acceptable that turning off one title cannot turn off its shared extdata alone?
4. Size limit per archive (server and 3DS memory)?
