# Native 3DS saves: the `main` file of the archive, home-made code inspired by Checkpoint

For the 8 3DS Pokémon games (X, Y, Omega Ruby, Alpha Sapphire, Sun, Moon, Ultra Sun, Ultra Moon), the save is an archive (`ARCHIVE_USER_SAVEDATA`) that holds a single file, `main`. Azahar stores the same archive as ordinary files (`…/title/00040000/<low>/data/00000001/main`). We sync this file only.

- 3DS, read: open the archive (SD, otherwise cartridge), read `/main`.
- 3DS, write: Backup copy, write `/main`, `ControlArchive(COMMIT)`, then delete the secure value (like Checkpoint, JKSM, Cloudpoint), otherwise the game refuses a restored save.
- PC: read the title ID in the NCSD/NCCH header of the ROM extracted by LaunchBox, then the Azahar `main` file. An existing `00000001.metadata` is never touched.
- Fixed table of the 8 title IDs (one per game, all regions).

## Rejected options

- **Copy the Checkpoint code**: C++ under GPL-3; it would force GPL-3 on the homebrew and a second language. We reuse its method (same order of operations), modelled on the MIT C code of DaeMoon.
- **All 3DS saves, multi-file**: requires a package format (zip or manifest) on the Server side. Useless for the 8 target games.

## Consequences

- A game never started has no archive on the 3DS: nothing is created, the user is asked to start the game once and save.
- Azahar (revised on 2026-11-02, issue #9): for a game never started, the Launcher creates the folder, `main` and a `00000001.metadata` with a known value (read on Moon and Ultra Sun, same value assumed for the 8 Pokémon). Test on a copy of Azahar: Pokémon X loads the game without reformatting. Without a known value, or if a `.metadata` already exists without a folder, the Launcher refuses.
- The `.cia` reuses the `FileSystemAccess` list of Checkpoint: a `.3dsx` may not have the right to open the save of another title (conflicting sources).
