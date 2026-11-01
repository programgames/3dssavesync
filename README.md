# 3dssavesync

Play the same game on a modded 3DS and on a PC emulator, without copying save files by hand. Covers **DS**, **Game Boy / Game Boy Color**, **GBA** and native **3DS** games (by default the 8 Pokémon games, X to Ultra Moon).

*[Version française](README.fr.md)*

- One small server keeps **every version** of every save and never deletes anything.
- The 3DS app and the PC launcher compare fingerprints: when only one side changed, the save is copied; when both changed, you choose (**never** an automatic overwrite).
- Before any save is replaced, a backup copy is made.

## How it works

```
       3DS (Luma3DS)                         Server                               PC
 ┌──────────────────────────┐      ┌────────────────────────────┐      ┌───────────────────────────┐
 │ 3dssavesync app (.cia)   │ ───► │ local server in the PC app │ ◄─── │ 3dssavesync (one program) │
 │ · DS  (TWiLight Menu++)  │ ◄─── │ or personal server (Docker,│ ───► │ started by your frontend  │
 │ · GB  (TWiLight, mGBA)   │      │ HTTPS)                     │      │ or a shortcut, instead of │
 │ · GBA (open_agb_firm,    │      │ keeps every version        │      │ the emulator:             │
 │        mGBA forwarders)  │      │ forever                    │      │ sync → play → sync        │
 │ · 3DS (save archives)    │      └────────────────────────────┘      └───────────────────────────┘
 └──────────────────────────┘
```

On the PC, `3dssavesync` replaces the emulator in your frontend: it downloads the newest save, starts the game, waits for the emulator to close, then uploads the new save. On the 3DS, the app lists your games with their state (in sync, to upload, to download, conflict); START syncs everything.

## Install in 3 steps

### 1. The server

Choose one ([server guide, in French](docs/guide-serveur.md)):

| | Local server on the PC | Personal server (Docker) |
|---|---|---|
| Setup | nothing more: step 3 sets it up | an always-on machine (NAS, Raspberry Pi, server), Docker, a domain name |
| Sync | at home, PC on | anywhere (HTTPS) |

For a personal server: `server/docker-compose.yml` (behind your HTTPS reverse proxy) or `server/docker-compose.caddy.yml` (automatic HTTPS with Caddy). Image: `ghcr.io/programgames/3dssavesync-server`. The 3DS needs a **Let's Encrypt** certificate and TLS 1.2 with ECDHE.

### 2. The 3DS

Requirements: a 3DS with custom firmware (Luma3DS) and FBI.

1. Download `3dssavesync.cia` from the [latest release](https://github.com/programgames/3dssavesync/releases/latest), copy it to the SD card, install it with FBI.
2. Start **3dssavesync**: the settings screen opens. Enter the server address and token, check the ROM and save folders (TWiLight Menu++ defaults are filled in), then **Test the connection**.

### 3. The PC

1. Download the program for your system from the [latest release](https://github.com/programgames/3dssavesync/releases/latest): `3dssavesync-windows-amd64.exe`, `-linux-amd64`, `-darwin-arm64`…
2. Double-click it (Linux: `chmod +x`, then run `./3dssavesync-linux-amd64 setup`; macOS: first `xattr -d com.apple.quarantine <file>`, the program is not signed).
3. A settings page opens in your browser: server, emulators (found automatically), LaunchBox, ES-DE, shortcuts. Click **Save**.

Other frontends (Steam ROM Manager, EmuDeck, RetroDECK, Playnite, RetroBat, Pegasus, Lutris, Attract-Mode): [frontend guide](docs/guide-frontends.md). Full PC guide: [docs/guide-client-pc.md](docs/guide-client-pc.md). The detailed guides are in French.

## Supported

| | |
|---|---|
| **3DS apps** | TWiLight Menu++ (DS, GB/GBC), open_agb_firm (GBA), mGBA forwarders (GB/GBC/GBA), native 3DS games (any game whose save is a single `main` file; default: the 8 Pokémon) |
| **PC emulators** | melonDS, mGBA, Azahar, RetroArch (cores melonDS DS, mGBA, Gambatte, SameBoy, VBA-M), VBA-M, SameBoy, and any emulator with a custom command line (raw save files) |
| **Set up automatically** | LaunchBox / BigBox, ES-DE, desktop shortcuts (`.lnk`, `.desktop`, `.app`) |
| **Guides** | Steam ROM Manager, EmuDeck, RetroDECK, Playnite, RetroBat, Pegasus, Lutris, Attract-Mode |
| **Systems** | Windows, Linux, macOS (amd64 and arm64) |
| **Languages** | English, French (system language) |

**Tested on real hardware**: New 3DS XL (Luma3DS, TWiLight Menu++, open_agb_firm, mGBA forwarders), Windows 11 with LaunchBox, melonDS, mGBA and Azahar, personal Docker server.
**Not tested on a device yet**: Linux, macOS, Steam Deck (EmuDeck, RetroDECK, Game mode), RetroArch, VBA-M, SameBoy, ES-DE, the 3DS talking to the local server. Reports are welcome.

## Known pitfalls

- **Save states**: loading a state replaces the save with an older one. mGBA reloads a state at start by default (`autoload`); the launcher warns you after a download if this setting (or RetroArch's, VBA-M's) is on.
- **Blank save** (only `0x00` or `0xFF`): counted as absent, never uploaded.
- **3DS games in Azahar**: start the game once in Azahar and save, so its save folder exists.
- **Gold / Silver / Crystal clock**: each machine keeps its own clock.
- **Not supported yet**: DeSmuME (`.dsv`), NO$GBA, gpSP, Beetle GBA (formats to convert), GBA games with EEPROM saves, GB MBC2 games.

## How sync decides

No dates (the 3DS clock is not reliable): each machine compares three SHA-256 fingerprints, its own, the server's, and the one from the last sync (the *base*). One side changed: copy. Both changed: **conflict**, you choose. Every game has a **key** read from the ROM, the same on 3DS and PC: DS game code (`ADAF`), `GBA-BPEF`, `GB-PM_CRYSTAL-F2E2` (title + checksum), `3DS-00040000001B5000` (title ID). Details (French): [docs/design.md](docs/design.md), [ADRs](docs/adr/).

## Development

```bash
cd server && python -m unittest test_server.py      # HTTP contract (also: NDSSYNC_URL=https://... against a deployed server)
cd client && go test ./...                           # PC launcher, against the real server (Python needed)
cd 3ds && make cia                                   # devkitPro, makerom and bannertool in PATH
```

C tests (gcc on the PC): see `.github/workflows/ci.yml`. GitHub Actions runs every test (Go on Windows, Linux and macOS) and builds the `.cia` / `.3dsx`; a `v*` tag publishes a release with the PC programs and the server image.

Code, comments and docs are written in French; the apps speak English and French.

## License

MIT.
