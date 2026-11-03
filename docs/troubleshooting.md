# Troubleshooting

Find your symptom or the message you see, then follow the fix. Messages are shown in English, and in French on a French system (the French text is given in italics).

Nothing in this list can lose a save: when 3dssavesync cannot finish, it leaves your save as it is.

- [Install](#install)
- [PC: settings page and games](#pc-settings-page-and-games)
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

> Open this address in the browser: … *(Ouvrez cette adresse dans le navigateur : …)*

3dssavesync could not open your browser. Copy the address into your browser by hand.

No browser on this machine (SSH, Steam Deck in Game mode)? Run `3dssavesync setup --sans-navigateur` to print the address, or `3dssavesync setup --texte` to answer the same questions in the terminal.

### The program has stopped

> The 3dssavesync program has stopped. Start it again. *(Le programme 3dssavesync s'est arrêté. Relancez-le.)*

The settings page is still open, but the program behind it has ended (after **Finish**, or 2 minutes after you closed the tab). Open the **3dssavesync settings** shortcut again.

## PC: settings page and games

### 3dssavesync is not set up yet

> 3dssavesync is not set up yet. Open "3dssavesync" to choose the server and the emulators.

Your frontend started a game before the setup was saved. Double-click 3dssavesync, finish the settings page, and click **Save**.

### My emulator is not in the list

In the **Emulators** part of the settings page, click **Add an emulator by hand**. Choose its profile and its executable. For an emulator that 3dssavesync does not know, choose **Other emulator (custom)**: see [Custom profile](guide-pc.md#custom-profile).

### Save folder not found

> DS save folder of melonds not found: set it in "3dssavesync settings"

3dssavesync could not read the save folder from the emulator settings. Open **3dssavesync settings** and fill in the save folder of this emulator.

### No save after the game

> No save after the game: … *(Aucune sauvegarde après la partie : …)*

The game had no save before and after you played. If you saved in the game, the save folder or the save extension of this emulator is wrong. Fix them in **3dssavesync settings**.

### The emulator does not start

> cannot start mgba: …

The emulator was moved or uninstalled. Open **3dssavesync settings** and fix its executable.

### The frontend command and the settings do not match

> no emulator configured for GBA
> several emulators for GBA (mgba, retroarch): use --emulator
> emulator mgba does not handle the DS system
> emulator missing from the configuration: …
> unknown ROM (extension): …

The frontend starts 3dssavesync with an emulator name or a ROM type that the settings do not have. Open **3dssavesync settings**, check the emulators, then click **Repair frontends**. For a frontend you set up by hand, check its command in the [frontend guide](guide-frontends.md).

### Games no longer sync after a frontend update

EmuDeck and some updates rewrite the frontend files (ES-DE, LaunchBox). Close the frontend. Open **3dssavesync settings** and click **Repair frontends**. What is already in place does not change. Or run `3dssavesync repair`.

> LaunchBox: open, nothing done. Close LaunchBox and BigBox, then run the repair again.

LaunchBox rewrites its files when it closes. Close LaunchBox and BigBox first. Same for ES-DE.

> mgba: emulator missing from LaunchBox (add it in LaunchBox, then run the repair again)

Add the emulator in LaunchBox first.

### EmuDeck: CloudSync seems active

> EmuDeck: CloudSync seems active. Turn it off for DS, GB, GBA and 3DS: two tools would upload the same saves.

Turn off CloudSync for these systems in EmuDeck.

### Game started without sync

> Game started without sync: 3dssavesync does not recognize this game (homebrew, or 3DS game not supported). Its save stays on this PC.

A homebrew has no game code. A 3DS game is synced only when it is in the list of 3DS games: tick it on the 3DS (**SELECT** > **3DS games**) and add it to `titres_3ds` in `config.json` ([PC guide](guide-pc.md#configuration)). Only games with a single save file can be synced.

> Game started without sync: ROM format not read (.7z). Use a .zip or uncompressed ROM.

3dssavesync reads `.zip` and plain ROMs only. Extract the ROM, or compress it as `.zip`.

### A ROM hack is not synced

> Pokemon Radical Red.gba is seen as the same game as Pokemon Fire Red.gba, already synced (ROM hack?). To sync it separately, add an alias in config.json.
> these ROMs are seen as the same game: … Keep one, or add an alias in config.json.
> 3DS: 2 ROMs hold this game: keep only one, or give the others an alias (alias.ini).

A ROM hack often keeps the game code of the original game (Radical Red keeps the code of Fire Red). 3dssavesync refuses to mix the two games, so it does not sync the second ROM. To sync the hack as its own game, give it an **alias**: the same alias on the PC and on the 3DS.

- **PC**: in `config.json`, add `"alias": {"Pokemon Radical Red": "GBA-0RAD"}`. The name is the ROM file name without its extension.
- **3DS**: create `sdmc:/3ds/3dssavesync/alias.ini` with one line per ROM: `Pokemon Radical Red=GBA-0RAD`.

The alias has the form of a game key of the ROM system:

| System | Form | Example |
|---|---|---|
| DS | 4 letters or digits | `0RAD` |
| GBA | `GBA-` + 4 letters or digits | `GBA-0RAD` |
| GB / GBC | `GB-` + 1 to 15 letters, digits or `_` + `-` + 4 hexadecimal digits | `GB-MONHACK-0001` |

Start the code with a digit, as in `0RAD`: no commercial game has such a code. Native 3DS games cannot have an alias.

> the alias "…" of … is not valid. Example for a GBA game: GBA-0RAD. Fix it in config.json.

The alias does not have the right form. Fix it with the table above. Until then, this ROM is not synced.

### The emulator is already running

> mgba is already running. Close it before starting a game.
> mgba is open outside 3dssavesync: its game is unknown. Close it before starting a game.

Close the emulator, then start the game again from your frontend. Two different games at the same time are fine when both are started by 3dssavesync.

> The game starts, but 3dssavesync cannot write to …. Do not start this game again before you close it.

3dssavesync cannot write its lock file in its settings folder. Check the rights of that folder.

### The game starts with an old save

> A save just came from the server. …

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

> another program is using this file: close the emulator, then start the game again
> disk full: free some space, then start the game again
> access denied to …: check the rights of this folder

Close the emulator (or the antivirus or cloud sync program that holds the file), free disk space, or fix the folder rights. Nothing was replaced.

> …: read-back differs

3dssavesync wrote a file and read back different bytes. Your disk or USB drive may be failing. A backup copy exists in `copies/` (see [Your data](../README.md#your-data)).

### gpSP and Beetle GBA (RetroArch)

> …: save size unknown while the server does not have this game. Sync it first from mGBA or the 3DS

These cores always write a file of fixed size, so 3dssavesync takes the size from the server. Sync the game once from mGBA or the 3DS.

> …: the .srm has data outside the … bytes of the server version. Sync refused

The `.srm` does not match the save on the server. Check the core and its save settings ([ADR 0011](adr/0011-srm-gpsp-beetle-taille-du-serveur.md)).

### DeSmuME footer missing or unknown

> DeSmuME footer missing or unknown

The `.dsv` file has a layout that 3dssavesync does not know. Use melonDS for this game, and [report it](#report-a-bug).

### Game never started in Azahar

> game never started in Azahar. Start it once in Azahar and save in game, then start it here again

Do what the message says. The 8 Pokémon games do not need this step.

## 3DS

### Server unreachable

> Server unreachable: check the address. *(Serveur injoignable : vérifiez l'adresse.)*

The 3DS cannot reach the server. With the local server, check each point:

1. The PC is on, and the local server runs. It starts with your session. To start it by hand, run `3dssavesync serve`.
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

### This address does not lead to a 3dssavesync server

> This address does not lead to a 3dssavesync server: check it.

The address leads to another program or site. Check the port (`:8765` by default for the local server) and that there is no extra path after it.

### Invalid server address

> Invalid server address: check it (SELECT).

Type the full address, with `http://` for the local server (for example `http://192.168.1.20:8765`) or `https://` for your own server.

### Certificate refused or secure connection failed

> Server certificate refused: check the 3DS date and time.
> Secure connection failed: check the address, and the 3DS date and time.

- Set the date and time of the 3DS (System Settings).
- The local server uses `http://`, not `https://`.
- Your own server needs a **Let's Encrypt** certificate and TLS 1.2 with ECDHE: see the [server guide](guide-server.md#https-and-the-3ds).

### The server does not answer, connection cut, network error

> The server does not answer: try again later.
> Connection cut: try again.
> Network error: try again.

Move closer to the router and press **Y** to try again. If it lasts, check the server.

### Out of memory, or the network does not start

> Out of memory: restart 3dssavesync.
> Network start failed. Restart the 3DS, then start 3dssavesync again.
> Cannot read the installed games. Restart the 3DS, then try again.

Do what the message says.

### Server not read

> Cannot read the server. … — games show **Unknown** and "server not read".

The list shows your games, but their state is unknown. **A** and **START** do nothing until the server is read. Fix the reason given in the message, then press **Y**.

### First start: enter at least the server and the token

The 3DS SD card has no `config.ini`. Prepare the SD card from the PC settings page, or enter **Server** and **Token** on this screen.

### A game is missing from the list

- Check the ROM and save folders in **SELECT** (TWiLight Menu++ defaults: `sdmc:/roms/nds` and `sdmc:/roms/nds/saves`).
- ROMs in subfolders are found up to 4 levels deep. Folders named `saves`, `3ds`, `luma`, `mGBA`, `Nintendo 3DS`, or starting with `.` or `_` are skipped.
- A game with a ROM but no save, and no save on the server, has nothing to sync yet. Play it and save once.
- Native 3DS games: open **SELECT** > **3DS games** and tick the game. Only games with a single save file can be ticked.

> 88 ROMs skipped: more than 512 games.

3dssavesync reads 512 games at most. Point the ROM folders to fewer games.

### Several ROMs hold this game

> 2 ROMs hold this game: keep only one, or give the others an alias (alias.ini).

Two ROMs have the same game key (two copies, or a ROM hack), or give the same save file. 3dssavesync does not guess which one is right. Delete or move the extra ROM, or [give it an alias](#a-rom-hack-is-not-synced).

### SD card errors

> …: cannot read the save. Check the SD card.
> …: backup copy failed, nothing written. Check the SD card.
> Settings not saved. Check the SD card.

The SD card is full, locked (lock switch), or damaged. Nothing was written. Free space on the card or check it on a PC.

> …: rename failed. Downloaded save: ….tmp. Old one copied to /3ds/3dssavesync/copies.

The new save is in the `.tmp` file next to the save. Put the SD card in a PC and rename the `.tmp` file to the save name.

> …: 3DS write refused, save intact.

Start the game once, then try again.

### A 3DS game cannot be ticked

> Several save files: this game cannot be synced.
> No save yet: start the game once and save.

Only native 3DS games with a single save file are supported.

### open_agb_firm: the GBA save does not change

open_agb_firm writes the `.sav` file only when you turn off the 3DS with POWER. Turn it off with POWER after you play, then sync at the next start.

## Server and network

The PC shows these messages in the settings page (**Test the connection**, **Save**), and in the "Sync failed" window before a game.

### The server refuses this token

> The server answers, but refuses this token. Copy the exact token of the server.

Copy the token again: from the local server box of the settings page, or from the `.env` file of your own server.

### This address does not lead to a 3dssavesync server (PC)

> This address does not lead to a 3dssavesync server. Check the address.

Check the address: right subdomain, no extra path. A reverse proxy may send it to another site.

### The server has an internal problem

> The server has an internal problem. Try again later; if it lasts, read the server log.

Wait, then read the logs of the server container and of the reverse proxy.

### Unexpected answer from the server

> Unexpected answer from the server. Check the server address.

Something answers that is not the 3dssavesync server: a redirect, a login page, or a block. Use the exact `https://` address, with nothing in front of the server.

### Invalid server certificate

> Secure connection refused: invalid server certificate. Check the address (https://) and the PC date.

Check the date of the PC. Check the certificate of your server. Check `https://` against `http://`: the local server uses `http://`.

### Server address not found

> Server address not found (no Internet, or wrong address). Check the connection and the address.

Check your Internet connection and the spelling of the address.

### The server refuses the connection

> The server refuses the connection: it is stopped, or the port is wrong. Start the server or check the address.

Start the server. For the local server, run `3dssavesync serve` or log in again. Check the port.

### The server does not answer (PC)

> The server does not answer (no Internet, or wrong address).

Check your network and the address. The save stays on the PC.

### Save too large for the server

> save too large for the server: raise the size limit of the server (or of its proxy)

Your reverse proxy limits the request size. Raise its limit (the server accepts up to 64 MiB).

### The server refuses this save

> the server refuses this save (empty, too large or upload cut). Start the game again

The upload was cut. Start the game again. If it repeats, check the size of the save file.

### Enter the server address

> Enter the server address, with https:// (or http:// for the local server).

Type the full address, with its `https://` or `http://`.

### Nothing is written

> … Nothing is written.

You clicked **Save** while the server was unreachable. Nothing changed. Fix the address or the token, then click **Save** again.

### Port already used

> port 8765 is already used by another program: the local server does not start. Choose another port in "3dssavesync settings", then enter the new address on the 3DS.

Choose another port in **3dssavesync settings**, click **Save**, then prepare the 3DS SD card again.

> no local server in the configuration: choose "Local server on this PC" in "3dssavesync settings".

`3dssavesync serve` needs the local server. Choose it in the settings page.

> the local server does not answer.

The local server could not start or is blocked. Check the [firewall](#windows-defender-firewall-asks-about-3dssavesync) and the port.

### My own server works on the PC, not on the 3DS

The 3DS knows only the Let's Encrypt root certificate, and needs TLS 1.2 with ECDHE. See the [server guide](guide-server.md#https-and-the-3ds).

## Sync and conflicts

### Sync failed before the game

> Sync failed … Play with the PC save? It will be sent at the next launch. [Yes / No]

3dssavesync could not reach the server before the game. The reason is on the lines above.

- **Yes**: play with the save of the PC. It goes to the server at the next launch.
- **No**: do not play now. Fix the reason first.

### Upload failed after the game

> Upload failed: the save stays on the PC and will be sent at the next launch.

The network was cut, or the server was off. The save waits on the PC. At the next launch of any game, 3dssavesync syncs it first, with the usual comparison. If the server changed meanwhile, you get the usual conflict choice. Nothing to do now.

### Conflict

You played on both sides since the last sync. 3dssavesync never chooses for you. Each version shows its play time and trainer (Pokémon games) or its date.

- **PC, before the game**: **Take the server**, **Keep the PC**, or **Don't play**. After the game: **Keep the PC** or **Do nothing**. Closing the window = the last button, the safest.
- **3DS**: **A** keeps the 3DS, **X** takes the server, **B** does nothing.

Nothing is lost: the other version stays in the server history, and a backup copy is made before a save is replaced. **START** (Sync all) on the 3DS skips conflicts: solve them one by one with **A**.

> Conflict on …: nothing is replaced. The game uses the PC save.

No window could open (no display, for example over SSH). Start the game again with a display to choose.

> …: the server changed meanwhile (conflict).

Another device uploaded while the 3DS was syncing. The list is read again: choose with **A**.

### The save received is damaged

> the save received from the server is damaged (incomplete transfer): nothing is replaced. Start the game again
> 3DS: …: received version damaged, nothing written. Try again.

The download was incomplete. Nothing was written. Try again.

### The save keeps changing on the server

> the save keeps changing on the server (another device is uploading at the same time). Start the game again in a moment

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

> the save changed on the server meanwhile (a device just uploaded it): nothing is restored. Show the list of versions again, then choose.

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
