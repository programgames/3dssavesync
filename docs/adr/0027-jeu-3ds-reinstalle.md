# A reinstalled 3DS game forgets its Base

Deleting a 3DS game from the console also deletes its save. When the user installs it again and starts a new game, the 3-way State sees a Local save that changed since the Base and a server save that did not: To upload. Sync all would then send the new game over the old progress, with no question, and the other devices would download it. The server keeps every version (ADR 0004), so nothing is lost for good, but the user must notice and restore it by hand.

## Decision

- **Signal: the date of the title's TMD on the SD card.** `sdmc:/Nintendo 3DS/<id0>/<id1>/title/<high>/<low>/content/*.tmd` is written when the title is installed. The app reads the newest `.tmd` date (`archive_getmtime`) at each scan, for the 3DS games and the GB/GBC Virtual Console games on the SD card. A game update is another title (`0004000E`): it does not change this date. A game card has no TMD on the SD card: no date, nothing changes.
- **Stored in `base.ini`**, as `INSTALL-<title ID>=<date as 64 hex digits>`. One file, written atomically with the Bases (`replace_file`). The value has 64 characters like a Hash, so an older version of the app reads and keeps the line.
- **A new date removes the Base** of the game (`base_id`: the Key, or `VC-<title ID>`). Without a Base, a Local save that differs from the server's is a Conflict: the user chooses, with the play time and trainer of each side for the Pokémon games. A save equal to the server's is In sync, and the Base comes back. The app says which game was reinstalled.
- **A first date keeps the Base.** After the update to this version, every game gets its first date without any Conflict. Cloudpoint, which uses the same signal, also resets on a first date; for us, that would show a Conflict for every game once, and the case it covers (a game reinstalled before the update) is rare.
- **Any doubt gives a Conflict, never an Upload.** If the date changes for another reason, the user sees one Conflict that a press settles.

## Rejected options

- **Install date through AM**: `AM_GetTitleInfo` gives the size and the version, not a date.
- **A blank or shorter Pokémon game as the signal**: it covers only the games whose save we can read, and a new game is not blank once saved.
- **A separate `installs.ini`**: two files written one after the other; `base.ini` alone keeps one atomic write.

## Consequences

- The PC and Android do not need this rule: Azahar has no install step that deletes a save.
- A game reinstalled while the app was not running is seen at the next scan, before any Sync all.
