# ROM hack under the Key of another game: refusal, then alias

A ROM hack often keeps the game code of its original (Pokémon Radical Red keeps `BPRE`, the one of FireRed). For ADR 0005, same Key = same Game. On the PC, the hack and the original have two Save files (named after the ROM), but a single Base and a single Current version: playing one then the other uploads the hack game over the original one, or the reverse.

Rule:

- **PC, `run`**: at each Base written, the Launcher remembers the ROM of this Key in `roms.json`, next to `config.json`: one file for the whole PC, because the Current version is shared by all emulators. The per-emulator files of the first version of this rule (`bases/roms/<emulator>.json`) are read and merged. `bases/<emulator>.json` does not change. Another ROM with the same Key, while the remembered ROM still exists (`os.SameFile`): question "Sync separately (ROM hack)" or "Play without sync" (no window possible: without sync, notification that names both files). Remembered ROM gone (renamed, moved, temporary extraction by LaunchBox): the new one is accepted and remembered. No remembered ROM, a Base for this Key in any emulator of the PC, and no Save (or a blank one) for this ROM: `run` asks "Sync", "Sync separately (ROM hack)" or "Play without sync" (no window possible: play without sync); `launch` refuses with a notification. This catches a hack started for the first time, without blocking the first launch of a game already synced elsewhere. Otherwise the started ROM is accepted and remembered.
- **PC, `launch`**: two ROMs with the same Key in the `roms` folders: none is synced, one single notification (like the 3DS Duplicate). Same for two ROMs that give the same Save file (case ignored), whatever their Keys. A ROM alone for its Key follows the same rule as `run`.
- **Alias**: `"alias": {"<ROM name without extension>": "<Key>"}` in `config.json`, and `alias.ini` (`<ROM name without extension>=<Key>`, one line per ROM) in `sd:/3ds/3dssavesync/` on the 3DS. The name is the file name on each side, with no case change. The Key is uppercased and trimmed; it must have the form of the Key of the ROM's System (ADR 0007): `0RAD` (DS), `GBA-0RAD` (GBA), `GB-MONHACK-0001` (GB). The same alias gives the same Key on both machines (`cle_alias` in `3ds/source/cle.c`, `cleAlias` in `client/cas.go`, same test cases). With an alias, the Duplicate rule no longer counts this ROM with the original.
- **Invalid alias**: the ROM is not synced, never under the Key of its header. The message gives a valid example.
- **Alias created by 3dssavesync** (2026-11-03, the user never types a Key nor edits a file): "Sync separately" in the question, or the "ROM hacks" section of the settings page (ROM file chosen, Add / Remove). The Key is derived from the ROM file name (SHA-256: 3 letters or digits after a leading `0`; `GB-ALIAS-` + 4 hexadecimal digits for GB), so the same ROM gets the same Key on every PC; a Key already used by another alias of this PC takes the next derivation. `config.json` is copied before each change (dated copy, `-1`, `-2`… within one second). "Prepare the 3DS SD card" writes `alias.ini` next to `config.ini` with the same aliases (other lines kept, dated copy, read back).
- **3DS**: no alias for 3DS games (Key = title ID, and Azahar stores the save by title ID: two ROMs of a 3DS game share it).

Documentation advice: start the alias with a digit (`GBA-0RAD`). Real DS and GBA game codes start with a letter, so the alias never meets a commercial game.

## Rejected options

- **Longer Key (`GBA-BPRE-RADICALRED`)**: the Key expression of the Python server and of the local server must change, then the server must be redeployed. An alias of the current form is enough. The addition stays possible later without breaking what exists.
- **Key = header + Hash of the ROM**: a patched ROM (translation patch, other dump) would change Game and lose its History.
- **Guess the hack** (internal title, file name): the tool never guesses (ADR 0005).

## Consequences

- A hack synced before the original becomes the remembered ROM: then the original is refused. The message says what to do (an alias for one of the two).
- A remembered ROM on an unplugged disk counts as gone: the started ROM is accepted.
- To share a hack game between the 3DS and the PC, the same alias is needed on both sides.
- A ROM format the Launcher cannot read (`.7z`…) never gets an alias: the game always starts without sync.
