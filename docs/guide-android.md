# Android guide

The Android app syncs the saves of the emulators on your phone or tablet with your 3dssavesync server, like the 3DS app and the PC program. It is a first version: test it with a copy of one save before you trust it with your real games.

- **Systems**: DS, Game Boy, Game Boy Color, GBA, and the 8 Pokémon games of the 3DS (in Azahar).
- **Android 7.0 or later.**
- **Your server must already run** (local server of the PC program, or your Docker server: see the [server guide](guide-server.md)). The app needs its address and its token.

## Install

1. Get `3dssavesync-android.apk` from the assets of a release on GitHub. Check it against `SHA256SUMS` if you want. (Development builds: the `android-debug-apk` artifact of the CI, or build it, see below.)
2. Open it on the phone. Android asks to allow installs from this source: allow it for your file manager or browser.
3. Start **3dssavesync**. **Settings** shows its version at the bottom.

## Update

**Never uninstall the app to update it: uninstalling deletes the backup copies**, the record of the last sync of each game and the settings. Install the new `3dssavesync-android.apk` over the old one: open it, then tap **Update**. Android keeps the app's data.

Every release APK is signed with the same key, so each one installs over the previous one. A development build (the `android-debug-apk` artifact, or one you built) has another key: Android refuses to install a release over it. Only then do you have to uninstall once. Back up the copies first (see below).

To keep one copy, use **Save elsewhere…** in the history of the game (see [Backup copies](#backup-copies)). To keep all the backup copies on a computer, at any time:

1. Connect the phone to the computer over USB and choose **File transfer** on the phone.
2. Copy the folder `Android/data/io.github.programgames.savesync/files/copies/` to the computer. The Android Files app may hide `Android/data`: use the computer.

With `adb`: `adb pull /sdcard/Android/data/io.github.programgames.savesync/files/copies/`. To put them back after a new install, copy the folder to the same place, or restore one copy by hand (see [Backup copies](#backup-copies)).

## First start

The first start opens a guided setup: a welcome screen, then three steps. Each step has **Skip** or **Later**; **Settings** > **Start the guided setup** opens it again.

1. **Connect to your server**: on the PC, open the settings page, **Server**, and click **Show the QR code**. On the phone, tap **Scan the PC's QR code** and point the phone at it. The app fills in the address and token, saves them and tests the connection. The scanner comes from Google Play services: no camera permission, nothing leaves the phone.
   No QR code (no Google Play services, or no PC settings page at hand)? Tap **Type the address** and type the server address with its port, for example `192.168.1.20:8080` (the app adds `http://`; type `https://` yourself if your server uses it), and the token. Both are on the PC settings page (**Server** step), or in your Docker `.env`. Then tap **Test the connection**.
2. **Choose your emulator**: the emulators installed on the phone come first, under **Installed on this phone**.
3. **Choose its folders**: the same screen as in **Emulators** (see below).

Then **Show my games** opens the list. To add another emulator later: **Emulators** > **Add an emulator**.

Plain `http://` is allowed for a server on your home network. An `https://` address works the same way. The app gives up on a connection after 5 seconds, so a home server that can't be reached (away from home) fails fast.

**HTTPS with your own certificate.** A home server with a self-signed certificate, or one signed by your own certificate authority, works once its certificate is installed on the phone: copy the certificate file (`.crt` or `.pem`) to the phone, then in the Android settings open **Security** (or **Security and privacy**) > **Encryption and credentials** > **Install a certificate** > **CA certificate**. The menu names change a little between makers. Without it, the app says that the phone doesn't trust the server's certificate. On your home network, the `http://` address of the server works without a certificate.

## Emulators

The app can only read the folders that you choose with the Android folder picker. It writes only the save files of these folders. It never deletes a file, except the files it has just created itself when the first save of a 3DS game fails (see [3DS games never started in Azahar](#3ds-games-never-started-in-azahar)).

| Emulator | Systems | What to choose |
|---|---|---|
| **RetroArch** | DS, GB/GBC, GBA | For each system, the core you use (the save format depends on it) and the ROM folder. Then, once, the RetroArch saves folder (`RetroArch/saves`), unless RetroArch writes the saves next to the ROMs. Play Store version: its saves are in its own storage; in the folder picker, open the side menu and choose **RetroArch**. |
| **melonDS** | DS | The ROM folder. melonDS writes the save next to the ROM by default; if you chose a save folder in melonDS, untick **Saves next to the ROMs** and choose the same folder. |
| **DraStic** | DS | The ROM folder, then the save folder `DraStic/backup` (`.dsv` files). |
| **NooDS** | DS, GBA | The ROM folder: NooDS writes the save next to the ROM by default. Its "saves folder" setting puts saves where no other app can read them: keep the default. |
| **Pizza Boy** | GB/GBC, GBA | Pizza Boy keeps its saves in `Android/data/it.dbtecno.pizzaboy…`, which no other app can open since Android 11 (see below). If your version of Pizza Boy lets you choose its save folder, choose a folder outside `Android/data` there, then choose the same folder in the app, with the ROM folder. **Not checked yet** (save names `<ROM name>.sav` assumed): test with a copy of one save first. |
| **Azahar** | 3DS | The user folder you chose in Azahar at its first start (the one that contains `sdmc`). Only the 8 Pokémon games. No ROM folder. If the folder has no `Nintendo 3DS` folder, Azahar was never set up there: the list says so. |
| **Other emulator** | DS, GB/GBC, GBA | The save folder and the extension of the save files (`.sav`, `.dsv`…). For My Boy!, My OldBoy!, Linkboy and others. **Not checked**: test with a copy of one save first. |

An emulator that handles several systems has a box per system: untick the ones you don't play, so that the app asks for nothing more. The last system can't be unticked.

The folder picker opens where the emulator usually keeps its folders when that folder exists (`RetroArch/saves`, `DraStic/backup`, `roms/nds`, `roms`…), or at the folder you chose before (Android 8 and later). After each choice, one line says what the app found there:

- "4 of 5 games have a save here.": the folder is right.
- "No game found in this folder" or "No save found in this folder": check that it is the right one. The most frequent mistake is a parent or a sibling folder.
- "This folder is in Android/data": since Android 11, Android closes these private folders to other apps. Set the emulator to save in another folder, then choose that one.

A folder on an SD card shows **SD card** before its path. If the card is out, the folder shows as not found: insert the card.

**Files not read.** The app reads `.nds`, `.gb`, `.gbc`, `.gba` and `.zip` ROMs, like the PC. It does not open `.7z` or `.rar` archives, nor `.dsi`, `.srl` or `.sgb` files. When a ROM folder has some, one line says how many, after the folder choice and above the games list. Unzip the archive, or make it a `.zip`, to sync that game.

If Android takes back the access to a folder (an app cleaner, a card that was changed), the folder shows **Access lost**: choose it again. The app gives back the access to the folders that no emulator uses any more, so Android never drops a folder still in use (it keeps 128 to 512 of them).

RetroArch supports these cores, with the same formats as on the PC: melonDS DS and DeSmuME (DS); Gambatte, SameBoy, mGBA, VBA-M (GB); mGBA, gpSP, Beetle GBA, VBA-M (GBA). gpSP and Beetle GBA write a fixed-size file: a game must first be on the server, uploaded from another emulator, before they can sync it ([ADR 0011](adr/0011-srm-gpsp-beetle-taille-du-serveur.md)).

**RetroArch saves.** The app looks for the save of each game in three places: the folder of the chosen core in the saves folder (`RetroArch/saves/mGBA`, "Sort saves into folders by core", the default), the saves folder itself (that option off), and next to the ROM ("Write saves to content directory"). With "Sort saves into folders by content directory", it also looks in the folder named after the ROM's folder, inside the core folder or the saves folder (`RetroArch/saves/mGBA/gba` for a ROM in `roms/gba`). It never takes the save of another core. If two places have a save, it takes the newest one: the one RetroArch writes now. A game with no save yet gets its first one in the first of these folders that exists, else next to the ROM.

**Not supported:** Lemuroid and SkyEmu. They keep their saves in `Android/data`, and since Android 11 no other app can open that folder, with or without "All files access". The same applies to Pizza Boy and My Boy! unless they can save elsewhere.

The folder picker cannot choose the root of the internal storage or the `Download` folder itself: choose a folder inside it.

When you change a folder, a core, an extension, the systems or **Saves next to the ROMs** of an emulator, the app forgets the last sync of its games. The record of the old folders must not decide for the new ones: each game that the server also has then shows **In sync** or **Conflict**, never **To upload**. Open each Conflict and choose. A new name keeps the record. Removing an emulator also deletes its record (never its saves or the backup copies).

**Two entries of one emulator.** You can add the same emulator twice, for example melonDS for two ROM folders. The lists then show the first folder of each entry after its name, for example `melonDS (roms/nds)`, so the same game in both entries can be told apart. To show a shorter name, open the entry and change **Name in the lists**.

## Games

Each line shows the short name of the game, as on the 3DS ("Mario Kart DS", without "(Europe) (En,Fr,De,Es,It)"), then its state and its system. The full file name is in the game details. Pull the list down to read the folders and the server again (TalkBack: the **Refresh** action of the list). With TalkBack, each line is one stop that reads the name, the system, the emulator and the state. The states are the same words as on the 3DS:

| State | Meaning | Action |
|---|---|---|
| **To upload** (yellow) | The save on this phone is newer. | **Upload** sends it to the server. |
| **To download** (cyan) | The server has a newer version. | **Download** replaces the phone save, after a backup copy. |
| **Conflict** (red) | The save changed on the phone and on the server since the last sync, or it came back to the save a download replaced (see below). | Open the game and choose which one to keep. |
| **Interrupted** (red) | The last write of this save on the phone did not finish. | Don't play the game. Open it and choose the server save or the backup copy (see [Interrupted write](#interrupted-write)). |
| **In sync** (green) | Same save on both sides. | Nothing. |
| **Duplicate** (grey) | Two ROMs hold the same game, or would use the same save file. | Same game: keep one ROM. A ROM hack: give it its own save (see [ROM hacks](#rom-hacks)). Same save file: rename one ROM. |
| **Unknown** (grey) | The server or the save could not be read. | The message says why. |

Games with no save anywhere are hidden; a line at the end of the list counts them. Use the chips (**All**, **To do**, and one per system when your emulators have several) and the search field (accents ignored) to find a game.

When the server can't be read, one banner says why, with **Retry**. The games that have no save on this phone are hidden until the server answers: the app can't tell if the server has one. Every error shows the same way: what happened and what to do, **Retry** when it helps, and the technical detail (exception, address, HTTP code) behind **Show details**, to report a problem.

**Sync all** is greyed out with its reason next to it: **Nothing to sync** (no game to upload or download) or **Server unreachable**. It syncs every game that has a state. Before a save of the phone is replaced, it lists those games and asks first: tap **Replace and sync** to go on. It replaces only the saves of that list: a save that changed after the list was shown stays as it is, and the summary names it. Conflicts are never resolved by Sync all: the summary lists them, open each one to choose. The summary also lists each failure with what to do and its technical detail. Sync all stops at the first failure of the server, of the network or of a write on the phone: the next games are not tried.

**Close the emulator before you sync.** Android gives no way to see if an emulator is running. An open emulator keeps the save in memory and writes it back later: RetroArch does it every few seconds while a game runs. It would write the old progress back over a downloaded save. So the app protects the saves it writes (Download, **Use the server save**, the downloads of Sync all):

- Before the first of these actions, or the first Sync all, since the app started, it shows a reminder to close the emulator: swipe it away from the recent apps, then tap **The emulator is closed**. Tick **Don't show again** to skip it from then on. Sync all shows it once for the whole pass.
- It refuses to replace a save that changed less than 30 seconds ago: the emulator may still be open. The message says to close it, then sync again. Sync all goes on with the next games and lists the refused ones in its summary. Some folders give no date for their files: there, the app relies on the reminder only.
- It remembers the save that each download replaced. If that exact save comes back later, the emulator was probably still open and wrote it back. The game then shows **Conflict** with this reason, never **To upload**, and the background upload does not send it. Open the game and choose: **Use the server save** writes the downloaded save again, **Keep this phone's save** uploads the old one. The app stops this check for a game after its next upload.

Uploads are always allowed: reading a save while the emulator is open loses nothing.

## Conflicts

The game details show two cards, **This phone** and **Server**, with the date, the device that uploaded the server version and, for Pokémon games, the play time and the trainer name. Nothing is replaced until you choose:

- **Keep this phone's save**: uploads it. The server keeps its previous version in its History.
- **Use the server save**: replaces the phone save, after a backup copy.

If the server changed while you were choosing, nothing is done and the screen shows the new version.

## Interrupted write

Before it replaces a save, the app writes a small note in its private storage, and it deletes the note after it has read the new save back. If Android stops the app during the write (low memory, the app swiped away, a battery saver, the phone switched off), the note stays and the save file may be cut. At the next refresh, the game shows **Interrupted**. Don't play it before you choose:

- **Use the server save**: the usual download, with its confirmation and a backup copy of the cut file.
- **Restore the backup copy**: puts back the copy that the app made before the interrupted write. The app first makes a copy of the cut file too, then reads the result back.

The app never uploads a cut file and never offers to keep it. If the backup copy is no longer there, only the server save is offered. A note whose file was fully written (the app stopped just after the write) is deleted without a question.

## History and Restore

The server keeps the older versions of every save. To get one back, open the game, then tap **Show the history**. The screen shows the versions on the server first, then the backup copies on this phone (see [Backup copies](#backup-copies)).

1. The list shows the versions on the server, newest first: number, age, device (3DS, PC, Android, or Restore for a version put back), size. **Current** marks the version that the devices download.
2. Tap a version. For a Pokémon game, the app downloads this version only and shows its play time and trainer name.
3. Tap **Restore**. The confirmation shows this version and the save on this phone, with both play times for a Pokémon game.
4. Confirm. The version becomes the current version on the server again. Nothing is deleted: the previous current version stays in the history.
5. The app goes back to the game, which now shows **To download**. Tap **Download** to get the version on this phone: it is the usual Download, with its backup copy, its confirmation and the protection against an open emulator.

A Restore changes only the server. It never writes a file on this phone and never changes the last sync. Each other device also gets the version at its next sync, after a backup copy of its save. Sync your other devices first: a game not uploaded yet could replace the restored version or cause a conflict. This is the same Restore as on the 3DS and the PC ([ADR 0016](adr/0016-restauration-depuis-le-pc-sans-ecriture-locale.md)).

If the Restore fails, nothing changes on the server, and the message says why, with the technical detail under it:

- **The save just changed on the server**: a device uploaded it while you were choosing. The app reads the list again: choose again.
- **This version is no longer on the server**: choose another one.
- **The version arrived damaged** (its content does not match the hash listed by the server): try again.
- The server cannot be reached: the usual network messages.

## ROM hacks

A ROM hack often keeps the code of its original game: Pokémon Radical Red keeps the code of Pokémon FireRed. The app then sees one game in two ROMs and shows **Duplicate**: neither is synced. Give the hack its own save in one of two ways:

- Open the **Duplicate** line, then tap **Give this ROM its own save** under the hack ROM. The list is read again and shows the two games.
- In **Settings**, tap **Open the ROM hacks**, then **Choose the ROM**. Choose the hack ROM file. The console comes from the extension (choose it for a `.zip`). Tap **Add this ROM hack**.

The hack then syncs under its own key on the server. The app makes this key from the ROM file name, with the same rule as the PC program: for example `Radical Red (4.10).gba` always gives `GBA-0EE3`. So the same ROM file name gives the same save on the PC, the 3DS and this phone. To sync the hack with them:

1. On the PC, add the same ROM file in the settings page, **ROM hacks** (or choose **Sync separately** when the PC asks).
2. On the PC, prepare the 3DS SD card again: it writes the hack to `alias.ini` on the card.

Keep the same ROM file name on every device: another name gives another key. 3DS games have no ROM hacks: their key is the title ID.

To remove a ROM hack, tap **Remove** next to it in **ROM hacks**. The app only forgets that this ROM is a hack: no save is deleted, on the phone or on the server. The ROM then counts again as its original game. See [ADR 0014](adr/0014-rom-hack-et-alias.md).

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

Since Android 11, file managers on the phone cannot open `Android/data`. Use the app instead: open the game, tap **Show the history**, then look under **This phone**. It lists the copies of this game, newest first, with their date and size. Tap a copy:

- **Put back**: the copy replaces the save on this phone, after a backup copy of the current save, with the usual confirmation and the protection against an open emulator. The game then shows **To upload**: upload it to get this copy on your other devices too.
- **Save elsewhere…**: Android's file picker opens. Choose a folder you can open, for example **Downloads**. The app writes the copy there; the copy in the app folder stays.

A computer connected over USB can also see the folder. Older versions of every save are also on the server: see [History and Restore](#history-and-restore).

Removing an emulator from the app, or uninstalling the app, deletes nothing in your save folders. **Uninstalling the app deletes its backup copies**: to update, install the new APK over the old one (see [Update](#update)), and copy the backups to a computer if you want to keep them.

## Background upload

In **Settings**, turn on **Upload saves in the background** (it is off by default). Then, about every 15 minutes (the shortest interval Android allows), when the phone has a network and its battery is not low, the app reads your folders and uploads the saves you played on this phone: the games in the state **To upload**.

What it never does:

- It never downloads. A background download could write under an emulator that is still open, and Android gives no way to see if one runs. Open the app and use **Sync all** to get the saves of your other devices.
- It never resolves a conflict: it only reports it. A save that came back after a download and an interrupted write are conflicts too.
- It never writes a save or a backup copy on this phone. The only thing it changes on the phone is its record of the last sync, after the server has accepted an upload.
- It never uploads a save that changed in the last 30 seconds: the emulator may still be writing it. That save waits for the next run.

On Android 13 and later, the app asks for the permission to show notifications when you turn the setting on. A new problem then shows as one notification: a conflict, a refused token, a folder the app can't read. The same problems are not notified again while they stay. A server that can't be reached is never notified: away from home, a server on your home network can't answer, and that is normal. If you refuse the permission, the background upload still works: the problems show only in **Settings**, with **Turn on notifications**, which opens the notification settings of the app. **Settings** always shows the last run: its time, the number of saves uploaded and the problems.

Battery and data: each run reads the save folders and the server list, and uploads only the saves that changed. It opens again only the ROM files that are new or changed since the last run. Android may run it later than every 15 minutes, for example in battery saver mode or when the app is rarely used. Turn the setting off to stop it.

**Phones that stop background work.** Xiaomi (HyperOS, MIUI), Samsung, Huawei, OnePlus and other makers often stop the work of an app to save battery. While Android optimises the battery use of the app, **Settings** shows a short help with **Open the app settings**: there, tap **Battery** and choose **Unrestricted** (or **Don't optimise**). On Xiaomi, also turn on **Autostart** for the app. The names change between makers; [dontkillmyapp.com](https://dontkillmyapp.com) lists the steps for each one.

## Look

On Android 12 and later, the app takes the colours of your wallpaper; before, it uses the standard Material colours. It follows the dark theme of the phone. The state colours stay the same everywhere and keep a readable contrast in both themes. On a tablet or an open foldable, the screens keep a phone width, centred.

## Not in this version

- No background download: open the app and tap **Sync all** before you play, to get the saves of your other devices.
- The app does not start games.

## Build it yourself

With JDK 17 and the Android SDK (platform 36; Gradle adds the build-tools it needs). The unit tests also need a JDK 21 (Robolectric runs Android 16 on it only): Gradle finds an installed one, or downloads one.

```
cd android
./gradlew testDebugUnitTest assembleDebug
```

The APK is in `android/app/build/outputs/apk/debug/`. `./gradlew assembleRelease` builds a shrunk (R8) release APK; it is unsigned unless `ANDROID_KEYSTORE_FILE`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` and `ANDROID_KEY_PASSWORD` are set. `ANDROID_VERSION_CODE` and `ANDROID_VERSION_NAME` set its version (default 1 and `dev`). The CI signs the release APK of each tag with the project key. The unit tests read the shared files in `tests/` and run a sync against `server/server.py` (Python 3 needed). The UI tests run every screen on the computer with Robolectric, in English, then in French on a small screen (320 dp), at the normal and at the largest font size (200 %). The storage tests run the real folder code (scan, replace with its backup copy, 3DS folder creation) against a test document provider over a temporary folder: no emulator or phone needed (the first run downloads Robolectric and its Android library). Design choices: [ADR 0026](adr/0026-client-android.md).
