# Research — Pokémon Virtual Console saves on 3DS

Date: 2026-11-02. Question: how can 3dssavesync sync the Pokémon Virtual Console (VC) games of a 3DS with mGBA on the PC?
Method: reading of source code at fixed commits, 3dbrew, and the eShop title database. No code was run on a 3DS. No VC title was opened.

Legend:
- **NOT FOUND** = no primary source found.
- **inference** = conclusion drawn from the cited code or pages, not tested.

Starting point: [ADR 0008](adr/0008-gb-gba-par-fichiers-sram-seule.md) (SRAM only, rejected VC injects), [ADR 0009](adr/0009-sauvegardes-3ds-natives.md) (native `/main` archives), [ADR 0010](adr/0010-trailer-d-horloge-par-reste-512.md) (clock Trailer = `size mod 512` in {16, 44, 48}), `3ds/source/titres3ds.c`, and the older notes in [research-v2.md](research-v2.md) §1b and §2 ("Injects GB/GBC VC").

## Pinned sources

| Project | Commit / revision |
|---|---|
| Checkpoint (3DS save manager, GPL-3) | `740f221a853db784401458fe74276977afcfa8b0` (v5.3.0 era; GBA VC support since v4.0.0, 2026-07-11) |
| PKSM (GPL-3) | `9cad15694c1e90cc058ddd285eb68d56c7ad9ac8` |
| PKSM-Core | `d9bca6fcdf622760a0aa963565b102276e2ae2a3` |
| PKHeX | `0660d1e1f4924b8478daec73281bb8071a82ad19` |
| GodMode9 (GPL-2) | `01aac721c4a3f34abb2eea71ca5a62e755c0918d` |
| Luma3DS | `a50bf165f04f91c1bcf8a9384f773d4855cf50e1` |
| libctru | `62b7c42c88b1ee837f513358409d2e5c8f9001a9` |
| 3dsdb (eShop title lists) | `5d9141a56275c974a25aaaf6856ec6a1e50e5c07` |
| pret pokered / pokegold / pokecrystal | `0dbf95cd12f5b1511c62ce69efe283000f780a8f` / `f2cde36f49b994c143bfd18582182302fdc76492` / `3c813b0b4d82b39bf62185aceffb76fd6026c70b` |
| 3dbrew "3DS Virtual Console" | revision `oldid=23004` |

Link prefixes:
- `CP` = `https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/`
- `PKSM` = `https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/`
- `PKC` = `https://github.com/FlagBrew/PKSM-Core/blob/d9bca6fcdf622760a0aa963565b102276e2ae2a3/`
- `PKH` = `https://github.com/kwsch/PKHeX/blob/0660d1e1f4924b8478daec73281bb8071a82ad19/PKHeX.Core/`
- `GM9` = `https://github.com/d0k3/GodMode9/blob/01aac721c4a3f34abb2eea71ca5a62e755c0918d/`
- `LUMA` = `https://github.com/LumaTeam/Luma3DS/blob/a50bf165f04f91c1bcf8a9384f773d4855cf50e1/`
- `CTRU` = `https://github.com/devkitPro/libctru/blob/62b7c42c88b1ee837f513358409d2e5c8f9001a9/libctru/include/3ds/`
- `DB` = `https://github.com/hax0kartik/3dsdb/blob/5d9141a56275c974a25aaaf6856ec6a1e50e5c07/jsons/`
- `3DBREW` = https://www.3dbrew.org/w/index.php?title=3DS_Virtual_Console&oldid=23004

---

## Summary

| Case | Feasibility | Effort | Main risk |
|---|---|---|---|
| **GB/GBC VC** (Red, Blue, Yellow, Gold, Silver, Crystal), read and Upload | **high** | **S** | Wrong Key (ROM revision differs between VC and PC) |
| GB/GBC VC, write back (Download) | **high** (inference) | **S–M** | Unknown 16-byte clock footer of Gen 2; a save state file, if one exists, could win over `sav.dat` |
| **GBA VC injects** (Gen 3, homemade with NSUI etc.), read | **medium** | **M** | Needs a Luma3DS-only syscall; title IDs are chosen by the inject tool |
| GBA VC injects, write back | **medium** | **M–L** | Two-slot AGBSAVE container and per-console CMAC; a bad write makes AGB_FIRM reject the save |
| Official GBA VC ("Ambassador") | **not applicable** | — | No Pokémon game was in that program |

Key points:
1. A GB/GBC VC save is a normal 3DS save archive with the file **`/sav.dat`**. It is the raw cartridge SRAM: 32 768 bytes for Gen 1, 32 784 bytes (SRAM + 16-byte clock footer) for Gen 2. The existing ADR 0010 rule (`size mod 512` = 16 → strip) already gives the same SRAM as mGBA's `.sav`. The existing write path (`titre_ecrire`: write, commit, delete the secure value) is the same sequence that PKSM uses for `sav.dat`.
2. The ADR 0008 reason to reject GBA VC ("AGBSAVE, extractible only with GodMode9") **no longer holds**: since 2026, Checkpoint (v4.0.0) and PKSM read and write the SD copy of the GBA VC save from a normal homebrew, through the `PxiFS0` service obtained with a Luma3DS syscall, and re-sign it with `FSPXI_CalcSavegameMAC`.
3. Each language of each VC game has its own title ID (33 titles in total). EUR and USA stores sell the same title IDs. The French versions: Rouge `0004000000171600`, Bleue `0004000000171700`, Jaune `0004000000171800`, Or `0004000000172C00`, Argent `0004000000172D00`, Cristal `0004000000172E00`.

Recommendation: implement GB/GBC VC first (§6). Leave GBA VC injects for later, and only if the user plays GBA through injects instead of open_agb_firm.

---

## 1. GB/GBC Virtual Console (Pokémon Red, Blue, Yellow, Gold, Silver, Crystal)

### 1.1 What is in the save archive

- 3dbrew lists the possible files of a NES/GB/GBC VC save: `rsm1.dat` (probably the restore point), `rsm2.dat` (emulator save state "for storing/loading state at VC-title launch/exit"), `sav.dat` ("the actual savedata used by the emulated ROM"), and a secure value ([3DBREW, §NES/GB/GBC VC → Savedata](https://www.3dbrew.org/w/index.php?title=3DS_Virtual_Console&oldid=23004)).
- 3dbrew also says: "The emulator officially used for Pokemon games removes savestate support in favor of a Link Cable 'implementation'", and "No known version of the emulator implements both savestates and secure value" (same page, §NES/GB/GBC VC).
- **inference**: the Pokémon VC archive holds `sav.dat` only, and uses the secure value (no `rsm*.dat`). This conflicts with one user report on Digiex that saw `rsm1.dat` / `rsm2.dat` in a VC Pokémon folder and says the VC "would automatically do a save state when closing the game" ([Digiex thread](https://digiex.net/threads/pokemon-gameboy-rom-save-transfer-to-3ds-red-blue-yellow-green.14646/)). Which game and emulator version that report used is unknown. **To check on the real 3DS** (list the archive root, §7).
- 3dbrew adds: "Overwriting sav.dat with 0xFF-bytes doesn't have any affect on the actual emulator" (same page). **inference**: this was seen with an emulator that restores a save state at launch. If a Pokémon archive had `rsm2.dat`, a written `sav.dat` could be ignored and later overwritten.

### 1.2 Layout of `sav.dat`

- **Gen 1 (Red, Blue, Yellow, JPN Green)**: raw SRAM, **32 768 bytes**, no header, no footer.
  - PKSM opens `/sav.dat` (instead of `/main`) and passes its bytes, unchanged, to the same parser as a cartridge save ([PKSM `3ds/source/titles/loader.cpp#L540-L543`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/titles/loader.cpp#L540-L543), [`Title.cpp#L160-L164`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/titles/Title.cpp#L160-L164)). That parser recognizes a GB save by its size, `0x8000` (international) or `0x10000` (Japanese) ([PKC `include/sav/SaveRecognition.hpp#L61-L62`](https://github.com/FlagBrew/PKSM-Core/blob/d9bca6fcdf622760a0aa963565b102276e2ae2a3/include/sav/SaveRecognition.hpp#L61-L62), [`source/sav/Sav.cpp#L89-L107`](https://github.com/FlagBrew/PKSM-Core/blob/d9bca6fcdf622760a0aa963565b102276e2ae2a3/source/sav/Sav.cpp#L89-L107)).
  - PKHeX tells a VC save from a cartridge save only by the file name (`sav*.dat`), not by its content ([PKH `Saves/SAV1.cs#L14`](https://github.com/kwsch/PKHeX/blob/0660d1e1f4924b8478daec73281bb8071a82ad19/PKHeX.Core/Saves/SAV1.cs#L14), [`Saves/SAV2.cs#L14`](https://github.com/kwsch/PKHeX/blob/0660d1e1f4924b8478daec73281bb8071a82ad19/PKHeX.Core/Saves/SAV2.cs#L14)); its raw GB size is `0x8000` ([`Saves/Util/SaveUtil.cs#L89-L94`](https://github.com/kwsch/PKHeX/blob/0660d1e1f4924b8478daec73281bb8071a82ad19/PKHeX.Core/Saves/Util/SaveUtil.cs#L89-L94)).
  - The Digiex guide says the file must be "32KB (32,768 bytes)" ([Digiex](https://digiex.net/threads/pokemon-gameboy-rom-save-transfer-to-3ds-red-blue-yellow-green.14646/)).
- **Gen 2 (Gold, Silver, Crystal)**: raw SRAM + **16-byte clock footer** = **32 784 bytes**. Japanese Crystal: 65 536 + 16 = 65 552 bytes.
  - Source: the Blue Moon Falls converter script. Cartridge → VC appends 16 bytes of `00`; VC → cartridge removes the last 16 bytes; a 32 816-byte emulator file (SRAM + 48-byte RTC trailer) keeps only the first 16 trailer bytes ([`gsc-save-converter.js`](https://bluemoonfalls.com/scripts/gsc-save-converter.js), [page](https://bluemoonfalls.com/pages/tools/gsc-save-converter): "an extra 16 bytes tacked on to the very end of VC saves for time-keeping purposes").
  - PKHeX accepts a GB file with a 0x0C–0x30-byte footer and keeps the footer apart ([PKH `Saves/Util/Recognition/SaveHandlerFooterRTC.cs#L13-L31`](https://github.com/kwsch/PKHeX/blob/0660d1e1f4924b8478daec73281bb8071a82ad19/PKHeX.Core/Saves/Util/Recognition/SaveHandlerFooterRTC.cs#L13-L31)). PKSM-Core does the same with a 0x0C–0x30 "RTC trailer" ([PKC `source/sav/SaveRecognition.cpp#L72-L91`](https://github.com/FlagBrew/PKSM-Core/blob/d9bca6fcdf622760a0aa963565b102276e2ae2a3/source/sav/SaveRecognition.cpp#L72-L91)).
  - **Field layout of the 16-byte footer: NOT FOUND.** The converter writes zeros and the page only warns that the in-game time "may be wrong" after a conversion.
- **Checksum**: no VC-specific checksum, header or CMAC inside `sav.dat` (**inference**: none of PKSM, PKHeX or the converter computes one; the game's own SRAM checksums are unchanged). The archive container (DISA) has its own CMAC, but the FS service updates it on commit, as for `/main`.

### 1.3 RTC of Gold, Silver, Crystal

- The VC clock state lives in the 16-byte footer (§1.2). What the VC emulator puts there (Unix time, latched registers, offset from the 3DS clock): **NOT FOUND**.
- **inference**: the effect is the same as ADR 0008 between open_agb_firm and mGBA. The game keeps its clock offset inside the SRAM; each machine keeps its own footer, so the in-game time can differ between the 3DS and the PC, but the game data is the same.

### 1.4 The ROM inside the VC title (for the Key)

- The ROM is in the title's RomFS under `rom:/rom/`; "the filename is determined by the emulator app by doing a directory read". The ROM patches live in `rom:/<rom_name>.patch` and are applied by the emulator ([3DBREW, §RomFS](https://www.3dbrew.org/w/index.php?title=3DS_Virtual_Console&oldid=23004)).
- pret builds each VC `.patch` file by comparing a `_vc` build with the normal cartridge build: Crystal from `pokecrystal11` (revision 1.1), Gold/Silver from `pokegold`/`pokesilver`, Red/Blue from `pokered`/`pokeblue` ([pokecrystal `Makefile#L7`, `#L136`](https://github.com/pret/pokecrystal/blob/3c813b0b4d82b39bf62185aceffb76fd6026c70b/Makefile#L136), [pokegold `Makefile#L195-L196`](https://github.com/pret/pokegold/blob/f2cde36f49b994c143bfd18582182302fdc76492/Makefile#L195-L196), [pokered `Makefile#L157-L158`](https://github.com/pret/pokered/blob/0dbf95cd12f5b1511c62ce69efe283000f780a8f/Makefile#L157-L158)). pret's README lists the VC patch `CGBBYTE1.784.patch` next to Crystal 1.1 ([README `#L7-L12`](https://github.com/pret/pokecrystal/blob/3c813b0b4d82b39bf62185aceffb76fd6026c70b/README.md#L7-L12)).
- **inference**: the ROM file in RomFS is the unmodified cartridge ROM (US Crystal = revision 1.1). Its header gives the usual GB Key (`GB-<title>-<checksum>`, ADR 0007), the same as on the PC **if the PC ROM is the same revision and language**.
- Whether the ROM file in RomFS is stored raw or compressed: **NOT FOUND** (3dbrew does not say).
- libctru can mount the RomFS of another title: `romfsMountFromTitle(u64 tid, FS_MediaType mediatype, const char* name)` ([CTRU `romfs.h#L78`](https://github.com/devkitPro/libctru/blob/62b7c42c88b1ee837f513358409d2e5c8f9001a9/libctru/include/3ds/romfs.h#L78)). It opens `ARCHIVE_SAVEDATA_AND_CONTENT`, the archive that `lire_smdh` (`3ds/source/titres3ds.c`) already opens for the icon, so **inference**: the existing `FileSystemAccess` list is enough.

### 1.5 Write back: secure value, CMAC, size

- PKSM writes a VC save with the same steps as a 3DS save: open `/sav.dat` (or `/main`) for writing, write the whole buffer, commit the archive ([PKSM `loader.cpp#L940-L960`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/titles/loader.cpp#L940-L960)), then delete the secure value `(SECUREVALUE_SLOT_SD << 32) | (lowId & 0xFFFFFF00)` ([`loader.cpp#L1291-L1292`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/titles/loader.cpp#L1291-L1292)).
- Checkpoint restores any CTR save the same way: copy files, `ARCHIVE_ACTION_COMMIT_SAVE_DATA`, `SECURESAVE_ACTION_DELETE` ([CP `source/io.cpp#L893-L906`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/io.cpp#L893-L906)).
- `titre_ecrire` (`3ds/source/titres3ds.c`) does exactly these steps, with the same secure value, but the path `/main` is fixed in the code.
- Forum guides also say to delete the secure value when a `sav.dat` is replaced by hand ([GBAtemp](https://gbatemp.net/threads/injecting-sav-to-pokemon-gold-vc-cant-find-titleid.484769/), [Project Pokémon](https://projectpokemon.org/home/forums/topic/63988-pok%C3%A9mon-crystal-virtual-console-sav-problem/)).
- **inference**: the existing write path handles `sav.dat` once the file name is a parameter. The CMAC of the archive is the FS service's job, as for `/main`.
- Size: `titre_ecrire` refuses a size change. For Gen 2 the Client must write the received 32 768-byte SRAM **followed by the footer already in the archive** (ADR 0008 rule: each machine keeps its own Trailer). The size is then unchanged.
- When the VC emulator writes `sav.dat` (at each in-game save, or at exit): **NOT FOUND**. **inference**: both are before the homebrew can run, because only one application runs at a time.
- Whether the archive (or `sav.dat`) exists before the first in-game save: **NOT FOUND**. The Digiex guide says to start the game and save once first.

### 1.6 Tools that convert VC GB saves

| Tool | What it does with a VC GB save | Source |
|---|---|---|
| Checkpoint | Copies the whole archive (all files) to `/3ds/Checkpoint/saves/...`. No conversion. | [CP `source/io.cpp`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/io.cpp#L893-L906) |
| PKSM | Reads and writes `/sav.dat` in place; default title IDs for the English versions only; other languages are set by the user | [PKSM `TitleIdOverlay.cpp#L39-L49`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/gui/overlay/TitleIdOverlay.cpp#L39-L49), [FAQ](https://github.com/FlagBrew/PKSM/wiki/FAQs) |
| PKHeX | Opens `sav.dat` as a raw GB save; marks it `[VC]` by file name | [PKH `SAV2.cs#L14-L17`](https://github.com/kwsch/PKHeX/blob/0660d1e1f4924b8478daec73281bb8071a82ad19/PKHeX.Core/Saves/SAV2.cs#L14-L17) |
| Blue Moon Falls GSC converter | Adds or removes the 16-byte Gen 2 footer | [script](https://bluemoonfalls.com/scripts/gsc-save-converter.js) |
| GodMode9 | Nothing VC-GB-specific: GB VC saves are ordinary title saves | (no GB VC code found in GM9; **inference**) |

---

## 2. GBA on 3DS

### 2.1 Official GBA VC ("Ambassador")

- The ten Ambassador GBA games (December 2011) do not include a Pokémon game ([Nintendo Life, 2011-12](https://www.nintendolife.com/news/2011/12/3ds_ambassador_game_boy_advance_games_revealed)). The eShop lists in 3dsdb have no Pokémon GBA title (**inference** from the [DB `list_GB.json`](https://github.com/hax0kartik/3dsdb/blob/5d9141a56275c974a25aaaf6856ec6a1e50e5c07/jsons/list_GB.json), `list_US.json`, `list_JP.json`). So only homemade injects matter for Gen 3.

### 2.2 How AGB_FIRM saves

- AGB_FIRM writes the save to the NAND partition AGBSAVE on exit. On reboot, NATIVE_FIRM checks `CFG_BOOTENV`, verifies the save and copies it to the SD ([3DBREW, §NAND Savegame](https://www.3dbrew.org/w/index.php?title=3DS_Virtual_Console&oldid=23004)).
- Container: 0x200-byte header (magic `.SAV`, AES-CMAC at 0x10, "number of times saved" at 0x34, title ID at 0x38, SD card CID at 0x40, save offset 0x200 at 0x50, save size at 0x54), then the raw save (same page).
- The SD copy has the same layout but another CMAC: `AES-CMAC(SHA256("CTR-SIGN" + titleID + SHA256("CTR-SAV0" + SHA256(header[0x30..0x200] + save))))`, keyslot 0x30, keyY from `movable.sed` (same page).
- Save types 0x4, 0x6, 0x8, 0xA, 0xC are Flash "+ RTC" (same page). **inference**: AGB_FIRM emulates the RTC; the save holds no RTC bytes (the save sizes are only 512, 8 K, 32 K, 64 K, 128 K, see below).
- The SD copy is `title/<high>/<low>/data/00000001.sav` of the title's SD save area. It holds **two slots** (top and bottom), each one header + save. GodMode9 injects into the slot that is not current and increments "times saved" ([GM9 `arm9/source/utils/nandutil.c#L171-L225`](https://github.com/d0k3/GodMode9/blob/01aac721c4a3f34abb2eea71ca5a62e755c0918d/arm9/source/utils/nandutil.c#L171-L225)). Checkpoint takes the slot with the higher "times saved", top slot on a tie, "matching GM9" ([CP `source/gbasave.cpp#L87-L121`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/gbasave.cpp#L87-L121)).
- GodMode9 dumps `0:/gm9/out/<TitleID>.gbavc.sav` (raw save; EEPROM bytes swapped in blocks of 8 so the file matches emulators) ([GM9 `nandutil.c#L133-L157`](https://github.com/d0k3/GodMode9/blob/01aac721c4a3f34abb2eea71ca5a62e755c0918d/arm9/source/utils/nandutil.c#L133-L157), [README `#L166`](https://github.com/d0k3/GodMode9/blob/01aac721c4a3f34abb2eea71ca5a62e755c0918d/README.md#L166)). Pokémon Gen 3 uses Flash 128 KiB: no swap.

### 2.3 Can a homebrew read and write it? Yes, since 2026

- A normal `ARCHIVE_USER_SAVEDATA` open fails for a GBA VC title. Checkpoint marks a title as GBA VC when that open fails **and** the raw PXI save opens ([CP `source/titleprobe.cpp#L74-L75`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/titleprobe.cpp#L74-L75)).
- Access path used by Checkpoint (and PKSM):
  1. Get a session to the `PxiFS0` service with the Luma3DS custom syscall `svcControlService(SERVICEOP_STEAL_CLIENT_SESSION, &h, "PxiFS0")` ([CP `source/archive.cpp#L47-L56`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/archive.cpp#L47-L56), [PKSM `3ds/source/io/Archive.cpp#L552`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/io/Archive.cpp#L552)). The syscall is SVC 0xB0, added by Luma3DS ([LUMA `k11_extension/source/svc.c#L105`](https://github.com/LumaTeam/Luma3DS/blob/a50bf165f04f91c1bcf8a9384f773d4855cf50e1/k11_extension/source/svc.c#L105), [`sysmodules/rosalina/include/csvc.h#L25-L30`](https://github.com/LumaTeam/Luma3DS/blob/a50bf165f04f91c1bcf8a9384f773d4855cf50e1/sysmodules/rosalina/include/csvc.h#L25-L30)). It is not in libctru; Checkpoint ships a 3-line assembly stub ([CP `source/csvc.s`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/csvc.s)).
  2. `FSPXI_OpenArchive(h, ARCHIVE_SAVEDATA_AND_CONTENT, {low, high, media, 1})` ([CP `archive.cpp#L81-L85`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/archive.cpp#L81-L85)).
  3. `FSPXI_OpenFile` with the binary path `{1, 1, 3, 0, 0}` (save data) ([CP `source/fsstream.cpp#L31`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/fsstream.cpp#L31), same path in [PKSM `loader.cpp#L525-L536`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/titles/loader.cpp#L525-L536)). The bytes come back decrypted: the two-slot AGBSAVE container.
- **Read**: find the current slot, copy `saveSize` bytes after its header ([CP `gbasave.cpp#L148-L207`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/gbasave.cpp#L148-L207)). No slot valid → the game was never saved; Checkpoint asks to launch it once.
- **Write** ([CP `gbasave.cpp#L209-L305`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/gbasave.cpp#L209-L305)):
  - keep the console's own header (it holds this console's SD CID and the title ID);
  - refuse a save whose size differs from the header's `saveSize`;
  - write the save after the header, then compute the slot CMAC: SHA-256 of `header[0x30..0x200] + save`, then `FSPXI_CalcSavegameMAC` on that hash ([CTRU `services/fspxi.h#L118`](https://github.com/devkitPro/libctru/blob/62b7c42c88b1ee837f513358409d2e5c8f9001a9/libctru/include/3ds/services/fspxi.h#L118)). The first call can return a stale value, so both tools call it until two results agree ([CP `gbasave.cpp#L124-L145`](https://github.com/BernardoGiordano/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/gbasave.cpp#L124-L145), [PKSM `loader.cpp#L188-L215`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/titles/loader.cpp#L188-L215));
  - write the CMAC at offset 0x10 of the slot. Checkpoint's comment: without it "AGB_FIRM will reject the save on next launch".
  - Checkpoint writes into the **current** slot. GodMode9 writes into the **other** slot with "times saved" + 1 (§2.2). **inference**: the GodMode9 way keeps the old slot valid if the write stops halfway.
- The path back from SD to NAND when the game starts: **NOT FOUND** (3dbrew documents only NAND → SD). **inference**: Checkpoint's restore suggests that writing the SD copy is enough.
- Secure value for GBA VC: neither Checkpoint nor PKSM touches one on this path (**inference**: none is used).

### 2.4 Why ADR 0008 rejected AGBSAVE, and does it still hold?

- ADR 0008: "Injects Virtual Console : sauvegarde GBA dans la NAND (AGBSAVE), extractible seulement avec GodMode9". research-v2 §1b concluded that each transfer needs a reboot into GodMode9.
- That was true of the tools known on 2026-11-01. It is **no longer true**: the SD copy is readable and writable from a homebrew through `PxiFS0` (§2.3), as Checkpoint does since v4.0.0 ([release notes](https://github.com/BernardoGiordano/Checkpoint/releases/tag/v4.0.0): "GBA Virtual Console save backup and restore").
- Reasons that still hold:
  - The user already plays GBA with open_agb_firm (ADR 0008), which writes a plain `.sav` and is supported.
  - The syscall exists only on Luma3DS (**inference**: acceptable, the homebrew already needs a CFW to install as a CIA).
  - Inject title IDs are chosen by the inject tool. PKSM uses user-set placeholders (`0x100`…`0x500`) for Gen 3 ([PKSM `TitleIdOverlay.cpp#L45-L49`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/gui/overlay/TitleIdOverlay.cpp#L45-L49)). The Key must come from the ROM: the GBA ROM sits at offset 0 of ExeFS `.code`, with a `.CAA` footer at its end ([3DBREW, §GBA VC](https://www.3dbrew.org/w/index.php?title=3DS_Virtual_Console&oldid=23004)). Whether `.code` is compressed in NSUI injects: **NOT FOUND**.
  - The SRAM compare would match: the raw Flash save is 131 072 bytes, and mGBA's `.sav` is 131 072 + 16 bytes for Ruby/Sapphire/Emerald; ADR 0010 strips the 16 bytes (**inference**).

---

## 3. Title IDs

Source: eShop title lists of 3dsdb ([DB `list_GB.json`](https://github.com/hax0kartik/3dsdb/blob/5d9141a56275c974a25aaaf6856ec6a1e50e5c07/jsons/list_GB.json), [`list_US.json`](https://github.com/hax0kartik/3dsdb/blob/5d9141a56275c974a25aaaf6856ec6a1e50e5c07/jsons/list_US.json), [`list_JP.json`](https://github.com/hax0kartik/3dsdb/blob/5d9141a56275c974a25aaaf6856ec6a1e50e5c07/jsons/list_JP.json)). The English IDs match PKSM's defaults ([PKSM `TitleIdOverlay.cpp#L45-L49`](https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/gui/overlay/TitleIdOverlay.cpp#L45-L49)) and the folder names in the Digiex guide.

Each language is a separate title. The same title ID is sold in the EUR store (`list_GB.json`, UK store) and in the USA store (`list_US.json`) when that store sells the language. All product codes end in `A` (all regions).

| Game | English (EUR + USA) | French (EUR + USA) | German (EUR) | Spanish (EUR + USA) | Italian (EUR) | Japanese |
|---|---|---|---|---|---|---|
| Red / Rouge | `0004000000171000` | `0004000000171600` | `0004000000171300` | `0004000000171900` | `0004000000171C00` | `0004000000170C00` (赤) |
| Green (JPN only) | — | — | — | — | — | `0004000000170D00` (緑) |
| Blue / Bleue | `0004000000171100` | `0004000000171700` | `0004000000171400` | `0004000000171A00` | `0004000000171D00` | `0004000000170E00` (青) |
| Yellow / Jaune | `0004000000171200` | `0004000000171800` | `0004000000171500` | `0004000000171B00` | `0004000000171E00` | `0004000000170F00` (ピカチュウ) |
| Gold / Or | `0004000000172600` | `0004000000172C00` | `0004000000172900` | `0004000000172F00` | `0004000000173200` | `0004000000172300` (金) |
| Silver / Argent | `0004000000172700` | `0004000000172D00` | `0004000000172A00` | `0004000000173000` | `0004000000173300` | `0004000000172400` (銀) |
| Crystal / Cristal | `0004000000172800` | `0004000000172E00` | `0004000000172B00` | `0004000000173100` | `0004000000173400` | `0004000000172500` |

Notes:
- The USA store sells English, French and Spanish only (`list_US.json`, names "(French Version)", "(Spanish Version)"). The German and Italian titles appear only in `list_GB.json`.
- Korean store ([DB `list_KR.json`](https://github.com/hax0kartik/3dsdb/blob/5d9141a56275c974a25aaaf6856ec6a1e50e5c07/jsons/list_KR.json)): Gold `0004000000173500` (금) and Silver `0004000000173600` (은) only; no Korean Gen 1 or Crystal. The Taiwan/Hong Kong store ([`list_TW.json`](https://github.com/hax0kartik/3dsdb/blob/5d9141a56275c974a25aaaf6856ec6a1e50e5c07/jsons/list_TW.json)) sells the Japanese titles (same IDs, "日文版"). Total: 33 title IDs.
- Each VC title has one language; the language of the ROM is the language of the title (**inference** from the separate title IDs).
- GBA Pokémon: no official title ID (§2.1). Injects: any ID.

---

## 4. Per case: what the homebrew needs

### 4.1 GB/GBC VC — read and Upload (feasibility high, effort S)

1. A fixed table of the 33 title IDs of §3 (System GB). Or detection by the archive root holding `sav.dat` (PKSM's way, `Title.cpp#L160-L164`). The table is simpler and avoids false positives.
2. Key: mount the title's RomFS (`romfsMountFromTitle`), read the first file of `/rom/`, take its 0x150-byte header, call `cle_rom(SYS_GB, …)`. Result: the same Key as the PC ROM if the revision and language are the same. If RomFS reading fails (compressed ROM, §1.4): fall back to a fixed table title ID → Key, to fill from real ROM headers (the global checksums per language: **NOT FOUND**).
3. Read `/sav.dat` with `titre_lire`, once the file name is a parameter (today `/main` is fixed).
4. `sram_taille(SYS_GB, size)` already gives 32 768 for 32 784 (remainder 16, ADR 0010) and 65 536 for 65 552. No new rule.
5. `pokemon_partie` already reads Gen 1 and Gen 2 SRAM (`3ds/source/pokemon.h`): play time and trainer name show without new code (**inference**).
6. PC: nothing changes. mGBA's `.sav` is SRAM + 48-byte Trailer for Gen 2, SRAM only for Gen 1.

### 4.2 GB/GBC VC — write back (feasibility high, effort S–M)

1. Copie de sécurité of the whole `sav.dat` first (rule of the project).
2. Refuse if the archive root holds anything other than `sav.dat` (for example `rsm2.dat`), the same way `titre_seul_main` guards `/main` today. **inference**: a save state could otherwise restore the old SRAM.
3. Build the file: received SRAM (32 768 bytes) + the 16-byte footer already in `sav.dat` (Gen 2), or the SRAM alone (Gen 1). Size check as today.
4. `titre_ecrire` with `/sav.dat`: write, commit, delete the secure value. Same steps as PKSM and Checkpoint.
5. Game never saved (no archive or no `sav.dat`): create nothing; ask to start the game and save once (ADR 0009 rule).

Risks to the save:
- Footer kept from the 3DS while the SRAM comes from the PC: the in-game clock can jump (Gen 2). The Blue Moon Falls page reports wrong time after a conversion, not lost data.
- Same game also played on the 3DS with TWiLight Menu++ or an mGBA forwarder: the same Key has two locations on the 3DS. The homebrew already handles "forwarder or ROM" for one Key (`3ds/source/main.c`, key X, `docs/guide-forwarders-mgba.md`); the VC title would be a third possible location.
- ROM revision mismatch (for example Crystal 1.0 on the PC, 1.1 in the VC): two Keys, no sync, no data loss.

### 4.3 GBA VC injects — read (feasibility medium, effort M)

1. Steal a `PxiFS0` session (Luma SVC 0xB0, small assembly stub), only when needed; close it after use.
2. Detect: user archive fails to open and the PXI save file opens (Checkpoint's rule). Inject title IDs are not fixed: list the installed titles, or let the user tick them (the `titres.ini` screen already lists installed applications).
3. Key: read the GBA header (game code at 0xAC) from ExeFS `.code`. Compression of `.code` in injects: NOT FOUND. Fallback: the user picks the Key among the server's GBA Keys.
4. Parse the two-slot container, take the current slot, return `saveSize` bytes (131 072 for Pokémon Gen 3). Hash as any GBA SRAM.

### 4.4 GBA VC injects — write back (feasibility medium, effort M–L)

1. Copie de sécurité of the whole container (both slots) first.
2. Refuse if the server SRAM size differs from the slot's `saveSize`.
3. Write the new save into the non-current slot with "times saved" + 1 (GodMode9 way), keep the console's header fields (SD CID, title ID), recompute the slot CMAC with `FSPXI_CalcSavegameMAC` (repeat until stable), write it last.
4. Test on a real 3DS with a throwaway save before any real save.

Risks: a wrong CMAC or slot choice makes AGB_FIRM reject the save at the next launch (Checkpoint comment). The NAND copy may still hold the old save until the game is launched (path SD → NAND NOT FOUND). Checkpoint and PKSM are GPL-3: as in ADR 0009, re-implement from 3dbrew and the observed method, do not copy code.

---

## 5. Not found / to check on a real 3DS

1. Files at the root of a Pokémon VC archive (`sav.dat` only, or also `rsm*.dat`): sources disagree (§1.1).
2. Field layout of the 16-byte Gen 2 footer: NOT FOUND.
3. ROM file in VC RomFS: raw or compressed, and its exact name: NOT FOUND.
4. Moment when the VC emulator writes `sav.dat`: NOT FOUND.
5. Archive existence before the first in-game save: NOT FOUND.
6. Global checksums of each language ROM (for a fallback Key table): NOT FOUND.
7. GBA VC: SD → NAND path at launch, and `.code` compression in injects: NOT FOUND.

---

## 6. Recommendation

1. **First: GB/GBC VC, read-only** (Upload and compare, no Download). Small change: file name as a parameter of `titre_lire` / `titre_seul_main`, a 33-entry table, Key from RomFS. It answers the open points 1, 3 and 5 of §5 on the real 3DS without any risk to the save.
2. **Second: GB/GBC VC write back**, after a manual test on one throwaway VC save: write SRAM + kept footer, commit, delete the secure value, then start the game and check the "Continue" screen.
3. **Later, only if the user uses injects**: GBA VC through `PxiFS0`. open_agb_firm stays the supported GBA path.
4. Do not touch AGBSAVE in NAND, and do not use GodMode9 from the sync flow.

### Proposed ADR outline (number: next free one when implementing)

**Title**: "Pokémon GB/GBC Virtual Console: `sav.dat`, SRAM only, the 3DS keeps its footer"

- **Context**: the 33 VC titles (§3); `sav.dat` = SRAM (+ 16-byte clock footer for Gen 2); same write steps as `/main` (ADR 0009).
- **Decision**:
  - System GB, Key from the ROM header in the VC RomFS (ADR 0007), so the VC and the PC ROM share one Key.
  - Hash, size and Upload on the SRAM only; the 16-byte footer is a Trailer (ADR 0010 already strips it).
  - Download: SRAM + the footer already on the 3DS; Copie de sécurité first; refuse if the archive holds more than `sav.dat`; commit; delete the secure value.
  - Fixed table of the 33 title IDs.
- **Options rejected**:
  - Detect any archive that holds `sav.dat`: catches non-Pokémon VC games whose save is not plain SRAM.
  - Key `3DS-<title ID>`: never matches the PC ROM.
  - Convert the footer to mGBA's Trailer: layout NOT FOUND, same reason as ADR 0008.
  - GBA VC injects now: needs the Luma `PxiFS0` path and a two-slot writer; open_agb_firm already works (reconsider in a later ADR; the ADR 0008 reason "GodMode9 only" is outdated, see §2.4).
- **Consequences**: in-game clock of Gold/Silver/Crystal can differ between machines (as ADR 0008); a VC title and a TWiLight/forwarder copy of the same game are one Key with two 3DS locations; a game never saved on the 3DS is not created.
