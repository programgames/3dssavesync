# Architecture decision records

Each record explains one decision that would be hard to undo: a data format on the server, the identity of a game, a trust model. It says what was decided and why, so nobody has to dig through old pull requests. Choices that are easy to change later, like the look of a screen, don't get a record.

The file names are French, like the code they come from; the records themselves are in English. Number 0013 was never used.

| # | Decision |
|---|---|
| [0001](0001-sync-trois-points-par-hash.md) | Sync compares three hashes (local, server, last sync), never file dates. |
| [0002](0002-outil-maison-plutot-que-daemoon-gamesync.md) | A purpose-built server, 3DS app and launcher, because no existing project covered 3DS + PC emulators + history. |
| [0003](0003-https-curl-mbedtls-sur-3ds.md) | The 3DS app uses curl and mbedTLS for HTTPS, since the system `httpc` service stops at TLS 1.1. |
| [0004](0004-serveur-sans-suppression.md) | The server never deletes or changes a version; every upload is a new file. |
| [0005](0005-identite-par-code-jeu.md) | A DS game is identified by its game code, not by the ROM file name. |
| [0006](0006-lanceur-autour-de-melonds.md) | The launcher replaces the emulator in the frontend and syncs before and after the game, with no file watching. |
| [0007](0007-cle-de-jeu-par-systeme.md) | Each game's key depends on its system and is read from the ROM header. |
| [0008](0008-gb-gba-par-fichiers-sram-seule.md) | GB, GBC and GBA: only the SRAM is synced, and each machine keeps its own clock data. |
| [0009](0009-sauvegardes-3ds-natives.md) | Native 3DS saves: the `main` file of the save archive, with code inspired by Checkpoint. |
| [0010](0010-trailer-d-horloge-par-reste-512.md) | GB/GBA clock data is recognised by the file size modulo 512. |
| [0011](0011-srm-gpsp-beetle-taille-du-serveur.md) | For gpSP and Beetle GBA, the save size comes from the version on the server. |
| [0012](0012-eeprom-et-mbc2-forme-du-serveur.md) | GBA EEPROM and GB MBC2 saves are stored in one single form on the server. |
| 0013 | Not used. |
| [0014](0014-rom-hack-et-alias.md) | A ROM hack that shares its original's key is refused, then synced under an alias. |
| [0015](0015-roms-en-sous-dossiers-3ds.md) | ROMs in subfolders on the 3DS: the save is looked up where the launcher writes it. |
| [0016](0016-restauration-depuis-le-pc-sans-ecriture-locale.md) | Old versions are restored from the PC, without writing any local file. |
| [0017](0017-config-ini-de-la-3ds-ecrit-depuis-le-pc.md) | The PC writes the 3DS `config.ini`, with the home network address first. |
| [0018](0018-nom-du-jeu-sur-le-serveur.md) | The game name is stored on the server, sent by the devices that play it. |
| [0019](0019-desinstallation.md) | Uninstalling gives LaunchBox and ES-DE their original emulators back. |
| [0020](0020-version-portable.md) | A `portable.txt` file next to the program keeps everything in that folder. |
| [0021](0021-mise-a-jour-du-homebrew-depuis-github.md) | The 3DS app updates itself from GitHub releases, only when asked, after a checksum check. |
| [0022](0022-icone-pres-de-l-horloge.md) | One resident program shows the icon near the clock and runs the local server. |
| [0023](0023-cia-inclus-et-mise-a-jour-du-pc.md) | The PC program carries the 3DS app and updates itself. |
