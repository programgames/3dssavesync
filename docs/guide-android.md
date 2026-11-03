# Android guide

The Android app syncs the saves of the emulators on your phone or tablet with your 3dssavesync server, like the 3DS app and the PC program. It is a first version: test it with a copy of one save before you trust it with your real games.

- **Systems**: DS, Game Boy, Game Boy Color, GBA, and the 8 Pokémon games of the 3DS (in Azahar).
- **Android 7.0 or later.**
- **Your server must already run** (local server of the PC program, or your Docker server: see the [server guide](guide-server.md)). The app needs its address and its token.

## Install

1. Get `3dssavesync-android-debug.apk` (for now: the `android-debug-apk` artifact of the CI, or build it, see below).
2. Open it on the phone. Android asks to allow installs from this source: allow it for your file manager or browser.
3. Start **3dssavesync**.

## First start

1. **Settings**: type the server address with its scheme and port, for example `http://<PC address>:8080`, and the token. Both are on the PC settings page (**Server** step), or in your Docker `.env`.
2. Tap **Test the connection**. "Connected. The server has N games." means it works. Otherwise the message says what to fix, with the technical detail under it.
3. **Emulators** > **Add an emulator**, choose yours, then choose its folders (see below).
4. **Games**: the list shows each game with its state. Tap **Sync all**, or a game for its details.

Plain `http://` is allowed for a server on your home network. An `https://` address works the same way.

## Emulators

The app can only read the folders that you choose with the Android folder picker. It writes only the save files of these folders. It never deletes a file, except the files it has just created itself when the first save of a 3DS game fails (see [3DS games never started in Azahar](#3ds-games-never-started-in-azahar)).

| Emulator | Systems | What to choose |
|---|---|---|
| **RetroArch** | DS, GB/GBC, GBA | The core you use for each system (the save format depends on it), the ROM folder, and the save folder of that core: `RetroArch/saves/<core name>` (for example `RetroArch/saves/mGBA`) when "Sort saves by core" is on, the default. Play Store version: its saves are in its own storage; in the folder picker, open the side menu, choose **RetroArch**, then `saves`. |
| **Azahar** | 3DS | The user folder you chose in Azahar at its first start (the one that contains `sdmc`). Only the 8 Pokémon games. No ROM folder. If the folder has no `Nintendo 3DS` folder, Azahar was never set up there: the list says so. |
| **melonDS** | DS | The ROM folder. melonDS writes the save next to the ROM by default; if you chose a save folder in melonDS, untick **Saves next to the ROMs** and choose the same folder. |
| **NooDS** | DS, GBA | The ROM folder: NooDS writes the save next to the ROM by default. Its "saves folder" setting puts saves where no other app can read them: keep the default. |
| **Other emulator** | DS, GB/GBC, GBA | The save folder and the extension of the save files (`.sav`, `.dsv`…). For DraStic, Pizza Boy, My Boy!, My OldBoy!, Linkboy. **Not checked yet**: their folder and format are not documented. Test with a copy of one save first. |

RetroArch supports these cores, with the same formats as on the PC: melonDS DS and DeSmuME (DS); Gambatte, SameBoy, mGBA, VBA-M (GB); mGBA, gpSP, Beetle GBA, VBA-M (GBA). gpSP and Beetle GBA write a fixed-size file: a game must first be on the server, uploaded from another emulator, before they can sync it ([ADR 0011](adr/0011-srm-gpsp-beetle-taille-du-serveur.md)).

**Not supported:** Lemuroid and SkyEmu. They keep their saves in `Android/data`, and since Android 11 no other app can open that folder, with or without "All files access". The same may apply to some versions of Pizza Boy and My Boy!: use their export feature to a normal folder, or a setting that moves the saves.

The folder picker cannot choose the root of the internal storage or the `Download` folder itself: choose a folder inside it.

## Games

Each line shows the state, the system and the game. The states are the same words as on the 3DS:

| State | Meaning | Action |
|---|---|---|
| **To upload** (yellow) | The save on this phone is newer. | **Upload** sends it to the server. |
| **To download** (cyan) | The server has a newer version. | **Download** replaces the phone save, after a backup copy. |
| **Conflict** (red) | The save changed on the phone and on the server since the last sync. | Open the game and choose which one to keep. |
| **In sync** (green) | Same save on both sides. | Nothing. |
| **Duplicate** (grey) | Two ROMs hold the same game, or would use the same save file. | Keep one ROM, or rename one. ROM hack aliases are not supported on Android yet. |
| **Unknown** (grey) | The server or the save could not be read. | The message says why. |

Games with no save anywhere are hidden; a line at the end of the list counts them. Use the chips (**All**, **To do**, **DS**, **GB**, **GBA**, **3DS**) and the search field (accents ignored) to find a game.

**Sync all** syncs every game that has a state. Before a save of the phone is replaced, it lists those games and asks first. It replaces only the saves of that list: a save that changed after the list was shown stays as it is, and the summary names it. Conflicts are never resolved by Sync all: the summary lists them, open each one to choose. The summary also lists each failure with what to do and its technical detail. Sync all stops at the first failure of the server, of the network or of a write on the phone: the next games are not tried.

**Close the emulator before you sync.** Android gives no way to see if an emulator is running. An open emulator keeps the save in memory and writes it back later: RetroArch does it every few seconds while a game runs. It would write the old progress back over a downloaded save. So the app protects the saves it writes (Download, **Use the server save**, the downloads of Sync all):

- Before the first of these actions, or the first Sync all, since the app started, it shows a reminder to close the emulator. Tick **Don't show again** to skip it from then on. Sync all shows it once for the whole pass.
- It refuses to replace a save that changed less than 30 seconds ago: the emulator may still be open. The message says to close it, then sync again. Sync all goes on with the next games and lists the refused ones in its summary. Some folders give no date for their files: there, the app relies on the reminder only.

Uploads are always allowed: reading a save while the emulator is open loses nothing.

## Conflicts

The game details show two cards, **This phone** and **Server**, with the date, the device that uploaded the server version and, for Pokémon games, the play time and the trainer name. Nothing is replaced until you choose:

- **Keep this phone's save**: uploads it. The server keeps its previous version in its History.
- **Use the server save**: replaces the phone save, after a backup copy.

If the server changed while you were choosing, nothing is done and the screen shows the new version.

## 3DS games never started in Azahar

The app knows the 8 Pokémon games of the 3DS even if you never started them in Azahar. Such a game shows in the list when the server has a save for it, as **To download**; its details say **Never started in Azahar**. **Download** (or **Sync all**) creates its save folder in Azahar, like the PC program does:

1. It creates the folder `sdmc/Nintendo 3DS/<id>/<id>/title/<high>/<low>/data/` if it is missing.
2. It writes `00000001.metadata` (the format of the save) and reads it back.
3. It creates the folder `00000001` and writes the save file `main` in it, then reads it back.

Then start the game in Azahar: it loads the save. The app never writes over an existing `00000001.metadata`: if one is there without the `00000001` folder, the app refuses. Start the game once in Azahar, save in the game, then refresh.

If a step fails, the app deletes the files and folders it has just created, and only these. A folder with `00000001.metadata` but no full `main` would be a broken save for Azahar; no folder is a game never started, which Azahar sets up normally. If the app cannot delete them either, the message names the folder: delete it with a file manager before you start the game.

## Backup copies

Before any save is replaced, the app writes a copy, reads it back and compares it. Then it reads the save again: if the emulator changed it since the sync read it, nothing is written. Then it writes the new save, reads it back and compares it; if the new save does not match, it puts the old one back. If the phone had no save for this game, a failed write leaves an empty file, which the app sees as no save. The copies are in:

`Android/data/io.github.programgames.savesync/files/copies/<emulator>/<game>-<date>.<ext>`

Connect the phone to a computer over USB to see them (the Android Files app may hide `Android/data`). To restore one, copy it into the emulator's save folder with the name of the save file (for example `Pokemon Emerald.srm`), then open the game in the app: it shows **To upload** or **Conflict**. Older versions of every save are also on the server: see [Restore an old version](guide-pc.md) on the PC.

Removing an emulator from the app, or uninstalling the app, deletes nothing in your save folders. Uninstalling the app deletes its backup copies: copy them first if you want to keep them.

## Not in this version

- No background sync: open the app and tap **Sync all** before and after you play.
- The app does not start games.
- No ROM hack aliases ([ADR 0014](adr/0014-rom-hack-et-alias.md)).

## Build it yourself

With JDK 17 and the Android SDK (platform 35, build-tools 35.0.0):

```
cd android
./gradlew testDebugUnitTest assembleDebug
```

The APK is in `android/app/build/outputs/apk/debug/`. The unit tests read the shared files in `tests/` and run a sync against `server/server.py` (Python 3 needed). The UI tests run every screen on the computer with Robolectric, in English and in French, on a small screen: no emulator or phone needed (the first run downloads Robolectric and its Android library). Design choices: [ADR 0026](adr/0026-client-android.md).
