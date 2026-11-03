# Portable version: portable.txt next to the program

Some users keep their whole setup in one folder they can move: `LaunchBox\ThirdParty\3dssavesync`, a USB drive, the SD card of a Steam Deck. The normal install puts the settings in the user folder (`%APPDATA%\3dssavesync`, `~/.config/3dssavesync`, `~/Library/Application Support/3dssavesync`) and copies the program to a fixed place, so it does not move with the folder.

## Decision

- A file `portable.txt` next to the running program makes it portable. The rule is checked in one place (`dossierPortable`).
- The settings folder is then the program folder: `config.json`, the log, `bases/`, the pending uploads, `copies/`, `serveur/`. Only the default changes: `--config` and the paths in `config.json` still win.
- The program is never copied elsewhere: the fixed place is the running program, so the settings page hides its "Program" part. Frontends and shortcuts always point to the running program, not to the path saved in `config.json` (a USB drive letter changes).
- The folder must be writable. If it is not (read-only drive, `Program Files`), the program stops with a message: move the folder, or delete `portable.txt`. It never falls back to the user folder without saying so: the settings would split in two places.
- The uninstall undoes frontends and shortcuts as usual, but leaves the program and its folder; the report names the folder to delete.
- The release adds `3dssavesync-windows-amd64-portable.zip` and `3dssavesync-linux-amd64-portable.tar.gz`: the program and `portable.txt`, built from `.github/portable.txt`.

## Consequences

- Deleting `portable.txt` goes back to the normal mode, with the settings of the user folder; the files in the program folder do not move.
- Autostart of the local server and shortcuts point to the portable folder: when the drive is missing, they do nothing until it comes back.
- `portable.txt` lives in `.github/`, not in `client/`: a development build in `client/` stays in the normal mode.
