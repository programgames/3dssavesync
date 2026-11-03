# HOME menu icons (forwarders)

A forwarder is a small title on the 3DS HOME menu that starts one game directly. 3dssavesync works with two kinds:

- [DS forwarders](#ds-games-nds-forwarders) (nds-bootstrap): they save in the same place as TWiLight Menu++, so nothing changes for 3dssavesync.
- [mGBA forwarders](#gb-and-gba-games-mgba-forwarders) for GB, GBC and GBA games: they save in another folder, and 3dssavesync follows them.

Native 3DS games already have their icon.

## DS games: NDS forwarders

The forwarder starts one DS ROM with nds-bootstrap. It saves **in the same place as TWiLight Menu++** (`sdmc:/roms/nds/saves/<name>.sav`). Everything is done on the console.

> **Warning**: first copy the `sdmc:/_nds/` folder to a PC. The forwarder pack can replace the nds-bootstrap of TWiLight Menu++ (not checked).

1. Open **Universal-Updater** and install **NDSForwarder**, then the **nds-bootstrap forwarder pack**.
2. Start **NDS Forwarder Generator** from the Homebrew Launcher.
3. Choose `sdmc:/roms/nds/<game>.nds`, then confirm. Do it again for each game.
4. Open `sdmc:/_nds/ntr_forwarder.ini` and check `SAVE_LOCATION=0` (the same value as TWiLight Menu++). The forwarder does **not** read the settings of TWiLight Menu++.
5. Do not move or rename these ROMs anymore: the path is fixed in each forwarder.

Limits: 40 forwarders at most; `.nds` only.

A game started from its icon syncs as before: open 3dssavesync and press START.

## GB and GBA games: mGBA forwarders

An mGBA forwarder is one `.cia` per game: mGBA and the ROM behind one HOME menu icon. A New 3DS is required for GBA games.

**Important difference**: a forwarder saves in `sdmc:/mGBA/forwarders/<ROM name>.sav`, not next to the ROM. 3dssavesync uses this file **as soon as it exists**; otherwise it keeps the `.sav` next to the ROM. The ROM stays in `sdmc:/roms/gba` (or `sdmc:/roms/gb`): 3dssavesync reads the game key from it.

> **Warning**: for a game that has a forwarder, play **only** with the forwarder, or switch with **X** first (see below). open_agb_firm and TWiLight Menu++ still read the old `.sav` next to the ROM: you would not play the same game.

### 1. On the PC: get the tools

1. Download the development build of mGBA for Windows (64-bit, portable): https://s3.amazonaws.com/mgba/mGBA-build-latest-win64.7z (page: https://mgba.io/downloads.html, "Development downloads").
2. Extract it in a **new** folder, for example `<mGBA-dev>`. Do not replace the mGBA you play with.
3. Do not open games with this build: it only creates the forwarders.

### 2. On the PC: the ROM

The name of the `.sav` comes from the name of the ROM you choose. It must be **the same** as the name of the ROM in `sdmc:/roms/gba`, without the extension. The safest: copy the ROM from the SD card itself.

### 3. On the PC: create the forwarder

Use the command-line tool `pc/forwarder3ds.py` of this repository. It does the same steps as mGBA, and reports each failure:

```
python pc/forwarder3ds.py --rom "<ROM>" --titre "Pokémon Ruby" --icone "<48x48.png>" --banniere "<256x128.png>" --sortie "<Game>.cia" --outils "<mGBA-dev>" --base "<mGBA 3DS dev .cia>"
```

It uses `ctrtool`, `3dstool`, `bannertool` and `makerom` from `<mGBA-dev>` (mGBA forwarder kit). The base is the `.cia` of the mGBA 3DS "dev" build named in https://mgba.io/latest.ini (`[platform.3ds]`, `dev.name`, checked with `dev.sha256`). Same ROM = same title ID: FBI replaces the forwarder already installed.

Why not the mGBA interface (Tools > Create forwarder…): it ignores the failures of the tools. At the first forwarder without an icon, it copies its default icon to `<mGBA-dev>\smdh.png` **read-only**; every later forwarder then keeps this icon, whatever you choose. If you use it anyway, delete this file first.

### 4. On the 3DS: install and continue your game

1. First open 3dssavesync and press START: your current game must be on the server.
2. Copy the `.cia` to the SD card, then install it with **FBI**.
3. Before the first start, copy the current `.sav` to `sdmc:/mGBA/forwarders/` with the same name. The original stays next to the ROM.
4. Start the forwarder. Check that your game loads.
5. Play, save in the game, then quit with HOME.
6. Open 3dssavesync. In the game details, "Played with" shows the mGBA forwarder. Press START.

> **Warning: automatic reload in mGBA 3DS.** By default, mGBA writes a state (`.ss0`) when it quits and reloads it at the next start (`autoload`, `autosave`). After a download from the PC, it would go back to the old position, and a save in the game would then replace the progress made on the PC. Set `autoload=0` and `autosave=0` in `[ports.3ds]` of `sdmc:/mGBA/config.ini`.

The mGBA app (without forwarder) also uses `savegamePath=/mGBA/forwarders`: the forwarder writes it in the shared settings.

### Switch program for a game (X button in 3dssavesync)

In 3dssavesync, on a GB or GBA game, **X** moves the save from one side to the other, with a backup copy of the file it replaces:

- forwarder → ROM: the forwarder `.sav` replaces the one next to the ROM, then it is removed from `mGBA/forwarders` (open_agb_firm or TWiLight Menu++ read it next);
- ROM → forwarder: the `.sav` next to the ROM is copied to `mGBA/forwarders`.

The game details show the active side ("Played with": mGBA forwarder, open_agb_firm or TWiLight Menu++). Press X **before** you start the other program.

### Limits

- To update mGBA, you must create each forwarder again.
- Gold, Silver, Crystal: each program keeps its own clock ([ADR 0008](adr/0008-gb-gba-par-fichiers-sram-seule.md)).
