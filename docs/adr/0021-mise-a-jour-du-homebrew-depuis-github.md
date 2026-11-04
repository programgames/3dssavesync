# The 3DS app updates itself from GitHub releases

Users install the 3DS app once with FBI, then rarely think of it again: old versions stayed on consoles. The app now checks the latest GitHub release and installs it, but only when the user asks.

- **Version**: the Makefile passes `VERSION` (`dev` by default) as a C define. The CI gives the tag version on tag builds (`v3.7.0` → `3.7.0`). A `dev` build never offers an update.
- **Check**: at start, at most once per day (day stored in `sdmc:/3ds/3dssavesync/maj.ini` with the latest version seen; `maj.ini` also holds `installee=`, the version that ran last, written at start for the PC: ADR 0023), and on demand with **SELECT** > **Update**. `GET https://api.github.com/repos/programgames/3dssavesync/releases/latest`, without the server token. Versions compare numerically (`3.10.0` > `3.9.1`). Any failure (no Wi-Fi, rate limit, GitHub down) only shows in the settings; sync works as before.
- **Install**: only after the user presses **A** on a confirmation. The app downloads `SHA256SUMS` and the file it runs as (`.cia`, or the `.3dsx` given by `argv[0]` under the Homebrew Launcher; a `.3dsx` without an `argv[0]` of the form `sdmc:/…/name.3dsx`, such as 3dslink, is refused: the running file is unknown) into memory (8 MB at most), and compares the SHA-256. Then: the `.cia` goes to AM (`AM_StartCiaInstall`, writes, `AM_FinishCiaInstall`; `AM_CancelCIAInstall` on any error); the `.3dsx` is replaced through `replace_program` (`.tmp` written and read back, the old file renamed `.bak`, the `.tmp` renamed and read back, then `.bak` removed; any failure puts `.bak` back, so a power cut always leaves a whole program). The app then closes. Saves, settings and `base.ini` are not touched.

## Trust model

- HTTPS to GitHub, with the certificate checked against embedded roots only: Sectigo Public Server Authentication Root E46 and USERTrust ECC Certification Authority for `api.github.com` and `github.com` (chains seen on 2026-11-03), ISRG Root X1 for the release files (`release-assets.githubusercontent.com`, Let's Encrypt). The user's server keeps ISRG Root X1 alone.
- `SHA256SUMS` comes from the same release, written by the CI `release` job. It detects a damaged or cut download. It does not protect against someone who can publish releases on the repository: the GitHub account and HTTPS are the trust anchors. No code signing (no key to manage, no key on the console).
- Nothing is installed without a user action, and a release without `SHA256SUMS` is never installed.

## Rejected options

- **Automatic install**: a broken release would break every console without a choice.
- **Install from the PC over FTP**: needs an FTP server on the 3DS and a PC; the user asked for an update from the 3DS itself.
- **Signed releases (minisign, ed25519)**: more safety only against a stolen GitHub account, for a key to keep and a verifier to embed.

## Consequences

- If GitHub moves to another certificate authority, checks fail with "Secure connection to GitHub failed" until a version with the new root is installed by hand.
- Releases before v3.7.0 have no `SHA256SUMS`: an app built with an older `VERSION` that finds such a release refuses it ("incomplete on GitHub").
- GitHub allows 60 unauthenticated requests per hour per Internet address; one check per day stays far below.
