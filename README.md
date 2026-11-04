# 3dssavesync

**Play the same game on your 3DS and on your PC. Your save follows you.**

[![Latest release](https://img.shields.io/github/v/release/programgames/3dssavesync)](https://github.com/programgames/3dssavesync/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/programgames/3dssavesync/total)](https://github.com/programgames/3dssavesync/releases)

*[Version française (résumé)](README.fr.md)*

| On the 3DS: your games and their state | When both sides changed: you choose |
|---|---|
| ![3DS app: game list and details](docs/images/homebrew-en-1.png) | ![3DS app: conflict screen](docs/images/homebrew-en-2.png) |

**On the PC: one settings page** (server status, emulators, history, ROM hacks)

![PC settings page: server status, emulators and their consoles](docs/images/settings-en.png)

- **DS, Game Boy, Game Boy Color, GBA** and native **3DS** games (the 8 Pokémon games by default, X to Ultra Moon).
- **On the PC**, your game starts as usual from LaunchBox, ES-DE or a shortcut. 3dssavesync gets the newest save first, then sends it back when you quit.
- **On the 3DS**, open the 3dssavesync app and press START: every game is synced.
- **Nothing is ever lost.** The server keeps every version of every save. A backup copy is made before any save is replaced. When both sides changed, you choose, with the play time and trainer name of each version (Pokémon games).

Requires a modded 3DS (Luma3DS).

## Install

About 10 minutes. Do the PC first: it prepares the 3DS for you.

### 1. The PC

1. Download the program for your computer:

   | System | File |
   |---|---|
   | Windows | [3dssavesync-windows-amd64.exe](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-windows-amd64.exe) |
   | Windows on ARM | [3dssavesync-windows-arm64.exe](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-windows-arm64.exe) |
   | Linux | [3dssavesync-linux-amd64](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-linux-amd64) · [arm64](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-linux-arm64) |
   | macOS (Apple silicon) | [3dssavesync-darwin-arm64](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-darwin-arm64) |
   | macOS (Intel) | [3dssavesync-darwin-amd64](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-darwin-amd64) |

2. Start it:
   - **Windows**: double-click it. If Windows shows "Windows protected your PC", [here's what to do](docs/troubleshooting.md#windows-protected-your-pc).
   - **macOS**: the program is not signed. In a terminal, run `xattr -d com.apple.quarantine <file>` once, then double-click it.
   - **Linux**: run `chmod +x 3dssavesync-linux-amd64`, then `./3dssavesync-linux-amd64 setup`.
3. A settings page opens in your browser and walks you through 4 steps. On **Server**, keep **Local server on this PC**: it's the default and needs nothing else. On **Emulators and frontends**, check the emulators it found and your frontends (LaunchBox, ES-DE, shortcuts), then click **Save and continue**.
4. If Windows asks about the firewall, allow **private networks**. Otherwise the 3DS can't reach the PC.
5. On **3DS SD card**, put the 3DS SD card in the PC and click **Prepare the 3DS SD card**. This writes the server address and token to the card and copies the 3DS app (`3dssavesync.cia`) to its `cias` folder. Nothing else on the card changes.
6. On **Everything is ready**, each line shows ✓ or tells you what's left to do.

### 2. The 3DS

1. Put the SD card back in the 3DS. Open FBI and go to **SD** > **cias** > **3dssavesync.cia** > **Install and delete CIA**. Don't have FBI? Get [FBI.3dsx](https://github.com/nh-server/FBI-NH/releases/latest), copy it to the `3ds` folder on the card and open it from the Homebrew Launcher.
2. Start **3dssavesync** from the HOME Menu. It connects to the PC on its own, with nothing to type.

No SD card reader? Copy [3dssavesync.cia](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync.cia) to the 3DS by FTP and install it with FBI. Then press SELECT on the 3DS and enter the **Server** and **Token** that the PC page shows, or click **Download config.ini** on the PC page and copy it to `sdmc:/3ds/3dssavesync/` by FTP.

### 3. Check that it works

1. On the 3DS, 3dssavesync lists your games. Press **START** to sync them all.
2. On the PC, start one of these games from your frontend (LaunchBox, ES-DE) or from the **3dssavesync &lt;emulator&gt;** shortcut. You pick up where you left off on the 3DS.
3. Save in the game and quit. Back on the 3DS, open 3dssavesync: the game now shows **Download**. Press **A** to get your save.

Something does not work? Read [Troubleshooting](docs/troubleshooting.md).

### Your own server (optional)

The local server syncs at home, when the PC is on. To sync from anywhere, run the server on an always-on machine (NAS, Raspberry Pi) with Docker and HTTPS. See the [server guide](docs/guide-server.md).

### Portable version

Keep everything in one folder, for example `LaunchBox\ThirdParty\3dssavesync`, a USB drive or the SD card of a Steam Deck:

1. Download [3dssavesync-windows-amd64-portable.zip](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-windows-amd64-portable.zip) or [3dssavesync-linux-amd64-portable.tar.gz](https://github.com/programgames/3dssavesync/releases/latest/download/3dssavesync-linux-amd64-portable.tar.gz).
2. Extract it to a folder where you can write (not `Program Files`, not a read-only drive).
3. Start `3dssavesync` from this folder (Linux: `./3dssavesync setup`), then continue at step 3 of [The PC](#1-the-pc).

The `portable.txt` file next to the program is what makes it portable: settings, log, backups and local server data all stay in this folder, and LaunchBox, ES-DE and the shortcuts start the program from here. Delete `portable.txt` to switch back to the normal install. See [ADR 0020](docs/adr/0020-version-portable.md).

## Everyday use

- **PC**: start your games from your frontend or the shortcuts, as before. Your save is uploaded when you close the emulator.
- **3DS**: open 3dssavesync before and after you play. Press **START** to sync everything, or **A** on one game.
- **Conflict** (you played on both sides): you choose which save to keep. Nothing is lost: the other one stays in the server history or in a backup.
- **Get back an old version**: open the **3dssavesync settings** shortcut, go to **Games**, pick the game and the version, and click **Restore this version** (or run `3dssavesync restore`). Each device gets it at its next sync, after a backup.
- **No network after a game on the PC**: the save stays on the PC and is uploaded the next time you start any game.
- **Icon near the clock** (Windows, Linux KDE): right-click it to open the settings, upload pending saves now or see the local server address. More in the [PC guide](docs/guide-pc.md#the-icon-near-the-clock).
- **Updates**: the settings page and the icon near the clock tell you when a new version is out. Click **Update** on the settings page and the program updates and restarts itself. For the 3DS app, update it from its settings (**SELECT** > **Update**), or prepare the SD card again. More in the [PC guide](docs/guide-pc.md#updates).

- **ROM hacks** (Radical Red and others) use the same game code as the original. 3dssavesync won't mix them up and offers to give the hack [its own save](docs/troubleshooting.md#a-rom-hack-is-not-synced).
- **ROMs in subfolders** on the 3DS are found, up to 4 levels deep. Each save goes where your 3DS app expects it.
- **More PC settings** (emulators, frontends, commands): [PC guide](docs/guide-pc.md). **3DS settings**: [3DS guide](docs/guide-3ds.md).

## Supported

| | |
|---|---|
| **3DS apps** | TWiLight Menu++ (DS, GB/GBC), open_agb_firm (GBA), mGBA forwarders (GB/GBC/GBA), native 3DS games (any game with a single save file; default: the 8 Pokémon) |
| **PC emulators** | melonDS, mGBA, Azahar, RetroArch (cores melonDS DS, mGBA, Gambatte, SameBoy, VBA-M, gpSP, Beetle GBA), VBA-M, SameBoy, DeSmuME, SkyEmu, NanoBoyAdvance, BGB, and any emulator with a custom command line |
| **Set up automatically** | LaunchBox / BigBox, ES-DE, desktop shortcuts |
| **Guides** | [Steam ROM Manager, EmuDeck, RetroDECK, Playnite, RetroBat, Pegasus, Lutris, Attract-Mode](docs/guide-frontends.md); [HOME menu icons (forwarders)](docs/guide-forwarders.md) |
| **PC systems** | Windows, Linux, macOS (amd64 and arm64) |
| **Languages** | English, French (follows the system language) |

**Tested on real hardware**: New 3DS XL (Luma3DS, TWiLight Menu++, open_agb_firm, mGBA forwarders) with the local server and with a Docker server; Windows 11 with LaunchBox, melonDS, mGBA and Azahar.
**Not tested on a device yet**: Linux, macOS, Steam Deck (EmuDeck, RetroDECK, Game mode), RetroArch, VBA-M, SameBoy, DeSmuME, SkyEmu, NanoBoyAdvance, BGB, ES-DE. Reports are welcome.

## Good to know

- **Save states**: loading a state brings back an older save. mGBA reloads a state at start by default (`autoload`). 3dssavesync warns you when this setting (or the one of RetroArch or VBA-M) is on.
- **Blank save** (only `0x00` or `0xFF`): counted as no save, never sent.
- **Gold / Silver / Crystal clock**: each machine keeps its own clock. The game progress is the same.
- **GBA EEPROM and GB MBC2 saves** are converted between emulators ([ADR 0012](docs/adr/0012-eeprom-et-mbc2-forme-du-serveur.md)). An EEPROM save sent from open_agb_firm before v3.5 is in the wrong byte order on the server: send it again from the 3DS first, then get it on the PC.
- **Not supported**: NO$GBA, Panda3DS.

## Your data

| Where | What |
|---|---|
| PC: `%APPDATA%\3dssavesync` (Windows), `~/.config/3dssavesync` (Linux), `~/Library/Application Support/3dssavesync` (macOS) | `config.json`, the log `3dssavesync.log`, backup copies in `copies/`, local server data in `serveur/` |
| 3DS: `sdmc:/3ds/3dssavesync/` | `config.ini` (server, token, folders), backup copies in `copies/` |
| Server: `data/<game key>/` | every version of every save, never deleted |

With the local server, nothing leaves your home network. Your saves stay in your emulator and 3DS app folders, as before.

## Uninstall

1. Close LaunchBox and ES-DE.
2. Open **3dssavesync settings**, part **Frontends and install**, then click **Uninstall 3dssavesync**.
3. On the 3DS, open FBI, **Titles**, **3dssavesync**, **Delete Title**.

LaunchBox and ES-DE get their original emulators back, so your games start as before. The shortcuts and the program go to the Recycle Bin (portable version: the program stays; delete its folder yourself). Your saves do not move; the settings, the backup copies and the local server data stay, and the report says where. Without the settings page: `3dssavesync uninstall`.

## How it works

```
   3DS (Luma3DS)                Server                         PC
 3dssavesync app   <--->  local server in the PC  <--->  3dssavesync, started by your
 (DS, GB, GBA, 3DS)       program, or Docker server      frontend instead of the emulator:
                          keeps every version            sync -> play -> sync
```

No dates: the 3DS clock is not reliable. Each machine compares three fingerprints of the save: its own, the server's, and the one from its last sync. Only one side changed: the save is copied. Both changed: you choose. Each game has a key read from the ROM, the same on the 3DS and the PC, so file names do not matter.

All documentation: [docs/](docs/README.md). Design and decisions: [design](docs/design.md), [ADRs](docs/adr/), [glossary](docs/GLOSSARY.md).

## Development

```bash
cd server && python -m unittest test_server.py      # HTTP contract (also: NDSSYNC_URL=https://... against a deployed server)
cd client && go test ./...                           # PC program, against the real server (Python needed)
cd 3ds && make cia                                   # devkitPro, makerom and bannertool in PATH
```

C tests (gcc on the PC): see `.github/workflows/ci.yml`. GitHub Actions runs every test (Go on Windows, Linux and macOS) and builds the `.cia`. A `v*` tag publishes a release with the PC programs and the server image.

The apps speak English and French. Most code identifiers and comments are still French and are being moved to English. See [CONTRIBUTING.md](CONTRIBUTING.md) for the build steps, the rules every change follows, and how AI tools are used here. Release notes: [CHANGELOG.md](CHANGELOG.md).

## License

MIT.
