# Frontend guide

The settings page sets up **LaunchBox**, **ES-DE** and the **shortcuts** by itself. For the other frontends, follow this page. Sources and details (French): [notes/research-v3-frontends.md](notes/research-v3-frontends.md).

Common rules:

- `<exe>` = the path of the installed program: `%LOCALAPPDATA%\Programs\3dssavesync\3dssavesync.exe` (Windows), `~/.local/bin/3dssavesync` (Linux), `~/Applications/3dssavesync/3dssavesync` (macOS).
- `<name>` = the name of the emulator in `config.json`: `melonds`, `mgba`, `azahar`, `retroarch`, `vbam`, `sameboy`, `desmume`…
- Always put `--emulator <name>` **before** the ROM.
- Turn off the frontend's own save sync for DS, GB, GBA and 3DS (EmuDeck CloudSync, for example): two tools would upload the same saves.
- After a frontend update or reset, open **3dssavesync settings** and click **Repair frontends**.

## Steam ROM Manager (Steam Deck and desktop)

Add a **new** parser (do not change the EmuDeck ones):

| Field | Value |
|---|---|
| Parser type | Glob |
| ROMs directory | the DS ROM folder (for example `~/Emulation/roms/nds`) |
| User's glob | `${title}@(.nds\|.NDS\|.zip\|.ZIP)` |
| Executable | `<exe>` |
| Command line arguments | `run --emulator <name> "${filePath}"` |

Then click "Add games" and "Save to Steam" in SRM. For a Flatpak, the profile in `config.json` needs `"processus"` (the settings page fills it in).

## EmuDeck (Steam Deck)

1. Run the settings page in Desktop mode: it sets up ES-DE (`~/ES-DE/custom_systems`), which EmuDeck uses.
2. For Game mode, add the Steam ROM Manager parser above.
3. Turn off EmuDeck CloudSync for DS, GB, GBA and 3DS. The settings page warns you if it seems active.
4. Do not change the scripts in `~/Emulation/tools/launchers`: EmuDeck puts them back at each start.
5. After an EmuDeck reset: **Repair frontends**.

## RetroDECK

The ES-DE inside RetroDECK runs in a sandbox: the settings page does not set it up. Use Steam ROM Manager or a shortcut, with a custom profile in `config.json`:

```json
"retrodeck-nds": {
  "exe": "/usr/bin/flatpak",
  "args": ["run", "net.retrodeck.retrodeck", "-s", "nds", "{rom}"],
  "sauvegardes": { "DS": "/home/deck/retrodeck/saves/nds/melonds" },
  "processus": "melonDS"
}
```

Not checked: does `flatpak run` wait for the end of the game?

## Playnite (Windows)

Library > Configure Emulators > add a custom profile:

| Field | Value |
|---|---|
| Executable | `<exe>` |
| Arguments | `run --emulator <name> "{ImagePath}"` |
| Supported file types | `nds, zip` (DS); `gb, gbc, gba, zip` (mGBA); `3ds, cci, cxi` (Azahar) |
| Tracking mode | Original process |

Then choose this profile for these games.

## RetroBat (Windows)

Create `RetroBat\emulationstation\.emulationstation\es_systems_3dssavesync.cfg`. RetroBat keeps its original file; to go back, delete this file.

```xml
<?xml version="1.0"?>
<systemList>
  <system>
    <name>nds</name>
    <command>"<exe>" run --rom %ROM% -- "%HOME%\emulatorLauncher.exe" -gameinfo %GAMEINFOXML% %CONTROLLERSCONFIG% -system %SYSTEM% -emulator %EMULATOR% -core %CORE% -rom %ROM%</command>
  </system>
</systemList>
```

`run --rom … --` syncs the ROM, then runs the RetroBat command as is. The emulator of `config.json` that handles this system gives the save folder.

## Pegasus

In the `metadata.pegasus.txt` file of the collection:

```
launch: "<exe>" run --emulator <name> "{file.path}"
```

## Lutris (Linux)

A game with the "Linux" runner:

| Field | Value |
|---|---|
| Executable | `<exe>` |
| Arguments | `run --emulator <name> "/path/to/game.nds"` |

Do not use the Lutris "Stop" button during the game: it kills 3dssavesync before the upload (the upload goes out at the next launch).

## Attract-Mode

New file `emulators/<name>-3dssavesync.cfg` (in `~/.attract`, or the Attract-Mode folder on Windows):

```
executable           <exe>
args                 run --emulator <name> "[romfilename]"
rompath              /path/to/roms/nds
romext               .nds;.zip
nb_mode_wait         0
```

Then link a ROM list to this emulator in the Attract-Mode settings.
