# The PC program carries the 3DS app and updates itself

Installing the 3DS app meant a second download (`3dssavesync.cia`), a copy to the SD card, and a version that could differ from the PC. The PC program also stayed at the version first downloaded: nothing told the user that a new one was out.

## The .cia inside the PC program

- The `release` job of the CI copies `dist/3dssavesync.cia` (built by the `homebrew-3ds` job of the same tag) to `client/embarque/` before `go build`; `//go:embed embarque` puts it in each PC binary (about 1 MB more). The folder always holds `LISEZMOI.txt`, so local and CI test builds compile without the `.cia`; `embarque/3dssavesync.cia` is in `.gitignore`.
- **Prepare the 3DS SD card** writes `config.ini` first, then `<card>/cias/3dssavesync.cia`. Before the copy: the card root has `Nintendo 3DS`, the free space (`GetDiskFreeSpaceExW`, `statfs`) covers the new file and the dated copy of the old one. The copy goes through the usual path: identical file → nothing; else dated copy of the old file, temporary file written and read back, rename, read back. A locked card (`ERROR_WRITE_PROTECT`, `EROFS`) gets its own message: slide the lock switch up.
- A build without the `.cia` (`dev`) says so and gives the release page. A card without `3ds/FBI.3dsx` or `3ds/FBI` gets a note with the FBI page ([nh-server/FBI-NH](https://github.com/nh-server/FBI-NH/releases/latest); `Steveice10/FBI` no longer changes). Neither blocks the preparation.
- The date of the last successful preparation goes to `config.json` (`carte_preparee`), for the **Everything is ready** list. Never guessed from the card.

## PC update

- **Version**: `var version = "dev"`, set by `-ldflags -X main.version=<tag without v>` in the `release` job. Shown in the page title and the tray menu. `dev` never offers an update.
- **Check**: `GET https://api.github.com/repos/programgames/3dssavesync/releases/latest`, at most once a day (`maj-pc.json` in the settings folder: the day and the release seen), from the settings page (**Everything is ready** list) and the tray (every 6 hours, same daily limit). Setting **Check for updates**, on by default (`"verifier_maj": false` turns it off). Versions compare like the 3DS app (`3.10.0` > `3.9.1`, `3.7.0` > `3.7.0-rc.1`).
- **Install**: only with the **Update** button of the page (the tray item opens the page). The release is read again, then `3dssavesync-<os>-<arch>[.exe]` and `SHA256SUMS` are downloaded into memory. Size equal to the asset size, SHA-256 equal to its `SHA256SUMS` line, free space for the file: else nothing changes. Then the new exe is written next to the running one (`.3dssavesync-tmp`, written and read back), the running exe is renamed `3dssavesync-<old version>[.exe]` (Windows allows renaming a running exe; a second update to the same version adds `-1`, never overwrites), and the temporary file takes its name. If that last rename fails, the old exe gets its name back. Portable version: the exe in its folder is the running one.
- **Restart**: the icon near the clock, if it runs, is stopped (`arret-tray`) and started from the new exe; the new exe opens the settings in a new tab; the old settings process stops.

## Trust model

Same as the 3DS app (ADR 0021): HTTPS to GitHub (system roots on the PC), and `SHA256SUMS` from the same release, written by the CI. It detects a damaged or cut download, not someone who can publish releases on the repository: the GitHub account and HTTPS are the trust anchors. No code signing. Nothing is installed without a click, and a release without `SHA256SUMS` or without the file of this system is refused.

## Consequences

- The `.cia` on the card always matches the PC program, and the README install has one download.
- Old exes pile up next to the program (`3dssavesync-3.7.0.exe`, …): the user can delete them. Keeping one is the manual rollback.
- The settings page updates the exe it runs from. Started from another copy (a download folder), it updates that copy, not the installed one: **Copy the program** then installs it.
- A `serve` process (local server without the icon) keeps the old version until the next session.
- GitHub allows 60 unauthenticated requests per hour per Internet address; one check per day and per program stays far below.
