# Recherche v3 — profils d'émulateurs pour le client PC (Go)

Date : 2026-11-01. Méthode : lecture du code source à un commit figé, plus les pages officielles (BGB, NO$GBA, Emulicious, Flatpak, Qt, SDL). Aucun émulateur n'a été lancé. Rien n'a été installé.

Légende :
- **NON VÉRIFIÉ** = pas de source primaire, ou déduction sans test.
- « déduction » = conclusion tirée du code cité, sans exécution.
- Les résultats déjà établis pour mGBA 0.10.5, Azahar 2126.2 et melonDS 1.1 sont dans [research.md](research.md) (§2) et [research-v2.md](research-v2.md) (Partie 3). Ce document les reprend en bref et les vérifie au commit `master` actuel.

## Synthèse

- **Profil RetroArch unique, paramétré par cœur.** Le dossier final est `savefile_directory` + `<library_name du cœur>` (car `sort_savefiles_enable` vaut `true` par défaut). Le nom est `<nom de la ROM>.srm`, plus `<nom de la ROM>.rtc` pour l'horloge GB. Les cœurs GB/GBC (Gambatte, SameBoy, mGBA, VBA-M, Gearboy, TGB Dual) écrivent la SRAM brute dans `.srm` et l'horloge à part. C'est le format le plus simple pour la 3DS.
- **SRAM brute, sans conversion** : melonDS, melonDS DS, mGBA (Trailer RTC déjà géré), SameBoy, VBA-M, BGB, SkyEmu, NanoBoyAdvance, Azahar (`main`).
- **Conversion obligatoire** : DeSmuME (pied de page `.dsv` de 122 octets), NO$GBA (en-tête `NocashGbaBackupMediaSavDataFile`, compression possible), gpSP (`.srm` toujours de 128 Kio), Beetle GBA (taille devinée par heuristique), Panda3DS (sauvegarde rangée par nom de fichier ROM, pas par title ID).
- **Deux écarts de format découverts, qui touchent l'ADR 0008 :**
  1. La règle `taille - (taille mod 1024)` donne **0** pour les sauvegardes de 512 octets (GBA EEPROM 4 Kbit, GB MBC2). Voir §8.
  2. **GBA EEPROM** : open_agb_firm attend un autre ordre d'octets que les émulateurs PC (inversion des 8 octets de chaque bloc). Les Pokémon GBA (Flash) ne sont pas touchés.
- **Savestate chargé au lancement, activé par défaut** : mGBA (`autoload=1`, slot 0). EmuDeck livre aussi `autoload=1` pour mGBA. RetroArch : `savestate_auto_load=false` par défaut, mais l'option « AutoSave » d'EmuDeck le met à `true`.
- **Statut 3DS** : Azahar est le seul émulateur 3DS actif. Citra est retiré (dépôt `citra-emu/citra` en 404). Le dépôt GitHub de Lime3DS redirige vers Azahar. Azahar a maintenant un cœur libretro dans son propre dépôt.
- **Steam Deck** : EmuDeck range tout sous `~/Emulation/saves/<émulateur>/` (dossiers réels ou liens symboliques). RetroDECK range tout sous `~/retrodeck/saves/<système>/<émulateur>/`.

## Sources figées

| Projet | Réf. | Commit (date) |
|---|---|---|
| RetroArch | `master` (dernière release : v1.22.2, 2025-11-20) | `98860742b68138613a68ecbf430d7f6844c8878e` (2026-11-01) |
| libretro-core-info | `master` | `445421549abc0638a2f5ecf05db8437e580dcc69` |
| melonDS | `master` (dernière release : 1.1, 2025-11-18) | `1e654443e4c0a54b40709c0208aa0eeba41e6142` |
| melonDS DS (cœur) | `main` (v1.4.0, 2026-09-27) | `f394adbacb5722ee97c1b37c8064da9a25818310` |
| libretro/melonDS (ancien cœur) | `master` | `66b5d2634cd0a79030562811e6e05f5532f800ba` |
| DeSmuME | `master` (dernière release : 0.9.13, 2022-05-23) | `fc504ee0bc9ac3434cafcaae7b129c025499a609` |
| libretro/desmume | `master` | `95b4d798731caa809125b6c3c11d17cc332ff6ef` |
| libretro/desmume2015 | `master` | `422b688009ccb9f2917c086a8fe79c8cecb030ae` |
| mGBA | `master` (0.11-dev ; dernière release : 0.10.5) | `52de910807c0178d6cfb088aa350952569e8f08f` |
| libretro/mgba | `master` | `7a12d6d4b9acb14c0ae62c9166b6a2f3d08007f6` |
| SameBoy | `master` (v1.0.4, 2026-10-09) | `3e271573e9fcd07d4093ca327cfbed2a7797b7d3` |
| libretro/SameBoy | `master` | `aa158a889a48b538a0302873704a34577c8eb67d` |
| gambatte-libretro | `master` | `77f2567d718b8973d02caa2daa8dce4953faf811` |
| VisualBoyAdvance-M | `master` (v2.2.3, 2025-10-25) | `beab4e0400160e645fc1b00fce275c74ed34d068` |
| vbam-libretro | `master` | `115defb3a318258ab84746d45258a1aec19d0b4b` |
| NanoBoyAdvance | `master` (v1.8.3) | `55b5cf0ae3d929582ac5bfd486558173502b8354` |
| SkyEmu | `dev` (v5) | `f4d3238bf42da483ed1b7cc35649398b2110e4b4` |
| gpSP (libretro) | `master` | `5819380c2ffb0900219d700a382ee68c464ebb99` |
| Beetle GBA | `master` | `b158166237b17253188cfdbe73a8a0b9fe4b3a8c` |
| Gearboy | `master` (3.8.20) | `997da465e1c89e8bcceddb6bf5812d8c6724392d` |
| TGB Dual | `master` | `0392c9c469e653205e471114c7949c07c83bfce9` |
| Azahar | `master` (dernière release : 2126.2, 2026-10-08) | `15983ee9bf93e069474a27caf679f17aa2b6f5c7` |
| libretro/citra | `master` | `a0483e9abe8134ae6b290cdca7d49cb77689f05b` (2026-09-20) |
| Panda3DS | `master` (v0.9, 2024-12-25) | `92e53d26b898eb89d865f8407460900a88e7c0fa` |
| EmuDeck | `main` (2.7EA) | `dddb30229adbbd54d5ff7283cfd70e11c4322c26` |
| RetroDECK | `main` (0.10.10b) | `7786a849179eb2ca94e8ca413259e28adbcb1e39` |
| RetroDECK components | `main` | `fcd0d72995d7aa2e741e2753ac6914025b2ff1ac` |
| open_agb_firm | `master` | `401c3dff900f658d3edf77ababcfaf5dca54a0a8` |
| gba-eeprom-save-fix (exelotl) | `main` | `fdc61d84c6b7779dd39786a70ee98739c9568001` |
| GameYob | `master` | `c4d8672806b4a1cff8012fd40de4acf69b0094c2` |

Préfixes de liens utilisés dans les sections « Sources » :

- `RA` = `https://github.com/libretro/RetroArch/blob/98860742b68138613a68ecbf430d7f6844c8878e/`
- `CI` = `https://github.com/libretro/libretro-core-info/blob/445421549abc0638a2f5ecf05db8437e580dcc69/`
- `MEL` = `https://github.com/melonDS-emu/melonDS/blob/1e654443e4c0a54b40709c0208aa0eeba41e6142/`
- `MDS` = `https://github.com/JesseTG/melonds-ds/blob/f394adbacb5722ee97c1b37c8064da9a25818310/`
- `LMEL` = `https://github.com/libretro/melonDS/blob/66b5d2634cd0a79030562811e6e05f5532f800ba/`
- `DSM` = `https://github.com/TASEmulators/desmume/blob/fc504ee0bc9ac3434cafcaae7b129c025499a609/`
- `LDSM` = `https://github.com/libretro/desmume/blob/95b4d798731caa809125b6c3c11d17cc332ff6ef/`
- `LDSM15` = `https://github.com/libretro/desmume2015/blob/422b688009ccb9f2917c086a8fe79c8cecb030ae/`
- `MG` = `https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/`
- `LMG` = `https://github.com/libretro/mgba/blob/7a12d6d4b9acb14c0ae62c9166b6a2f3d08007f6/`
- `SB` = `https://github.com/LIJI32/SameBoy/blob/3e271573e9fcd07d4093ca327cfbed2a7797b7d3/`
- `LSB` = `https://github.com/libretro/SameBoy/blob/aa158a889a48b538a0302873704a34577c8eb67d/`
- `GBT` = `https://github.com/libretro/gambatte-libretro/blob/77f2567d718b8973d02caa2daa8dce4953faf811/`
- `VBM` = `https://github.com/visualboyadvance-m/visualboyadvance-m/blob/beab4e0400160e645fc1b00fce275c74ed34d068/`
- `LVBM` = `https://github.com/libretro/vbam-libretro/blob/115defb3a318258ab84746d45258a1aec19d0b4b/`
- `NBA` = `https://github.com/nba-emu/NanoBoyAdvance/blob/55b5cf0ae3d929582ac5bfd486558173502b8354/`
- `SKY` = `https://github.com/skylersaleh/SkyEmu/blob/f4d3238bf42da483ed1b7cc35649398b2110e4b4/`
- `GPSP` = `https://github.com/libretro/gpsp/blob/5819380c2ffb0900219d700a382ee68c464ebb99/`
- `BGBA` = `https://github.com/libretro/beetle-gba-libretro/blob/b158166237b17253188cfdbe73a8a0b9fe4b3a8c/`
- `GEAR` = `https://github.com/drhelius/Gearboy/blob/997da465e1c89e8bcceddb6bf5812d8c6724392d/`
- `TGB` = `https://github.com/libretro/tgbdual-libretro/blob/0392c9c469e653205e471114c7949c07c83bfce9/`
- `AZ` = `https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/`
- `LCIT` = `https://github.com/libretro/citra/blob/a0483e9abe8134ae6b290cdca7d49cb77689f05b/`
- `P3DS` = `https://github.com/wheremyfoodat/Panda3DS/blob/92e53d26b898eb89d865f8407460900a88e7c0fa/`
- `ED` = `https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/`
- `RD` = `https://github.com/RetroDECK/RetroDECK/blob/7786a849179eb2ca94e8ca413259e28adbcb1e39/`
- `RDC` = `https://github.com/RetroDECK/components/blob/fcd0d72995d7aa2e741e2753ac6914025b2ff1ac/`
- `OAF` = `https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/`
- `GY` = `https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/`

Pages officielles consultées le 2026-11-01 :
- BGB : https://bgb.bircd.org/ , https://bgb.bircd.org/manual.html , https://bgb.bircd.org/rtcsave.html , https://bgb.bircd.org/mbc2save.html
- NO$GBA : https://problemkaputt.de/gba.htm
- Emulicious : https://emulicious.net/
- Flatpak (dossiers XDG dans le bac à sable) : https://docs.flatpak.org/en/latest/conventions.html
- Qt `QStandardPaths` : https://doc.qt.io/qt-6/qstandardpaths.html
- SDL `SDL_GetPrefPath` : https://wiki.libsdl.org/SDL2/SDL_GetPrefPath
- API Flathub (existence des identifiants) : `https://flathub.org/api/v2/appstream/<id>` (réponse 200 ou 404)
- Outil EEPROM : https://github.com/exelotl/gba-eeprom-save-fix/blob/fdc61d84c6b7779dd39786a70ee98739c9568001/index.html

---

## 1. RetroArch (règles générales)

### Réponse courte

- **Dossier.** Clé `savefile_directory` dans `retroarch.cfg`. Vide → dossier de la ROM. `savefiles_in_content_dir = true` → dossier de la ROM aussi.
- **Tri.** `sort_savefiles_enable` vaut **`true`** par défaut (depuis au moins v1.19.1). RetroArch ajoute alors un sous-dossier au nom `library_name` du cœur : `saves/mGBA/`, `saves/Gambatte/`, `saves/melonDS DS/`. `sort_savefiles_by_content_enable` (défaut `false`) ajoute avant le nom du dossier parent de la ROM (`saves/gba/` par exemple). Si les deux sont vrais : `saves/<dossier ROM>/<library_name>/`.
- **Nom.** `<nom de la ROM sans la dernière extension>.srm`. Le fichier `.rtc` a le même nom avec l'extension `.rtc` (la fonction `fill_pathname` remplace l'extension). Les cœurs qui écrivent eux-mêmes (DeSmuME, ancien melonDS, Azahar, Citra) reçoivent le **même dossier trié** par `RETRO_ENVIRONMENT_GET_SAVE_DIRECTORY`.
- **Format `.srm`.** Copie brute du bloc `RETRO_MEMORY_SAVE_RAM` exposé par le cœur. La taille dépend du cœur (voir §2 à §5).
- **Écriture.** `autosave_interval` vaut **10 s** par défaut sur Windows, macOS, x86 et Android. RetroArch écrit aussi à la fermeture du contenu.
- **Archives.**
  - Entrée de playlist `X.zip#Y.gba` (release v1.22.2) : nom de base = `Y` (le membre).
  - Au `master` actuel : même règle, sauf si le cœur déclare `block_extract` ; alors le nom vient de l'archive (`X`). Aucun des cœurs étudiés ne déclare `block_extract = true`.
  - `retroarch -L <cœur> X.zip` (archive nue) : le nom de base est calculé sur le chemin donné, donc `X` (déduction ; NON VÉRIFIÉ par test).
- **Ligne de commande.** `retroarch -L <chemin du cœur> [-f] [--appendconfig=<fichier>] "<rom>"`. `-f` = plein écran. `--appendconfig` permet de surcharger `savefile_directory` pour une seule session. `-s/--save` existe mais est marqué DEPRECATED. `-e/--entryslot=N` charge un savestate au démarrage.
- **Savestate au lancement.** `savestate_auto_load = false` et `savestate_auto_save = false` par défaut. Avec `auto_save`, RetroArch écrit `<nom>.state.auto` à la sortie. Avec `auto_load`, il le recharge au lancement.
- **Danger.** `block_sram_overwrite = false` par défaut : charger un savestate remplace la SRAM en mémoire. 10 s plus tard, l'autosave écrit cette ancienne SRAM dans le `.srm` fraîchement récupéré.
- **Dossiers par défaut** (`savefile_directory = default`) :
  - Windows : `<dossier de retroarch.exe>\saves` (préfixe `:\`).
  - Linux natif : `$XDG_CONFIG_HOME/retroarch/saves`, sinon `~/.config/retroarch/saves`.
  - Linux Flatpak `org.libretro.RetroArch` : `~/.var/app/org.libretro.RetroArch/config/retroarch/saves` (Flatpak fixe `XDG_CONFIG_HOME` ; EmuDeck utilise ce chemin).
  - macOS : `~/Documents/RetroArch/saves`.
  - Les paquets de distribution Linux peuvent livrer un `retroarch.cfg` système différent : NON VÉRIFIÉ.

### Noms de dossier (`library_name`) des cœurs étudiés

| Cœur | `library_name` | `block_extract` | Extensions (`.info`) |
|---|---|---|---|
| melonDS DS | `melonDS DS` | `false` | `nds|ids|dsi` |
| melonDS (ancien) | `melonDS` | non déclaré | `nds|ids|dsi` |
| DeSmuME | `DeSmuME` | `false` | `nds|ids|bin` |
| DeSmuME 2015 | `DeSmuME 2015` | `false` | `nds|ids|bin` |
| mGBA | `mGBA` | `false` | `gb|gbc|gba` |
| Gambatte | `Gambatte` | `false` | `gb|gbc|dmg` |
| SameBoy | `SameBoy` | non déclaré | `gb|gbc` |
| Gearboy | `Gearboy` | non déclaré | `gb|dmg|gbc|cgb|sgb` |
| TGB Dual | `TGB Dual` | non déclaré | `cgb|dmg|gb|gbc|sgb` |
| gpSP | `gpSP` | `false` | `gba|bin` |
| Beetle GBA | `Beetle GBA` | non déclaré | `gba|agb|bin` |
| VBA-M | `VBA-M` | `false` | `dmg|gb|gbc|cgb|sgb|gba` |
| Citra | `Citra` | non déclaré | `3ds|3dsx|elf|axf|cci|cxi|app` |
| Azahar | `Azahar` | non déclaré | `3ds|3dsx|z3dsx|elf|axf|cci|zcci|cxi|zcxi|app` |

« Non déclaré » = le champ reste à zéro (`false`) dans `retro_system_info`.

### Sources

- Défauts `sort_savefiles_*`, `savefiles_in_content_dir` : `RA config.def.h#L1121-L1128`
- `autosave_interval` (10 s / 0) : `RA config.def.h#L1615-L1626`
- `block_sram_overwrite` : `RA config.def.h#L1685-L1687`
- `savestate_auto_save` / `savestate_auto_load`, fichier `.auto` : `RA config.def.h#L1728-L1733`
- Même défaut `sort_savefiles_enable true` en v1.22.2, v1.21.0, v1.19.1 : `https://github.com/libretro/RetroArch/blob/v1.22.2/config.def.h` (et tags `v1.21.0`, `v1.19.1`)
- Noms des clés : `RA intl/msg_hash_lbl.h#L2353-L2366` , `#L2477-L2490` , `#L267-L271` ; `savefiles_in_content_dir` : `RA configuration.c#L2989` ; `savefile_directory` : `RA configuration.c#L2221`
- Calcul du dossier trié et du nom `.srm` : `RA runloop.c#L11287-L11496`
- Nom de base, archives (`#`) : `RA runloop.c#L11208-L11248` ; nom d'archive pour `block_extract` : `RA runloop.c#L6544-L6570` et `#L6675-L6688`
- Ancienne règle en v1.22.2 (pas de `runloop_path_set_basename_to_archive`) : `https://github.com/libretro/RetroArch/blob/v1.22.2/runloop.c` (commentaire « The choice I take here is », L8123)
- Dossier passé aux cœurs : `RA runloop.c#L2714-L2718` et `RA retroarch.c#L3487-L3490`
- Fichier `.rtc` : `RA save.c#L676-L689` ; `fill_pathname` remplace l'extension : `RA libretro-common/file/file_path.c#L330-L351`
- Ligne de commande : `RA retroarch.c#L7852` (`-c`), `#L7876` (`--appendconfig`), `#L7904` (`-L`), `#L7941` (`-f`), `#L8066-L8072` (`-e`, `-s` DEPRECATED)
- Auto-chargement différé : `RA runloop.c#L11030-L11071`
- Défauts de dossier : Windows `RA frontend/drivers/platform_win32.c#L1334-L1335` ; Linux `RA frontend/drivers/platform_unix.c#L3494-L3511` et `#L3705-L3706` ; macOS `RA frontend/drivers/platform_darwin.m#L440-L456`
- `library_name` / `block_extract` : `MDS src/libretro/libretro.cpp#L172-L173` + `MDS CMakeLists.txt#L21` ; `LMEL src/libretro/libretro.cpp#L114` ; `LDSM desmume/src/frontend/libretro/libretro.cpp#L517-L525` ; `LDSM15 desmume/src/libretro/libretro.cpp#L399-L407` ; `LMG src/platform/libretro/libretro.c#L1341-L1342` + `LMG version.cmake#L1-L2` ; `GBT libgambatte/libretro/libretro.cpp#L1655-L1665` ; `LSB libretro/libretro.c#L1292` ; `GEAR platforms/libretro/libretro.cpp#L246` + `GEAR src/definitions.h#L44` ; `TGB libretro/libretro.cpp#L101` ; `GPSP libretro/libretro.c#L579-L586` + `GPSP gpsp_config.h#L5` ; `BGBA libretro.cpp#L72` et `#L523` ; `LVBM src/libretro/libretro.cpp#L527-L528` ; `LCIT src/citra_libretro/environment.cpp#L180` ; `AZ src/citra_libretro/environment.cpp#L269`
- Fichiers `.info` : `CI melondsds_libretro.info`, `CI melonds_libretro.info`, `CI desmume_libretro.info`, `CI desmume2015_libretro.info`, `CI mgba_libretro.info`, `CI gambatte_libretro.info`, `CI sameboy_libretro.info`, `CI gearboy_libretro.info`, `CI tgbdual_libretro.info`, `CI gpsp_libretro.info`, `CI mednafen_gba_libretro.info`, `CI vbam_libretro.info`, `CI citra_libretro.info`, `CI azahar_libretro.info`

---

## 2. DS

### 2.1 melonDS (standalone)

**Réponse courte.**
- Lancement : `melonDS "<rom>"` ; `-f/--fullscreen` ; `-b/--boot auto|always|never` ; `-a/--archive-file <nom>` choisit le fichier dans une archive.
- Processus : `melonDS.exe` (Windows), `melonDS` (Linux, nom de sortie CMake). Un seul processus pour toutes les fenêtres (voir research.md §2).
- Config : `melonDS.toml`, table `[Instance0]`, clé `SaveFilePath` (vide = dossier de la ROM). Pas d'option de ligne de commande pour le dossier.
- Emplacement de `melonDS.toml` :
  - dossier `portable` à côté de l'exécutable s'il existe ;
  - build Windows `WIN32_PORTABLE` : à côté de `melonDS.exe` ;
  - sinon `QStandardPaths::ConfigLocation` + `/melonDS` : Linux `~/.config/melonDS`, macOS `~/Library/Preferences/melonDS` (table Qt).
  - Flatpak `net.kuribo64.melonDS` : `~/.var/app/net.kuribo64.melonDS/config/melonDS/melonDS.toml` (chemin utilisé par EmuDeck).
- Nom : `<SaveFilePath ou dossier ROM>/<nom de la ROM sans extension>.sav`.
- **Archive** : la sauvegarde prend le nom du **fichier ROM dans l'archive**, dans le dossier de l'archive. Ce point était NON VÉRIFIÉ dans research.md ; il est maintenant lu dans le code.
- Format : SRAM brute, taille fixée par la base `ROMList` (research.md §2). Identique au `.sav` de TWiLight.
- Savestate : aucun chargement automatique au lancement (aucune option trouvée). Charger un `.mlN` réécrit le `.sav` (research.md §2).
- `melonDS.ini` (ancien format) n'est lu que si `melonDS.toml` manque.

**Sources.**
- Dossier `portable`, `ConfigLocation` : `MEL src/frontend/qt_sdl/main.cpp#L178-L213`
- `getAssetPath` : `MEL src/frontend/qt_sdl/EmuInstance.cpp#L451-L490`
- Nom dans une archive : `MEL src/frontend/qt_sdl/EmuInstance.cpp#L1820-L1846` et `#L1863-L1881`
- CLI : `MEL src/frontend/qt_sdl/CLI.cpp#L41-L56`
- Clés : `MEL src/frontend/qt_sdl/Config.cpp#L302-L304` ; `melonDS.ini` hérité : `#L44` , `#L682-L795`
- Nom de l'exécutable : `MEL src/frontend/qt_sdl/CMakeLists.txt#L95` et `#L240`
- Flathub `net.kuribo64.melonDS` : API Flathub, réponse 200
- Table Qt : https://doc.qt.io/qt-6/qstandardpaths.html

### 2.2 melonDS DS (cœur RetroArch, recommandé)

- Le cœur alloue un tampon de la taille SRAM de la cartouche et l'expose par `RETRO_MEMORY_SAVE_RAM`. RetroArch écrit donc `saves/melonDS DS/<rom>.srm`.
- Format : SRAM brute (même taille que melonDS standalone). Pas de `.rtc`.
- Le cœur installe la SRAM au premier `retro_run`.
- Les titres DSiWare écrivent `<jeu>.public.sav`, `<jeu>.private.sav`, `<jeu>.banner.sav` : hors périmètre.

Sources : `MDS src/libretro/core/core.cpp#L1165-L1205` ; `MDS src/libretro/core/core.cpp#L472-L490` ; `MDS src/libretro/sram.cpp#L100-L140` ; `MDS src/libretro/config/console.cpp#L1065-L1085`.

### 2.3 melonDS (ancien cœur libretro)

- Le cœur écrit lui-même `<dossier de sauvegarde RetroArch>/<nom ROM sans extension>.sav`. Le dossier est le dossier trié : `saves/melonDS/<rom>.sav`.
- Pas d'extension `.srm`. Format : SRAM brute (même moteur melonDS ancien).
- Dernier commit : 2026-07-19.

Sources : `LMEL src/libretro/libretro.cpp#L85-L95` , `#L887-L895` , `#L119-L120` (`need_fullpath = false`).

### 2.4 DeSmuME (standalone)

**Réponse courte.**
- Version : dernière release 0.9.13 (2022-05-23). Le `master` reste actif (2026-10-25).
- Lancement : `<exe> [options] "<rom>"`. Une seule ROM positionnelle. Windows : `--windowed-fullscreen`. Linux : pas d'option plein écran dans la table. `--load-slot N` charge un savestate.
- Processus : release Windows 0.9.13 : `DeSmuME_0.9.13_x64.exe` (contenu du zip `desmume-0.9.13-win64.zip`, relevé le 2026-11-02). Linux GTK : `desmume` (NON VÉRIFIÉ). Flathub `org.desmume.DeSmuME` existe.
- **Nom et dossier.**
  - Windows : clé `Battery` de la section `[PathSettings]` de `desmume.ini`. Défaut : `.\Battery` relatif au dossier de l'exécutable.
  - Linux (GTK) : `g_get_user_config_dir()/desmume/`, donc `~/.config/desmume/<rom>.dsv` (déduction : aucune autre affectation trouvée).
  - macOS (Cocoa) : NON VÉRIFIÉ.
  - Nom : `<nom ROM sans extension>.dsv`. Avec une archive sous Windows, le nom logique est `C:\jeux.zip|Sonic.nds` : la sauvegarde prend le nom du **fichier interne** (`Sonic.dsv`).
- **Format `.dsv` = SRAM + remplissage + pied de page.**
  - Texte lisible de 82 octets : `|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:`.
  - Puis 6 × `u32` petit-boutiste : taille écrite, taille remplie (`padSize`), type, taille d'adresse, taille mémoire, version (0).
  - Puis le cookie de 16 octets `|-DESMUME SAVE-|`.
  - Pied total : 82 + 24 + 16 = **122 octets**.
  - DeSmuME lit comme SRAM tout ce qui précède le pied (taille du fichier - 122), égal à la taille remplie. La taille écrite est l'adresse qui a fait grandir le fichier : elle reste souvent en deçà de la puce (`mc.cpp` `BackupDevice::ensure`, chargement `#L344-L390`). Import d'un `.sav` brut : type et taille d'adresse pris dans `save_types` selon la taille (`mc.cpp#L300-L320`).
- Si le `.dsv` manque, DeSmuME lit un `.sav` brut (ou NO$GBA) du même nom et le convertit en `.dsv`.
- Option `backupSave` (défaut `false`) : copie `<rom>.dsv.bak` au chargement.
- Savestate au lancement : aucune option automatique (seulement Lua `AutoLoad`, défaut `false`, et `--load-slot`).

**Sources.**
- Pied de page : `DSM desmume/src/mc.cpp#L72-L73` , `#L969-L978` ; structure `DSM desmume/src/mc.h#L69-L75` ; taille `DSM desmume/src/mc.cpp#L1756-L1759`
- Nom `.dsv`, `.dsv.bak`, import `.sav` : `DSM desmume/src/mc.cpp#L232-L310`
- Dossiers : `DSM desmume/src/path.cpp#L192-L232` ; clés `DSM desmume/src/path.h#L77-L80`
- Nom de ROM après `|` : `DSM desmume/src/path.cpp#L178-L187` et `#L485-L503`
- Archive Windows : `DSM desmume/src/frontend/windows/OpenArchive.cpp#L401-L481`
- CLI : `DSM desmume/src/commandline.cpp#L263-L338` , `#L476-L479`
- `backupSave`, Lua `AutoLoad` : `DSM desmume/src/frontend/windows/main.cpp#L1948` , `#L2031`

### 2.5 DeSmuME et DeSmuME 2015 (cœurs RetroArch)

- Les deux cœurs **n'exposent pas** `RETRO_MEMORY_SAVE_RAM`. Ils écrivent eux-mêmes `<dossier RetroArch trié>/<rom>.dsv` (`saves/DeSmuME/` ou `saves/DeSmuME 2015/`).
- Format : `.dsv` avec le pied de 122 octets (même code `mc.cpp`).

Sources : `LDSM desmume/src/frontend/libretro/libretro.cpp#L2473-L2490` ; `LDSM desmume/src/path.cpp#L196-L232` ; `LDSM desmume/src/mc.cpp#L234-L235` (pied : `LDSM desmume/src/mc.cpp#L961-L970`) ; `LDSM15 desmume/src/path.h#L121-L141` ; `LDSM15 desmume/src/mc.cpp#L240`.

### 2.6 NO$GBA (Windows)

- Version 3.06 (14 avril 2025). Windows et DOS seulement. Code fermé. Source : page officielle.
- La page officielle ne dit rien des sauvegardes, de la ligne de commande ni des zip.
- Format (lu dans le code de DeSmuME qui l'importe, pas chez l'auteur) : en-tête `NocashGbaBackupMediaSavDataFile` + `0x1A`, puis `SRAM` à `0x40`, méthode `u32` à `0x44` : 0 = brut (données à `0x4C`), 1 = compression RLE (données à `0x50`).
- NO$GBA peut aussi écrire des `.sav` bruts : NON VÉRIFIÉ. Dossier (`BATTERY\`) : NON VÉRIFIÉ.

Sources : https://problemkaputt.de/gba.htm ; `DSM desmume/src/mc.cpp#L1132-L1180` (lecture de l'en-tête) et `#L1232-L1290` (décompression).

---

## 3. GB / GBC

### 3.1 mGBA (standalone, GB/GBC/GBA)

Rappel de research-v2 (0.10.5), toujours vrai au `master` :
- Nom : nom du **zip** sans extension (pas le fichier interne). Zip et 7z selon les options de build (`USE_LIBZIP`/`USE_MINIZIP`, `USE_LZMA`).
- Dossier : clé `savegamePath` de `config.ini`, ou `-C savegamePath=<dossier>` (surcharge de session, priorité maximale). Vide = dossier de la ROM.
- Trailer RTC : 48 octets GB (MBC3+TIMER), 16 octets GBA. GB MBC2 : **256 octets** (voir §8).
- `autoload` vaut **`true`** : au reset, mGBA charge le slot 0 (`.ss0`) sans la SRAM (drapeaux `SCREENSHOT | RTC`), mais la RAM du jeu revient en arrière. Clé `autoload` dans `config.ini`. Le port 3DS (`gui-runner`) a le même défaut.

Nouveau au `master` (0.11-dev) :
- La forme longue `--config` existe maintenant dans la table d'options (elle manquait en 0.10.5). `-C` reste sûr pour les deux versions.
- Dossier de config : `portable.ini` à côté de l'exécutable (Windows) ou dans le dossier courant (autres) → mode portable. Sinon Windows `%APPDATA%\mGBA`, autres systèmes `$XDG_CONFIG_HOME/mgba` ou `~/.config/mgba` (**y compris macOS** : pas de branche Apple dans `mCoreConfigDirectory`).
- Flatpak `io.mgba.mGBA` : `~/.var/app/io.mgba.mGBA/config/mgba/config.ini` (déduction : code + règle Flatpak).
- Exécutable : `mGBA.exe` (Windows), `mGBA.app` (macOS), `mgba-qt` (Linux). `-f/--fullscreen`.

Sources : `MG src/core/config.c#L235-L330` ; `MG src/feature/commandline.c#L29-L50` ; `MG src/platform/qt/CoreController.cpp#L42-L62` et `#L297-L305` ; `MG src/feature/gui/gui-runner.c#L231` ; `MG src/platform/qt/CMakeLists.txt#L455` et `#L532-L534` ; `MG src/util/vfs.c#L107-L121` ; `MG src/gb/mbc.c#L441-L446` ; research-v2.md Partie 3 §1.

### 3.2 SameBoy (standalone)

- Version v1.0.4 (2026-10-09). Frontends : SDL (Windows, Linux), Cocoa (macOS), iOS.
- Lancement SDL : `sameboy [--fullscreen|-f] [--nogl] [--model <modèle>] "<rom>"`.
- Nom : `.sav` **à côté de la ROM**, extension remplacée. Pas d'option de dossier dans le frontend SDL. Fichier `.isx` → `.ram`.
- Zip : pas de lecture d'archive dans le chargeur SDL (seulement `GB_load_rom` sur le fichier). Déduction ; NON VÉRIFIÉ pour Cocoa.
- Format : SRAM brute + trailer RTC **48 octets au format BGB/VBA-M** (`vba64`) pour MBC3 avec horloge. HuC3 et TPP1 ont leurs propres trailers. MBC2 : 512 octets.
- Écriture : `fopen("wb")` puis écriture complète (non atomique).
- Préférences : `prefs.bin` dans `SDL_GetPrefPath("", "SameBoy")` (ou à côté de l'exécutable).
- Savestate au lancement : aucune option trouvée (SDL, Cocoa).
- Flathub `io.github.sameboy.SameBoy` existe. Dossier de la ROM dans le bac à sable : NON VÉRIFIÉ.

Sources : `SB SDL/main.c#L1205-L1250` , `#L1454-L1462` , `#L1490-L1493` ; `SB Core/gb.c#L776-L813` (structures), `#L826-L845` (taille), `#L896-L960` (écriture) ; `SB Core/mbc.c#L205-L213`.

### 3.3 Gambatte

- **Standalone** : le dépôt `sinamas/gambatte` ne contient plus qu'un fichier : « This project has been taken private ». Pas de profil v3.
- **Cœur RetroArch** : `.srm` = SRAM brute (seulement si la cartouche a une pile). `.rtc` = 8 octets (`uint64` `baseTime`, ordre natif de la machine). HuC3 : sa propre structure.

Sources : `sinamas/gambatte` commit `4f0157e843c00105a2154a44d4760e49003c2c20` (fichier unique) ; `GBT libgambatte/libretro/libretro.cpp#L2877-L2916` ; `GBT libgambatte/src/mem/cartridge_libretro.cpp#L43-L92` ; `GBT libgambatte/src/mem/rtc.h#L40-L82`.

### 3.4 BGB (Windows, Wine)

- Version 1.6.6 (2025-07-20). Windows et Wine. Code fermé.
- Dossier : réglage « Save files » (onglet Misc). Vide = dossier de la ROM.
- Ligne de commande : `-rom <fichier>`, `-ini <fichier>`, `-nowriteini`, `-setting nom=valeur`, `-nobatt`, `-nobattsave`, `-loadbatt <fichier>`. Pas d'option plein écran documentée.
- Zip et GZIP : lus. Nom du `.sav` pour un zip : NON VÉRIFIÉ.
- Trailer RTC : 48 octets (44 en ancien format VBA), dwords petit-boutistes, horodatage Unix 64 bits. C'est la référence du format.
- MBC2 : 512 octets, valeur dans le quartet bas.
- Emplacement de `bgb.ini` : NON VÉRIFIÉ.

Sources : https://bgb.bircd.org/ , https://bgb.bircd.org/manual.html , https://bgb.bircd.org/rtcsave.html , https://bgb.bircd.org/mbc2save.html.

### 3.5 Emulicious

- Java ; Windows, Linux, macOS (page officielle). Pas de version affichée ; dernière nouvelle datée 2026-03-19.
- Dossier, nom, format et ligne de commande : **NON VÉRIFIÉ** (aucune documentation officielle trouvée).

Source : https://emulicious.net/.

### 3.6 VisualBoyAdvance-M (standalone, GB/GBC/GBA)

- Version v2.2.3 (2025-10-25). Exécutable `visualboyadvance-m(.exe)`.
- Lancement : `visualboyadvance-m [-f] [-c <config>] "<rom>" [Option=Valeur ...]`. Un paramètre `Nom=Valeur` règle une option (par ex. `General/BatteryDir=D:/Saves`). Persistance de cette valeur dans `vbam.ini` : NON VÉRIFIÉ. L'option `-o/--option` est commentée dans le code.
- `--print-cfg-path` affiche le chemin de la configuration : le client peut l'appeler au lieu de deviner.
- Config : `vbam.ini`. Linux GTK : `$XDG_CONFIG_HOME/visualboyadvance-m/` (ou `~/.vbam/` s'il existe). Autres : candidats wx (`GetUserLocalDataDir`, `GetUserDataDir`, dossier de l'exe…). Ordre exact par système : NON VÉRIFIÉ.
- Dossier : `General/BatteryDir`. Vide = dossier de la ROM. Chemin relatif = relatif à l'exécutable. `%s` est remplacé par `GameBoy`, `GameBoy Advance` ou `Super GameBoy`.
- Nom : `<nom du fichier chargé sans extension>.sav` ; pour un zip, c'est le nom du **zip** (déduction : `loaded_game` = chemin ouvert, extension retirée).
- Format GB : SRAM brute + trailer MBC3 de **48 octets** (10 `int` + `uint64`), même agencement que BGB. MBC2 : 512 octets. Lecture aussi en gzip.
- Format GBA : données brutes (Flash 64/128 Kio, SRAM 32 Kio, EEPROM 512 o/8 Kio). Pas de trailer RTC.
- `General/AutoLoadLastState` vaut **`false`** par défaut. Si l'option charge un état, VBA-M **ne lit pas** le `.sav` du tout.
- Flathub : l'identifiant AppStream du dépôt (`org.visualboyadvance_m.visualboyadvance_m`) donne 404 sur Flathub. Paquet Flatpak : NON VÉRIFIÉ.

Sources : `VBM src/wx/panel.cpp#L1195-L1215` , `#L1335-L1375` , `#L1575-L1600` , `#L870-L872` ; `VBM src/wx/config/internal/option-internal.cpp#L309` , `#L455-L456` , `#L682-L685` ; `VBM src/wx/wxvbam.cpp#L234-L235` , `#L455-L497` , `#L1480-L1512` , `#L1656-L1685` ; `VBM src/core/gb/gb.cpp#L143-L163` , `#L455-L470` ; `VBM src/core/gb/gbMemory.h#L22-L43` , `#L208` ; `VBM src/core/gb/gbCartData.cpp#L268-L282` ; `VBM src/core/gba/gba.cpp#L1801-L1830` ; `VBM src/wx/visualboyadvance-m.metainfo.xml`.

### 3.7 Cœurs GB RetroArch (résumé)

| Cœur | `.srm` | `.rtc` |
|---|---|---|
| Gambatte | SRAM brute | 8 octets (`uint64` natif) |
| SameBoy | SRAM brute (`mbc_ram`) | section RTC interne de SameBoy (format propre) |
| mGBA | SRAM brute (`sramSize`) | 48 octets (`GBMBCRTCSaveBuffer`, même struct que le trailer mGBA) |
| VBA-M | SRAM brute (`gbRamSize`) | `gb_rtcdata_size()` octets |
| Gearboy | SRAM brute | `GetRTCSize()` octets |
| TGB Dual | SRAM brute | `sizeof(fixed_time)` |

Pour tous : le `.srm` est exactement la SRAM attendue par la 3DS. Le `.rtc` reste local (règle ADR 0008).

Sources : `LSB libretro/libretro.c#L1708-L1745` ; `LMG src/platform/libretro/libretro.c#L2305-L2345` ; `LVBM src/libretro/libretro.cpp#L319-L360` ; `GEAR platforms/libretro/libretro.cpp#L1545-L1572` ; `TGB libretro/libretro/libretro.cpp#L534-L555` ; Gambatte : §3.3.

### 3.8 Gearboy et TGB Dual (standalone)

- Gearboy standalone : `-f/--fullscreen`, dossier `savefiles_dir_option` (« Default Location / Same as ROM / Custom Location ») et `savefiles_path`. Format exact du fichier : NON VÉRIFIÉ.
- TGB Dual : cœur RetroArch seulement dans cette étude.

Sources : `GEAR platforms/shared/desktop/main.cpp#L70` , `#L211` ; `GEAR platforms/shared/desktop/emu.cpp#L1502-L1511` ; `GEAR platforms/shared/desktop/gui_menus.cpp#L487-L512`.

---

## 4. GBA

### 4.1 mGBA

Voir §3.1. Trailer RTC GBA 16 octets, placé à `taille & ~0xFF`. Cœur RetroArch : `.srm` = `GBASavedataSize()` (sans trailer RTC). **Piège** : tant que le type n'est pas détecté (`GBA_SAVEDATA_AUTODETECT`), le cœur annonce 128 Kio. Un `.srm` écrit à ce moment peut faire 131072 octets (déduction).

Sources : `LMG src/platform/libretro/libretro.c#L2305-L2320` , `#L2053-L2054`.

### 4.2 VisualBoyAdvance-M

Voir §3.6 (GBA brut, pas de trailer). Cœur RetroArch : `.srm` = taille exacte du type (EEPROM, Flash, SRAM).

Source : `LVBM src/libretro/libretro.cpp#L319-L335`.

### 4.3 NanoBoyAdvance

- Version v1.8.3. Qt. Exécutable `NanoBoyAdvance` (NON VÉRIFIÉ pour le nom exact par système).
- Lancement : `NanoBoyAdvance "<rom>"` (premier argument). Le programme change le dossier courant vers celui de l'exécutable. Plein écran : clé `window.fullscreen` dans la config, pas d'option CLI.
- Dossier : clé `general.save_folder` de `config.toml`. Vide ou dossier absent → `.sav` à côté de la ROM.
- Nom : nom du fichier chargé avec l'extension `.sav` (zip, rar, 7z, tar acceptés par la boîte de dialogue ; nom = nom de l'archive, déduction).
- `config.toml` : macOS bundle → `~/Library/Application Support/org.github.fleroviux.NanoBoyAdvance/` ; build portable → dossier courant ; sinon `QStandardPaths::ConfigLocation/NanoBoyAdvance/`.
- Format : données brutes du type de sauvegarde. Trailer RTC : NON VÉRIFIÉ.

Sources : `NBA src/platform/core/src/loader/rom.cc#L23-L31` ; `NBA src/platform/qt/src/widget/main_window.cc#L926-L937` , `#L695` ; `NBA src/platform/qt/src/main.cc#L35-L60` ; `NBA src/platform/core/src/config.cc#L31` ; `NBA src/platform/qt/src/config.hh#L121-L142` ; `NBA src/platform/qt/src/config.cc#L42`.

### 4.4 SkyEmu (GB, GBC, GBA, NDS)

- Version v5. Lancement : `SkyEmu [fullscreen] "<rom>"` (le mot `fullscreen` en premier argument).
- Nom : `<dossier ROM>/<nom ROM sans extension>.sav`. Si ce fichier manque et que le réglage « save to path » est actif (ou que le fichier existe déjà là), SkyEmu utilise le dossier de sauvegarde des réglages.
- Réglages : `user_settings.bin` (binaire) dans `SDL_GetPrefPath("Sky", "SkyEmu")`. Pas de clé texte à modifier.
- Format : SRAM brute, sans trailer (GB : `ram_size` ; GBA : 512 o, 8 Kio, 32 Kio, 64 Kio ou 128 Kio).
- **Auto save state actif par défaut** (`auto_save_state_enable = true` depuis la version 4 des réglages) : écrit `<nom>.autosave.state.raw`. La restauration reste **manuelle** (bouton « Restore »).

Sources : `SKY src/main.c#L616-L626` , `#L2493-L2522` , `#L2628-L2639` , `#L2651-L2680` , `#L6050-L6057` , `#L7732-L7735` , `#L9012-L9016`.

### 4.5 NO$GBA

Voir §2.6.

### 4.6 Cœurs GBA RetroArch

- **mGBA** : voir §4.1.
- **VBA-M** : taille exacte.
- **gpSP** : `retro_get_memory_size(SAVE_RAM)` renvoie **toujours `0x20000`** (128 Kio). Le `.srm` fait 131072 octets quel que soit le jeu. Les données utiles sont au début. Un fichier rempli de `0x00` est remplacé par `0xFF`.
- **Beetle GBA** : tampon de `0x22000` octets. Après le chargement, la taille est devinée par le contenu (512 o, 8 Kio, 64 Kio, 128 Kio). Option `gba_use_mednafen_save_method` (défaut : méthode libretro).

Sources : `GPSP libretro/libretro.c#L1358-L1366` ; `GPSP gba_memory.c#L359-L360` , `#L3065-L3083` ; `BGBA libretro.cpp#L51-L55` , `#L92-L127` , `#L234-L241` , `#L655-L680`.

### 4.7 Ordre des octets de l'EEPROM GBA

- mGBA, VBA-M et gpSP rangent l'EEPROM de la même façon : octet = adresse de bit / 8, bit 7 en premier (lecture du code).
- open_agb_firm : « Most emulators output EEPROM saves differently » ; le README renvoie à l'outil d'exelotl.
- L'outil inverse les 8 octets de chaque bloc de 8 octets (`a b c d e f g h` → `h g f e d c b a`). La conversion est son propre inverse.
- Conséquence : un jeu GBA à EEPROM synchronisé entre open_agb_firm et un émulateur PC **demande cette conversion**. Les Pokémon GBA utilisent la Flash : pas concernés.

Sources : `MG src/gba/savedata.c#L507-L545` ; `VBM src/core/gba/gbaEeprom.cpp#L16-L62` ; `GPSP gba_memory.c#L586-L588` ; `OAF README.md#L221-L224` ; https://github.com/exelotl/gba-eeprom-save-fix/blob/fdc61d84c6b7779dd39786a70ee98739c9568001/index.html (L52-L69).

---

## 5. 3DS

### 5.1 Statut des projets

| Projet | Statut au 2026-11-01 | Source |
|---|---|---|
| Azahar | Actif. Release 2126.2 (2026-10-08). Fusion de forks de Citra. | API GitHub releases ; `CI azahar_libretro.info` (description) |
| Citra | Retiré. `github.com/citra-emu/citra` répond 404. Flathub `org.citra_emu.citra` : 404. | requête HTTP du 2026-11-01 |
| Lime3DS | Le dépôt `Lime3DS/Lime3DS` redirige vers `azahar-emu/azahar`. Flathub `io.github.lime3ds.Lime3DS` existe encore. | API GitHub (`full_name: azahar-emu/azahar`) ; API Flathub |
| Panda3DS | v0.9 (2024-12-25), commits jusqu'en 2026-10. | API GitHub |
| Cœur RetroArch « Citra » | Dépôt `libretro/citra`, dernier commit 2026-09-20. | clone |
| Cœur RetroArch « Azahar » | Dans le dépôt Azahar (`src/citra_libretro`) ; fichier `azahar_libretro.info` présent. Disponibilité sur le buildbot : NON VÉRIFIÉ. | `AZ src/citra_libretro/` ; `CI azahar_libretro.info` |

### 5.2 Azahar (standalone)

**Réponse courte.**
- Lancement : `azahar [-f|--fullscreen] [-w|--windowed] "<rom>"`. Le chemin doit être le dernier argument. Pas d'option de dossier utilisateur en CLI.
- Processus : `azahar(.exe)` (nom de sortie CMake de `citra_meta`).
- **Dossier utilisateur** (`<user>`) :
  - Windows : `<dossier de l'exe>\user\` s'il existe (portable), sinon `%APPDATA%\Azahar\`. Config : `<user>\config\qt-config.ini`.
  - Linux : `./user/` dans le dossier courant s'il existe, sinon `$XDG_DATA_HOME/azahar-emu/` (`~/.local/share/azahar-emu/`). Config : `$XDG_CONFIG_HOME/azahar-emu/qt-config.ini`.
  - macOS : `~/Library/Application Support/Azahar/` (sauf si des dossiers XDG `azahar-emu` existent déjà).
  - Flatpak `org.azahar_emu.Azahar` : `~/.var/app/org.azahar_emu.Azahar/data/azahar-emu/` (déduction : code + règle Flatpak).
  - AppImage : chemins Linux natifs (EmuDeck lit `~/.config/azahar-emu/qt-config.ini`).
- **SDMC** : `<user>/sdmc/`. Remplacé par `sdmc_directory` dans `[Data%20Storage]` si `use_custom_storage=true` et si le dossier existe.
- **Chemin de la sauvegarde (confirmé)** : `<sdmc>/Nintendo 3DS/00000000000000000000000000000000/00000000000000000000000000000000/title/<high 8 hex>/<low 8 hex>/data/00000001/main`. Les deux identifiants de 32 zéros sont des constantes. `00000001.metadata` (16 octets) est à côté du dossier.
- Archives : pas de zip. Extensions `.cci`, `.zcci`, `.bcci`, `.3ds`, `.cxi`, `.zcxi`, `.app`, `.3dsx`, `.z3dsx`, `.elf`, `.axf`.
- Savestate au lancement : aucun mécanisme trouvé.
- Détails d'écriture, title ID, `.metadata` : research-v2.md Partie 3 §2 (toujours valables au `master`).

**Sources.**
- `AZ src/common/common_paths.h#L19-L48` ; `AZ src/common/file_util.cpp#L938-L1030`
- `AZ src/core/hle/service/fs/archive.h#L22-L24`
- `AZ src/core/loader/loader.cpp#L48-L67`
- `AZ src/citra_meta/common_strings.h#L11-L31` ; `AZ src/citra_meta/CMakeLists.txt#L7`
- `AZ src/citra_qt/configuration/config.cpp#L502-L514` ; `AZ src/citra_qt/configuration/config.h#L20`
- Flathub `org.azahar_emu.Azahar` : API Flathub, réponse 200, développeur « Azahar Emulator Developers »

### 5.3 Cœurs RetroArch Azahar et Citra

- Option `citra_use_libretro_save_path` (« LibRetro Default » par défaut).
- Avec cette valeur, le cœur Azahar prend le dossier de sauvegarde RetroArch (déjà trié : `saves/Azahar/`), ajoute `Azahar/`, et en fait le dossier utilisateur. Le SDMC devient donc `saves/Azahar/Azahar/sdmc/` (déduction ; NON VÉRIFIÉ par test).
- Cœur Citra : même logique avec `Citra/` → `saves/Citra/Citra/sdmc/` (déduction).
- Le fichier `main` garde le même chemin relatif sous `sdmc/`.

Sources : `AZ src/citra_libretro/core_settings.cpp#L23-L25` , `#L82-L83` , `#L1080-L1110` ; `AZ src/citra_libretro/environment.cpp#L191-L199` ; `LCIT src/citra_libretro/citra_libretro.cpp#L157` , `#L446-L460`.

### 5.4 Panda3DS

- Dossier racine : `SDL_GetPrefPath(NULL, "Alber")` (Windows `%APPDATA%\Alber\`, Linux `~/.local/share/Alber/`, macOS `~/Library/Application Support/Alber/` selon la doc SDL), ou `<exe>/Emulator Files/` en mode portable.
- Sauvegarde : `<racine>/<nom du fichier ROM sans extension>/SaveData/main`. **Le dossier dépend du nom de fichier, pas du title ID.**
- Format de `SaveData` : fichiers ordinaires, plus un fichier d'info de format. Compatibilité binaire avec `main` d'Azahar : probable mais NON VÉRIFIÉ.

Sources : `P3DS src/emulator.cpp#L199-L237` ; `P3DS src/core/fs/archive_save_data.cpp#L12` , `#L181-L192` ; https://wiki.libsdl.org/SDL2/SDL_GetPrefPath.

---

## 6. Savestate restauré au lancement

| Émulateur | Défaut | Réglage | Effet sur une sauvegarde récupérée |
|---|---|---|---|
| mGBA (Qt et port 3DS) | **actif** | `autoload` (`config.ini`) ; `loadStateExtdata` pour les drapeaux | Slot 0 chargé au reset ; SRAM non remplacée (drapeaux par défaut), mais RAM ancienne : une sauvegarde en jeu écrase la Version récupérée |
| mGBA via EmuDeck | **actif** | `autoload=1` livré dans `configs/mgba/config.ini` | idem |
| RetroArch | inactif | `savestate_auto_load` (+ `savestate_auto_save`) | `.state.auto` chargé ; SRAM remplacée en mémoire (`block_sram_overwrite=false`), puis écrite dans `.srm` (autosave 10 s) |
| RetroArch via EmuDeck | inactif, **actif si l'utilisateur coche « AutoSave »** (`RAautoSave`) | `RetroArch_autoSaveOn` met les deux clés à `true` | idem |
| RetroArch via RetroDECK | inactif | `savestate_auto_load = "false"` livré | — |
| RetroArch `-e N` | à la demande | argument CLI | charge le slot N |
| VBA-M | inactif | `General/AutoLoadLastState` | si actif : état chargé **et `.sav` non lu** |
| SkyEmu | écriture auto **active**, restauration manuelle | `auto_save_state_enable` | aucun chargement automatique |
| melonDS | aucune option trouvée | — | charger un `.mlN` réécrit le `.sav` après 2 s |
| DeSmuME | aucune option (sauf `--load-slot`) | — | — |
| Azahar | aucune option trouvée | — | — |
| SameBoy, NanoBoyAdvance | aucune option trouvée | — | — |
| BGB | non documenté (`-state` en CLI) | — | — |

Recommandation : le client vérifie ces clés avant le lancement et avertit (ou refuse) si une restauration automatique est active.

Sources : `MG src/platform/qt/CoreController.cpp#L42-L62` , `#L297-L305` ; `MG src/feature/gui/gui-runner.c#L231` ; `ED configs/mgba/config.ini#L137` ; `RA config.def.h#L1685-L1733` ; `ED functions/EmuScripts/emuDeckRetroArch.sh#L2425-L2431` , `#L2523-L2528` ; `ED settings.sh#L69` ; `RDC retroarch/rd_assets/rd_config/retroarch.cfg#L3236-L3237` ; `VBM src/wx/panel.cpp#L1197-L1199` ; `VBM src/wx/config/internal/option-internal.cpp#L309` ; `SKY src/main.c#L6050-L6057` , `#L7732-L7735` ; `DSM desmume/src/frontend/windows/main.cpp#L2031` ; research.md §2 (melonDS) ; https://bgb.bircd.org/manual.html.

---

## 7. Steam Deck : EmuDeck et RetroDECK

### 7.1 EmuDeck (Linux)

- Racine : `emulationPath=~/Emulation`, `savesPath=~/Emulation/saves`, `storagePath=~/Emulation/storage`. L'utilisateur peut choisir la carte SD : les modèles de config contiennent `/run/media/mmcblk0p1/Emulation`, remplacé par `emulationPath`.
- `linkToSaveFolder <émulateur> <nom> <cible>` crée le lien `~/Emulation/saves/<émulateur>/<nom>` → `<cible>`.

| Émulateur (type) | Config | Sauvegardes |
|---|---|---|
| melonDS (Flatpak `net.kuribo64.melonDS`) | `~/.var/app/net.kuribo64.melonDS/config/melonDS/melonDS.toml` | `SaveFilePath = ~/Emulation/saves/melonds/saves` (dossier réel) ; états `…/melonds/states` |
| mGBA (AppImage `~/Applications/mGBA.AppImage`) | `~/.config/mgba/config.ini` | `savegamePath=~/Emulation/saves/mgba/saves` ; `autoload=1` |
| Azahar (AppImage `~/Applications/azahar.AppImage`) | `~/.config/azahar-emu/qt-config.ini` | `use_custom_storage=true`, `sdmc_directory=~/Emulation/storage/azahar/sdmc/` ; lien `~/Emulation/saves/azahar/saves` → ce SDMC |
| RetroArch (Flatpak `org.libretro.RetroArch`) | `~/.var/app/org.libretro.RetroArch/config/retroarch/retroarch.cfg` | `savefile_directory = ~/Emulation/saves/retroarch/saves` ; lien vers `~/.var/app/org.libretro.RetroArch/config/retroarch/saves` ; **`sort_savefiles_enable = "false"`** livré → `.srm` directement dans ce dossier |

- EmuDeck cherche aussi un ancien SDMC Flatpak sous `~/.var/app/io.github.azahar.Azahar/…`. Cet identifiant donne 404 sur Flathub (l'identifiant officiel est `org.azahar_emu.Azahar`).
- EmuDeck pour Windows : NON VÉRIFIÉ (autre dépôt, non lu).

Sources : `ED settings.sh#L79-L84` ; `ED vars.sh#L1-L6` ; `ED functions/helperFunctions.sh#L344-L349` , `#L480-L509` ; `ED functions/EmuScripts/emuDeckMelonDS.sh#L1-L8` , `#L83-L95` ; `ED configs/net.kuribo64.melonDS/config/melonDS/melonDS.toml#L11-L14` ; `ED functions/EmuScripts/emuDeckMGBA.sh#L4-L6` , `#L72-L80` ; `ED configs/mgba/config.ini#L137` , `#L161` , `#L183` ; `ED functions/EmuScripts/emuDeckAzahar.sh#L4-L7` , `#L67-L108` , `#L176-L195` , `#L219-L223` ; `ED functions/EmuScripts/emuDeckRetroArch.sh#L4-L8` , `#L232-L236` ; `ED configs/org.libretro.RetroArch/config/retroarch/retroarch.cfg#L48-L49` , `#L3134-L3138` , `#L3176-L3177`.

### 7.2 RetroDECK

- Un seul Flatpak `net.retrodeck.retrodeck` (Flathub : 200). Chaque émulateur est un « component » lancé à l'intérieur.
- Racine par défaut : `~/retrodeck/` ; `saves_path=~/retrodeck/saves`, `states_path=~/retrodeck/states`.
- Les configs sont sous `$XDG_CONFIG_HOME` du Flatpak, donc `~/.var/app/net.retrodeck.retrodeck/config/` (déduction : règle Flatpak).

| Composant | Config | Sauvegardes |
|---|---|---|
| melonDS | `…/config/melonDS/melonDS.toml` | `SaveFilePath = ~/retrodeck/saves/nds/melonds` |
| Azahar | `…/config/azahar-emu/qt-config.ini` | `use_custom_storage=true`, `sdmc_directory=~/retrodeck/saves/n3ds/azahar/sdmc/` ; Azahar lancé avec `--f` |
| RetroArch | `…/config/retroarch/retroarch.cfg` | `savefile_directory=~/retrodeck/saves` ; **`sort_savefiles_by_content_enable = "true"`**, `sort_savefiles_enable = "false"` → `~/retrodeck/saves/<dossier ROM>/<jeu>.srm` (par ex. `saves/gba/`) |

- Pas de composant mGBA standalone dans la liste actuelle : le GBA passe par RetroArch.

Sources : `RD config/retrodeck/retrodeck.json#L3-L8` ; `RDC melonds/component_functions.sh#L3` ; `RDC melonds/component_prepare.sh#L15` , `#L30-L31` ; `RDC melonds/component_launcher.sh` ; `RDC azahar/component_functions.sh#L3-L4` ; `RDC azahar/component_prepare.sh#L16-L23` ; `RDC azahar/component_launcher.sh` ; `RDC retroarch/component_functions.sh#L3` ; `RDC retroarch/component_prepare.sh#L27-L28` ; `RDC retroarch/rd_assets/rd_config/retroarch.cfg#L49` , `#L3233-L3238` , `#L3275-L3276` ; liste des composants : API GitHub `RetroDECK/components` (arbre `HEAD`).

### 7.3 Détection du processus sous Linux

- Un Flatpak ou un AppImage lance un binaire dont le nom reste visible sur l'hôte (`retroarch`, `melonDS`, `azahar`). Nom exact vu par `/proc` pour un AppImage ou un composant RetroDECK : NON VÉRIFIÉ.
- Conseil : le client attend la fin du **processus qu'il a lancé lui-même** (PID enfant), et ne cherche par nom qu'en mode « tous les jeux ».

---

## 8. Écarts de format qui touchent les règles actuelles

### 8.1 Sauvegardes de 512 octets et règle de l'ADR 0008

- L'ADR 0008 calcule la SRAM par `taille - (taille mod 1024)`.
- Pour une sauvegarde de 512 octets, cette règle donne **0**. Cas réels :
  - GBA EEPROM 4 Kbit = 512 octets (`GBA_SIZE_EEPROM512 = 0x200` chez mGBA ; `SIZE_EEPROM_512` chez VBA-M ; 512 chez SkyEmu).
  - GB MBC2 = 512 octets chez SameBoy, VBA-M et BGB.
- Proposition (à décider en ADR) : retirer un trailer seulement si `taille mod 512 ∈ {16, 44, 48}`, sinon garder le fichier entier.

### 8.2 GB MBC2 : trois tailles différentes

- mGBA : **256 octets** (`sramSize = 0x100`, deux valeurs par octet).
- SameBoy, VBA-M, BGB : **512 octets** (une valeur par octet).
- GameYob (TWiLight) : `numRamBanks = 1` pour MBC2 → fichier de **8 Kio**. Agencement interne : NON VÉRIFIÉ.
- Conséquence : un jeu MBC2 n'a pas le même Hash d'une machine à l'autre. Il faut une conversion, ou exclure MBC2 en v3.

### 8.3 GameYob et les petites SRAM

- GameYob alloue `numRamBanks × 0x2000` octets ; les tailles d'en-tête 1 et 2 donnent toutes deux 1 banque (8 Kio).
- Un jeu à 2 Kio de SRAM aurait donc 8 Kio sur la 3DS et 2 Kio dans mGBA (taille de l'en-tête). NON VÉRIFIÉ pour la taille exacte côté mGBA ; à tester avant d'activer ces jeux.

Sources : `MG include/mgba/internal/gba/memory.h#L74-L75` ; `MG src/gb/mbc.c#L441-L446` ; `SB Core/mbc.c#L205-L213` ; `VBM src/core/gb/gbCartData.cpp#L268-L282` ; `VBM src/core/gba/gbaEeprom.cpp#L16-L29` ; https://bgb.bircd.org/mbc2save.html ; `GY arm9/source/inputhelper.cpp#L896-L939` ; ADR 0008.

---

## (a) Profils prêts à convertir en configuration

Champs : `exe` (Windows / Linux / macOS), `lancer`, `plein_écran`, `processus`, `dossier_défaut`, `réglage_dossier`, `surcharge_cli`, `nom`, `format`, `rtc`, `archive`, `autoload`. « ROM » = nom du fichier ROM sans sa dernière extension.

### melonDS (DS)

| Champ | Valeur |
|---|---|
| exe | `melonDS.exe` / `melonDS` (Flatpak `net.kuribo64.melonDS`) / `melonDS.app` |
| lancer | `melonDS "<rom>"` |
| plein_écran | `-f` |
| processus | `melonDS.exe` / `melonDS` |
| dossier_défaut | dossier de la ROM |
| réglage_dossier | `melonDS.toml` → `[Instance0] SaveFilePath` ; `melonDS.toml` à côté de l'exe (Windows portable, ou dossier `portable/`), `~/.config/melonDS/`, `~/Library/Preferences/melonDS/`, Flatpak `~/.var/app/net.kuribo64.melonDS/config/melonDS/` |
| surcharge_cli | aucune |
| nom | `ROM.sav` ; archive → nom du fichier interne |
| format | SRAM brute |
| rtc | aucun fichier |
| archive | zip/7z (via `-a` pour choisir) ; nom = fichier interne |
| autoload | aucun |
| EmuDeck / RetroDECK | `~/Emulation/saves/melonds/saves/` / `~/retrodeck/saves/nds/melonds/` |

### RetroArch (générique ; paramètre `cœur`)

| Champ | Valeur |
|---|---|
| exe | `retroarch.exe` / `retroarch` (Flatpak `org.libretro.RetroArch`) / `RetroArch.app` |
| lancer | `retroarch -L <fichier du cœur> "<rom>"` |
| plein_écran | `-f` |
| processus | `retroarch.exe` / `retroarch` |
| dossier_défaut | Windows `<exe>\saves` ; Linux `~/.config/retroarch/saves` ; Flatpak `~/.var/app/org.libretro.RetroArch/config/retroarch/saves` ; macOS `~/Documents/RetroArch/saves` ; vide → dossier de la ROM |
| réglage_dossier | `retroarch.cfg` : `savefile_directory`, `sort_savefiles_enable` (défaut `true`), `sort_savefiles_by_content_enable` (défaut `false`), `savefiles_in_content_dir` (défaut `false`) |
| sous-dossier | `<library_name>` si tri par cœur ; `<dossier parent de la ROM>` si tri par contenu |
| surcharge_cli | `--appendconfig=<fichier>` contenant `savefile_directory = "..."` |
| nom | `ROM.srm` ; playlist `X.zip#Y.gba` → `Y.srm` ; archive nue → `X.srm` (déduction) |
| format | selon le cœur (tableau suivant) |
| rtc | `ROM.rtc` (même dossier) pour les cœurs GB |
| autoload | `savestate_auto_load` (défaut `false`) ; `-e N` |
| EmuDeck | `~/Emulation/saves/retroarch/saves/ROM.srm` (tri désactivé) |
| RetroDECK | `~/retrodeck/saves/<dossier ROM>/ROM.srm` |

| Cœur | Fichier du cœur (préfixe) | `library_name` | Fichier | Conversion |
|---|---|---|---|---|
| melonDS DS | `melondsds_libretro` | `melonDS DS` | `ROM.srm` | aucune |
| melonDS (ancien) | `melonds_libretro` | `melonDS` | `ROM.sav` (écrit par le cœur) | aucune |
| DeSmuME / 2015 | `desmume_libretro` / `desmume2015_libretro` | `DeSmuME` / `DeSmuME 2015` | `ROM.dsv` | pied de 122 octets |
| mGBA | `mgba_libretro` | `mGBA` | `ROM.srm` (+ `ROM.rtc` 48 o en GB) | aucune (GBA : voir taille 128 Kio avant détection) |
| Gambatte | `gambatte_libretro` | `Gambatte` | `ROM.srm` (+ `.rtc` 8 o) | aucune |
| SameBoy | `sameboy_libretro` | `SameBoy` | `ROM.srm` (+ `.rtc`) | aucune |
| VBA-M | `vbam_libretro` | `VBA-M` | `ROM.srm` (+ `.rtc`) | aucune |
| Gearboy | `gearboy_libretro` | `Gearboy` | `ROM.srm` (+ `.rtc`) | aucune |
| TGB Dual | `tgbdual_libretro` | `TGB Dual` | `ROM.srm` (+ `.rtc`) | aucune |
| gpSP | `gpsp_libretro` | `gpSP` | `ROM.srm` (toujours 128 Kio) | tronquer / compléter `0xFF` |
| Beetle GBA | `mednafen_gba_libretro` | `Beetle GBA` | `ROM.srm` (taille devinée) | tronquer / compléter |
| Azahar | `azahar_libretro` | `Azahar` | `<saves>/Azahar/Azahar/sdmc/…/main` (déduction) | aucune |
| Citra | `citra_libretro` | `Citra` | `<saves>/Citra/Citra/sdmc/…/main` (déduction) | aucune |

Nom de fichier du cœur : `<préfixe>.dll` (Windows), `.so` (Linux), `.dylib` (macOS). Convention générale de libretro, NON VÉRIFIÉ pour chaque build.

### mGBA (GB, GBC, GBA)

| Champ | Valeur |
|---|---|
| exe | `mGBA.exe` / `mgba-qt` (Flatpak `io.mgba.mGBA`) / `mGBA.app` |
| lancer | `mGBA -C savegamePath=<dossier> "<rom>"` |
| plein_écran | `-f` |
| processus | `mGBA.exe` / `mgba-qt` |
| dossier_défaut | dossier de la ROM |
| réglage_dossier | `config.ini` → `savegamePath` ; `%APPDATA%\mGBA\`, `~/.config/mgba/` (Linux et macOS), Flatpak `~/.var/app/io.mgba.mGBA/config/mgba/`, ou `portable.ini` |
| surcharge_cli | `-C savegamePath=<dossier>` (session seulement) |
| nom | `ROM.sav` ; archive → nom de l'**archive** |
| format | SRAM + Trailer (GB MBC3+RTC 48 o ; GBA RTC 16 o) ; GB MBC2 256 o |
| rtc | dans le trailer |
| archive | zip (libzip/minizip), 7z (LZMA) selon le build |
| autoload | **`autoload` = true par défaut** (slot 0) |
| EmuDeck | `~/Emulation/saves/mgba/saves/` |

### Azahar (3DS)

| Champ | Valeur |
|---|---|
| exe | `azahar.exe` / `azahar` (Flatpak `org.azahar_emu.Azahar`, AppImage) / `Azahar.app` (NON VÉRIFIÉ) |
| lancer | `azahar "<rom>"` (chemin en dernier) |
| plein_écran | `-f` |
| processus | `azahar.exe` / `azahar` |
| dossier_défaut | SDMC = `<user>/sdmc/` ; `<user>` = `<exe>\user\` (portable) ou `%APPDATA%\Azahar\` ; `~/.local/share/azahar-emu/` ; `~/Library/Application Support/Azahar/` ; Flatpak `~/.var/app/org.azahar_emu.Azahar/data/azahar-emu/` |
| réglage_dossier | `qt-config.ini` `[Data%20Storage]` : `use_custom_storage=true` + `sdmc_directory=<dossier>` |
| surcharge_cli | aucune |
| nom | `<sdmc>/Nintendo 3DS/<32 zéros>/<32 zéros>/title/<high>/<low>/data/00000001/main` (hex minuscules) |
| format | fichier `main` tel que le jeu le voit ; ne jamais toucher `00000001.metadata` |
| rtc | — |
| archive | aucune (pas de zip) |
| autoload | aucun |
| EmuDeck / RetroDECK | `~/Emulation/storage/azahar/sdmc/` (lien `~/Emulation/saves/azahar/saves`) / `~/retrodeck/saves/n3ds/azahar/sdmc/` |

### VisualBoyAdvance-M (GB, GBC, GBA)

| Champ | Valeur |
|---|---|
| exe | `visualboyadvance-m.exe` / `visualboyadvance-m` / app macOS (NON VÉRIFIÉ) |
| lancer | `visualboyadvance-m "<rom>" General/BatteryDir=<dossier>` (persistance NON VÉRIFIÉ) |
| plein_écran | `-f` |
| processus | `visualboyadvance-m(.exe)` |
| dossier_défaut | dossier de la ROM |
| réglage_dossier | `vbam.ini` → `General/BatteryDir` (`%s` = système) ; chemin : `--print-cfg-path` |
| surcharge_cli | `-c <fichier ini>` ; paramètre `Nom=Valeur` |
| nom | `ROM.sav` ; archive → nom de l'archive (déduction) |
| format | GB : SRAM + trailer 48 o (MBC3 RTC) ; GBA : brut |
| rtc | dans le trailer (GB) |
| archive | zip, 7z (fex) |
| autoload | `General/AutoLoadLastState` = false |

### SameBoy (GB, GBC)

| Champ | Valeur |
|---|---|
| exe | `sameboy(.exe)` (SDL) / `SameBoy.app` (Cocoa) ; Flatpak `io.github.sameboy.SameBoy` |
| lancer | `sameboy "<rom>"` |
| plein_écran | `-f` / `--fullscreen` |
| processus | `sameboy(.exe)` (NON VÉRIFIÉ pour la casse exacte) |
| dossier_défaut | dossier de la ROM (fixe) |
| réglage_dossier | aucun (SDL) |
| surcharge_cli | aucune |
| nom | `ROM.sav` |
| format | SRAM + trailer 48 o (MBC3 RTC, format BGB) ; MBC2 512 o |
| archive | non |
| autoload | aucun |

### DeSmuME (DS) — plus tard

| Champ | Valeur |
|---|---|
| exe | Windows : nom de release NON VÉRIFIÉ ; Linux `desmume` (Flatpak `org.desmume.DeSmuME`) |
| lancer | `<exe> "<rom>"` ; `--windowed-fullscreen` (Windows) |
| dossier_défaut | Windows `<exe>\Battery\` ; Linux `~/.config/desmume/` |
| réglage_dossier | `desmume.ini` `[PathSettings] Battery` (Windows) |
| nom | `ROM.dsv` ; archive Windows → fichier interne |
| format | SRAM + remplissage + pied 122 o |
| autoload | aucun (`--load-slot` à la demande) |

### SkyEmu (GB, GBC, GBA) — plus tard

| Champ | Valeur |
|---|---|
| lancer | `SkyEmu [fullscreen] "<rom>"` |
| dossier_défaut | dossier de la ROM |
| réglage_dossier | `user_settings.bin` (binaire, `SDL_GetPrefPath("Sky","SkyEmu")`) |
| nom | `ROM.sav` |
| format | SRAM brute |
| autoload | écriture `.autosave.state.raw` active ; restauration manuelle |

### NanoBoyAdvance (GBA) — plus tard

| Champ | Valeur |
|---|---|
| lancer | `NanoBoyAdvance "<rom>"` |
| dossier_défaut | dossier de la ROM |
| réglage_dossier | `config.toml` → `general.save_folder` |
| nom | `ROM.sav` |
| format | brut (RTC NON VÉRIFIÉ) |

### BGB (GB, GBC ; Windows/Wine) — plus tard

| Champ | Valeur |
|---|---|
| lancer | `bgb -rom "<rom>"` (`-ini`, `-nowarn`) |
| dossier_défaut | dossier de la ROM |
| réglage_dossier | « Save files » (Misc) dans `bgb.ini` (clé exacte NON VÉRIFIÉ) |
| nom | `ROM.sav` (zip : NON VÉRIFIÉ) |
| format | SRAM + trailer 48 o ; MBC2 512 o |

### Panda3DS (3DS) — plus tard

| Champ | Valeur |
|---|---|
| dossier_défaut | `<SDL pref "Alber">/<nom du fichier ROM>/SaveData/` ou `<exe>/Emulator Files/…` |
| nom | `…/SaveData/main` |
| piège | dossier lié au **nom de fichier ROM**, pas au title ID |

---

## (b) Conversion de format ou SRAM brute

| Émulateur | Format côté PC | Action pour passer à la SRAM brute de la 3DS |
|---|---|---|
| melonDS, melonDS DS, ancien cœur melonDS | SRAM brute | aucune |
| DeSmuME (standalone et cœurs) | `.dsv` = SRAM + remplissage + pied 122 o | **conversion** : lire `padSize` dans le pied, couper ; à l'écriture, recréer le pied (ou écrire un `.sav` brut et retirer le `.dsv`, NON VÉRIFIÉ comme sûr) |
| NO$GBA | en-tête Nocash, RLE possible | **conversion** (décompression ; recompression ou mode brut NON VÉRIFIÉ) |
| mGBA standalone | SRAM + Trailer RTC | retirer / remettre le Trailer (déjà dans l'ADR 0008) |
| SameBoy, VBA-M (GB), BGB | SRAM + trailer 48 o | même règle que mGBA (même taille) |
| Cœurs GB RetroArch | `.srm` brut + `.rtc` à part | aucune ; garder `.rtc` local |
| VBA-M (GBA), cœur VBA-M, SkyEmu, NanoBoyAdvance | brut | aucune |
| Cœur mGBA (GBA) | brut ; 128 Kio possible avant détection | **contrôle de taille** |
| gpSP | 128 Kio fixes | **conversion** : couper à la taille du type ; compléter avec `0xFF` vers le PC |
| Beetle GBA | taille devinée (`0x22000` au départ) | **conversion** de taille |
| Tous les émulateurs PC ↔ open_agb_firm, **GBA EEPROM** | ordre d'octets différent | **conversion** : inverser chaque bloc de 8 octets |
| GB MBC2 | mGBA 256 o, autres 512 o, GameYob 8 Kio | **conversion** ou exclusion |
| Azahar, cœurs Azahar/Citra | `main` | aucune (chemin seulement) |
| Panda3DS | `main` probable | chemin par nom de ROM (pas de Clé directe) |

---

## (c) Recommandation v3

### Liste courte v3

1. **melonDS** (DS) — référence actuelle, SRAM brute, `SaveFilePath` simple, Flatpak et EmuDeck/RetroDECK documentés.
2. **mGBA** (GB, GBC, GBA) — référence actuelle, `-C savegamePath` par lancement ; il faut seulement avertir sur `autoload`.
3. **Azahar** (3DS) — seul émulateur 3DS actif ; chemin `main` confirmé ; `sdmc_directory` lisible dans `qt-config.ini`.
4. **RetroArch, profil générique** avec les cœurs **melonDS DS, mGBA, Gambatte, SameBoy, VBA-M** — `.srm` brut, `.rtc` séparé, un seul calcul de dossier (`savefile_directory` + tri). Couvre EmuDeck et RetroDECK, donc le Steam Deck.
5. **VisualBoyAdvance-M** — très répandu sous Windows ; format compatible mGBA (trailer 48 o) ; `--print-cfg-path` aide à trouver la config.
6. **SameBoy** standalone — coût presque nul : `.sav` toujours à côté de la ROM, trailer 48 o.

À faire en même temps (pré-requis pour ces profils) :
- corriger la règle de l'ADR 0008 pour les sauvegardes de 512 octets (§8.1) ;
- détecter et signaler un savestate restauré au lancement (§6) ;
- calculer le nom d'un jeu en archive selon l'émulateur (archive pour mGBA/VBA-M, fichier interne pour melonDS/DeSmuME, membre de playlist pour RetroArch).

### Plus tard

| Émulateur | Raison |
|---|---|
| DeSmuME (standalone, cœurs DeSmuME/2015) | pied `.dsv` à convertir dans les deux sens ; release ancienne (2022) |
| Cœurs gpSP, Beetle GBA | taille de `.srm` fixe ou devinée : conversion à écrire et à tester |
| Cœur Azahar / Citra | chemin `saves/<cœur>/<cœur>/sdmc` déduit, pas testé ; cœur Azahar récent |
| NO$GBA | code fermé, Windows seulement, format Nocash, aucune doc officielle sur les sauvegardes |
| BGB | code fermé, Windows/Wine, clé de dossier dans `bgb.ini` non documentée |
| Emulicious | aucune documentation officielle sur les sauvegardes |
| SkyEmu | réglages binaires (`user_settings.bin`), pas de dossier en CLI |
| NanoBoyAdvance | pas de CLI pour le dossier ; RTC non vérifié ; moins répandu |
| Gearboy, TGB Dual (standalone et cœurs) | peu utilisés ; cœurs déjà couverts par le profil RetroArch si besoin |
| Panda3DS | sauvegarde rangée par nom de fichier ROM ; projet jeune |
| Lime3DS, Citra standalone | projets arrêtés ; leurs utilisateurs passent à Azahar |
| Gambatte standalone | dépôt devenu privé |
| Jeux GBA à EEPROM, jeux GB MBC2 | conversion d'ordre d'octets ou de taille à écrire et tester d'abord |

## Questions ouvertes (NON VÉRIFIÉ, à tester sur machine)

- Nom de base RetroArch pour une archive nue passée en CLI (`X.zip`).
- Chemin réel du SDMC des cœurs Azahar et Citra.
- Nom du processus d'un AppImage et d'un composant RetroDECK vu depuis l'hôte.
- Persistance de `General/BatteryDir=` passé en paramètre à VBA-M.
- Taille de la sauvegarde mGBA pour un jeu GB à 2 Kio de SRAM, comparée aux 8 Kio de GameYob.
- Agencement MBC2 dans le fichier de 8 Kio de GameYob.
