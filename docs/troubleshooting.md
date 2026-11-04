# Troubleshooting

Find your symptom or the message you see, then follow the fix. Messages are shown in English, and in French on a French system (the French text is given in italics).

Nothing in this list can lose a save: when 3dssavesync cannot finish, it leaves your save as it is.

- [Install](#install)
- [PC: settings page and games](#pc-settings-page-and-games)
- [PC: 3DS SD card, checks and updates](#pc-3ds-sd-card-checks-and-updates)
- [3DS](#3ds)
- [Server and network](#server-and-network)
- [Sync and conflicts](#sync-and-conflicts)
- [Report a bug](#report-a-bug)

## Install

### Windows protected your PC

The program is not signed, so Windows SmartScreen stops it the first time.

1. Click **More info**.
2. Click **Run anyway**.

Windows does not ask again for this file.

### Windows Defender Firewall asks about 3dssavesync

The local server waits for the 3DS on your home network. Windows asks the first time it starts.

1. Tick **Private networks**.
2. Click **Allow access**.

You clicked **Cancel**, or the 3DS still cannot connect:

1. Open **Windows Security** > **Firewall & network protection** > **Allow an app through firewall**.
2. Click **Change settings**, find **3dssavesync**, tick **Private**, then click **OK**.
3. Check that your Wi-Fi network is a private network: **Settings** > **Network & internet** > your network > **Private network**.

### macOS: the program cannot be opened

> "3dssavesync-darwin-arm64" cannot be opened because the developer cannot be verified.

The program is not signed. Run this once in a terminal, in the download folder:

```
xattr -d com.apple.quarantine 3dssavesync-darwin-arm64
```

### Linux: Permission denied

Make the file executable, then start the setup:

```
chmod +x 3dssavesync-linux-amd64
./3dssavesync-linux-amd64 setup
```

### The settings page does not open

> Open this address in your browser: … *(Ouvrez cette adresse dans votre navigateur : …)*

3dssavesync could not open your browser. Copy the address into your browser by hand.

No browser on this machine (SSH, Steam Deck in Game mode)? Run `3dssavesync setup --no-browser` to print the address, or `3dssavesync setup --text` to answer the same questions in the terminal.

### The icon near the clock does not show

> Can't show the icon near the clock: your desktop has no compatible notification area. … *(Impossible d'afficher l'icône près de l'horloge : votre bureau n'a pas de zone de notification compatible. …)*

- **Windows**: click the arrow `^` next to the clock. The icon can hide there. Drag it next to the clock.
- **Linux, GNOME** (Ubuntu, Fedora): GNOME has no system tray. Install the extension **AppIndicator and KStatusNotifierItem Support** (Ubuntu has it already), then log in again.
- **Linux, other desktops**: the desktop must show StatusNotifierItem icons (KDE Plasma, Steam Deck in Desktop mode, Xfce and Cinnamon with their tray plugin).
- **macOS**: there is no icon yet. Use the **3dssavesync settings** shortcut.

The local server keeps running without the icon. To stop the message, turn off **Show the icon near the clock** in the settings (part **Frontends and install**), then click **Save**.

### The program has stopped

> 3dssavesync has stopped. Start it again to continue. *(3dssavesync s'est arrêté. Relancez-le pour continuer.)*

The settings page is still open, but the program behind it has ended (after **Close settings**, or 2 minutes after you closed the tab). Open the **3dssavesync settings** shortcut again.

## PC: settings page and games

### 3dssavesync is not set up yet

> 3dssavesync isn't set up yet. Open "3dssavesync" to choose the server and the emulators.

Your frontend started a game before the setup was saved. Double-click 3dssavesync, finish the settings page, and click **Save**.

### My emulator is not in the list

In the **Emulators** part of the settings page, click **Add an emulator by hand**. Choose its profile and its executable. For an emulator that 3dssavesync does not know, choose **Other emulator (custom)**: see [Custom profile](guide-pc.md#custom-profile).

### Save folder not found

> can't find the DS save folder of melonds. Set it in the settings.

3dssavesync could not read the save folder from the emulator settings. Open **3dssavesync settings** and fill in the save folder of this emulator.

### No save after the game

> No save found after playing: … *(Aucune sauvegarde trouvée après la partie : …)*

The game had no save before and after you played. If you saved in the game, the save folder or the save extension of this emulator is wrong. Fix them in **3dssavesync settings**.

### The emulator does not start

> couldn't start mgba: …

The emulator was moved or uninstalled. Open **3dssavesync settings** and fix its executable.

### The frontend command and the settings do not match

> no emulator configured for GBA
> several emulators handle GBA (mgba, retroarch): pick one with --emulator
> emulator mgba doesn't support the DS system
> emulator not set up: …
> Unknown ROM extension: …

The frontend starts 3dssavesync with an emulator name or a ROM type that the settings do not have. Open **3dssavesync settings**, check the emulators, then click **Repair frontends**. For a frontend you set up by hand, check its command in the [frontend guide](guide-frontends.md).

### Games do not start after the program was deleted

The program was deleted without the uninstall, so LaunchBox or ES-DE still start the games through it. Download the program again, open it, then use **Uninstall 3dssavesync** (part **Frontends and install**): it gives LaunchBox and ES-DE their original emulators back. By hand: in LaunchBox, **Tools**, **Manage**, **Emulators**, edit your original emulator and tick **Default Emulator** for its platforms, then delete the "(3dssavesync)" emulators. An emulator with its own title that still starts 3dssavesync (set up by versions 1 and 2): edit it and set **Application Path** back to the emulator program. In ES-DE, delete the `<command label="3dssavesync (…)">` lines from `custom_systems/es_systems.xml`.

### Games no longer sync after a frontend update

EmuDeck and some updates rewrite the frontend files (ES-DE, LaunchBox). Close the frontend. Open **3dssavesync settings** and click **Repair frontends**. What is already in place does not change. Or run `3dssavesync repair`.

> LaunchBox is open, so nothing was changed. Close LaunchBox and BigBox, then run the repair again.

LaunchBox rewrites its files when it closes. Close LaunchBox and BigBox first. Same for ES-DE.

> mgba: emulator not found in LaunchBox. Add it in LaunchBox and run the repair again.

Add the emulator in LaunchBox first.

### EmuDeck: CloudSync seems active

> EmuDeck CloudSync seems to be on. Turn it off for DS, GB, GBA and 3DS, or both tools will upload the same saves.

Turn off CloudSync for these systems in EmuDeck.

### Game started without sync

> Game started without syncing: 3dssavesync doesn't recognize this game (homebrew or unsupported 3DS game). Its save stays on this PC.

A homebrew has no game code. A 3DS game is synced only when it is in the list of 3DS games: tick it on the 3DS (**SELECT** > **3DS games**) and add it to `titres_3ds` in `config.json` ([PC guide](guide-pc.md#configuration)). Only games with a single save file can be synced.

> Game started without syncing: 3dssavesync can't read this ROM format (.7z). Use a .zip or uncompressed ROM.

3dssavesync reads `.zip` and plain ROMs only. Extract the ROM, or compress it as `.zip`.

### A ROM hack is not synced

> Pokemon Radical Red.gba looks like Pokemon Fire Red.gba, which is already synced. It's probably a ROM hack.
> these ROMs contain the same game: … Keep only one, or add the others in the settings, ROM hacks section.
> 3DS: 2 ROMs hold this game: keep only one, or add the others in the PC settings (ROM hacks), then prepare the SD card.

A ROM hack often keeps the game code of the original game (Radical Red keeps the code of Fire Red). 3dssavesync refuses to mix the two games. To give the hack its own save:

1. Start the hack from your frontend (LaunchBox…). In the question, choose **Sync separately**. Or open **3dssavesync settings**, section **ROM hacks**, choose the ROM file, then click **Add**.
2. If you also play the hack on the 3DS: in **3dssavesync settings**, click **Prepare the 3DS SD card** again. Keep the same ROM file name on the PC and on the SD card.

Native 3DS games cannot have a separate save.

> The alias "…" of … isn't valid (example for a GBA game: GBA-0RAD). Remove it in the settings, ROM hacks section, and add the ROM again.

`config.json` was edited by hand with a wrong key. Follow the message. Until then, this ROM is not synced.

### The emulator is already running

> mgba is already open. Close it before starting a game.
> mgba was opened without 3dssavesync. Close it before starting a game.

Close the emulator, then start the game again from your frontend. Two different games at the same time are fine when both are started by 3dssavesync.

> The game is starting, but 3dssavesync can't write to …. Don't start this game again while it's open.

3dssavesync cannot write its lock file in its settings folder. Check the rights of that folder.

### The game starts with an old save

> A save just arrived from the server. …

The emulator reloads a save state when the game starts. The state is older than the save that just came from the server. If you save in the game, you replace your progress with the older one. Turn the setting off:

| Emulator | Setting |
|---|---|
| mGBA | `autoload=0` in its `config.ini` |
| RetroArch | `savestate_auto_load = "false"` in `retroarch.cfg` |
| VBA-M | `AutoLoadLastState=0` in `vbam.ini` |
| mGBA on the 3DS (forwarders) | `autoload=0` and `autosave=0` in `[ports.3ds]` of `sdmc:/mGBA/config.ini` |

### Some games of an emulator are not synced

> GBA games not synced: mgba has no GBA ROM folder. Add it in "3dssavesync settings".

The **3dssavesync &lt;emulator&gt;** shortcut syncs the ROMs of the emulator ROM folders. Add the ROM folder of this system in **3dssavesync settings**.

### A file is in use, the disk is full, or access is denied

> another program is using this file. Close the emulator and start the game again
> disk full. Free up some space and start the game again
> access denied to …. Check this folder's permissions

Close the emulator (or the antivirus or cloud sync program that holds the file), free disk space, or fix the folder rights. Nothing was replaced.

> …: the written file doesn't match

3dssavesync wrote a file and read back different bytes. Your disk or USB drive may be failing. A backup copy exists in `copies/` (see [Your data](../README.md#your-data)).

### gpSP and Beetle GBA (RetroArch)

> …: sync this game from mGBA or the 3DS first, so the server knows its save size

These cores always write a file of fixed size, so 3dssavesync takes the size from the server. Sync the game once from mGBA or the 3DS.

> …: sync stopped, the .srm has data beyond the … bytes of the server version

The `.srm` does not match the save on the server. Check the core and its save settings ([ADR 0011](adr/0011-srm-gpsp-beetle-taille-du-serveur.md)).

### DeSmuME footer missing or unknown

> DeSmuME footer missing or unknown

The `.dsv` file has a layout that 3dssavesync does not know. Use melonDS for this game, and [report it](#report-a-bug).

### Game never started in Azahar

> this game has never been started in Azahar. Start it once in Azahar, save in the game, then start it here again

Do what the message says. The 8 Pokémon games do not need this step.

## PC: 3DS SD card, checks and updates

Each failure gives what to do, then a **Detail:** line (path, system error, HTTP code): copy it if you [report a bug](#report-a-bug).

### The SD card is locked

> The SD card is write-protected. Take it out, slide the small lock switch on its side (or on the microSD adapter) up, put it back and try again. *(La carte SD est verrouillée en écriture. …)*

The lock switch is on the left side of a full-size SD card or adapter. Slide it toward the contacts (up), then click **Prepare the 3DS SD card** again.

### Not enough space on the SD card for 3dssavesync.cia

> Not enough space on the SD card (0 MB free, 2 MB needed). Free up some space and try again. *(Pas assez de place sur la carte SD (0 Mo libres, 2 Mo nécessaires). …)*

`config.ini` is already written. Delete files you do not need from the card (old `.cia` files in `cias`, the `*.avant-3dssavesync-*` copies; never the `Nintendo 3DS` folder or the saves), then try again.

### FBI was not found on the card

> FBI isn't on the card. If it's already installed on the HOME Menu, ignore this message. … *(FBI n'est pas sur la carte. …)*

Only a note: the card is prepared. FBI is the 3DS app that installs `.cia` files. If it is on your HOME menu, ignore the note. Otherwise, download `FBI.3dsx` from [FBI releases](https://github.com/nh-server/FBI-NH/releases/latest), copy it to the `3ds` folder of the card, and open it from the Homebrew Launcher.

### The 3DS already has a newer version

> The 3DS already has a newer 3dssavesync (3.8.0). There's no need to install version 3.7.0. *(La 3DS a déjà une version plus récente de 3dssavesync …)*

Only a note: `config.ini` is written. The 3DS app updated itself; the PC program is older. Update the PC program when the settings page offers it.

### This program does not include 3dssavesync.cia

> This program doesn't include 3dssavesync.cia (version dev). Download it from … *(Ce programme ne contient pas 3dssavesync.cia …)*

You built the PC program yourself: only release builds carry the 3DS app. Download `3dssavesync.cia` from the page given in the message, and copy it to the `cias` folder of the card.

### Checks: something to do

The **Checks** list (settings page) says what to do on each line. The usual ones:

- **Server unreachable**: see [Server and network](#server-and-network); the line gives the same message.
- **Save folder not found**: start a game once in the emulator (it creates the folder), or set the right folder in **Emulators** with the **Configure** button.
- **LaunchBox: … isn't the default emulator yet** or **ES-DE isn't set up yet**: close LaunchBox (or ES-DE), then click **Repair frontends**. In LaunchBox, someone chose the original emulator as default again; the repair sets the "(3dssavesync)" one back.
- **The 3DS SD card hasn't been prepared from this PC yet**: prepare it once. The date is noted only after a success; a card prepared by hand or by FTP is not seen.
- **The icon near the clock isn't running**: click **Save** to start it. Or turn it off in **Frontends and install**.

### PC update messages

After any of these failures, the old program stays in place and works as before.

> GitHub isn't responding. Check your Internet connection and try again. *(GitHub ne répond pas. Vérifiez votre connexion Internet et réessayez.)*

Check the Internet connection, then try again.

> GitHub is limiting requests from your connection for now. Try again in an hour. *(GitHub limite pour l'instant les demandes venant de votre connexion. …)*

GitHub accepts 60 checks per hour from one Internet address (the **Detail** line shows HTTP 403 or 429). Wait one hour, then try again.

> A file is missing from this version on GitHub, so the update wasn't installed. … *(Il manque un fichier dans cette version sur GitHub …)*
> GitHub sent a response that couldn't be read. … *(GitHub a envoyé une réponse illisible. …)*
> The file from GitHub is bigger than expected, so the update wasn't installed. … *(Le fichier de GitHub est plus gros que prévu …)*
> Couldn't make a secure connection to GitHub. Check the PC's date and time and try again. *(Impossible d'établir une connexion sécurisée avec GitHub. …)*

Try again later. If the message stays, download the program by hand from the [releases page](https://github.com/programgames/3dssavesync/releases) and replace the old one.

> An update is already running in another tab or window. Wait for it to finish, then reload this page. *(Une mise à jour est déjà en cours …)*
> Version 3.7.0 is already installed from another window. … *(La version 3.7.0 est déjà installée, depuis une autre fenêtre. …)*

Another settings page did the update. Close this tab, then open the "3dssavesync settings" shortcut.

> Couldn't copy the program: the installed version is still running. Quit the icon near the clock, close the other 3dssavesync windows and save again. *(Impossible de copier le programme : la version installée est encore ouverte. …)*

The settings page stops the icon near the clock before it copies the program, but another 3dssavesync window still uses the installed file. Right-click the icon near the clock, **Quit**, close the other settings pages, then click **Save** again.

> Version 3.7.0 on GitHub is incomplete (3dssavesync-windows-amd64.exe or SHA256SUMS is missing). … *(La version 3.7.0 sur GitHub est incomplète …)*
> The downloaded file is damaged, so the update wasn't installed. … *(Le fichier téléchargé est abîmé …)*

The program installs nothing that it cannot check. Try again later; if the message stays, download the program by hand from the [releases page](https://github.com/programgames/3dssavesync/releases) and replace the old one.

> Not enough disk space for the update (… MB free, … MB needed). … *(Pas assez de place sur le disque pour la mise à jour …)*

Free some space on the disk of the program (the **Detail** line gives the folder), then try again.

> The update couldn't replace the program, so the old one is still in place. Close the other 3dssavesync windows and try again. *(La mise à jour n'a pas pu remplacer le programme …)*

Another program holds the file (an antivirus scan, a second settings page). Close the other 3dssavesync windows, wait a minute, then click **Update** again.

> The update failed and the old program couldn't be put back. To get it back, rename … to …. *(La mise à jour a échoué et l'ancien programme n'a pas pu être remis en place. …)*

Rare. Open the folder of the program, and rename the file as the message says (for example `3dssavesync-3.6.1.exe` to `3dssavesync.exe`). Until then, LaunchBox, ES-DE and the shortcuts cannot start it.

> Updated to 3.7.0. The settings page couldn't reopen. Open the "3dssavesync settings" shortcut. *(Mise à jour vers 3.7.0 terminée. Les réglages n'ont pas pu se rouvrir …)*
> The icon near the clock couldn't restart. It'll come back the next time you sign in. *(L'icône près de l'horloge n'a pas pu redémarrer. …)*

The update is done. Open the shortcut, or log in again for the icon.

## 3DS

On the 3DS, a message shows in a band above the buttons. A red line on its left means a failure, green a success. The grey line under a network failure gives the code and the address that was called, for example `curl 7 · http://192.168.1.20:8765/saves` or `HTTP 401 · …`: copy it if you [report a bug](#report-a-bug).

### Server unreachable

> Server unreachable: check the address. *(Serveur injoignable : vérifiez l'adresse.)*

The 3DS cannot reach the server (`curl 6` or `curl 7`). With the local server, check each point:

1. The PC is on, and the local server runs. It starts with your session. The menu of the [icon near the clock](guide-pc.md#the-icon-near-the-clock) shows its state. To start it by hand, run `3dssavesync serve`.
2. The 3DS and the PC are on the same home network. A guest Wi-Fi or a router with "AP isolation" or "client isolation" blocks the 3DS.
3. Windows allows 3dssavesync on private networks: see [Windows Defender Firewall](#windows-defender-firewall-asks-about-3dssavesync).
4. The PC address did not change. Open **3dssavesync settings** on the PC: compare the address it shows with the address in **SELECT** > **Server** on the 3DS. If they differ, prepare the SD card again. A DHCP reservation in your router keeps the PC address fixed.
5. A VPN runs on the PC? The first address can be wrong. Try one of the **Other addresses** of the settings page.

With your own server, check the address in **SELECT** > **Server**.

### No network

> No network: turn on the 3DS Wi-Fi. *(Pas de réseau : activez le Wi-Fi de la 3DS.)*

Turn on the Wi-Fi switch of the 3DS, and check its connection in the System Settings.

### Token refused

> Token refused: copy the exact server token (SELECT). *(Jeton refusé : copiez le jeton exact du serveur (SELECT).)*

Copy the token from the PC settings page (box "To enter on the 3DS"), or prepare the SD card again. Check that it has no space.

### This address isn't a 3dssavesync server

> This address isn't a 3dssavesync server. Check it. *(Cette adresse n'est pas un serveur 3dssavesync. Vérifiez-la.)*

The address leads to another program or site (`HTTP 404`). Check the port (`:8765` by default for the local server) and that there is no extra path after it.

### Check the server address

> Check the server address (SELECT). *(Vérifiez l'adresse du serveur (SELECT).)*

The address is not a valid one (`curl 3`). Type the full address, with `http://` for the local server (for example `http://192.168.1.20:8765`) or `https://` for your own server.

### Certificate refused or secure connection failed

> Server certificate refused: check the 3DS date and time.
> Secure connection failed: check the address, and the 3DS date and time.

- Set the date and time of the 3DS (System Settings).
- The local server uses `http://`, not `https://`.
- Your own server needs a **Let's Encrypt** certificate and TLS 1.2 with ECDHE: see the [server guide](guide-server.md#https-and-the-3ds).

### The server isn't responding, connection cut, network error

> Server not responding. Try again later. *(Le serveur ne répond pas : réessayez plus tard.)*
> Connection cut: try again.
> Network error: try again.

Move closer to the router and press **Y** to try again. If it lasts, check the server.

### Out of memory, or the network doesn't start

> Out of memory: restart 3dssavesync.
> Couldn't start the network. Restart the 3DS and 3dssavesync. *(Impossible de démarrer le réseau. Redémarrez la 3DS et relancez 3dssavesync.)*
> Can't read the installed games. Restart the 3DS and try again. *(Impossible de lire la liste des jeux. Redémarrez la 3DS et réessayez.)*

Do what the message says.

### Server not read

> Cannot read the server. … — games show **Unknown**, and the bottom line of the top screen shows "not read" in red.

The list shows your games, but their state is unknown. **A** and **START** do nothing until the server is read. Fix the reason given in the message, then press **Y**.

### First start: enter the server and the token

> Welcome! Enter the server and the token first. *(Bienvenue ! Entrez d'abord le serveur et le jeton.)*

The 3DS SD card has no `config.ini`. Prepare the SD card from the PC settings page, or enter **Server** and **Token** on this screen (lines marked **Required**). **B** (**Test and continue**) tests the connection first; after a failure, **B** (**Continue anyway**) opens the list without a working server.

### A game is missing from the list

- Check the ROM and save folders in **SELECT** (TWiLight Menu++ defaults: `sdmc:/roms/nds` and `sdmc:/roms/nds/saves`).
- ROMs in subfolders are found up to 4 levels deep. Folders named `saves`, `3ds`, `luma`, `mGBA`, `Nintendo 3DS`, or starting with `.` or `_` are skipped.
- A game with a ROM but no save, and no save on the server, has nothing to sync yet. Play it and save once.
- Native 3DS games: open **SELECT** > **3DS games** and tick the game. Only games with a single save file can be ticked.

> 88 ROMs skipped: more than 512 games.

3dssavesync reads 512 games at most. Point the ROM folders to fewer games.

### Several ROMs hold this game

> 2 ROMs hold this game. Keep one, or add the others to ROM hacks on the PC, then prepare the SD card. *(2 ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC, puis préparez la carte SD.)*

Two ROMs have the same game key (two copies, or a ROM hack), or give the same save file. 3dssavesync does not guess which one is right. Delete or move the extra ROM, or [give the hack its own save](#a-rom-hack-is-not-synced).

### SD card errors

> …: can't read the save. Check the SD card.
> …: backup copy failed, nothing written. Check the SD card.
> Settings not saved. Check the SD card.

The SD card is full, locked (lock switch), or damaged. Nothing was written. Free space on the card or check it on a PC.

> …: rename failed. Downloaded save: ….tmp. Old one copied to /3ds/3dssavesync/copies.

The new save is in the `.tmp` file next to the save. Put the SD card in a PC and rename the `.tmp` file to the save name.

> …: 3DS write refused, save intact.

Start the game once, then try again.

### A 3DS game cannot be ticked

> This game can't be synced (several save files). *(Ce jeu ne peut pas être synchronisé (plusieurs fichiers de sauvegarde).)*
> No save yet: start the game once and save.

Only native 3DS games with a single save file are supported.

### open_agb_firm: the GBA save does not change

open_agb_firm writes the `.sav` file only when you turn off the 3DS with POWER. Turn it off with POWER after you play, then sync at the next start.

### Update messages

The **Version** line of the settings (**SELECT**) shows the installed version and its state: **Up to date**, **3.7.1 available**, **Not checked** or **Check failed**. On that line, the bottom screen shows the result of the last check or install. The grey line under a failure gives a code (`curl 6`, `HTTP 403`, `AM 0x…`): copy it if you [report a bug](#report-a-bug). After any of these failures, the old version of the app stays, and your saves do not change.

> No network: turn on the 3DS Wi-Fi, then try again. *(Pas de réseau : activez le Wi-Fi de la 3DS, puis réessayez.)*

Turn on the Wi-Fi, then press **A** on **Version** again.

> GitHub unreachable: check the Internet access, then try again. *(GitHub injoignable : vérifiez l'accès à Internet, puis réessayez.)*

The 3DS reaches your home network, but not the Internet. Check the Internet access of the router. A local server works without Internet; only updates need it.

> GitHub limits the checks: try again in an hour. *(GitHub limite les vérifications : réessayez dans une heure.)*

GitHub accepts 60 checks per hour from one Internet address, for all the devices of your home. Wait one hour, then try again.

> Secure connection to GitHub failed: check the 3DS date and time. *(Connexion sécurisée à GitHub impossible : vérifiez la date et l'heure de la 3DS.)*

Set the date and time of the 3DS (System Settings). If the message stays, GitHub changed its certificates: install the new version by hand (see the end of this section).

> Connection to GitHub cut: move closer to the access point, then try again. *(Connexion à GitHub coupée : rapprochez-vous du point d'accès, puis réessayez.)*
> Download failed. … *(Le téléchargement a échoué. …)*
> Couldn't download SHA256SUMS. … *(Impossible de télécharger SHA256SUMS. …)*

The download stopped. Move closer to the router, then press **A** on **Version** again.

> The downloaded file is damaged. Try again. *(Le fichier téléchargé est abîmé. Réessayez.)*

The downloaded file is not the file of the release (its SHA-256 differs from `SHA256SUMS`). Try again. If the message stays, install the new version by hand.

> Not enough space on the SD card. Free 2 MB and try again. *(Pas assez de place sur la carte SD. Libérez 2 Mo et réessayez.)*

Delete files you do not need from the SD card (not the saves), then try again.

> The 3DS refused the install, the old version stays. Restart the 3DS and try again. *(La 3DS a refusé l'installation, l'ancienne version reste. Redémarrez la 3DS et réessayez.)*

The 3DS refused the `.cia`. Restart the 3DS, then try again. If the message stays, install the new version by hand with FBI.

> Version 3.7.0 is incomplete on GitHub. Install it by hand (see the 3DS guide). *(Version 3.7.0 incomplète sur GitHub. Installez-la à la main (voir le guide 3DS).)*
> SHA256SUMS has no 3dssavesync.cia. … *(SHA256SUMS ne contient pas 3dssavesync.cia. …)*

The release has no `SHA256SUMS` file, or no file for your 3DS. The app installs nothing that it cannot check. Install the new version by hand.

> Can't write to the SD card, the old version stays. Check the card and try again. *(Impossible d'écrire sur la carte SD, l'ancienne version reste. Vérifiez la carte et réessayez.)*

Only with the `.3dsx`. The SD card is full, locked (lock switch), or damaged.

> No version in place. On a PC, rename …/3dssavesync.3dsx.bak to 3dssavesync.3dsx to get the old one back. *(Aucune version en place. Sur un PC, renommez … pour retrouver l'ancienne.)*

Only with the `.3dsx`. The old version is complete in the `.bak` file. Put the SD card in a PC, and rename the file as the message says.

> Update not possible here: start 3dssavesync from the Homebrew Launcher, or install the .cia. *(Mise à jour impossible ici : lancez 3dssavesync depuis le Homebrew Launcher, ou installez le .cia.)*

The app was started with 3dslink (or without its file path), so it does not know which `.3dsx` file to replace. Start it from the Homebrew Launcher, or install the `.cia`.

> Development build: no update offered. *(Version de développement : aucune mise à jour proposée.)*

You built the app yourself from the source code. Install a release to get updates.

**Install a version by hand**: download `3dssavesync.cia` from the [releases page](https://github.com/programgames/3dssavesync/releases), copy it to the SD card, and install it with FBI. Your saves and settings stay.

## Server and network

The PC shows these messages in the settings page (**Test the connection**, **Save**), and in the "Sync failed" window before a game.

### The server refuses this token

> The server rejected this token. Copy the exact token from the server.

Copy the token again: from the local server box of the settings page, or from the `.env` file of your own server.

### This address does not lead to a 3dssavesync server (PC)

> This address isn't a 3dssavesync server. Check it.

Check the address: right subdomain, no extra path. A reverse proxy may send it to another site.

### The server ran into an error

> The server ran into an error. Try again later. If it keeps happening, check the server log.

Wait, then read the logs of the server container and of the reverse proxy.

### Unexpected answer from the server

> The server sent an unexpected response. Check its address.

Something answers that is not the 3dssavesync server: a redirect, a login page, or a block. Use the exact `https://` address, with nothing in front of the server.

### Invalid server certificate

> The server's certificate isn't valid. Check the address (https://) and the PC's date.

Check the date of the PC. Check the certificate of your server. Check `https://` against `http://`: the local server uses `http://`.

### Server not found

> Can't find the server. Check your Internet connection and the server address.

Check your Internet connection and the spelling of the address.

### The server refuses the connection

> The server refused the connection. It may be stopped, or the port is wrong. Start the server or check the address.

Start the server. For the local server, run `3dssavesync serve` or log in again. Check the port.

### The server does not answer (PC)

> The server isn't responding. Check your Internet connection and the server address.

Check your network and the address. The save stays on the PC.

### Save too large for the server

> save too large for the server: raise the size limit on the server or its proxy

Your reverse proxy limits the request size. Raise its limit (the server accepts up to 64 MiB).

### The server refuses this save

> the server rejected this save (empty, too large or cut off). Start the game again

The upload was cut. Start the game again. If it repeats, check the size of the save file.

### Enter the server address

> Enter the server address, with https:// (or http:// for the local server).

Type the full address, with its `https://` or `http://`.

### Your settings weren't saved

> … Your settings weren't saved.

You clicked **Save** while the server was unreachable. Nothing changed. Fix the address or the token, then click **Save** again.

### Port already used

> the local server can't start: another program is already using port 8765. Choose another port in the settings and enter the new address on the 3DS.

Choose another port in **3dssavesync settings**, click **Save**, then prepare the 3DS SD card again.

> no local server is set up. Choose "Local server on this PC" in the settings.

`3dssavesync serve` needs the local server. Choose it in the settings page.

> the local server isn't responding.

The local server could not start or is blocked. Check the [firewall](#windows-defender-firewall-asks-about-3dssavesync) and the port.

### My own server works on the PC, not on the 3DS

The 3DS knows only the Let's Encrypt root certificate, and needs TLS 1.2 with ECDHE. See the [server guide](guide-server.md#https-and-the-3ds).

## Sync and conflicts

### Sync failed before the game

> Sync failed … Play with the PC save? It'll be uploaded the next time you start a game. [Yes / No]

3dssavesync could not reach the server before the game. The reason is on the lines above.

- **Yes**: play with the save of the PC. It goes to the server at the next launch.
- **No**: do not play now. Fix the reason first.

### Upload failed after the game

> Upload failed. The save stays on the PC and will upload the next time you start a game.

The network was cut, or the server was off. The save waits on the PC. At the next launch of any game, 3dssavesync syncs it first, with the usual comparison. If the server changed meanwhile, you get the usual conflict choice. Nothing to do now.

### Conflict

You played on both sides since the last sync. 3dssavesync never chooses for you. Each version shows its play time and trainer (Pokémon games) or its date.

- **PC, before the game**: **Use server save**, **Keep PC save**, or **Don't play**. After the game: **Keep PC save** or **Do nothing**. Closing the window = the last button, the safest.
- **3DS**: **A** keeps the 3DS save (**3DS save** card), **X** uses the server save (**Server save** card), **B** does nothing. Both cards show the same lines: the date, the play time (Pokémon games), and what happens next.

Nothing is lost: the other version stays in the server history, and a backup copy is made before a save is replaced. **START** (Sync all) on the 3DS skips conflicts: solve them one by one with **A**.

> Conflict on …. The game starts with the PC save, nothing was replaced.

No window could open (no display, for example over SSH). Start the game again with a display to choose.

> …: conflict, the server changed meanwhile.

Another device uploaded while the 3DS was syncing. The list is read again: choose with **A**.

### The save received is damaged

> the save from the server arrived incomplete, so yours wasn't replaced. Start the game again
> 3DS: …: received version damaged, nothing written. Try again.

The download was incomplete. Nothing was written. Try again.

### The save keeps changing on the server

> another device is uploading this save at the same time. Start the game again in a moment

Wait for the other device to finish, then start the game again.

### A save is never uploaded

A save that holds only `0x00` or `0xFF` bytes is blank: no game is saved in it yet. 3dssavesync counts it as no save. Save in the game once.

### Gold, Silver or Crystal: the clock differs

Each machine keeps its own game clock. The game progress is the same.

### I want an old version back

The server keeps every version.

1. Open the **3dssavesync settings** shortcut.
2. In the **History** part, choose the game.
3. Click **Restore this version** on the version you want, then confirm.

Or in a terminal: `3dssavesync restore` lists the games, `3dssavesync restore <key>` lists the last 30 versions, `3dssavesync restore <key> <number>` restores one.

Nothing is written on the PC now. The restored version becomes the current version on the server, and the old current version stays in the history. Each device gets it at its next sync, after a backup copy. A device with a game not yet uploaded shows a conflict.

> This save just changed on the server (a device uploaded it meanwhile). Nothing was restored. Open the version list again and choose again.

Load the list again and choose again.

### Where are the backup copies?

- **PC**: `copies/` in the settings folder (`%APPDATA%\3dssavesync` on Windows, `~/.config/3dssavesync` on Linux, `~/Library/Application Support/3dssavesync` on macOS).
- **3DS**: `sdmc:/3ds/3dssavesync/copies/`.
- **Server**: `data/<game key>/`, every version.

## Report a bug

Open an [issue](https://github.com/programgames/3dssavesync/issues) with:

- the release you use (for example v3.5.0), your PC system, your emulator and frontend, local server or your own server;
- the exact message, or a photo of the 3DS screen;
- the log `3dssavesync.log`, next to `config.json` in the settings folder. Read it before you post it, and remove anything private.
