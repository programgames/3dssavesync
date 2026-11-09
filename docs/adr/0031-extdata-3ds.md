# 3DS extra data: one archive per extdata, synced with its games

Some 3DS games keep data outside their save archive, in their **Extra data** (extdata): ghosts, photos, unlocked content, sometimes progress. ADR 0028 syncs the save archive only. Cloudpoint syncs the extdata of every game the same way as its save, with a short skip list; Checkpoint backs it up for every game too. Facts and sources: `docs/notes/research-extdata-gba-vc.md`.

## Decision

- **Which extdata.** The SD extdata (`ARCHIVE_EXTDATA`, media SD, high word 0) of every application that 3dssavesync syncs (ADR 0028), plus the skip list below. Only its `user` part: SpotPass data (`boss`), the icon and Azahar's `metadata` are never read or written. An extdata that does not exist is not created (no `FSUSER_CreateExtSaveData`): the user starts the game once, as for a save.
- **Skip list** (`extdata-ignorees` in the code): NetPass `000400000F657400` (SpotPass downloads), and the Super Mario Maker titles already ignored by ADR 0028.
- **Its own Key, per extdata, not per game:** `3DSX-<extdata ID low, 8 upper-case hex digits>`. Several games share one extdata (Pokémon X and Y, Omega Ruby and Alpha Sapphire, Sun and Moon, the Fire Emblem Fates editions), so a Key per game would give two Keys for the same files. The server Key pattern accepts `3DSX-[0-9A-F]{8}` too.
- **Extdata ID.**
  - 3DS: `AM_GetTitleExtDataId(&id, MEDIATYPE_SD, titleId)`, as cloudpoint; if AM fails, `titleLow >> 8`. Never a hand-made table (Checkpoint's and JKSM's disagree).
  - PC and Android (Azahar): every folder `…/Nintendo 3DS/<id0>/<id1>/extdata/00000000/<ID>/` that has a `user/` subfolder; `<ID>` is the folder name (Azahar writes upper-case hex; compare without case). Azahar gives no exheader, so these devices don't map an extdata to its games, except by `titleLow >> 8` for the name and the on/off switch below.
- **On/off.** There is no separate switch. The 3DS syncs an extdata while at least one of its games is on (ADR 0028 on/off list). PC and Android sync every extdata found, except one whose ID is `titleLow >> 8` of a 3DS game that is turned off on that device.
- **Content = the 3dssavesync archive of ADR 0028**, same deterministic ustar, of the files under `user/` (paths relative to `user/`). Same rules: regular files only, path at most 100 bytes, sorted, 16 MiB at most; otherwise the extdata is **Not synced**, with the reason.
- **3DS, read.** Each file at its full `FSFILE_GetSize` length. A read that fails past the allocated data (`0xD900458B`, sparse file) leaves zeros for the bytes it could not read: the file keeps the size the game created. (Checkpoint shortens the file there; that would change the size, so the Hash, at each pass.)
- **3DS, write.** Backup copy first (`.tar` in `copies/`, read back). Then, in the opened extdata: delete the files that are not in the version; a file of another size is deleted and created again at its new size (extdata files can't be resized); create missing folders; write each file. A file of size 0 that is missing can't be created (the system refuses it): the write stops, with the reason, and the backup copy stays. Then a read back of the whole extdata, compared with the version. There is no commit and no secure value for extdata.
- **PC and Android, write.** As ADR 0028 for a save archive: backup copy (`.tar`), each file through a checked temporary file, read back. Files absent from the version: the PC moves them to the backup folder (never deleted); Android refuses the download, as for every archive (ADR 0026). `metadata`, `icon` and `boss/` are never touched. If `user/` is missing, the extdata is Not synced: the user starts the game once in Azahar.
- **Name on the server.** The 3DS sends the name of the first game of the extdata, with " (données)" / " (data)"; PC and Android send the same for a mapped game, else `Extdata <ID>`.
- **Reinstalled game (ADR 0027).** When the install date of a game changes, the 3DS removes the Base of its extdata too: a reinstall may have reset it, and a silent Upload would replace the server's. An extdata equal to the server's stays In sync.
- **State.** The shared State rule (`docs/design.md`), with its size check, as for every Key. A game's save and its extdata are two Keys: each can be In sync, To upload, To download or a Conflict on its own.

## Rejected options

- **Save and extdata in one archive, one Key**: a shared extdata would be in two Keys (Pokémon X and Y), and a change of the extdata alone would make a Conflict on the save.
- **A separate on/off switch for extdata**: one more list on three devices, for a need nobody has shown. The game's switch covers it.
- **Off by default**: cloudpoint and Checkpoint treat it like the save; the backup copy and the read back cover the risks.
- **Create the extdata when it is missing** (`FSUSER_CreateExtSaveData`): the number of files, folders and the size limit are the game's choice; neither tool does it.

## Consequences

- A server of an older version answers 404 to `3DSX-` Keys: update the server first, as for ADR 0028.
- Writes are atomic per file only: a stop between two files leaves old and new files mixed. The verified backup copy is the safety net, and the next pass shows the extdata To download again.
- A game may compare its save and its extdata; they can come back from two different times (two Keys). Sync All handles both in one pass, which keeps this rare.
- On PC and Android, an extdata whose ID is not `titleLow >> 8` of a game (Pokémon Moon uses Sun's) can't be turned off with its game there; it still syncs.
- Older versions ignore `3DSX-` Keys.
