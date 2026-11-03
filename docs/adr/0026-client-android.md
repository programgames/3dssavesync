# Android client: same repository, Kotlin port with shared fixtures, SAF only, minSdk 24

An Android phone runs DS, GB, GBC, GBA and 3DS emulators. It becomes a third kind of Client, next to the 3DS app and the PC program.

## Decision

- **One repository, several clients.** The app lives in `android/`, next to `3ds/`, `client/` and `server/`. One pull request can change the protocol, the server and every client together, and the CI checks them together.
- **The sync logic is ported to Kotlin**, not shared as a library. The rules are small (`etat`, SRAM rules, Keys), and Go on Android (gomobile) would add a native build, a JNI layer and a larger APK for a few hundred lines. To keep the three ports equal, the Kotlin JVM tests read the same files as the others, in place: `tests/cas_etat.csv` (3-way State) and `tests/saves.json` (server list). They also run the Sync against the real `server/server.py` on 127.0.0.1, like `client/launcher_test.go`.
- **Same protocol as the PC**: `Authorization: Bearer`, `GET /saves`, conditional `PUT` (`If-Match` / `If-None-Match: *`, a `412` starts a new pass), `X-Client: android`, `PUT /saves/<key>/name` (ADR 0018). The address keeps its own scheme: plain HTTP for a home server, HTTPS the same way.
- **Storage Access Framework only.** The user picks each ROM folder and save folder with the system folder picker (`ACTION_OPEN_DOCUMENT_TREE`, persisted read and write grants). No "All files access" permission: SAF gives the same reach for the supported emulators, and Android/data stays closed to both (see the [Android guide](../guide-android.md#emulators)). The app never writes outside a chosen folder and never deletes a file. Backup copies go to the app's own folder (`getExternalFilesDir`), visible over USB, named like the PC (`<emulator>/<name>-<date>.<ext>`). Bases stay in app-private storage, one JSON file per emulator entry.
- **Replacing a save without an atomic rename.** SAF has no rename that replaces a file, and a provider may not even honour truncation (`"w"` can leave old trailing bytes). So the app: (1) writes a Backup copy in its own folder, reads it back and compares it; (2) opens the save with `"wt"` (truncate), writes the new content and syncs it to the disk; (3) reads it back and compares. On a mismatch it writes the old content back and checks it again; if that fails too, it stops and names the Backup copy. A verified copy outside the folder exists before the file is touched, so no failure can lose the old save.
- **minSdk 24 (Android 7.0), target 35.** 24 is the floor of the next Compose and WorkManager versions and of the network security config (cleartext opt-in), and every supported emulator needs Android 7 or later anyway. Target 35 keeps LAN access without the Android 17 `ACCESS_LOCAL_NETWORK` permission. The AndroidX versions are the last ones that build with compileSdk 35.

## Rejected options

- **A separate repository**: the fixtures, the protocol and the server would drift; a protocol change would need two pull requests in step.
- **gomobile (share `client/` Go code)**: native toolchain in the build, JNI glue, and the PC code reads files by path, not SAF documents.
- **`MANAGE_EXTERNAL_STORAGE`**: direct file paths, but it does not open other apps' `Android/data` folders either, and Play allows it only for a few app types.
- **Write to a temporary document, then rename**: `DocumentsContract.renameDocument` cannot replace an existing file, and some providers add " (1)" to the new name.

## Consequences

- A new rule in `etat` or in the SRAM rules is written three times (C, Go, Kotlin); the shared fixtures catch a port that differs.
- Aliases for ROM hacks (ADR 0014) are not ported yet: two ROMs with one Key are a Duplicate, never synced.
- The app syncs only when asked (no background sync) and does not start games; both can come later.
- Emulators that keep their saves in `Android/data` (Lemuroid, SkyEmu) cannot be synced.
