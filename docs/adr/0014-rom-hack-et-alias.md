# ROM hack under the Key of another game: refusal, then alias

A ROM hack often keeps the game code of its original (Pokémon Radical Red keeps `BPRE`, the one of FireRed). For ADR 0005, same Key = same Game. On the PC, the hack and the original have two Save files (named after the ROM), but a single Base and a single Current version: playing one then the other uploads the hack game over the original one, or the reverse.

Rule:

- **PC, `run`**: at each Base written, the Launcher remembers the ROM of this Key in `bases/roms/<emulator>.json`, next to `bases/<emulator>.json` (unchanged, still readable by an older version). Another ROM with the same Key, while the remembered ROM still exists (`os.SameFile`): game without sync, notification that names both files and suggests an alias. Remembered ROM gone (renamed, moved, temporary extraction by LaunchBox): the new one is accepted and remembered. No remembered ROM (Base from before this rule): the started ROM is accepted and remembered.
- **PC, `launch`**: two ROMs with the same Key in the `roms` folders: none is synced, one single notification (like the 3DS Duplicate). A ROM alone for its Key follows the same rule as `run`.
- **Alias**: `"alias": {"<ROM name without extension>": "<Key>"}` in `config.json`, and `alias.ini` (`<ROM name without extension>=<Key>`, one line per ROM) in `sd:/3ds/3dssavesync/` on the 3DS. The name is the file name on each side, with no case change. The Key is uppercased and trimmed; it must have the form of the Key of the ROM's System (ADR 0007): `0RAD` (DS), `GBA-0RAD` (GBA), `GB-MONHACK-0001` (GB). The same alias gives the same Key on both machines (`cle_alias` in `3ds/source/cle.c`, `cleAlias` in `client/cas.go`, same test cases). With an alias, the Duplicate rule no longer counts this ROM with the original.
- **Invalid alias**: the ROM is not synced, never under the Key of its header. The message gives a valid example.
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
