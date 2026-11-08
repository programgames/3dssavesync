# Changelog

What changed in each release, for people who use 3dssavesync. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **3DS: a game you install again asks first.** Deleting a 3DS game deletes its save. Before, after you installed it again, its new save was uploaded over your progress on the server. Now the app sees that the game was installed again, and you choose which save to keep.
- **Every 3DS game now syncs, not only Pokémon.** On the 3DS, the PC (Azahar) and Android (Azahar), every 3DS game with a save is synced, with all its save files. New games are on; turn one off in the 3DS settings, in the game details on Android, or with `titres_3ds_off` on the PC. The 8 Pokémon games work as before. **Update the server and every device first**: an older server refuses these games, and an older device doesn't see them.
- **3DS: a journal of what the app did.** `sdmc:/3ds/3dssavesync/journal.log` lists each upload, download and restore, with its result. Join it to a bug report. It never holds your server address or token.

### Changed

- **3DS: closing the lid no longer cuts a sync.** The console doesn't go to sleep during an upload, a download or an app update.

## [3.12.0] - 2026-11-08

### Added

- **Android: the app updates itself.** When a new version is out, a card at the top of **Games** offers it. **Update** downloads it from GitHub, checks it, and Android asks you to confirm. Your settings, saves and backup copies stay. You can turn the check off in **Settings**. Install this version by hand one last time.

## [3.11.4] - 2026-11-08

### Fixed

- **PC: no more false "ROM hack" question with zipped ROMs.** LaunchBox extracts the same archive into a different temporary folder from time to time. 3dssavesync now sees that both copies hold the same game, and syncs it without asking.

## [3.11.3] - 2026-11-07

### Fixed

- **PC settings page: no false "SD card not prepared" warning.** If you installed the 3DS homebrew another way (FTP, FBI), the "all ready" list now sees that your 3DS already syncs with the server.

## [3.11.2] - 2026-11-06

### Added

- **Android: the backup copies are in the history of each game.** Open a game, tap **Show the history**, and look under **This phone**: put a copy back, or save it to a folder you can open (Downloads, for example). Before, the copies were in a folder that file managers on the phone cannot open.

### Fixed

- **PC: no more "unknown games folder" notification after each Windows start.** If you start your games from a frontend such as LaunchBox, you don't need a games folder: these games sync anyway. The settings page now says which games folders are not set, and **Sync all** still lists them.

## [3.11.1] - 2026-11-04

### Fixed

- **Android: "Scan the PC's QR code" works again.** In 3.11.0 the scanner said it was not ready on every phone. Install 3.11.1 over 3.11.0.
- **3DS: no more frozen screen during a transfer.** After A on a game, "Uploading…" or "Downloading…" shows at once with the game name.
- **3DS: the next DS game starts faster** after 3dssavesync used the network (about 10 s instead of 20 s on a New 3DS XL).

### Changed

- **PC notifications**: each problem is notified once, and a notification stays as long as your system setting says (Windows: "Dismiss notifications after").

## [3.11.0] - 2026-11-04

### Added

- **Android app.** The first release of the Android client: download `3dssavesync-android.apk` from this release and install it. It syncs melonDS, DraStic, RetroArch, Azahar, NooDS and other emulators with your server, and scans the QR code of the PC settings page to set itself up. Later versions install over it: never uninstall it, because that deletes its backup copies. See the [Android guide](docs/guide-android.md).
- **Guided first run on Android**: server, emulator, folders, with the emulators installed on your phone first. The folder picker opens at the usual folder and tells you how many saves it found.

### Safety

- On Android, a save an emulator writes back after a download, or a write the phone interrupted, is shown as a conflict to resolve, never uploaded on its own.

## [3.10.0] - 2026-11-04

### Added

- **Virtual Console on the 3DS.** Game Boy and Game Boy Color games bought on the eShop are synced like your ROMs (Luma3DS needed). A Virtual Console game and the mGBA forwarder of the same game are both synced: play either one, and **Sync all** brings your progress to the other, even to a Virtual Console game you never started.
- **Pokémon play time of the server version on the 3DS.** For a Pokémon game that is not in sync, the bottom screen shows the play time and trainer of the server version under **On the server**, and **Sync all** lists both before replacing a save.
- **QR code for your phone.** On the **Server** step of the PC settings page, **Show the QR code** gives the server address and token to the Android app, which is coming soon. The code stays hidden until you click.

## [3.9.0] - 2026-11-04

Not released on its own: these changes come with 3.10.0.

### Added

- **Sync without the Launcher.** The icon near the clock uploads your save when you close an emulator you opened yourself, and every 15 minutes gets new versions from the server for closed emulators, only over a save that did not change, after a backup copy. A conflict gives a notification and replaces nothing. You can turn it off in **Frontends and install**.

## [3.8.0] - 2026-11-04

Not released on its own: these changes come with 3.10.0.

### Added

- **Big game lists on the 3DS.** Tabs for All, To do, DS, GB, GBA and 3DS (L and R), page jumps (left and right), and a **Search** button on the touch screen. The search ignores accents: "pokemon" finds "Pokémon".
- **List order** in the 3DS settings: to do first (as before), by name, or last played.
- **Games page** in the PC settings (it was History): a filter per console, a sort by name or last save, the date and device of each game's last save, and the play time and trainer of Pokémon games. The search ignores accents and matches every word.
- **Sync all on the PC**, in the Games page and in the menu of the icon near the clock: syncs the games of every closed emulator without starting one. A game in conflict is left as it is until you start it.

### Changed

- The 3DS app has no game limit any more (it stopped at 512 ROMs), and you can tick up to 256 3DS games (64 before).
- In the 3DS game choice, ticked games come first.

## [3.7.0] - 2026-11-04

Not released on its own: these changes come with 3.10.0.

### Added

- **Icon near the clock** (Windows, Linux). Right-click it to open the settings, upload pending saves now, or see whether the local server runs. It starts with your session and runs the local server itself, so only one program stays in memory.
- **Portable version.** Put `portable.txt` next to the program and everything stays in that folder: settings, backup copies, server data. Handy on a USB stick or in the LaunchBox folder. Releases include a Windows `.zip` and a Linux `.tar.gz` ready to use.
- **The 3DS app updates itself** from GitHub, when you ask for it in its settings. It checks the download before it installs it.
- **The PC program updates itself** from the settings page. The old program stays next to the new one, in case you want to go back.
- **Guided first start.** Without settings, the page walks you through four steps: server, emulators, 3DS SD card, done.
- **Prepare the 3DS SD card** now also copies `3dssavesync.cia`, the same version as the PC program, so you install the 3DS app with FBI from the card. The page warns when the card is locked, full, or has no FBI, and never replaces a newer app with an older one.
- **Checks** ("Is everything ready?"): one list in the settings page that tells you what's left to do, with a button to fix each line.
- When you add an emulator that LaunchBox already knows, its extra arguments (like `-f` for full screen) are offered for 3dssavesync too.
- A new icon for the 3DS app, its banner, and the Windows program.
- Each release now publishes `SHA256SUMS`.

### Changed

- **Redesigned settings page**: status badges at the top, one row per emulator with a switch, a dark theme, and emulators you don't use folded away.
- **Redesigned 3DS app**: one word per game state, clearer details ("Same version as the server", "5 h ago · from pc"), side-by-side cards for a conflict, a progress bar, and a footer that shows when the server list was last read.
- Messages on the PC, in the settings page and on the 3DS are rewritten in plainer French and English. Errors say what to do, then give the technical detail on its own line.
- A Scoop install of an emulator shows once, with its real program, instead of twice.
- Command-line flags have English names: `setup --text`, `setup --no-browser`, `restore --yes`, `uninstall --yes`. The French names (`--texte`, `--sans-navigateur`, `--oui`) still work.

### Fixed

- Uninstalling no longer removes the LaunchBox emulators that versions 1 and 2 changed in place (melonDS, mGBA, Azahar, RetroArch). It points them back to their real program, gives RetroArch its core back, and tells you when a platform ends up with two default emulators.
- The 3DS app no longer shows an impossible Pokémon play time.
- The 3DS list of 3DS games (`titres.ini`) can no longer end up empty after a power cut while it is saved.
- Two backup copies of the same game made in the same second on the 3DS no longer share a name.
- The 3DS app reads the server list more strictly: a field missing from one game is no longer taken from the next one.
- Uninstalling recognises LaunchBox emulators whose title contains an apostrophe.
- The server answers with an error instead of dropping the connection when it cannot write to its disk.

### Removed

- The old PowerShell launcher (`pc/sync.ps1`) and the `migrate-v2` command that imported its settings. Version 2 installs should set up 3dssavesync from the settings page.

## [3.6.1] - 2026-11-03

### Added

- **Uninstall**, from the settings page or `3dssavesync uninstall`. LaunchBox and ES-DE get their original emulators back, and the shortcuts and the program go to the Recycle Bin. Your saves and backups stay.

## [3.6.0] - 2026-11-03

### Added

- **Restore an old version** of any save from the PC, in the new History part of the settings page.
- **ROM hacks** can keep a save of their own, without changing any file.
- **Game names** are stored on the server, so lists show names instead of keys.
- ROMs in subfolders on the 3DS.
- A failed upload is retried at the next game.

### Changed

- **Simpler first start**: the local server is the default, and the PC writes the 3DS settings to the SD card, so there's nothing to type on the 3DS.
- New settings page with a side menu and a status bar.
- Clearer messages on the PC and the 3DS, each saying what to do.
- All documentation is now in English, with a troubleshooting guide.

### Fixed

- Some Game Boy ROMs crashed the PC program while it read their key.
- ROMs in a format the program can't read (`.7z`) now start without sync, with a message.
- The 3DS writes its settings and sync state through a checked temporary file.

## [3.5.0] - 2026-11-02

### Fixed

- GBA games with EEPROM saves and Game Boy games with MBC2 saves now sync between emulators that store them differently.

## [3.4.0] - 2026-11-02

### Added

- Profiles for SkyEmu, NanoBoyAdvance and BGB.
- RetroArch cores gpSP and Beetle GBA.
- **Open** buttons in the settings page start an emulator with a sync before and after.

## [3.3.1] - 2026-11-02

### Added

- Two different games can run at the same time (for a Pokémon trade, for example).

## [3.3.0] - 2026-11-02

### Added

- DeSmuME profile.
- Play time and trainer name for every main Pokémon game from Red and Blue to Ultra Moon, shown when you have to choose between two versions.

### Fixed

- Game Boy and GBA clock data is recognised for every save size, so saves with a clock sync between emulators.

## [3.2.0] - 2026-11-02

### Changed

- The 3DS app is now drawn with graphics instead of a text console, and shows the date and play time of both versions in a conflict.

## [3.1.0] - 2026-11-02

### Added

- A 3DS Pokémon game never started in Azahar gets its save from the server.

## [3.0.0] - 2026-11-01

First release under the name 3dssavesync.

- One PC program for Windows, Linux and macOS, with profiles for melonDS, mGBA, Azahar, RetroArch, VBA-M and SameBoy.
- A settings page in your browser, which sets up LaunchBox, ES-DE and shortcuts.
- A local server built into the PC program, or your own Docker server with automatic HTTPS.
- A 3DS app for DS, Game Boy, GBA and native 3DS games, in French and English.

[Unreleased]: https://github.com/programgames/3dssavesync/compare/v3.12.0...HEAD
[3.12.0]: https://github.com/programgames/3dssavesync/compare/v3.11.4...v3.12.0
[3.11.4]: https://github.com/programgames/3dssavesync/compare/v3.11.3...v3.11.4
[3.11.3]: https://github.com/programgames/3dssavesync/compare/v3.11.2...v3.11.3
[3.11.2]: https://github.com/programgames/3dssavesync/compare/v3.11.1...v3.11.2
[3.11.1]: https://github.com/programgames/3dssavesync/compare/v3.11.0...v3.11.1
[3.11.0]: https://github.com/programgames/3dssavesync/compare/v3.10.0...v3.11.0
[3.10.0]: https://github.com/programgames/3dssavesync/compare/v3.6.1...v3.10.0
[3.9.0]: https://github.com/programgames/3dssavesync/compare/v3.6.1...v3.10.0
[3.8.0]: https://github.com/programgames/3dssavesync/compare/v3.6.1...v3.10.0
[3.7.0]: https://github.com/programgames/3dssavesync/compare/v3.6.1...v3.10.0
[3.6.1]: https://github.com/programgames/3dssavesync/compare/v3.6.0...v3.6.1
[3.6.0]: https://github.com/programgames/3dssavesync/compare/v3.5.0...v3.6.0
[3.5.0]: https://github.com/programgames/3dssavesync/compare/v3.4.0...v3.5.0
[3.4.0]: https://github.com/programgames/3dssavesync/compare/v3.3.1...v3.4.0
[3.3.1]: https://github.com/programgames/3dssavesync/compare/v3.3.0...v3.3.1
[3.3.0]: https://github.com/programgames/3dssavesync/compare/v3.2.0...v3.3.0
[3.2.0]: https://github.com/programgames/3dssavesync/compare/v3.1.0...v3.2.0
[3.1.0]: https://github.com/programgames/3dssavesync/compare/v3.0.0...v3.1.0
[3.0.0]: https://github.com/programgames/3dssavesync/releases/tag/v3.0.0
