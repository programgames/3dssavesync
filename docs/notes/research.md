# Phase 0 — Recherche

> Working notes (French), kept for the record of design decisions.

Date : 2026-10-31. Sources primaires (code source à un commit fixé, docs officielles, 3dbrew, GBATEK).
Légende : **NON VÉRIFIÉ** = aucune source primaire trouvée. « Déduit » = conclusion tirée du code, sans test sur matériel.

Ce fichier contient : la synthèse (ci-dessous), puis la partie 1 (formats de sauvegarde, melonDS, game code, bonus Pokémon), puis la partie 2 (réseau 3DS, build, outils existants).

## Synthèse

### Ce qui simplifie le projet

- **melonDS et TWiLight utilisent le même format** : dump brut, sans en-tête ni pied. Aucune conversion `.sav` ↔ `.dsv` n'est nécessaire (la partie DeSmuME du prompt d'origine disparaît).
- **Pokémon Gen 4 et Gen 5 = 512 KiB des deux côtés** (D/P/Pt/HG/SS/N/B/N2/B2, codes vérifiés dans les deux bases `ROMList`). Le fichier passe tel quel.
- **Les deux côtés nomment le `.sav` d'après le nom de fichier de la ROM**, sans extension. Le game code (offset `0x0C`) se lit dans la ROM, qui existe des deux côtés.
- `melonDS.exe <rom>` démarre le jeu directement, dans **un seul processus**. Un lanceur peut attendre la fin du processus.

### Ce qui impose des règles de sécurité

1. **Le `mtime` du `.sav` sur la 3DS ne change pas quand le jeu sauvegarde** (nds-bootstrap écrit les secteurs sans toucher l'entrée FAT — déduit du code). → Comparer des **hashes de contenu**. La décision « pas de dates » du prompt est confirmée.
2. **melonDS garde la sauvegarde en RAM et réécrit le fichier entier 2 à 3 s après chaque sauvegarde en jeu, puis à la fermeture.** Il ne verrouille pas le fichier. → Ne jamais lire ni écrire le `.sav` pendant que `melonDS.exe` tourne.
3. **L'écriture melonDS n'est pas atomique** (troncature puis écriture). → Avant un envoi, vérifier que la taille est la taille attendue (512 KiB).
4. **Charger un savestate melonDS réécrit le `.sav`** avec la SRAM du savestate. → Risque réel d'annuler une partie venue de la 3DS. À signaler dans le README.
5. **Taille différente = corruption possible** : melonDS tronque ou complète (`0xFF`) à la taille de sa base ; nds-bootstrap écrit modulo la taille du fichier. → Refuser de synchroniser un fichier dont la taille diffère de celle du serveur.
6. **Plusieurs `.sav` pour un même game code** : slots TWiLight `.sav1`..`.sav9`, copies de ROM sous un autre nom, instances melonDS `X.sav.2`. → Ne synchroniser que `<nom ROM>.sav` (slot 0) et signaler les doublons de game code.
7. **Game code ambigu** pour les ROM hacks (code du jeu de base) et les homebrews (`####`). → Exclure `####`/vide ; codes de région différents = jeux différents.

### Réseau 3DS

- **`httpc` (service système) ne marche pas avec le serveur de test.** Le module SSL de la 3DS s'arrête à TLS 1.1 (3dbrew). Test fait le 2026-10-31 sur `le serveur de test (port 443)` : le serveur accepte TLS 1.2 et 1.3, et **refuse toute suite sans ECDHE** (`handshake failure` avec `AES128-SHA:AES256-SHA:AES128-SHA256:AES256-SHA256`). ISRG Root X1 n'est pas dans les racines intégrées.
- Chaîne actuelle du serveur de test : `CN=<domaine>` (RSA) ← `YR2` ← `Root YR` ← `ISRG Root X1` (expire le 2027-01-06, renouvelé par Let's Encrypt).
- **`3ds-curl` 8.4.0 + `3ds-mbedtls` 2.28.8** (portlibs devkitPro) : TLS 1.2, ECDHE, SNI. Pas de CA livré → embarquer ISRG Root X1 avec `CURLOPT_CAINFO_BLOB`. Appeler `socInit` et `sslcInit`.
- En LAN en HTTP clair, `httpc` suffit, mais le jeton circule en clair et le corps doit tenir en mémoire partagée.

### Build

- L'image Docker `devkitpro/devkitarm` contient `libctru`, `3ds-curl` et `3ds-mbedtls`. Build : `docker run --rm -v ${PWD}:/project -w /project devkitpro/devkitarm make`.
- Boucle de test : `3dslink.exe` **natif** sous Windows (`-a <ip>`), pas dans Docker (broadcast UDP).
- `.cia` : `makerom` (3DSGuy/Project_CTR) + `bannertool` (fork carstene1ns). Hors devkitPro. Optionnel.

### Existant (vérifié dans les dépôts le 2026-10-31)

| Projet | Couvre | Manque | Maturité |
|---|---|---|---|
| **pepasjc/GameSync** (GPL-3.0) | Client 3DS (layout TWiLight, game code), serveur FastAPI avec historique (5 versions) et hash à 3 points, client PC (profil melonDS) | HTTPS depuis la 3DS (`httpc` + vérification désactivée), Dockerfile, lanceur melonDS automatique. Projet géant (15 plateformes). | 2 étoiles, 1 auteur, 2026 |
| **mirusu400/DaeMoon** (MIT) | Client 3DS sur matériel (saves nds-bootstrap dans `sdmc:/roms/nds/saves`), curl + mbedTLS + ISRG Root X1, serveur Go + SQLite + Dockerfile, conflits jamais fusionnés | Aucun client melonDS (seulement un CLI `daemoonctl` avec son propre layout). Lourd : appairage QR, panneau web, i18n. | 2 étoiles, 1 auteur, v0.1.2 |
| branflakes2/ftp_sync (MIT) | Script Python ftpd ↔ serveur pour des saves Pokémon, sans écraser si les deux côtés changent | Pas de serveur HTTP, pas d'historique, ftpd manuel | petit script |
| Autres (Cloudpoint, 3DavSync, 3DSync, Checkpoint, Waystone…) | — | Titres 3DS seulement, ou dates, ou Dropbox seulement | — |

**Conclusion** : aucun outil ne fait tout le scénario. DaeMoon couvre la partie la plus difficile (client 3DS HTTPS) mais pas le PC. GameSync couvre 3DS + PC en LAN mais pas HTTPS, et embarque 15 plateformes.

---

# Partie 1 — Recherche partie 1 : sauvegardes NDS entre 3DS (TWiLight Menu++ / nds-bootstrap) et melonDS (Windows)

Date : 2026-10-31. Sources primaires lues via l'API GitHub, aux commits suivants :

| Projet | Réf. | Commit |
|---|---|---|
| TWiLight Menu++ | `master` | `4bdc34bd194efb9ebe0f20859f111b2b3fd30bd9` |
| nds-bootstrap | `master` | `45fda1811a98a97bb2ec5f27fb71a9f588158543` |
| melonDS | tag `1.1` (dernière release stable, 2025-11-18) | `b86390e4428bf38ce4c1ce0e9ca446d6d25955e8` |
| PKHeX | tag `26.10.18` | `bfe89c74a3f5265805e364b79bc04ad38ebbc5ea` |

Abréviations : `TWL` = https://github.com/DS-Homebrew/TWiLightMenu/blob/4bdc34bd194efb9ebe0f20859f111b2b3fd30bd9 ; `NB` = https://github.com/DS-Homebrew/nds-bootstrap/blob/45fda1811a98a97bb2ec5f27fb71a9f588158543 ; `MEL` = https://github.com/melonDS-emu/melonDS/blob/b86390e4428bf38ce4c1ce0e9ca446d6d25955e8 ; `PKX` = https://github.com/kwsch/PKHeX/blob/bfe89c74a3f5265805e364b79bc04ad38ebbc5ea. Chaque lien ci-dessous = préfixe + chemin + `#Lx`.

Note : les numéros de ligne TWiLight citent le thème `romsel_dsimenutheme`. Les autres thèmes (`quickmenu`, `romsel_r4theme`, `romsel_aktheme`) dupliquent la même logique (vu par recherche de code, pas relu ligne à ligne).

---

## 1. TWiLight Menu++ et nds-bootstrap

### Réponse courte

- Réglage `Save Location` (`SAVE_LOCATION`, section `[SRLOADER]` de `sd:/_nds/TWiLightMenu/settings.ini`). Trois valeurs :
  - `0` = `ESavesFolder` (« 'saves' folder », défaut) → `<dossier ROM>/saves/<nom ROM sans ext>.sav`
  - `1` = `EGamesFolder` (« ROM folder ») → `<dossier ROM>/<nom ROM sans ext>.sav`
  - `2` = `ETWLMFolder` (« TWLMenu folder ») → `sd:/_nds/TWiLightMenu/saves/<nom ROM sans ext>.sav`
- Pour l'utilisateur (ROM `sd:/roms/nds/X.nds`, réglage par défaut) : `sd:/roms/nds/saves/X.sav`. Le nom vient du **nom de fichier de la ROM**, extension remplacée. Aucun nommage par game code.
- Emplacements de sauvegarde multiples : OUI. Réglage par jeu `SAVE_NUMBER` (0..9). Slot 0 → `.sav`. Slot N (1..9) → `.savN` (ex. `X.sav1`). Les slots sont désactivés si `Save Location = ROM folder`.
- Format : dump brut de la mémoire de sauvegarde (EEPROM/FLASH), sans en-tête ni pied. Taille = taille du fichier existant.
- Taille à la création : base de données `ROMList.h` (copiée de melonDS) indexée par game code. Défaut 512 KiB si le jeu est absent de la liste. Homebrew : pas de fichier créé.
- Pokémon Gen 4 (D/P/Pt/HG/SS) et Gen 5 (N/B, N2/B2) : type 6 = **512 KiB** (524288 octets) dans les deux bases (TWiLight et melonDS), pour les codes vérifiés.
- Fichier existant de taille différente : TWiLight **agrandit** un fichier trop petit (remplissage `0x00`). Il ne **réduit jamais** un fichier trop grand. nds-bootstrap utilise ensuite la taille réelle du fichier.
- Écriture : nds-bootstrap écrit **directement sur la carte SD**, secteur par secteur, à chaque écriture de sauvegarde du jeu. Pas de cache RAM ni de vidage différé. Pas de fichier `.bak`.
- **Surprise** : l'écriture passe par la chaîne de clusters FAT, sans mettre à jour l'entrée de répertoire. Donc la **date de modification du `.sav` sur la SD ne change pas** quand le jeu sauvegarde (déduit du code, NON VÉRIFIÉ par un test).

### Détails

**Construction du chemin.** Au lancement, TWiLight calcule `savename = replaceAll(filename, typeToReplace, getSavExtension())`. `typeToReplace` = sous-chaîne depuis le dernier `.` du nom de fichier (`filename.substr(filename.rfind('.'))`). Ensuite `savepath = romFolderNoSlash + "/saves/" + savename`, puis le code remplace ce chemin selon `ms().saveLocation`. Le chemin final va dans `sd:/_nds/nds-bootstrap.ini`, clé `[NDS-BOOTSTRAP] SAV_PATH`. nds-bootstrap lit cette clé.
- `romFolderNoSlash` = dossier courant du navigateur de ROM (`ms().romfolder[...]`). Une ROM dans un sous-dossier a donc son propre sous-dossier `saves/`.
- Effet de bord de `replaceAll` : il remplace **toutes** les occurrences de la chaîne d'extension dans le nom, pas seulement la fin. Cas rare (ex. `a.nds.b.nds`).
- `sd:/_nds/nds-bootstrap.ini` contient le `SAV_PATH` du **dernier** jeu lancé. Un outil de synchro peut le lire pour savoir quel `.sav` a servi en dernier.
- Fichier de réglages par jeu : `sd:/_nds/TWiLightMenu/gamesettings/<nom ROM avec ext>.ini`, clé `[GAMESETTINGS] SAVE_NUMBER`.

**Création / extension (`createSaveFile`).**
- `savesize = 524288` par défaut. Recherche du game code dans `ROMList[]`. Si trouvé et `SaveMemType != 0xFFFFFFFF`, alors `savesize = sramlen[SaveMemType]`.
- `sramlen[] = {0, 512, 8192, 65536, 128K, 256K, 512K, 1M, 8M, 16M, 64M}` (octets).
- Condition : `(orgsavesize == 0 && savesize > 0) || (orgsavesize < savesize)`. Le fichier est ouvert en `"r+"` (s'il existe) ou `"wb"`. TWiLight écrit un `\0` tous les 0x8000 octets jusqu'à `savesize`. Résultat : zone ajoutée remplie de `0x00`.
- Aucun cas ne réduit le fichier. Un `.sav` plus grand que prévu reste tel quel.
- `ROMList.h` porte l'en-tête de copyright de melonDS (Arisotura). Les deux bases ont la même origine.

**Tailles Pokémon vérifiées** (`ROMList.h` TWiLight, valeur `0x00000006` = 512 KiB) :

| Jeu | Codes vérifiés |
|---|---|
| Diamant / Perle | `ADAF`, `ADAE`, `ADAJ` / `APAF`, `APAE` |
| Platine | `CPUF`, `CPUE`, `CPUJ` |
| Or HeartGold / Argent SoulSilver | `IPKF`, `IPKE` / `IPGF`, `IPGE` |
| Noir / Blanc | `IRBF`, `IRBO` / `IRAF`, `IRAO` |
| Noir 2 / Blanc 2 | `IREF`, `IREO` / `IRDF`, `IRDO` |

Platine et HG/SS = 512 KiB aussi. PKHeX confirme `SIZE_G4RAW = 0x80000` et `SIZE_G5RAW = 0x80000` (voir §4). Les autres codes régionaux ne sont pas vérifiés un par un.

**Écriture par nds-bootstrap.**
- `conf->saveSize = getFileSize(conf->savPath)`. nds-bootstrap prend la taille du fichier, pas une base de données.
- Mode DSi (`retail/cardenginei`, le mode normal sur 3DS) : `eepromPageWrite()` appelle `fileWrite(src, savFile, dst % saveSize, len)`. L'adresse est prise **modulo la taille du fichier**. Si le fichier est trop petit pour le jeu, les écritures se replient et écrasent le début. C'est la raison de l'extension par TWiLight.
- `fileWrite()` (`my_fat.c`) appelle `CARD_WriteSector` / `CARD_WriteSectors` directement. Aucune mise à jour de l'entrée de répertoire (taille, date). Le fichier n'est jamais agrandi pendant le jeu.
- Protection coupure : `saveTimer = 1` pendant une écriture. Le gestionnaire du bouton POWER (`i2cIRQHandler`) fait `if (saveTimer != 0) return;`. Le reset logiciel (L+R+Start+Select) attend aussi `saveTimer == 0`. `saveTimer` revient à 0 après 60 trames. Ce mécanisme retarde l'arrêt **logiciel**. Il ne protège pas contre un retrait de batterie ou de carte SD.
- Si la SD est éjectée (`isSdEjected()`), l'écriture est ignorée (`return false`).
- Option `delayWrites` : la première écriture peut attendre une trame dans les 2 premières secondes. Effet négligeable.
- Mode B4DS (`retail/cardengine/arm9`) : même principe, `fileWrite(..., &savFile, dst % ce9->saveSize, len)`, écriture directe.
- Risque de fichier incomplet : faible. La taille ne change pas. Une coupure brutale pendant une écriture peut laisser **un bloc** à moitié écrit. Pokémon Gen 4/5 gardent deux copies (Gen 4) ou des sommes de contrôle, donc le jeu détecte la corruption.
- Pas de `.bak`, pas de savestate dans nds-bootstrap (rien trouvé : seuls fichiers annexes écrits = pagefile, RAM dump, captures, paramètres de reset). Recherche non exhaustive.

**`Save Relocation`** (`[NDS-BOOTSTRAP] SAVE_RELOCATION`, défaut `1` = SD). Valeur « Game Card » : la sauvegarde va sur la **carte physique insérée** si le title ID correspond. Dans ce cas le `.sav` sur la SD n'est pas mis à jour.

**B4DS vs mode DSi.** `B4DS_MODE` est un réglage nds-bootstrap (lu seulement si `dsiFeatures()`). Le chemin vient toujours de `SAV_PATH` fourni par TWiLight. Le format reste un dump brut. Aucune différence de chemin ni de format trouvée. Exception : `usePicoLoader` (chargeur alternatif) exige `saveLocation == EGamesFolder` sur carte flash (`fat:`), sans effet sur une 3DS qui lance depuis `sd:`.

### Sources

- Enum `TSaveLoc` / `TSaveRelocation` : `TWL/universal/include/common/twlmenusettings.h#L241-L252`
- Défaut et clés ini : `TWL/universal/source/common/twlmenusettings.cpp#L85`, `#L282`, `#L379`
- Libellés EN : `TWL/settings/nitrofiles/languages/en/language.ini` (clés `SAVE_LOCATION`, `SAVES_FOLDER`, `ROM_FOLDER`, `TWLMENU_FOLDER`, `DESCRIPTION_SAVE_LOCATION`, `DESCRIPTION_SAVE_RELOCATION`) ; option : `TWL/settings/arm9/source/main.cpp#L1370`
- Chemins ini : `TWL/universal/include/common/bootstrappaths.h#L12-L16`
- Construction du chemin et `SAV_PATH` : `TWL/romsel_dsimenutheme/arm9/source/main.cpp#L2097-L2129` et `#L2017-L2038` (variante Pico Loader)
- `createSaveFile` : `TWL/romsel_dsimenutheme/arm9/source/main.cpp#L873-L935`
- Slots `.savN` : `TWL/romsel_dsimenutheme/arm9/source/perGameSettings.cpp#L1562-L1568` (`getSavExtension`), `#L137` (`SAVE_NUMBER`), `#L1276-L1277` et `#L1374-L1375` (plage 0..9), `#L125` (chemin ini par jeu)
- Tailles : `TWL/universal/include/saveMap.h#L4-L12` ; `TWL/universal/include/ROMList.h` (ex. `IRBO` L4461, `IRBF` L2145, `CPUE` L1781, `IPKE` L1201)
- `SAV_PATH`, `SAVE_RELOCATION`, `saveSize` : `NB/retail/arm9/source/conf_sd.cpp#L362`, `#L572`, `#L849`, `#L1944`
- Écriture DSi : `NB/retail/cardenginei/arm7/source/cardengine.c#L2317-L2372` ; `saveTimer` `#L2062-L2068` ; POWER `#L2114-L2119` ; soft reset `#L1999-L2008`
- Écriture B4DS : `NB/retail/cardengine/arm9/source/cardengine.c#L587-L607`, `#L887-L897`
- `fileWrite` : `NB/retail/common/source/my_fat.c#L1584-L1766`

---

## 2. melonDS 1.1 (Windows)

### Réponse courte

- Chemin du `.sav` : `<SaveFilePath ou dossier de la ROM>/<nom ROM sans dernière ext>.sav`. Avec `SaveFilePath = C:\Saves\DS` : `C:/Saves/DS/X.sav` pour `X.nds`. Nommage par nom de fichier, pas par game code.
- Instance N>0 dans le même processus : suffixe `.N+1` → `X.sav.2`, `X.sav.3`...
- Config : `melonDS.toml`, table `[Instance0]`, clé `SaveFilePath`. Le build Windows est **portable par défaut** : `melonDS.toml` est à côté de `melonDS.exe`.
- Format : dump brut, sans pied (pas de footer DeSmuME). Accepte un `.sav` TWiLight tel quel si la taille correspond.
- Taille : choisie par la **base `ROMList` (game code)**, jamais par la taille du fichier. Si le fichier a une autre taille : copie de `min(taille fichier, taille attendue)`, reste rempli de `0xFF`. À la première écriture, melonDS **réécrit le fichier à la taille attendue** (troncature ou extension).
- Écriture : buffer en RAM, puis écriture du fichier entier après **2 s sans nouvelle écriture** (thread `SaveManager`, boucle de 100 ms). Vidage aussi à la fermeture propre de la ROM ou de l'application.
- Ouverture en mode `Write` (= `fopen "w"`, troncature) puis écriture complète. **Non atomique.** Un kill pendant l'écriture peut laisser un fichier tronqué. Un kill dans les 2 s après une sauvegarde en jeu perd cette sauvegarde.
- Ligne de commande : `melonDS.exe <rom>` démarre le jeu (option `-b auto` par défaut = boot si ROM donnée). Options : `-b/--boot auto|always|never`, `-f/--fullscreen`, `-a/--archive-file`. Un seul processus. Les instances multiples sont des fenêtres du **même** processus.

### Détails

**Nommage (`getAssetPath`).** Base = `SaveFilePath` si non vide, sinon `baseROMDir` (dossier de la ROM). Le code retire les `/` et `\` finaux, ajoute `/`, puis `baseAssetName` = `romname.substr(0, romname.rfind('.'))`, puis l'extension `.sav`. Pour une archive, `romname` vient de `loadROMData` : probablement le nom du fichier dans l'archive (NON VÉRIFIÉ).

**Chargement (`loadROM`).**
1. `savname = getAssetPath(..., "SaveFilePath", ".sav") + instanceFileSuffix()`.
2. Si ce fichier n'existe pas, melonDS lit `origsav` (sans suffixe d'instance). Il vérifie d'abord que le chemin est inscriptible, sinon la ROM ne se charge pas (`getSavErrorString`).
3. Si aucun fichier n'existe, `savelen = 0`. Aucun fichier n'est créé avant la première écriture du jeu.
4. `NDSCart::ParseROM` → `CartRetail` : `SRAMLength = sramlengths[ROMParams.SaveMemType]`. Même table que TWiLight. Si `sramlen == SRAMLength`, données prises telles quelles. Sinon : buffer `0xFF`, puis `memcpy(min(...))`.
5. Jeu absent de `ROMList` : `SaveMemType = 2` (EEPROM 8 KiB, commentaire « TODO FIXME »). Homebrew : 0 (pas de sauvegarde). **Écart avec TWiLight** : TWiLight crée 512 KiB pour un jeu inconnu, melonDS suppose 8 KiB. Pas d'impact sur Pokémon (présents dans les deux bases avec 512 KiB : `IRBO` L4455, `IRBF` L2139, `CPUF` L2238, `IPKF` L2193, `ADAF` L2122 de `src/ROMList.cpp`).

**Écriture (`SaveManager`).**
- Le cœur appelle `Platform::WriteNDSSave` → `SaveManager::RequestFlush` (copie dans `Buffer`). `EmuThread` appelle `CheckFlush()` à chaque tour de boucle (copie dans `SecondaryBuffer`, note l'heure).
- Le thread `run()` dort 100 ms. Il écrit quand `difftime(now, TimeAtLastFlushRequest) >= 2`. Résolution `time()` = 1 s, donc délai réel ≈ 2 à 3 s.
- `FlushSecondaryBuffer` : `OpenFile(Path, FileMode::Write)` puis `FileWrite` de tout le buffer. `FileMode::Write` est documenté « similar to fopen "w" » (troncature).
- Destructeur : `Running = false; wait(); FlushSecondaryBuffer();`. Une fermeture propre écrit donc les données en attente.
- melonDS ne garde **aucun verrou** sur le fichier. Il garde la sauvegarde en RAM. Un `.sav` remplacé par un outil externe pendant que le jeu tourne sera écrasé à la prochaine écriture du jeu.

**Savestates.**
- Fichiers `<nom>.ml1` .. `<nom>.ml8` dans `SavestatePath` (défaut : dossier de la ROM). Ce sont des fichiers séparés du `.sav`.
- **Charger un savestate réécrit le `.sav`** : `CartRetail::DoSavestate` appelle `WriteNDSSave(SRAM, SRAMLength, 0, SRAMLength)` avec la SRAM contenue dans le savestate. Un vieux savestate peut donc remplacer une sauvegarde plus récente (synchronisée depuis la 3DS) après 2 s.
- Option `Savestate.RelocSRAM` (défaut `false`) : au chargement d'un savestate, melonDS bascule vers `<SaveFilePath>/<nom du savestate>.sav` (ex. `X.ml1.sav`). Le `.sav` principal n'est alors plus mis à jour.
- Autres fichiers à côté : `.mch` (cheats, `CheatFilePath`). Pas d'autre fichier de sauvegarde.

**Processus et instances.**
- `main.cpp` : tableau `emuInstances[kMaxEmuInstances = 16]` dans un seul processus. « Open new window » / `onMPNewInstance` appelle `createEmuInstance()` dans ce processus. Le processus se termine après `melon.exec()` et `deleteAllEmuInstances()`. Un lanceur peut donc attendre la fin de `melonDS.exe`.
- Deux processus `melonDS.exe` séparés : chacun a l'instance 0, donc le même `X.sav`, sans verrou. Aucune détection d'instance unique trouvée (pas de `QSharedMemory` / `QLocalServer` dans le dépôt). Risque d'écrasement mutuel.
- Une instance secondaire (même processus) écrit `X.sav.2`. Un outil de synchro qui ne lit que `X.sav` ne voit pas ces sauvegardes.

### Sources

- `getAssetPath` : `MEL/src/frontend/qt_sdl/EmuInstance.cpp#L445-L485`
- `loadROM` : `MEL/src/frontend/qt_sdl/EmuInstance.cpp#L1867-L1977`
- Suffixe d'instance : `MEL/src/frontend/qt_sdl/EmuInstance.cpp#L176-L182`
- Savestates : `MEL/src/frontend/qt_sdl/EmuInstance.cpp#L695-L700`, RelocSRAM `#L761-L768`
- `SaveManager` : `MEL/src/frontend/qt_sdl/SaveManager.cpp#L53-L67` (destructeur), `#L97-L123`, `#L125-L146`, `#L148-L164` (délai 2 s), `#L166-L193` (écriture)
- `FileMode::Write` : `MEL/src/Platform.h#L95`
- Appel `CheckFlush` : `MEL/src/frontend/qt_sdl/EmuThread.cpp#L312-L319`
- `WriteNDSSave` : `MEL/src/frontend/qt_sdl/Platform.cpp#L416-L421`
- Taille SRAM : `MEL/src/NDSCart.cpp#L399-L445` ; défaut jeu inconnu `#L1633-L1645` ; savestate `#L460-L488`
- Config : `MEL/src/frontend/qt_sdl/Config.cpp#L42` (`melonDS.toml`), `#L301` (`SaveFilePath`, local à l'instance), `#L822-L827` (`InstanceN`)
- Portable Windows : `MEL/src/frontend/qt_sdl/CMakeLists.txt#L196-L201`, `MEL/src/frontend/qt_sdl/main.cpp#L178-L212`
- CLI : `MEL/src/frontend/qt_sdl/CLI.cpp#L41-L95`
- Instances : `MEL/src/frontend/qt_sdl/main.cpp#L75`, `#L118-L135`, `#L355-L395` ; `MEL/src/frontend/qt_sdl/Window.cpp#L1784-L1787`

---

## 3. Identité du jeu (game code)

### Réponse courte

- Offset `0x00C`, 4 octets ASCII majuscules (`NTR-<code>` sans préfixe). Dernière lettre = destination/langue.
- Lettres (GBATEK) : `E` English/USA, `F` French, `D` German, `I` Italian, `S` Spanish, `J` Japanese, `K` Korean, `P` Europe, `O` « Int » (international), `V` EUR+AUS, `U` Australian, etc.
- `IRBO` : `O` = « Int ». Probablement la version anglaise internationale de Pokémon Noir. **Pokémon Noir FR = `IRBF`**, pas `IRBO`. Les deux codes existent dans `ROMList` (attribution exacte `IRBO` = anglais : NON VÉRIFIÉ, pas de titre dans `ROMList`).
- Clé fiable entre 3DS et PC quand les noms de fichiers diffèrent : oui pour des dumps officiels. Pas suffisante seule pour les ROM hacks et les homebrews.

### Détails

- Champs utiles de l'en-tête (GBATEK) : `000h` titre (12 car.), `00Ch` game code (`0` = homebrew), `010h` maker code, `012h` unitcode (`00h` NDS, `02h` NDS+DSi, `03h` DSi), `01Eh` version ROM, `15Eh` CRC-16 de l'en-tête.
- Homebrew : `ndstool` met `"####"` par défaut. melonDS traite comme homebrew `ARM9ROMOffset < 0x4000` **ou** game code `"####"`. TWiLight ne crée pas de `.sav` pour un homebrew. Un game code vide ou `####` n'identifie rien : exclure ces ROM de la synchro par game code.
- Régions : codes différents (`IRBF` vs `IRBO`). Le cas « sauvegarde FR chargée dans une ROM EN » : NON VÉRIFIÉ (aucune source primaire trouvée). PKHeX détecte Gen 4/5 par taille et structure, pas par langue. La compatibilité inter-langue n'est donc pas exclue, mais elle n'est pas prouvée. Recommandation : ne jamais apparier deux codes différents automatiquement.
- ROM hacks : ils gardent en général le game code du jeu de base (ex. un hack de Platine garde `CPUE`). Leur sauvegarde peut être incompatible avec le jeu original. Le game code seul ne les sépare pas.
- Jeux DSi-enhanced (unitcode `02h`) : même game code, même `.sav` brut. Pas d'impact sur la clé. (Pokémon N/B DSi-enhanced : NON VÉRIFIÉ.)
- Deux copies du même jeu avec des noms de fichiers différents sur la 3DS (ou slots `.sav1..9`) : même game code, **plusieurs** `.sav`. La clé doit permettre ce cas.

**Clé recommandée :** game code complet (4 lettres, région incluse) **+** une désambiguïsation choisie par l'utilisateur pour les cas ambigus. Candidats : CRC de l'en-tête `0x15E`, version ROM `0x01E`, ou un hash du fichier ROM. Hash ROM : un patch de hack change le hash même si l'en-tête reste. Le slot TWiLight (`SAVE_NUMBER`) doit aussi faire partie de l'identité côté 3DS.

### Sources

- GBATEK « DS Cartridge Header » : https://problemkaputt.de/gbatek-ds-cartridge-header.htm (offsets `00Ch`, `010h`, `012h`, `01Eh`, `15Eh` ; table des lettres de destination)
- Détection homebrew melonDS : `MEL/src/NDS_Header.h#L213-L216`
- `ndstool` défaut `"####"` : https://github.com/devkitPro/ndstool/blob/master/source/ndscreate.cpp#L352 (branche `master`, pas de permalien de commit)
- Codes dans `ROMList` : voir §1 et §2

---

## 4. Bonus : nom du dresseur et temps de jeu (PKHeX)

### Réponse courte

**Gen 4** (`.sav` 512 KiB = deux partitions de `0x40000`) :
- Bloc « General » actif : comparer les pieds des deux partitions. Pied à `offset = begin + length - 0x14` (partition 1) et `offset + 0x40000` (partition 2). Comparer le compteur majeur (u32 LE à `+0`), puis le compteur mineur (u32 LE à `+4`). Le plus grand gagne. `0xFFFFFFFF` = non initialisé.
- Taille du bloc General (`gSize`) : D/P `0xC100`, Pt `0xCF2C`, HG/SS `0xF628`.
- `Trainer1` (dans le bloc General) : D/P `0x64`, Pt `0x68`, HG/SS `0x64`.
- Nom : `Trainer1 + 0x00`, 16 octets, 7 car. max. Encodage propriétaire Gen 4 (u16 LE par caractère, table `StringConverter4`), terminateur `0xFFFF`.
- Temps de jeu : heures u16 LE à `Trainer1 + 0x22`, minutes u8 `+0x24`, secondes u8 `+0x25`.

**Gen 5** :
- Bloc « Trainer Data » à l'offset absolu `0x19400` (N/B et N2/B2). PKHeX lit une seule copie, sans choix de bloc actif. Copie miroir dans le jeu : NON VÉRIFIÉ.
- Nom : `0x19400 + 0x04`, 16 octets, 7 car. max, UTF-16 LE (avec remappage de quelques caractères spéciaux), terminateur `0xFFFF`.
- Temps de jeu : heures u16 LE à `0x19400 + 0x24`, minutes u8 `+0x26`, secondes u8 `+0x27`.

### Sources

- `PKX/PKHeX.Core/Saves/SAV4.cs#L20` (`PartitionSize`), `#L53-L66` (choix du bloc), `#L180-L184` (pied), `#L261-L266` (nom), `#L349-L365` (temps)
- `PKX/PKHeX.Core/Saves/Substructures/Gen4/SAV4BlockDetection.cs` (`CompareFooters`, `CompareCounters`)
- `PKX/PKHeX.Core/Saves/SAV4DP.cs#L36` / `#L58` ; `SAV4Pt.cs#L33` / `#L62` ; `SAV4HGSS.cs#L34-L38` / `#L70`
- `PKX/PKHeX.Core/Saves/Substructures/Gen5/PlayerData5.cs#L11-L90`
- `PKX/PKHeX.Core/Saves/Access/SaveBlockAccessor5BW.cs#L40`, `SaveBlockAccessor5B2W2.cs#L41`
- `PKX/PKHeX.Core/PKM/Strings/StringConverter4.cs#L12`, `StringConverter5.cs#L11`
- `PKX/PKHeX.Core/Saves/Util/SaveUtil.cs#L77-L81` (`SIZE_G4RAW = SIZE_G5RAW = 0x80000`)

---

## Risques et surprises pour un outil qui ne doit jamais détruire une sauvegarde

1. **Date de modification inutilisable sur la 3DS.** nds-bootstrap écrit les secteurs sans toucher l'entrée de répertoire. Le `mtime` du `.sav` sur la SD ne bouge pas quand on joue. Comparez le **contenu** (hash), pas la date. Pour Pokémon, comparez le compteur de sauvegarde ou le temps de jeu (§4).
2. **melonDS réécrit le fichier à sa taille.** Un `.sav` de taille différente est tronqué ou complété (`0xFF`) à la première sauvegarde en jeu. Vérifiez que la taille attendue est la même des deux côtés. Pour un jeu absent de `ROMList` : TWiLight 512 KiB, melonDS 8 KiB. Ce cas détruit des données.
3. **TWiLight n'agrandit qu'avec `0x00`, ne réduit jamais.** Un `.sav` 8 KiB envoyé sur la 3DS devient 512 KiB (si le jeu est inconnu). Un fichier trop petit pour le jeu fait replier les écritures de nds-bootstrap (modulo taille) : corruption.
4. **Écriture melonDS non atomique et différée de 2 à 3 s.** Ne lisez jamais le `.sav` pendant que `melonDS.exe` tourne. Attendez la fin du processus. Un kill du processus peut perdre la dernière sauvegarde ou laisser un fichier tronqué : vérifiez la taille avant de copier.
5. **melonDS écrase un fichier remplacé pendant l'exécution.** Il garde la sauvegarde en RAM et n'a aucun verrou. Ne poussez jamais un `.sav` vers le PC pendant que melonDS tourne.
6. **Charger un savestate melonDS réécrit le `.sav`** avec la SRAM du savestate. Un vieux `.ml1` peut annuler une sauvegarde venue de la 3DS. Avec `Savestate.RelocSRAM`, la sauvegarde part dans un autre fichier (`X.ml1.sav`).
7. **Fichiers multiples pour un même jeu** : slots TWiLight `.sav1`..`.sav9`, instances melonDS `X.sav.2`.., copies de ROM avec des noms différents. Le nom du `.sav` suit le nom de la ROM, pas le game code.
8. **Deux processus melonDS** sur la même ROM écrivent le même `X.sav` sans verrou.
9. **`Save Relocation = Game Card`** : la sauvegarde va sur la carte physique, le `.sav` SD ne change pas.
10. **Game code ambigu** : ROM hacks avec le code du jeu de base, homebrews `####` ou `0`. Codes de région différents = jeux différents pour l'outil (compatibilité inter-langue NON VÉRIFIÉE).
11. **Remplissage différent** : zone vide `0x00` (TWiLight) contre `0xFF` (melonDS). Deux fichiers « vides » n'ont donc pas le même hash.
12. **Coupure sur 3DS** : nds-bootstrap retarde l'arrêt par le bouton POWER pendant une écriture (`saveTimer`). Il ne protège pas contre un retrait de batterie ou de SD. Gardez toujours une copie horodatée avant chaque écrasement.

---

# Partie 2 — Recherche partie 2 : réseau 3DS, chaîne de build, outils existants

Date de la recherche : 2026-10-31. Les permaliens pointent sur les commits lus ce jour.
Légende : **NON VÉRIFIÉ** = je n'ai pas trouvé de source primaire. « Déduit » = conclusion tirée d'un code source, sans test sur matériel.

---

## 1. Réseau sur 3DS

### Réponse courte

- **`httpc` (service système) ne fonctionne pas en HTTPS avec un serveur moderne.** Le module SSL de la 3DS supporte au plus TLS 1.1 (source : 3dbrew). Nginx, Caddy et Traefik refusent TLS 1.1 par défaut (minimum TLS 1.2). De plus, ISRG Root X1 (Let's Encrypt) n'est pas dans le magasin de racines intégré.
- **`3ds-curl` + `3ds-mbedtls` (portlibs devkitPro) fonctionne avec Let's Encrypt.** mbedTLS 2.28.8 supporte TLS 1.2, ECDHE-ECDSA et SNI. TLS 1.3 n'est pas activé. Aucun bundle CA n'est livré : l'app doit fournir le sien.
- **HTTP clair sur LAN** : `httpc` et curl fonctionnent tous les deux. `httpc` est le plus simple (pas de `socInit`). Mais l'upload via `httpc` doit tenir en entier en mémoire.
- **Recommandation** :
  - Internet + Let's Encrypt → **curl + mbedTLS**, avec ISRG Root X1 embarqué (`CURLOPT_CAINFO_BLOB`).
  - LAN seulement → **`httpc`** suffit (HTTP clair). curl reste valable si tu veux un seul code pour les deux cas.

### Détails

#### 1.1 `httpc` : API (libctru)

Fichier : `libctru/include/3ds/services/httpc.h` ([permalien](https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/include/3ds/services/httpc.h)).

- **Méthodes** : enum `HTTPC_RequestMethod` = GET, POST, HEAD, **PUT (0x4, l.18)**, DELETE. PUT est donc supporté au niveau de l'API.
- **En-têtes custom** : `httpcAddRequestHeaderField(ctx, name, value)` (l.75). `Authorization` et `If-Match` passent par cette fonction. Aucune liste noire n'apparaît dans le header. Comportement du sysmodule sur ces en-têtes : **NON VÉRIFIÉ**.
- **Lecture d'en-têtes de réponse (ETag)** : `httpcGetResponseHeader(ctx, name, value, valuebuf_maxsize)` (l.162). L'exemple officiel lit `Location` avec ([3ds-examples network/http/source/main.c](https://github.com/devkitPro/3ds-examples/blob/be2001fee08c/network/http/source/main.c)). Lire `ETag` doit donc fonctionner pareil (déduit).
- **Corps de requête** : `httpcAddPostDataRaw(ctx, const u32* data, u32 len)` (l.100). Le buffer est passé en un seul descripteur IPC ([httpc.c](https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/source/services/httpc.c)). Il n'y a **pas d'API d'upload chunked / streamé**.
- **Taille max du corps** : `httpcInit(u32 sharedmem_size)` (l.44) crée un bloc partagé passé au service comme « POST buffer ». L'exemple officiel commente : `httpcInit(0); // Buffer size when POST/PUT.` La limite exacte n'est pas documentée (3dbrew ne donne rien). **Preuve empirique** : GameSync appelle `httpcInit(0x80000)` (512 Ko) et refuse les corps > 448 Ko ([GameSync 3ds/source/network.c l.6 et l.18](https://github.com/pepasjc/GameSync/blob/8766c36bd830/3ds/source/network.c)). Les `.sav` DS font au plus 512 Ko à 1 Mo (taille max réelle : **NON VÉRIFIÉ** ici). Prévois `sharedmem_size` ≥ taille du save + marge.
- **Certificats** : `httpcAddTrustedRootCA(ctx, cert, size)` attend un **certificat DER** (commentaire l.167-170). `httpcAddDefaultCert(ctx, SSLC_DefaultRootCert)` (l.177) ajoute une racine intégrée.
- **Racines intégrées** (`sslc.h` l.15-25, [permalien](https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/include/3ds/services/sslc.h)) : 6 racines Nintendo, GTE CyberTrust, AddTrust External CA, COMODO RSA, USERTrust RSA, DigiCert High Assurance EV. **Pas d'ISRG Root X1.**
- **Options SSL** : `SSLCOPT_DisableVerify = BIT(9)` et `SSLCOPT_TLSv10 = BIT(11)` (sslc.h l.35-36).
- **SNI** : `sslcCreateContext(..., const char *hostname)` (sslc.h l.157) prend un hostname. 3dbrew ne dit pas s'il part en extension SNI. **NON VÉRIFIÉ.**

#### 1.2 Module SSL système (3dbrew)

Page [SSL Services](https://www.3dbrew.org/wiki/SSL_Services) :
- « The highest supported TLS protocol version is v1.1 (this is the version used by default). »
- Le bit `0x800` désactive TLS 1.1 et repasse en TLS 1.0.
- Les dates not-before/not-after du certificat serveur ne sont pas validées.
- Le sysmodule utilise RSA BSAFE.
- Liste des cipher suites, support ECDHE/ECDSA : **absents de 3dbrew → NON VÉRIFIÉ**.

Page [HTTP Services](https://www.3dbrew.org/wiki/HTTP_Services) : « Initially a HTTP context will not trust any root-CAs at all ». Limite de 2 RootCertChain par processus. Aucune limite de taille documentée.

#### 1.3 Compatibilité Let's Encrypt avec `httpc`

- Défaut serveur : nginx `ssl_protocols TLSv1.2 TLSv1.3` ([doc nginx](https://nginx.org/en/docs/http/ngx_http_ssl_module.html)). Caddy : « Default min: tls1.2 » ([doc Caddy](https://caddyserver.com/docs/caddyfile/directives/tls)).
- Donc TLS 1.1 max côté 3DS ⇒ **échec du handshake avec la config par défaut** (déduit des deux sources ci-dessus).
- Chaîne Let's Encrypt actuelle : leaf ← YR1/YR2 (RSA) ← Root YR ← cross-sign **ISRG Root X1**, ou leaf ← YE1/YE2 (ECDSA) ← Root YE ← X2 ← X1 ([letsencrypt.org/certificates](https://letsencrypt.org/certificates/)). Une chaîne plus longue avec cross-signs. Comportement du module 3DS sur cette chaîne : **NON VÉRIFIÉ**.
- Contournement possible : `SSLCOPT_DisableVerify` + rabaisser le serveur à TLS 1.0/1.1 avec des suites RSA. C'est une mauvaise idée pour un serveur public. Caddy indique que les vieilles versions TLS peuvent être retirées à tout moment.
- Témoignage d'un projet tiers (pas une source primaire) : DaeMoon écrit que httpc:C « fails against ordinary modern servers - including, in practice, anything behind Let's Encrypt » ([DaeMoon platform/common/net_curl.c l.3-8](https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a29/platform/common/net_curl.c)). Une issue ouverte devkitPro dit aussi que la pile TLS 3DS est « quite outdated » ([3ds-examples#75](https://github.com/devkitPro/3ds-examples/issues/75)).

#### 1.4 `3ds-curl` + `3ds-mbedtls`

PKGBUILD [3ds/curl](https://github.com/devkitPro/pacman-packages/blob/b9e8b0de6a64/3ds/curl/PKGBUILD) :
- `pkgver=8.4.0` (l.5), `depends=('3ds-zlib' '3ds-mbedtls')` (l.12), `groups=('3ds-portlibs')` (l.21).
- `--with-mbedtls` (l.42), **`--with-ca-bundle=sdmc:/config/ssl/cacert.pem`** (l.43). Le chemin par défaut est donc un fichier sur la SD. **Aucun cacert n'est livré** par le paquet (aucun fichier CA dans le PKGBUILD).
- Patch 3DS minimal (`cf-socket.c`, `curl_addrinfo.c`) : désactive keepalive TCP/bindlocal, corrige `ai_addrlen`.

PKGBUILD [3ds/mbedtls](https://github.com/devkitPro/pacman-packages/blob/b9e8b0de6a64/3ds/mbedtls/PKGBUILD) :
- `pkgver=2.28.8`. Active `MBEDTLS_ENTROPY_HARDWARE_ALT`, `MBEDTLS_NO_PLATFORM_ENTROPY`.
- Le patch 3DS ([mbedtls-2.28.8.patch](https://github.com/devkitPro/pacman-packages/blob/b9e8b0de6a64/3ds/mbedtls/mbedtls-2.28.8.patch)) prend l'entropie via `sslcGenerateRandomData()`. Cette fonction utilise le handle `ssl:C` ([sslc.c l.216](https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/source/services/sslc.c)). ⇒ **L'app doit appeler `sslcInit(0)`** en plus de `socInit()` (déduit).
- `config.h` de mbedTLS 2.28.8 ([lien](https://github.com/Mbed-TLS/mbedtls/blob/v2.28.8/include/mbedtls/config.h)) : `MBEDTLS_SSL_PROTO_TLS1_2` actif, `MBEDTLS_SSL_SERVER_NAME_INDICATION` actif, `MBEDTLS_KEY_EXCHANGE_ECDHE_ECDSA_ENABLED` actif, `//#define MBEDTLS_SSL_PROTO_TLS1_3_EXPERIMENTAL` (désactivé). Le PKGBUILD ne change pas ces options. ⇒ **TLS 1.2 oui, TLS 1.3 non**. TLS 1.2 suffit : nginx et Caddy l'acceptent par défaut.
- CA embarqué : `CURLOPT_CAINFO_BLOB` est supporté par mbedTLS depuis curl 7.81.0 ([doc curl](https://curl.se/libcurl/c/CURLOPT_CAINFO_BLOB.html)). curl 8.4.0 l'a. Tu peux donc lier ISRG Root X1 (PEM, ~2 Ko) dans le binaire. DaeMoon fait exactement ça ([vendor/cacert/roots.txt](https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a29/vendor/cacert/roots.txt)). Il note que parser les 121 racines Mozilla (~185 Ko) à chaque requête coûte cher sur l'ARM11.
- **Coût en taille binaire** : **NON VÉRIFIÉ** (aucun chiffre dans les sources lues).
- **`socInit`** : obligatoire pour curl (sockets BSD). Une issue devkitPro le rappelle ([3ds-examples#75](https://github.com/devkitPro/3ds-examples/issues/75)). `httpc` n'en a pas besoin.
- Upload streamé : curl lit le corps par callback (`CURLOPT_READFUNCTION`). DaeMoon l'utilise : « A save never exists whole in memory here » (net_curl.c l.17-20).
- Projets réels qui utilisent curl sur 3DS : DaeMoon, 3DavSync ([webdav.cpp](https://github.com/szclsya/3DavSync/blob/911ef0da996e/source/webdav.cpp)), Checkpoint (couche HTTP basée sur libcurl, voir [scripts/README.md](https://github.com/FlagBrew/Checkpoint/blob/740f221a853d/scripts/README.md), codes `-(CURLcode + 100)`).

#### 1.5 HTTP clair sur LAN

- `httpc` accepte une URL `http://192.168.x.x:port` (le paramètre `url` est libre). GameSync utilise `server_url=http://192.168.1.100:8000` ([3ds/README.md](https://github.com/pepasjc/GameSync/blob/8766c36bd830/3ds/README.md)).
- Réserves : l'appel bloque. Utilise les variantes `...Timeout` (`httpcGetResponseStatusCodeTimeout`, `httpcReceiveDataTimeout`). GameSync ajoute un délai de 50 ms entre requêtes « to let httpc clean up » (network.c l.11-14). C'est un retour d'expérience, pas une doc officielle.
- Pas de chiffrement : la clé API circule en clair sur le Wi-Fi.

### Sources

- libctru `httpc.h`, `httpc.c`, `sslc.h`, `sslc.c` : https://github.com/devkitPro/libctru/tree/1f8d386e375c/libctru
- 3dbrew SSL Services : https://www.3dbrew.org/wiki/SSL_Services
- 3dbrew HTTP Services : https://www.3dbrew.org/wiki/HTTP_Services
- 3ds-examples http : https://github.com/devkitPro/3ds-examples/blob/be2001fee08c/network/http/source/main.c
- pacman-packages curl / mbedtls : https://github.com/devkitPro/pacman-packages/tree/b9e8b0de6a64/3ds
- mbedTLS 2.28.8 config.h : https://github.com/Mbed-TLS/mbedtls/blob/v2.28.8/include/mbedtls/config.h
- curl CAINFO_BLOB : https://curl.se/libcurl/c/CURLOPT_CAINFO_BLOB.html
- nginx ssl_protocols : https://nginx.org/en/docs/http/ngx_http_ssl_module.html
- Caddy tls : https://caddyserver.com/docs/caddyfile/directives/tls
- Let's Encrypt chaînes : https://letsencrypt.org/certificates/

---

## 2. Chaîne de build sous Windows

### Réponse courte

- L'image **`devkitpro/devkitarm`** contient `3ds-dev` (libctru, 3dslink, outils) **et** `3ds-portlibs` (donc `3ds-curl` et `3ds-mbedtls` sont déjà installés). `dkp-pacman` est présent dans l'image.
- Build : monte le dossier projet et lance `make`. Sortie : `.3dsx` + `.smdh`.
- Boucle de test : Homebrew Launcher → **Y** → `3dslink -a <ip> app.3dsx`. Ajoute `-s` pour recevoir `stdout` sur le PC (l'app doit appeler `link3dsStdio()`).
- **Lance 3dslink nativement sous Windows**, pas dans Docker. Le broadcast UDP et le serveur `-s` (TCP 17491 entrant) traversent mal le NAT de Docker Desktop.
- `.cia` : **makerom** (3DSGuy/Project_CTR, maintenu) + **bannertool** (dépôt original supprimé ; fork maintenu carstene1ns/3ds-bannertool). Aucun des deux n'est un paquet devkitPro. Optionnel pour ce projet.

### Détails

#### 2.1 Image Docker

- [devkitarm/Dockerfile](https://github.com/devkitPro/docker/blob/bee28ad01763/devkitarm/Dockerfile) :
  - l.1 `FROM devkitpro/toolchain-base`
  - l.6 `dkp-pacman -S --needed --noconfirm 3ds-dev nds-dev gp32-dev gba-dev gp2x-dev`
  - l.7 `dkp-pacman -S --needed --noconfirm 3ds-portlibs nds-portlibs armv4t-portlibs`
  - l.10 `ENV DEVKITARM=${DEVKITPRO}/devkitARM`
- [toolchain-base/Dockerfile](https://github.com/devkitPro/docker/blob/bee28ad01763/toolchain-base/Dockerfile) : base `debian:bookworm-slim`, installe `dkp-pacman` via le script `install-devkitpro-pacman`, `ENV DEVKITPRO=/opt/devkitpro`, `PATH` inclut `${DEVKITPRO}/tools/bin`. Contient make, cmake, git, python3, etc.
- `libctru` 2.7.0 est dans le groupe `3ds-dev` ([libctru/PKGBUILD l.4 et l.16](https://github.com/devkitPro/pacman-packages/blob/b9e8b0de6a64/libctru/PKGBUILD)).
- `3ds-curl` est dans le groupe `3ds-portlibs` (curl PKGBUILD l.21) ⇒ déjà présent dans l'image.
- Commande type (PowerShell, depuis le dossier projet) :
  `docker run --rm -v ${PWD}:/project -w /project devkitpro/devkitarm make`
  (syntaxe classique ; le README Docker devkitPro ne donne pas d'exemple d'usage).
- Comparaison avec l'installeur Windows : la page devkitpro.org/wiki/Getting_Started a renvoyé HTTP 403 à ma lecture → **NON VÉRIFIÉ** ici. Fait connu : l'installeur Windows fournit un shell MSYS2 avec `dkp-pacman`. GameSync documente ce chemin : « open the devkitPro MSYS2 shell (not Git Bash) » + `pacman -S 3ds-dev 3ds-zlib` ([3ds/README.md](https://github.com/pepasjc/GameSync/blob/8766c36bd830/3ds/README.md)).
  - Docker : rien à installer sur Windows, versions figées, mais pas de 3dslink natif.
  - Natif : 3dslink.exe inclus (groupe `3ds-dev`), boucle de test directe.
  - Option pratique : build dans Docker + 3dslink.exe natif.

#### 2.2 Template

- [templates/application/Makefile](https://github.com/devkitPro/3ds-examples/blob/be2001fee08c/templates/application/Makefile) : `TARGET` = nom du dossier, `SOURCES := source`, `LIBS := -lctru -lm`, `include $(DEVKITARM)/3ds_rules`. Sortie : `$(OUTPUT).3dsx` (+ `.smdh` sauf `NO_SMDH`). Pas de cible 3dslink, pas de cible CIA.
- Pour curl : ajouter `-lcurl -lmbedtls -lmbedx509 -lmbedcrypto -lz` à `LIBS` et `$(PORTLIBS)` à `LIBDIRS` (ordre exact des libs : **NON VÉRIFIÉ**, à confirmer au premier link).

#### 2.3 Boucle de test avec 3dslink

- README hbmenu ([3ds-hbmenu README.md l.66-75](https://github.com/devkitPro/3ds-hbmenu/blob/5a0110a6e8f7/README.md)) : « Press Y to activate as usual then run `3dslink <3dsxfile>` if your network can cope with UDP broadcast messages. » Sinon utiliser `-a <ip address>`. « 3dslink is provided [...] as part of the 3ds-dev package group. »
- Source 3dslink ([host/src/main.c](https://github.com/devkitPro/3dslink/blob/c2df7362a156/host/src/main.c)) :
  - l.39 `#define NETLOADER_COMM_PORT 17491`
  - l.180-236 `find3DS()` : broadcast UDP (`SO_BROADCAST`, `INADDR_BROADCAST`) sur le port 17491.
  - l.487-497 options : `-a/--address`, `-r/--retries`, `-0/--arg0`, `-s/--server`.
  - l.650-690 : avec `-s`, le PC **écoute en TCP sur 17491** et recopie sur `stdout` ce que la 3DS envoie (l.719).
- Côté app : [libctru 3dslink.h](https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/include/3ds/3dslink.h) fournit `link3dsStdio()` (redirige stdout+stderr) et `__3dslink_host`. Exemple officiel : `3ds-examples/network/3dslink-demo`.
- **Docker sous Windows** (déduit + doc Docker) :
  - Le broadcast UDP depuis un conteneur en réseau bridge reste dans le réseau Docker.
  - Le mode `-s` exige que la 3DS se connecte au PC sur TCP 17491. Il faut publier ce port (`-p 17491:17491`).
  - Docker Desktop ≥ 4.34 offre un « host networking » opt-in, « works on layer 4 ». La doc ne parle pas du broadcast ni du multicast ([doc Docker](https://docs.docker.com/engine/network/drivers/host/)) → **NON VÉRIFIÉ**.
  - **Contournement fiable** : 3dslink.exe natif, `3dslink -a 192.168.x.y build\app.3dsx -s`. Autorise TCP 17491 entrant dans le pare-feu Windows pour `-s`.

#### 2.4 Paquet `.cia` (court)

- **makerom** : [3DSGuy/Project_CTR](https://github.com/3DSGuy/Project_CTR), actif (push 2026-10-11). Dernière release `makerom-v0.19.0` (2026-01-18), binaire `win_x86_64` fourni.
- **bannertool** : le dépôt original `Steveice10/bannertool` renvoie 404. Fork maintenu : [carstene1ns/3ds-bannertool](https://github.com/carstene1ns/3ds-bannertool), « Backup of Steveice10's bannertool with CMake build system ». Licence MIT, release 1.2.3 (2026-06-13), binaires Windows fournis.
- Ces outils ne sont pas dans les paquets devkitPro (aucune entrée dans l'arbre `pacman-packages`).
- Pour ce projet, le `.3dsx` lancé depuis le Homebrew Launcher suffit. Le `.cia` n'ajoute qu'une icône sur le menu HOME.

### Sources

- https://github.com/devkitPro/docker/tree/bee28ad01763
- https://github.com/devkitPro/pacman-packages/blob/b9e8b0de6a64/libctru/PKGBUILD
- https://github.com/devkitPro/3ds-examples/tree/be2001fee08c/templates/application
- https://github.com/devkitPro/3ds-hbmenu/blob/5a0110a6e8f7/README.md
- https://github.com/devkitPro/3dslink/blob/c2df7362a156/host/src/main.c
- https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/include/3ds/3dslink.h
- https://docs.docker.com/engine/network/drivers/host/
- https://github.com/3DSGuy/Project_CTR/releases/tag/makerom-v0.19.0
- https://github.com/carstene1ns/3ds-bannertool

---

## 3. Outils existants

### Réponse courte

**Oui, deux projets récents couvrent une grande part du besoin.** Les deux sont très jeunes, avec un seul auteur et peu d'utilisateurs.

1. **GameSync (pepasjc/GameSync)** couvre environ 80 % du besoin. Il fournit un client 3DS qui lit les `.sav` à côté des ROM ou dans `saves/` (layout TWiLight), un serveur auto-hébergé avec historique (5 versions par défaut) et une détection de conflit à 3 hashes. Il fournit aussi des clients PC qui savent lire les saves melonDS. Limites : HTTP en LAN via `httpc` (donc pas de HTTPS Let's Encrypt depuis la 3DS), pas de Dockerfile serveur, 2 étoiles.
2. **DaeMoon (mirusu400/DaeMoon)** a un client 3DS « complete and running on hardware ». Il découvre les saves nds-bootstrap (`sdmc:/roms/nds/saves`). Il utilise curl + mbedTLS avec ISRG Root X1 embarqué, donc il marche avec Let's Encrypt. Il a un serveur Go + SQLite + Dockerfile, ne fusionne jamais automatiquement et garde les deux versions en conflit. Il **n'a pas de support melonDS documenté** : seul un CLI `daemoonctl` existe côté PC.

Aucun des deux n'a de lanceur Windows pour melonDS (sync avant/après la partie). Détails et réserves ci-dessous.

### Détails

#### 3.1 Candidats côté 3DS

| Projet | Ce qu'il fait | Saves TWiLight (SD) ? | Serveur auto-hébergé ? | Dernier commit | Licence |
|---|---|---|---|---|---|
| [pepasjc/GameSync](https://github.com/pepasjc/GameSync) | Sync multi-plateforme (3DS, DS, Wii U, PSP, Vita, PS3, Android, Steam Deck, desktop) vers un serveur FastAPI | **Oui** : `nds_dir` configurable ; cherche `<dir>/<stem>.sav` puis `<dir>/saves/<stem>.sav` ([nds.c](https://github.com/pepasjc/GameSync/blob/8766c36bd830/3ds/source/nds.c)) ; ID = `00048000` + gamecode de la ROM | Oui (Python/uv, pas de Dockerfile ; unité systemd `server/3dssync@.service`) | 2026-09-23 (push 2026-10-14) ; release 0.5.4 | GPL-3.0 |
| [mirusu400/DaeMoon](https://github.com/mirusu400/DaeMoon) | Sync 3DS + Switch + nds-bootstrap, serveur Go/SQLite, panneau web, appairage QR | **Oui** : « The layout is TWiLightMenu's » ; défaut `sdmc:/roms/nds` + `sdmc:/roms/nds/saves` ([daemoon_3ds.h l.61-62](https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a29/platform/3ds/source/daemoon_3ds.h)) | Oui (`docker/server.Dockerfile`) | 2026-09-12 (push 2026-10-31) ; v0.1.2 | MIT |
| [dwalker109/cloudpoint](https://github.com/dwalker109/cloudpoint) | Sync des saves de titres 3DS + extdata, gestion des conflits, versions gardées côté serveur | **Non** : titres 3DS installés seulement (README) | Oui (Docker + Caddyfile dans `cloudpoint_server/`) | 2026-10-28 | MIT |
| [FlagBrew/Checkpoint](https://github.com/FlagBrew/Checkpoint) v5.3.0 | Gestion de saves 3DS/Switch ; **moteur de scripts C** avec SD I/O + HTTP (méthodes, en-têtes, upload streamé) ; scripts fournis `googledrive` et `webdav` | Pas nativement (titres, cartouches DS, DSiWare). Un **script** peut lire `sdmc:/…` et faire des PUT | WebDAV via script ; HTTP via libcurl, « does not verify TLS certificates » ([webdav.md](https://github.com/FlagBrew/Checkpoint/blob/740f221a853d/scripts/webdav.md)) | 2026-10-13 | GPL-3.0 |
| [szclsya/3DavSync](https://github.com/szclsya/3DavSync) | Sync de dossiers SD arbitraires vers WebDAV (curl) | Oui (dossier libre `LocalPath`) | Oui (tout WebDAV) | 2025-08-23 | GPL-3.0 |
| [Kyraminol/3DSync](https://github.com/Kyraminol/3DSync) | Upload de dossiers vers Dropbox | Dossier custom possible | **Non** (Dropbox seulement) | 2020-06-01 | aucune |
| [regimensocial/Citrahold-3DS](https://github.com/regimensocial/Citrahold-3DS) | Sync 3DS ↔ Citra via service web + client PC | Non documenté (**NON VÉRIFIÉ** dans le code) | Serveur Node publié ([Citrahold-Server](https://github.com/regimensocial/Citrahold-Server)) | 2024-12-24 | GPL-3.0 |
| [save-cloud/save-cloud-3ds](https://github.com/save-cloud/save-cloud-3ds) | Sauvegarde vers un cloud chinois | Non documenté | Non (API propriétaire à réimplémenter) | 2025-03-03 | GPL-3.0 |
| [bretheskevin/waystone](https://github.com/bretheskevin/waystone) | Sync E2EE via WebDAV (dufs) | « Planned (M4) », milestone 3DS « Not started » | Oui | 2026-10-26 | GPL-3.0 |
| J-D-K/JKSM | Gestion de saves 3DS | Aucun réseau dans le README | — | 2025-08-28 | GPL-3.0 |

Notes de lecture du code :
- **3DavSync** compare des **mtimes** (`local_mtime > remote_file.last_modified`) et demande A/B à l'utilisateur. Il n'a pas de base commune, donc pas de vraie détection de conflit. Il désactive `CURLOPT_SSL_VERIFYPEER` ([webdav.cpp l.29, l.417-450](https://github.com/szclsya/3DavSync/blob/911ef0da996e/source/webdav.cpp)).
- **GameSync** : modèle à 3 hashes (local, serveur, dernier sync), états `upload / download / up-to-date / conflict` ([docs/technical-overview.md](https://github.com/pepasjc/GameSync/blob/8766c36bd830/docs/technical-overview.md)). Historique : `SYNC_MAX_HISTORY_VERSIONS=5` (README). Le client 3DS utilise `httpc` avec `SSLCOPT_DisableVerify` (network.c l.99). Contributeurs GitHub : `pepasjc`, `claude`, `Copilot`.
- **DaeMoon** : règles écrites dans le README : « never auto merge; the user chooses and both versions are kept » et « never decide freshness by timestamp; the console clock is user settable ». Contributeur unique.

#### 3.2 Côté PC

- **GameSync desktop** (PyQt6, Windows/macOS) : profils de sync « Generic Folder » pour NDS. L'ID est dérivé du nom de fichier (`NDS_slug`). Le serveur a une fonction `canonicalize_slug_title_id` qui ramène un slug vers l'ID hex de la 3DS ([shared/sync_id.py l.319-329](https://github.com/pepasjc/GameSync/blob/8766c36bd830/shared/sync_id.py)). Le scanner Steam Deck lit le gamecode de la ROM pour melonDS ([steamdeck/scanner/melonds.py](https://github.com/pepasjc/GameSync/blob/8766c36bd830/steamdeck/scanner/melonds.py)). La convergence 3DS ↔ melonDS Windows sur le même slot est **prévue par le code mais NON VÉRIFIÉE de bout en bout**.
- **DaeMoon `daemoonctl`** : CLI qui lie le même cœur C. Layout attendu `saves/3ds_<titleid>/main.sav` (README). Pas d'adaptateur melonDS documenté.
- **Ludusavi** ([mtkennerly/ludusavi](https://github.com/mtkennerly/ludusavi), MIT, actif) : backup de saves PC, entrées custom, backup cloud via rclone. Pas de 3DS. Utile seulement comme pièce PC.
- **RetroArch** : le build 3DS a `HAVE_CLOUDSYNC = 1` ([Makefile.ctr l.112](https://github.com/libretro/RetroArch/blob/fab8d5fe5dce/Makefile.ctr)). Mais RetroArch 3DS ne lance pas les jeux DS via TWiLight/nds-bootstrap. Hors sujet.
- **melonDS** : aucune fonction cloud trouvée (**NON VÉRIFIÉ** en détail).
- **ftp_sync** ([branflakes2/ftp_sync](https://github.com/branflakes2/ftp_sync), MIT, 2026-08) : script Python pour des paires de fichiers via ftpd. Il synchronise si un seul côté a changé et **ne fait rien si les deux ont changé**. Il garde des backups. Il est conçu pour « sync pokemon saves between my hacked 3ds running ftpd and my server ».
- **3DS-Save-Sync** ([Lokthran](https://github.com/Lokthran/3DS-Save-Sync), MIT, 2025-08) : GUI Python FTP, transfert manuel avec backup horodaté, scan réseau pour trouver la 3DS.

#### 3.3 Option B : ftpd + script PC

- **Découverte réseau** : ftpd contient un **répondeur mDNS** ([source/mdns.cpp](https://github.com/mtheall/ftpd/blob/8cd5e109fedc/source/mdns.cpp)). Il répond aux requêtes A/ANY (qtype 1/255, l.559) pour `<hostname>.local`. Il n'annonce pas de service `_ftp._tcp` (aucun SRV/PTR de service trouvé). Hostname par défaut sur 3DS : `"3ds-ftpd"` ([source/3ds/platform.cpp l.680](https://github.com/mtheall/ftpd/blob/8cd5e109fedc/source/3ds/platform.cpp)), modifiable avec `SITE HOST`. Port par défaut 5000 (ftpConfig.cpp l.49). ⇒ Le PC peut viser `3ds-ftpd.local:5000` sans IP fixe, si le résolveur mDNS de Windows répond (**NON VÉRIFIÉ** sur Windows).
- **Mtimes** : `FEAT` annonce `MDTM` et `MLST ... Modify` ([ftpSession.cpp l.2733-2753](https://github.com/mtheall/ftpd/blob/8cd5e109fedc/source/ftpSession.cpp)). Les commandes `MDTM`, `MLSD`, `MLST` existent (l.2782, l.2873, l.2886). Les mtimes viennent de `archive_getmtime()` moins le décalage de fuseau lu via `svcGetSystemInfo(..., 0x10000, 0x103)` ([ftpServer.cpp](https://github.com/mtheall/ftpd/blob/8cd5e109fedc/source/ftpServer.cpp)). `SITE MTIME [0|1]` active ou coupe cette lecture. **Pas de `MFMT`** : un client ne peut pas fixer le mtime d'un fichier uploadé.
- **Fiabilité des mtimes 3DS** :
  - libctru `archive_getmtime()` lit `ARCHIVE_ACTION_GET_TIMESTAMP` (ms depuis 2000) ([archive_dev.c l.1330-1360](https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/source/archive_dev.c)). L'horloge est réglable par l'utilisateur (remarque DaeMoon).
  - **Point important** : nds-bootstrap écrit les saves avec son propre driver FAT. `fileWrite()` écrit les secteurs de données du fichier existant. Je n'y ai trouvé aucune mise à jour de l'entrée de répertoire (date de modification) ([retail/common/source/my_fat.c l.1584+](https://github.com/DS-Homebrew/nds-bootstrap/blob/45fda1811a98/retail/common/source/my_fat.c)). ⇒ **Le mtime d'un `.sav` ne change probablement pas quand tu sauvegardes en jeu** (déduit du source, NON VÉRIFIÉ sur matériel). Conséquence : compare des **hashes de contenu**, pas des mtimes. C'est aussi le choix de GameSync et DaeMoon.
- **Outils PC qui synchronisent avec ftpd** :
  - WinSCP `synchronize both` : « both local and remote directories can be modified ». `-criteria` et `-delete` sont ignorés en mode `both`. La doc ne décrit pas de détection de conflit ([doc WinSCP](https://winscp.net/eng/docs/scriptcommand_synchronize)). Avec des mtimes non fiables, c'est risqué.
  - FreeFileSync « Two way » : base `sync.ffs_db` qui compare l'état actuel au dernier sync ([manuel](https://freefilesync.org/manual.php?topic=synchronization-settings)). Elle détecte les changements par mtime/taille. Même problème de mtime. Support FTP : voir le sujet « (S)FTP Setup » du manuel (détails **NON VÉRIFIÉ**).
  - ftp_sync (voir 3.2) : le plus proche d'un sync sûr par paire, mais sans historique serveur.
- Bilan option B : faisable sans code 3DS. Mais il faut lancer ftpd à la main, et le PC doit joindre la 3DS (LAN seulement). Le timestamp ne marche pas pour détecter les changements, et il n'y a pas d'historique sans script maison.

#### 3.4 Conclusion honnête

- **Un outil couvre-t-il ≥ 80 % ?** Oui, **GameSync** sur LAN : client 3DS avec layout TWiLight, serveur avec historique, détection de conflit, clients PC compatibles melonDS. Il manque : HTTPS public depuis la 3DS, image Docker serveur, lanceur melonDS Windows, et une maturité prouvée (2 étoiles, projet de 2026).
- **DaeMoon** est la meilleure base technique pour un accès Internet avec Let's Encrypt. Il a curl + mbedTLS, un CA embarqué, Docker, des règles anti-perte de données et des tests. Mais il n'a pas de client melonDS : il faudrait écrire l'adaptateur PC.
- **Aucun projet** ne fait le scénario complet « 3DS TWiLight + melonDS Windows + serveur auto-hébergé en HTTPS public + historique + conflit sûr + lanceur ».
- Avant d'écrire du code, essaie :
  1. GameSync sur LAN (serveur `uv run`, `nds_dir` sur la 3DS, profil melonDS sur PC).
  2. DaeMoon (serveur Docker personnel, client 3DS).
  Puis choisis : contribuer (adaptateur melonDS pour DaeMoon, ou Dockerfile + client curl pour GameSync) ou écrire ton propre outil.

### Sources

- GameSync : https://github.com/pepasjc/GameSync/tree/8766c36bd830
- DaeMoon : https://github.com/mirusu400/DaeMoon/tree/d6d6ff390a29
- Cloudpoint : https://github.com/dwalker109/cloudpoint/tree/dd3ec875ac32
- Checkpoint : https://github.com/FlagBrew/Checkpoint/tree/740f221a853d
- 3DavSync : https://github.com/szclsya/3DavSync/tree/911ef0da996e
- 3DSync : https://github.com/Kyraminol/3DSync
- Citrahold : https://github.com/regimensocial/Citrahold-3DS , https://github.com/regimensocial/Citrahold-Server
- Waystone : https://github.com/bretheskevin/waystone
- ftp_sync : https://github.com/branflakes2/ftp_sync
- 3DS-Save-Sync : https://github.com/Lokthran/3DS-Save-Sync
- ftpd : https://github.com/mtheall/ftpd/tree/8cd5e109fedc
- nds-bootstrap my_fat.c : https://github.com/DS-Homebrew/nds-bootstrap/blob/45fda1811a98/retail/common/source/my_fat.c
- libctru archive_dev.c : https://github.com/devkitPro/libctru/blob/1f8d386e375c/libctru/source/archive_dev.c
- WinSCP synchronize : https://winscp.net/eng/docs/scriptcommand_synchronize
- FreeFileSync : https://freefilesync.org/manual.php?topic=synchronization-settings
- RetroArch Makefile.ctr : https://github.com/libretro/RetroArch/blob/fab8d5fe5dce/Makefile.ctr
- Ludusavi : https://github.com/mtkennerly/ludusavi
- Recherche GBAtemp : aucun fil pertinent trouvé par la recherche web (résultat négatif, pas une preuve d'absence).
