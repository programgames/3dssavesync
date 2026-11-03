# 3DS guide

The 3dssavesync app lists your games with their state, and syncs them with the server.

![3DS app: game list and details](images/homebrew-en-1.png)

## Buttons

| Button | Action |
|---|---|
| Up / Down | choose a game |
| **A** | the action of the game: upload, download, or resolve the conflict |
| **X** | GB and GBA games: switch the save between the mGBA forwarder and the ROM ([forwarder guide](guide-forwarders.md#switch-program-for-a-game-x-button-in-3dssavesync)) |
| **Y** | read the server again |
| **START** | sync all games. Conflicts and duplicates are skipped. One confirmation lists the downloads that replace a 3DS save. |
| **SELECT** | settings |
| **B** | quit |

You can also touch the buttons on the bottom screen.

## Game states

| State | Meaning |
|---|---|
| ok | the 3DS and the server have the same save |
| Upload | the 3DS save is newer: **A** sends it to the server |
| Download | the server save is newer: **A** gets it, after a backup copy |
| Conflict | both changed since the last sync: **A** lets you choose (A keep the 3DS, X take the server, B do nothing) |
| Duplicate | two ROMs give the same game: keep one, or [give the hack its own save](troubleshooting.md#a-rom-hack-is-not-synced) |
| Unknown | the server could not be read: press **Y** |

## Settings (SELECT)

| Line | Default | Meaning |
|---|---|---|
| Server | — | address of the server, for example `http://192.168.1.20:8765` (local server) or `https://saves.example.com` |
| Token | — | the token of the server |
| DS ROMs | `sdmc:/roms/nds` | DS ROM folder (TWiLight Menu++) |
| DS saves | `sdmc:/roms/nds/saves` | TWiLight Menu++ default. Empty = next to the ROM. |
| GB ROMs | `sdmc:/roms/gb` | GB and GBC ROM folder (TWiLight Menu++) |
| GB saves | empty | empty = next to the ROM (TWiLight Menu++). The mGBA forwarder save wins if it exists. |
| GBA ROMs | `sdmc:/roms/gba` | GBA ROM folder |
| GBA saves | empty | empty = next to the ROM (open_agb_firm with `useSavesFolder=false`). With `useSavesFolder=true`: `sdmc:/3ds/open_agb_firm/saves`. |
| 3DS games | the 8 Pokémon games | native 3DS games to sync. Only games with a single save file can be ticked. |
| Test the connection | | checks the address and the token |

The PC fills in **Server** and **Token** when you prepare the SD card from its settings page. The other lines keep their value.

## ROMs in subfolders

3dssavesync reads the ROM folders and their subfolders, up to 4 levels deep. It skips folders named `saves`, `3ds`, `luma`, `mGBA`, `Nintendo 3DS`, and folders that start with `.` or `_` (such as `_nds`).

The save of a ROM in a subfolder is where your 3DS app writes it:

| Save setting | Save of `<roms>/<subfolder>/X.nds` |
|---|---|
| `<roms>/saves` (TWiLight Menu++ default) | `<roms>/<subfolder>/saves/X.sav` |
| empty, or the ROM folder | `<roms>/<subfolder>/X.sav` |
| another folder | `<saves>/X.sav` |

3dssavesync reads 512 games at most.

## Files on the SD card

All in `sdmc:/3ds/3dssavesync/`:

| File | Content |
|---|---|
| `config.ini` | server, token, folders |
| `base.ini` | the last synced state of each game. Do not delete it: without it, every changed game shows a conflict. |
| `titres.ini` | the 3DS games to sync (absent = the 8 Pokémon games) |
| `alias.ini` | optional: ROM hacks with their own save, written by the PC (settings page, Prepare the 3DS SD card): `Pokemon Radical Red=GBA-0A7K` ([details](troubleshooting.md#a-rom-hack-is-not-synced)) |
| `copies/` | backup copies made before a save is replaced |

## Native 3DS games

Open **SELECT** > **3DS games** to tick the games to sync. Only a game with a single save file can be synced. A game that is not in the list: start it once and save.

On the PC, add the same games to `titres_3ds` in `config.json` ([PC guide](guide-pc.md#configuration)).

Something does not work? See [Troubleshooting](troubleshooting.md#3ds).
