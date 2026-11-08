# Every 3DS game: the whole save archive, as one tar file per version

Until now, only 3DS games whose save archive holds a single `/main` file could sync (ADR 0009): the 8 Pokémon games by default, others chosen by hand in `titres.ini`. Most 3DS games keep several files, sometimes in folders, so they never synced. Cloudpoint shows that copying the whole archive works for almost every game.

## Decision

- **Which games.** Every application (title ID high word `00040000`) that has a save archive, on the SD card or the game card, except a short skip list of games whose save is tied to one console (`titres-ignores` in the code: Super Mario Maker `00040000001A0300`, `00040000001A0400`, `00040000001A0500`, `00040000001BB800`). A game never started has no archive: nothing is created, as before.
- **On by default, off per game.** A new game is synced as soon as it has a save. The user can turn a game off; turned-off games are kept in `titres-off.ini` on the 3DS (one title ID per line) and in the PC settings (`titres_3ds_off`). `titres.ini` (the old allow-list) is read once: a title that is not in it, among the games found, is turned off, so an existing choice is kept; then the file is no longer used.
- **Two Keys, by title, never by content.**
  - The 8 Pokémon games keep `3DS-<title ID>` and the raw bytes of `/main` (ADR 0009). Nothing changes for them, on any device or version.
  - Every other game uses `3DSA-<title ID>`, whose server versions are a **3dssavesync archive** (below), even when the archive holds only `/main`. The Key never changes when a game adds a file later, so two devices can't drift apart on two Keys.
  - A game that an older version synced as `3DS-<title ID>` (chosen in `titres.ini`) starts over as `3DSA-`: its first pass uploads, and the old Key stays on the server with its history. Every device must run this version: an older one would keep using the old Key.
- **3dssavesync archive = deterministic ustar.** Same files, same bytes on the 3DS (C), the PC (Go) and Android (Kotlin), so the same archive gives the same Hash everywhere:
  - regular files only, no directory entries (a folder exists through its files; an empty folder is not kept);
  - path relative to the archive root, `/` separators, no leading `/`, at most 100 bytes in the `name` field (`prefix` empty); a longer path makes the game Not synced, with the reason;
  - entries sorted by path, byte order;
  - header fields: mode `0000644`, uid and gid `0000000`, size in 11 octal digits, mtime `00000000000`, each followed by NUL; typeflag `0`; magic `ustar\0`, version `00`; uname, gname, linkname, prefix empty; devmajor and devminor `0000000\0`; checksum computed as ustar says (6 octal digits, NUL, space);
  - data padded with zeros to 512 bytes; two 512-byte zero blocks at the end.
  A reader accepts only this shape (regular files, no `..`, no absolute path, no duplicate) and refuses anything else as damaged.
- **3DS, write.** Backup copy first (the archive as a `.tar` in `copies/`, read back). Then, in the opened archive: delete the files that are not in the new version; a file of another size is deleted and created again at its new size (as Checkpoint restores; a save file keeps its creation size, no SetSize); write each file, `ControlArchive(COMMIT)`, then remove the secure value (ADR 0009). Until the commit, the archive's file table keeps its old state. A save formatted without duplicate data may change file contents in place before the commit, as the `/main` write of ADR 0009 already can: the verified Backup copy is the safety net. Then a read back of the whole archive, compared with the version.
- **PC and Android (Azahar).** The archive is the folder `…/Nintendo 3DS/<id0>/<id1>/title/<high>/<low>/data/00000001/`. Write: Backup copy (`.tar`), then each file written through a checked temporary file, the files absent from the version moved to the backup folder (never deleted), and a read back of the folder. `00000001.metadata` (the archive format) is never written for these games: a game never started in Azahar is not synced there, and the user is asked to start it once.
- **Conflicts.** The Pokémon play time and trainer stay for the 8 Pokémon games. Other games show the server date and device, as today for DS games.

## Rejected options

- **One format for all, with the 8 Pokémon as archives too**: every existing Base and server version of the 8 games would change Key or format, and older versions would read an archive as `/main`.
- **Choose the format from the content (`/main` alone → raw)**: a game that adds a file later would switch Key, and two devices could each keep playing their own copy without a Conflict.
- **zip or a compressed format**: zip has timestamps and several valid encodings, so two writers can give two Hashes for the same files; 3DS saves are small and the server already stores every version.
- **Cloudpoint's 256 KiB chunks with dedup**: worth it for large extdata on a public cloud; our server stores whole versions and has no shared storage to save.

## Consequences

- The server stores bytes per Key, as before; only its Key pattern accepts `3DSA-` too. A server of an older version answers 404 to `3DSA-` Keys: it must be updated first, or the other 3DS games can't sync.
- The shared State rule keeps its size check: two sides of different sizes are a Conflict, even when one is the Base. A 3DS game usually keeps its files at a fixed size, so this stays rare; when a game adds a file, the user is asked instead of a silent Download.
- Older versions ignore `3DSA-` Keys (they are not in their game list), so they can't damage an archive version.
- Extra data (extdata) and GBA Virtual Console saves are separate archives with their own Keys, decided in their own ADRs.
