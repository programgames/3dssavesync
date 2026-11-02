# ROMs in subfolders on the 3DS: the Save where the launcher writes it

Before, the homebrew read only the root of each ROM folder (decision Q10). It now reads subfolders, up to 4 levels. It never reads `saves` folders, hidden folders (`.…`) or folders starting with `_` (`_nds` of TWiLight Menu++), nor `3ds`, `luma`, `mGBA` and `Nintendo 3DS` (useful if the ROM folder is the root of the SD card).

Save of a ROM `<roms>/<subfolder>/X.nds` (or `.gb`, `.gbc`, `.gba`), depending on the "Saves" setting of its System (`docs/research-v2.md`, French: `SAVE_LOCATION` of TWiLight Menu++ and NTR_Forwarder, GameYob, `useSavesFolder` of open_agb_firm):

| "Saves" setting | Save | Launcher |
|---|---|---|
| `<roms>/saves` | `<roms>/<subfolder>/saves/X.sav` | TWiLight Menu++ and DS forwarders (`SAVE_LOCATION=0`, default) |
| empty, or equal to `<roms>` | `<roms>/<subfolder>/X.sav` | GameYob (GB), open_agb_firm `useSavesFolder=false`, `SAVE_LOCATION=1` |
| other folder | `<saves>/X.sav` | open_agb_firm `useSavesFolder=true`, `SAVE_LOCATION=2` (`_nds/TWiLightMenu/saves`) |

A ROM at the root keeps the same path as before. The mGBA forwarder does not change (`sdmc:/mGBA/forwarders/<name>.sav`). The comparison between `saves` and `roms` ignores case (FAT SD card).

Two ROMs that give the same Save file (same name in two subfolders, with a shared save folder) are a Duplicate, like two ROMs with the same Key. Two Backup copies with the same name in the same second get the Key in their name.

## Consequences

- Paths deduced from the launchers' code, not verified on the 3DS for a ROM in a subfolder.
- The 512-game limit (`MAX_JEUX`) also counts ROMs in subfolders; beyond it, ROMs are skipped and reported.
