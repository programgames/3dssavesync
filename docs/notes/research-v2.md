# Recherche v2 — 3DS natif, GBA, GB/GBC, forwarders

> Working notes (French), kept for the record of design decisions.

Date : 2026-11-01. Sources primaires (code à un commit fixe, docs officielles, 3dbrew). Les trois rapports détaillés suivent la synthèse.
Légende : **NON VÉRIFIÉ** = pas de source primaire, ou déduit du code sans test.

## Synthèse

### État des lieux du setup réel (relevé le 2026-10-31, lecture seule)

| Système | PC (LaunchBox) | Sauvegardes PC | Observé |
|---|---|---|---|
| 3DS | Azahar 2126.2 | `C:\Saves\3DS\sdmc` (sdmc = nand) | 1 sauvegarde Azahar (Ultra Soleil), 8 dossiers Checkpoint (X, Y, RO, SA, Soleil, Lune, US, UL), horodatages 2030 = horloge 3DS fausse |
| GBA | mGBA | **`C:/Saves/GB`** (un seul `savegamePath` pour GB, GBC et GBA) | `GBA\` contient des `.sav` de 131 072 o, Émeraude 131 088 o (trailer RTC mGBA 16 o), des `.st0` et un `.dsv` mal rangé |
| GB / GBC | mGBA (Sameboy, RetroArch en secours) | `C:\Saves\GB` | Or/Argent/Cristal 32 816 o (trailer RTC mGBA 48 o), Gen 1 32 768 o, vieux `.rtc` de 23 o lus par aucun émulateur actuel |
| DS | melonDS (déjà synchronisé) | `C:\Saves\DS` | — |

ROM PC : `C:\Roms\{gb,gbc,gba,3ds}\*.zip` (No-Intro). État de la carte SD (open_agb_firm, payloads Luma, ROM GB/GBA) : **à relever** (ftpd éteint pendant l'enquête).

### Ce que dit la recherche, par système

**3DS natif (Pokémon X à Ultra Lune)**
- Côté 3DS : la sauvegarde s'ouvre avec `ARCHIVE_USER_SAVEDATA` ; pour ces 8 jeux c'est **un seul fichier `main`** (415 232 à 483 328 o). Restaurer = écrire `main`, `ControlArchive(COMMIT)`, puis **supprimer la secure value** (ce que font Checkpoint, JKSM, Cloudpoint).
- Côté PC : Azahar range la même archive en fichiers ordinaires dans `…/title/00040000/<low>/data/00000001/main`, à garder avec `00000001.metadata` (16 o). Correspondance 1:1 avec un dossier Checkpoint. Azahar n'émule pas la secure value : une sauvegarde 3DS y est toujours acceptée.
- Identité : un title ID unique par jeu, toutes régions (ex. Ultra Soleil `00040000001B5000`). Cartouche et version installée donnent les mêmes fichiers.
- **Code réutilisable** : Checkpoint est en **C++ sous GPL-3** (réutiliser son code impose la GPL-3 à tout le homebrew). DaeMoon (**MIT, C**, un fichier `save_backend.c`) couvre ouverture, commit et secure value : meilleur modèle. Effort estimé : 350 à 600 lignes de C côté 3DS.
- Point ouvert : droits d'accès FS d'un `.3dsx` (sources contradictoires) ; le `.cia` avec la liste `FileSystemAccess` de Checkpoint lève le doute.
- Azahar : pas de `.zip` (LaunchBox doit extraire) ; title ID lisible dans l'en-tête NCSD/NCCH ; écrit pendant le jeu, donc synchroniser **après** la fin de `azahar.exe`.

**GBA (Rubis, Saphir, Émeraude, Rouge Feu, Vert Feuille)**
- **open_agb_firm** : matériel GBA natif, compatibilité parfaite. Sauvegarde brute `sd:/3ds/open_agb_firm/saves/<nom ROM>.sav`, écrite **seulement à l'extinction par POWER**. RTC réglée sur l'horloge 3DS, non stockée dans le `.sav`. Lancement : chainloader Luma au démarrage (ou `autoboot.txt`). **Pas d'icône HOME sûre** (le PoC exige de remplacer boot9strap).
- **mGBA 3DS** : fichiers identiques à mGBA PC, mais **New 3DS obligatoire** pour le GBA.
- Injects Virtual Console GBA : sauvegarde dans la partition NAND AGBSAVE, extractible seulement avec GodMode9 → **à écarter**.
- TWiLight (GBARunner2) : compatibilité NON VÉRIFIÉE → à écarter.

**GB / GBC (Rouge, Bleu, Jaune, Or, Argent, Cristal)**
- **mGBA 3DS** : même code que mGBA PC → fichiers **identiques à l'octet**, trailer RTC compris (Gen 2). Fonctionne sur Old 3DS pour le GB/GBC.
- TWiLight (GameYob) : SRAM compatible, mais trailer RTC de sens différent (champs `latch` jamais mis à jour) → l'horloge d'Or/Argent/Cristal peut reculer après un aller-retour. Conversion possible mais à tester.

**Forwarders (icône HOME par jeu)**
- **DS** : ndsForwarder + NTR_Forwarder. Le forwarder sauvegarde par défaut dans `<dossier ROM>/saves/<nom>.sav`, **le même chemin que TWiLight** : la synchro actuelle marche sans changement.
- **GB / GBC / GBA via mGBA** : forwarders seulement avec **mGBA 0.11-dev** (outil « Create forwarder » du mGBA PC). Sauvegarde dans `sdmc:/mGBA/forwarders/<nom ROM>.sav`.
- **open_agb_firm** : pas de forwarder sûr.
- **3DS** : icône native (cartouche ou CIA installé).

**PC (mGBA)**
- `.zip` passé directement : le `.sav` prend le nom **du zip**.
- `savegamePath` unique, mais `-C savegamePath=<dossier>` en ligne de commande le remplace par lancement (jamais `--config`, qui fait quitter mGBA).
- Fichier mappé en mémoire, vidé à la fermeture : synchroniser après la fin du processus.
- `autoload` des savestates actif par défaut : un vieux `.ss0` peut recharger une ancienne RAM.

### Règles de format pour la synchro
- GB/GBC/GBA : comparer et hacher **la partie SRAM seulement** (32 Kio GB, 128 Kio GBA) ; les trailers RTC (48 o GB, 16 o GBA) sont des données d'horloge. Avec mGBA des deux côtés, aucun traitement : fichiers identiques.
- open_agb_firm lit seulement les N premiers octets et ne tronque jamais : il garde un éventuel trailer mGBA sans le mettre à jour.
- 3DS : synchroniser le fichier `main` seul ; garder `.metadata` côté Azahar ; supprimer la secure value après chaque restauration sur la 3DS.
- Les dates ne servent jamais : l'horloge de la 3DS est déjà fausse (Checkpoint a daté des sauvegardes de 2030).

---

# Partie 1 — Synchronisation des sauvegardes 3DS natives (3DS réelle <-> Azahar) — recherche sur sources primaires

Date de recherche : 2026-11-01. Toutes les sources de code sont figées à un commit précis :

| Projet | Commit | Licence |
|---|---|---|
| FlagBrew/Checkpoint | `740f221a853db784401458fe74276977afcfa8b0` | GPL-3.0 + termes additionnels 7b/7c |
| azahar-emu/azahar | `15983ee9bf93e069474a27caf679f17aa2b6f5c7` | GPL-3.0 (fichiers `file_sys` en GPLv2+) |
| mirusu400/DaeMoon | `d6d6ff390a292baf2f5aed42551d3f21245034e0` | MIT |
| regimensocial/Citrahold-3DS | `1c71efcacb23cd69f235dbc9492b099cf38e5fbb` | GPL-3.0 |
| regimensocial/citraholdUI (Citrahold PC) | `c4947c13119e41ad540155d2dff6414ca900d2e5` | GPL-3.0 |
| dwalker109/cloudpoint | `dd3ec875ac3236176b807f251efe94f22e15fd84` | MIT (Rust) |
| J-D-K/JKSM | `f883dff8fccbbb90851c3c6c2a16ea8bda250356` | GPL-3.0 |
| devkitPro/libctru | `62b7c42c88b1ee837f513358409d2e5c8f9001a9` | zlib |
| LumaTeam/Luma3DS | `cef6015b5d7005e51675855ff8b451299dcf5706` | GPL-3.0 |
| FlagBrew/PKSM | `9cad15694c1e90cc058ddd285eb68d56c7ad9ac8` | GPL-3.0 |
| kwsch/PKHeX | `de2e53d19466f72d186bd4ee189e6c18085c7ff7` | GPL-3.0 |

Note : le dépôt `regimensocial/Citrahold-PC` n'existe pas. Le client PC de Citrahold s'appelle `regimensocial/citraholdUI`.

Observations locales (lecture seule, aucun fichier modifié) :

- `C:\Saves\3DS\` contient 8 dossiers Checkpoint : `0x0055D Pokémon X`, `0x0055E Pokémon Y`, `0x011C4 Pokémon Omega Ruby`, `0x011C5 Pokémon Alpha Sapphire`, `0x01648 Pokémon Sun`, `0x0175E Pokémon Moon`, `0x01B50 Pokémon Ultra Sun`, `0x01B51 Pokémon Ultra Moon`. Chaque sauvegarde contient un seul fichier `main`. Tailles : X/Y 415232, OR 483328, Sun 441856. Plusieurs dossiers sont vides (AS, Moon, US, UM).
- Plusieurs horodatages Checkpoint commencent par `2030...` (exemple `20300924-224846`). L'horloge de la 3DS est donc fausse. Ne prenez pas ces noms comme une date fiable.
- Azahar `title/00040000/001b5000/data/00000001.metadata` = `00 00 04 00 | 01 00 00 00 | 01 00 00 00 | 01 00 00 00` : total_size = 0x00040000, 1 répertoire, 1 fichier, duplicate_data = 1 (3 octets de bourrage).
- Azahar contient aussi l'extdata `extdata/00000000/00001B50/` avec `icon`, `metadata`, `boss/`, `user/` (sous-dossiers `btvideo`, `dummy2`, `dummy3`, `finder_studio`, `reg`).
- Le projet actuel construit un `.3dsx` et un `.cia`. Son `app.rsf` déclare seulement `CategoryFileSystemTool` et `DirectSdmc` dans `FileSystemAccess` (`3ds/app.rsf`, lignes 35-43).

---

## Question 1 — Checkpoint : sauvegarde et restauration d'une sauvegarde de titre 3DS

### Réponse courte

- Checkpoint ouvre `ARCHIVE_USER_SAVEDATA` (0x567890B2) avec un chemin binaire `{mediatype, lowid, highid}`. Le même code sert pour la cartouche (`MEDIATYPE_GAME_CARD`) et pour le SD (`MEDIATYPE_SD`). L'extdata passe par `ARCHIVE_EXTDATA` avec `{MEDIATYPE_SD, extdataId, 0}`.
- La sauvegarde (backup) copie toute l'arborescence de la racine `/` de l'archive dans `sdmc:/3ds/Checkpoint/saves/0x%05X <nom court>/<nom de backup>/`. Le nom par défaut est `AAAAMMJJ-HHMMSS`. Les fichiers sont copiés tels quels, sans en-tête ni conteneur.
- La restauration : vérification de la source, puis `FSUSER_DeleteDirectoryRecursively(archive, "/")`, puis copie, puis `FSUSER_ControlArchive(..., ARCHIVE_ACTION_COMMIT_SAVE_DATA, ...)`, puis `FSUSER_ControlSecureSave(SECURESAVE_ACTION_DELETE, ...)`. Checkpoint n'a pas d'étape de formatage de l'archive.
- Checkpoint est en C++ (std::u16string, std::vector, classes). Notre homebrew est en C. Une réutilisation directe demande un portage.
- Licence GPL-3.0 avec termes additionnels 7b/7c. Si vous copiez du code Checkpoint dans le homebrew et que vous distribuez le binaire, le homebrew entier doit être distribué sous GPL-3.0. Le serveur et le client PC ne sont pas concernés (programmes séparés qui communiquent par HTTP).

### Détails

**Ouverture des archives**

- Sauvegarde de titre : `Archive::save()` utilise `ARCHIVE_USER_SAVEDATA` avec `{mediatype, lowid, highid}`, 12 octets. Pour `MEDIATYPE_NAND`, il utilise `ARCHIVE_SYSTEM_SAVEDATA` (sauvegardes système). [archive.cpp L68-L79]
- Extdata : `Archive::extdata()` utilise `ARCHIVE_EXTDATA` avec `{MEDIATYPE_SD, ext, 0}`. [archive.cpp L87-L91]
- L'identifiant d'extdata vient de `TitleQuirks::extdataIdFor()`. La valeur par défaut est `low >> 8`. Il y a des exceptions : Pokémon Y -> 0x055D, Omega Ruby -> 0x11C5, Moon -> 0x1648, Ultra Moon -> 0x1B50. Donc X/Y, OR/AS, Sun/Moon et US/UM partagent une même archive extdata par paire. [titlequirks.cpp L57-L84]
- Constantes libctru : `ARCHIVE_SAVEDATA = 0x4`, `ARCHIVE_EXTDATA = 0x6`, `ARCHIVE_USER_SAVEDATA = 0x567890B2`. [fs.h L54-L55, L75]

**Liste des titres**

- Cartouche : `FSUSER_GetCardType()`. Si `CARD_CTR`, alors `AM_GetTitleCount(MEDIATYPE_GAME_CARD)` et `AM_GetTitleList(..., MEDIATYPE_GAME_CARD, 1, &id)`, puis `TitleProbe::probe()`. [loader.cpp L335-L375]
- SD : `AM_GetTitleCount(MEDIATYPE_SD)`, `AM_GetTitleList()`, filtre `validId()`, puis `probe()`. Le titre est gardé si la sauvegarde ou l'extdata est accessible. [loader.cpp L479-L497]
- Filtre : exclusion des manuels, des mises à jour (`0004000E`) et des données DSi `0004800F`. [titlequirks.cpp L29-L55], [loader.cpp L51-L58]
- Probe CTR : lecture du SMDH, nom court en anglais (`applicationTitles[1]`), calcul des chemins, test d'ouverture de l'archive (`accessible()`), création du dossier de backup. [titleprobe.cpp L56-L107]
- `accessible()` ouvre simplement l'archive. Une ouverture réussie = la sauvegarde existe. [archive.cpp L151-L178]

**Format du dossier de backup**

- Racine : `/3ds/Checkpoint/saves/` et `/3ds/Checkpoint/extdata/`. [paths.cpp L70-L78]
- Nom du dossier de titre : `"0x%05X "` de `(u32)id >> 8` (unique ID), puis le nom court nettoyé. Exemple : `0x0055D Pokémon X`. [paths.cpp L85-L91], [title.cpp L231-L234]
- Nom de backup par défaut : `"%04i%02i%02i-%02i%02i%02i"` (heure locale de la console). [common.cpp L38-L46]
- Contenu : copie récursive de la racine `/` de l'archive vers `<dossier>/`. [io.cpp L647-L689]

**Backup (`io::backup`)**

1. Ouvre l'archive. [io.cpp L609-L613]
2. Supprime le dossier de destination s'il existe, puis le recrée. [io.cpp L615-L628]
3. Liste l'arbre (`collectTree`), vérifie la longueur des chemins (limite 0x100 unités UTF-16). [io.cpp L655-L679], [io.hpp L89]
4. Copie (`copyTree` -> `copyFile`, tampon 0x50000). [io.cpp L688-L689], [io.cpp L223-L296], [io.hpp L39]
5. En cas d'échec ou d'annulation, supprime le dossier de backup partiel. [io.cpp L691-L700]

Attention : l'étape 2 supprime un backup existant du même nom avant de copier. Un backup qui échoue à l'étape 4 détruit donc l'ancien backup du même nom.

**Restore (`io::restore`)**

1. Ouvre l'archive. [io.cpp L798-L802]
2. Liste la source SD **avant** de toucher la console. [io.cpp L821-L829]
3. Retire les fichiers de métadonnées d'hôte (par exemple AppleDouble `._*`). [io.cpp L831-L839]
4. Vérifie la longueur des chemins. [io.cpp L849-L853]
5. Refuse un backup sans fichier. [io.cpp L862-L867]
6. Supprime tout le contenu de l'archive : `FSUSER_DeleteDirectoryRecursively(handle.fs(), "/")`. [io.cpp L875-L878]
7. Copie les fichiers. `FSStream` en écriture ouvre le fichier ; s'il n'existe pas, il le crée avec `FSUSER_CreateFile(archive, path, 0, taille_source)`. [io.cpp L885-L891], [fsstream.cpp L53-L73]
8. Commit : `FSUSER_ControlArchive(archive, ARCHIVE_ACTION_COMMIT_SAVE_DATA, NULL, 0, NULL, 0)`. [io.cpp L893-L899]
9. Secure value : `FSUSER_ControlSecureSave(SECURESAVE_ACTION_DELETE, &v, 8, &out, 1)` avec `v = (SECUREVALUE_SLOT_SD << 32) | (uniqueId << 8)`. Checkpoint fait cette étape pour toutes les sauvegardes CTR, y compris la cartouche. [io.cpp L901-L907]

Checkpoint ne fait pas de backup automatique avant une restauration (recherche de `io::backup`/`io::restore` dans `MainScreen.cpp`, `transferjob.cpp`, `main.cpp` : seul l'appel de `transferjob.cpp` L167 existe). NON VÉRIFIÉ au-delà de cette recherche.

**Code réutilisable (idées, pas copie)**

- `Archive::save()` et `Archive::extdata()` : 10 lignes chacune. Les appels libctru sont triviaux à réécrire en C.
- `io::copyFile()` / `copyTree()` / `collectTree()` : boucle de copie avec gestion de l'écriture courte et de l'extdata creuse (`0xD900458B`). [io.cpp L35-L55], [io.cpp L223-L327]
- Séquence de restauration L821-L907 : c'est la partie à reproduire fidèlement.
- `TitleQuirks::extdataIdFor()` pour les IDs d'extdata Pokémon.

**Licence**

- Fichier `LICENSE` = GPL-3.0. Chaque fichier ajoute les termes 7b (conserver les mentions d'auteur) et 7c (interdire de présenter une version modifiée comme l'originale). [archive.cpp L1-L25]
- Conséquences pour notre projet (aucune licence actuellement) :
  - Usage privé sans distribution : la GPL ne demande rien. Voir la FAQ GNU « GPLRequireSourcePostedPublic ».
  - Distribution du binaire 3DS qui contient du code Checkpoint : le homebrew complet doit être sous GPL-3.0 (source fournie, mentions conservées).
  - Serveur HTTP et client PC : programmes séparés. La communication par HTTP est une « communication à distance » entre programmes distincts. Ils ne deviennent pas GPL. Voir la FAQ GNU « MereAggregation ».
  - Réécrire la séquence d'appels libctru sans copier le texte du code ne crée pas d'œuvre dérivée. Les appels d'API et la séquence d'un protocole ne sont pas, en pratique, l'élément protégé. (Avis non juridique.)

### Sources

- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/archive.cpp#L68-L91
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/archive.cpp#L151-L178
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/archive.cpp#L1-L25
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/titlequirks.cpp#L29-L84
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/loader.cpp#L335-L375
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/loader.cpp#L479-L497
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/titleprobe.cpp#L56-L107
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/paths.cpp#L70-L91
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/title.cpp#L231-L234
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/common/common.cpp#L38-L46
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/io.cpp#L223-L327
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/io.cpp#L599-L704
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/io.cpp#L788-L910
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/fsstream.cpp#L53-L73
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/include/io.hpp#L39
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/transferjob.cpp#L167
- https://github.com/devkitPro/libctru/blob/62b7c42c88b1ee837f513358409d2e5c8f9001a9/libctru/include/3ds/services/fs.h#L54-L75
- https://www.gnu.org/licenses/gpl-faq.html#MereAggregation
- https://www.gnu.org/licenses/gpl-faq.html#GPLRequireSourcePostedPublic

---

## Question 2 — Azahar : disposition sur disque

### Réponse courte

- Sauvegarde de titre : `<sdmc_directory>Nintendo 3DS/<32 zéros>/<32 zéros>/title/<high %08x>/<low %08x>/data/00000001/` = racine de l'archive, en fichiers ordinaires. Plus `.../data/00000001.metadata` (16 octets).
- Pour Pokémon Ultra Sun : `...\title\00040000\001b5000\data\00000001\main`. Cela correspond à votre observation.
- Un dossier de backup Checkpoint correspond 1:1 au dossier `data/00000001/` : même arbre, mêmes fichiers (`main`). La copie dans les deux sens est une copie de fichiers.
- `.metadata` = `ArchiveFormatInfo` : `total_size`, `number_directories`, `number_files` (u32 LE), `duplicate_data` (u8), 3 octets de bourrage. Azahar l'écrit au formatage. L'ouverture de l'archive ne le lit pas. `GetFormatInfo` le lit et renvoie `NotFormatted` s'il manque. Conservez toujours le fichier existant. Créez-le si le dossier n'existe pas encore.
- Cartouche ou SD : Azahar range toutes les sauvegardes de titre au même endroit (dossier SDMC par `program_id`).
- Extdata : `.../extdata/<high %08x>/<low %08x>/` avec `user/` (= racine de l'archive), `boss/`, `metadata`, `icon`.
- Écriture : Azahar écrit dans le fichier hôte à chaque écriture du jeu (`fwrite`). Il ne vide le tampon que si le jeu le demande, ou à la fermeture. Synchronisez seulement après la fin du processus.
- Lancement : `azahar.exe [-f | -w] "<chemin du jeu>"`. Un seul processus `azahar.exe`. Je n'ai trouvé aucun mécanisme d'instance unique. Un lanceur peut donc attendre la fin du processus.

### Détails

**Chemins**

- `GetSaveDataContainerPath()` = `{sdmc}Nintendo 3DS/{SYSTEM_ID}/{SDCARD_ID}/title/`. [archive_source_sd_savedata.cpp L21-L23]
- `SYSTEM_ID` et `SDCARD_ID` = 32 zéros. [archive.h L22-L24]
- `GetSaveDataPath()` = `{mount}{high:08x}/{low:08x}/data/00000001/` (minuscules). [archive_source_sd_savedata.cpp L25-L29]
- `GetSaveDataMetadataPath()` = `.../data/00000001.metadata`. [archive_source_sd_savedata.cpp L31-L35]
- `Open()` : si le dossier n'existe pas, renvoie `ResultNotFormatted`. Le jeu formate alors une nouvelle sauvegarde. Sinon, `SaveDataArchive` monte ce dossier comme racine. [archive_source_sd_savedata.cpp L44-L63]
- `Format()` : `DeleteDirRecursively(dossier)`, `CreateFullPath`, puis écrit `sizeof(format_info)` octets dans `.metadata`. [archive_source_sd_savedata.cpp L65-L100]
- `GetFormatInfo()` : lit `.metadata` ; s'il manque, renvoie `ResultNotFormatted`. [archive_source_sd_savedata.cpp L102-L139]
- Structure `ArchiveFormatInfo` (16 octets, `static_assert`). [archive_backend.h L111-L118]
- `FormatSaveData` : `total_size = block_size * 512`. [fs_user.cpp L858-L893]
- L'archive `SaveData` (0x4) utilise toujours la source SD, quel que soit le support. [archive_savedata.cpp L19-L22], [archive.cpp L372-L375]

**Correspondance avec Checkpoint**

- Checkpoint copie la racine `/` de l'archive dans le dossier de backup (Q1). Azahar monte `data/00000001/` comme racine. Les deux arbres sont identiques.
- Preuve locale : backup Checkpoint X = un fichier `main` de 415232 octets ; Azahar US = un fichier `main` de 445440 octets. PKHeX attend exactement ces tailles : `SIZE_G6XY = 0x65600` (415232), `SIZE_G6ORAS = 0x76000` (483328), `SIZE_G7SM = 0x6BE00` (441856), `SIZE_G7USUM = 0x6CC00` (445440). [SaveUtil.cs L72-L75]
- Une validation simple avant tout envoi : taille de `main` égale à la valeur PKHeX du jeu, et pied de page `BEEF` (PKHeX `HasSaveFooterBEEF`). [SaveUtil.cs L383-L387]

**Fichier `.metadata`**

- Si vous copiez une sauvegarde 3DS dans Azahar et que le dossier `data/00000001/` existe déjà : remplacez seulement les fichiers. Ne touchez pas `.metadata`.
- Si le dossier n'existe pas (jeu jamais lancé dans Azahar) : il faut créer le dossier **et** `.metadata`. Sans `.metadata`, un jeu qui appelle `GetFormatInfo` reçoit `NotFormatted` et peut reformater (perte de données). NON VÉRIFIÉ pour Pokémon : je ne sais pas si ces jeux appellent `GetFormatInfo`.
- Méthode la plus sûre : lancez une fois le jeu dans Azahar pour qu'il crée sa propre sauvegarde. Ensuite seulement, remplacez `main`.
- Les valeurs de `.metadata` peuvent venir de la 3DS avec `FSUSER_GetFormatInfo()` (libctru fs.h L700). NON VÉRIFIÉ : je n'ai pas vérifié que cet appel accepte `ARCHIVE_USER_SAVEDATA` avec un chemin d'un autre titre.

**Extdata**

- Racine : `{sdmc}Nintendo 3DS/{0}/{0}/extdata/`. [archive_extsavedata.cpp L193-L198]
- Dossier : `{high:08X}/{low:08X}/` (majuscules ici) ; `GetExtDataPathFromId` utilise des minuscules. Sous Windows, la casse ne compte pas. [archive_extsavedata.cpp L186-L205]
- `Open()` monte `user/` (ou `boss/` pour BOSS). Il exige seulement que `user/` existe. [archive_extsavedata.cpp L270-L293]
- Le formatage crée `user/`, `boss/`, `metadata` et `icon`. [archive_extsavedata.cpp L296-L350]
- Donc : backup extdata Checkpoint (racine de l'archive) = contenu de `extdata/00000000/<id>/user/`.
- Pokémon : l'extdata contient sur votre PC `btvideo`, `finder_studio`, `reg`, `dummy2`, `dummy3`. Ce sont probablement les vidéos de combat et les photos du Poké Finder. NON VÉRIFIÉ : je n'ai pas de source sur le fait que le jeu exige l'extdata pour jouer. Le jeu l'a créée lui-même dans Azahar. Le cœur de la progression est dans `main`.
- Attention : X/Y partagent l'extdata 0x55D, OR/AS 0x11C5, Sun/Moon 0x1648, US/UM 0x1B50 (Q1). Une synchronisation d'extdata de Y écrit aussi dans celle de X.

**Moment de l'écriture**

- `DiskFile::Write()` fait `Seek` + `WriteBytes` immédiatement, puis `Flush()` seulement si le jeu le demande. [disk_archive.cpp L30-L39]
- `ControlArchive` (commit) est un « stub » qui renvoie succès : il n'y a pas d'étape de commit sur disque dans Azahar. [archive_backend.h L208-L215]
- Conséquence : pendant le jeu, le fichier sur disque peut être incomplet. Lisez-le seulement après la fin de `azahar.exe`.

**Ligne de commande et processus**

- Le binaire Windows s'appelle `azahar` (`OUTPUT_NAME`). [citra_meta/CMakeLists.txt L7]
- `main()` lance l'interface Qt (`LaunchQtFrontend`). [citra_meta/main.cpp L52-L83]
- Arguments : un seul argument non optionnel = chemin du jeu ; options `-f/--fullscreen`, `-w/--windowed`, `-g`, `-i` (installe un CIA puis quitte), `-v`, `-h`. Le dernier argument non optionnel est le jeu. [citra_qt.cpp L281-L407]
- Le jeu démarre à la fin du constructeur (`BootGame(game_path)`). [citra_qt.cpp L557-L559]
- Azahar crée lui-même des raccourcis avec `-f "<jeu>"`. [citra_qt.cpp L2322-L2325]
- Je n'ai trouvé ni `QLocalServer`, ni `QSharedMemory`, ni instance unique dans `citra_qt.cpp`. Un deuxième lancement crée donc un deuxième processus. NON VÉRIFIÉ sur la version Windows distribuée.

**Autre approche possible : notre homebrew dans Azahar**

- Azahar émule `ARCHIVE_USER_SAVEDATA` (`OtherSaveDataGeneral = 0x567890B2`). [archive.h L50], [archive.cpp L376-L383]
- Il refuse `MEDIATYPE_GAME_CARD` sur cette archive (`ResultGamecardNotInserted`). Il faut passer `MEDIATYPE_SD`. [archive_other_savedata.cpp L109-L122]
- Cloudpoint indique que son client fonctionne dans un émulateur si on installe le titre et qu'on passe par le menu HOME. [cloudpoint README L106]
- Conséquence : le même code C pourrait tourner sur la 3DS et dans Azahar. Le côté PC n'aurait plus à connaître la disposition des dossiers. NON VÉRIFIÉ : je n'ai pas testé l'accès réseau (curl/mbedTLS) dans Azahar.

### Sources

- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/archive_source_sd_savedata.cpp#L21-L139
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/hle/service/fs/archive.h#L22-L24
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/hle/service/fs/archive.h#L50
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/archive_backend.h#L111-L118
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/archive_backend.h#L208-L215
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/hle/service/fs/fs_user.cpp#L858-L893
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/archive_savedata.cpp#L19-L22
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/hle/service/fs/archive.cpp#L372-L383
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/archive_extsavedata.cpp#L186-L350
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/disk_archive.cpp#L30-L39
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/archive_other_savedata.cpp#L109-L122
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/citra_meta/CMakeLists.txt#L7
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/citra_meta/main.cpp#L52-L83
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/citra_qt/citra_qt.cpp#L281-L407
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/citra_qt/citra_qt.cpp#L557-L559
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/citra_qt/citra_qt.cpp#L2322-L2325
- https://github.com/kwsch/PKHeX/blob/de2e53d19466f72d186bd4ee189e6c18085c7ff7/PKHeX.Core/Saves/Util/SaveUtil.cs#L72-L75
- https://github.com/kwsch/PKHeX/blob/de2e53d19466f72d186bd4ee189e6c18085c7ff7/PKHeX.Core/Saves/Util/SaveUtil.cs#L383-L387
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/README.md?plain=1#L106

---

## Question 3 — Secure value (anti-savegame restore) et autres pièges

### Réponse courte

- La 3DS garde, par titre, une « secure value » hors de la sauvegarde. Le jeu compare sa valeur à celle de FS. Si elles diffèrent, le jeu refuse la sauvegarde (« données corrompues »).
- Pokémon gen 6/7 : sources secondaires (Project Pokémon, 2016) pour Sun/Moon. DaeMoon a mesuré sur une console qu'un seul de deux titres Pokémon avait une valeur. Liste exacte par jeu : NON VÉRIFIÉ.
- Solution commune (Checkpoint, JKSM par défaut, Cloudpoint, DaeMoon pour les anciens paquets) : après le commit, supprimez la valeur avec `FSUSER_ControlSecureSave(SECURESAVE_ACTION_DELETE, ...)`. Le jeu écrit une nouvelle valeur à la sauvegarde suivante.
- Azahar ne vérifie rien : toutes les fonctions de secure value sont des stubs. Une sauvegarde de la 3DS est donc toujours acceptée dans Azahar.
- Pour Azahar -> 3DS, il n'y a pas de valeur à restaurer. La suppression est la seule option.
- Cartouche et SD : les fichiers vus par FS sont identiques (FS déchiffre la cartouche). Même title ID. La même copie de `main` fonctionne entre cartouche et version dématérialisée. Le comportement de la secure value pour la cartouche : NON VÉRIFIÉ.

### Détails

**Mécanisme**

- 3dbrew : les commandes `SetSaveDataSecureValue`, `GetSaveDataSecureValue`, `ControlSecureSave`, etc. existent depuis 4.0.0-7. Les applications s'en servent « to verify the value known to the game » (3dbrew, Filesystem services).
- `ControlSecureSave` (0x086700C4) : action 0 = suppression de la secure value. Entrée u64 `(SecureValueSlot << 32) | (TitleUniqueId << 8) | TitleVariation`, sortie u8 « Value Existed ». Action 1 = format. (3dbrew FS:ControlSecureSave)
- libctru : `SECUREVALUE_SLOT_SD = 0x1000`, `SECURESAVE_ACTION_DELETE = 0`, `ARCHIVE_ACTION_COMMIT_SAVE_DATA = 0`. [fs.h L88-L93, L136, L144-L145, L941]
- Masque d'accès des commandes secure value : `0x121004` = « filesystem tool » (bit 2) ou « system settings » (bit 12) ou « savedata move » (bit 17) ou « home menu » (bit 20). Notre `app.rsf` a déjà `CategoryFileSystemTool`. (3dbrew Filesystem services et NCCH/Extended Header)

**Comment les outils le gèrent**

- Checkpoint : commit, puis suppression systématique avec `uniqueId << 8` (variation 0). [io.cpp L893-L907]
- JKSM : option `PreserveSecureValues`. Activée : au backup, il lit la valeur et l'écrit dans `._secure_value` ; au restore, il la réécrit. Désactivée : il la supprime. [BackupMenuState.cpp L221-L235], [BackupMenuState.cpp L289-L309], [FS.cpp L76-L89]
- JKSM fait un commit après chaque fichier copié. [IO.cpp L166-L172]
- Cloudpoint : commit, puis suppression. [driver.rs L133-L141], [ffi.rs L487-L500]
- DaeMoon : il lit la valeur au backup et la place dans le manifeste. Au restore, il la réécrit après le commit. Un paquet sans valeur laisse la console inchangée. [save_backend.c L1022-L1043], [save_backend.c L1061-L1076], [save_backend.c L1121-L1136], [CLAUDE.md L574-L600]
- Note : la valeur de Checkpoint/Cloudpoint ne contient pas `TitleVariation`. Pour les jeux Pokémon, l'octet bas du title ID vaut 00. Le résultat est donc identique.

**Recommandation pour notre outil**

1. Ne restaurez la valeur que si elle vient de la même console et de la même sauvegarde. Sinon, supprimez-la.
2. Pour une sauvegarde qui vient d'Azahar : supprimez toujours la valeur après un commit réussi.
3. Supprimez la valeur seulement après un commit réussi. Si le commit échoue, ne touchez pas la valeur.

**Autres pièges**

- Accès FS d'un `.3dsx` : DaeMoon affirme qu'un `.3dsx` ne peut pas ouvrir la sauvegarde d'un autre titre. [DaeMoon CLAUDE.md L37-L39] Luma3DS donne cependant à tout `.3dsx` lancé par son chargeur `fs_access_info = 0xFFFFFFFF` (« Give access to everything »). [hbldr.c L284] Checkpoint est aussi distribué en `.3dsx`. Conclusion : sous Luma3DS, un `.3dsx` doit fonctionner. NON VÉRIFIÉ sur votre console.
- Accès FS d'un `.cia` : 3dbrew donne le masque `0x6` pour `UserSaveData for check` (0x567890B2), soit « hardware check » ou « filesystem tool ». Notre `app.rsf` a `CategoryFileSystemTool`. DaeMoon réussit avec `CategorySystemApplication`, `DirectSdmc`, `DirectSdmcWrite`. [DaeMoon app.rsf L66-L69], [DaeMoon CLAUDE.md L15-L18] Ces deux faits ne concordent pas. NON VÉRIFIÉ. Solution simple : reprenez la liste complète de Checkpoint. [Checkpoint app.rsf L36-L50]
- Archive inexistante : si le jeu n'a jamais été lancé sur la 3DS, `FSUSER_OpenArchive` échoue. Checkpoint ne formate pas l'archive. Lancez le jeu une fois et faites une sauvegarde en jeu avant le premier restore.
- Liaison à la console : Cloudpoint exclut Super Mario Maker parce qu'il « locks saves to originating movable.sed ». [title_config.toml L5-L9] Pokémon ne figure pas dans cette liste. NON VÉRIFIÉ pour Pokémon, mais Citrahold existe précisément pour échanger des sauvegardes entre 3DS et émulateurs. [Citrahold-3DS README L2]
- Chiffrement : les sauvegardes cartouche ont une couche AES-CTR propre au jeu, et les sauvegardes SD sont des conteneurs DISA chiffrés par la couche SD de la console (3dbrew Savegames). L'API FS (`ARCHIVE_USER_SAVEDATA`) donne les fichiers déjà déchiffrés. Ne lisez jamais les fichiers `.sav` bruts de la carte SD : ils ne sont pas transférables.
- Tolérance aux coupures : l'archive DISA garde l'ancien état jusqu'au commit (duplicate data). Une coupure avant le commit devrait donc laisser l'ancienne sauvegarde intacte. NON VÉRIFIÉ (je n'ai pas lu la page DISA/DIFF de 3dbrew en détail).
- Horloge 3DS fausse (horodatages 2030 observés) : ne choisissez jamais la version la plus récente avec la date de la console.

### Sources

- https://www.3dbrew.org/wiki/Filesystem_services (section Anti Savegame Restore, table des archives)
- https://www.3dbrew.org/wiki/FS:ControlSecureSave
- https://www.3dbrew.org/wiki/NCCH/Extended_Header (table File System Access Info)
- https://www.3dbrew.org/wiki/Savegames
- https://github.com/devkitPro/libctru/blob/62b7c42c88b1ee837f513358409d2e5c8f9001a9/libctru/include/3ds/services/fs.h#L88-L145
- https://github.com/devkitPro/libctru/blob/62b7c42c88b1ee837f513358409d2e5c8f9001a9/libctru/include/3ds/services/fs.h#L941
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/secure_value_backend.cpp#L12-L68
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/io.cpp#L893-L907
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/assets/app.rsf#L36-L50
- https://github.com/J-D-K/JKSM/blob/f883dff8fccbbb90851c3c6c2a16ea8bda250356/JKSM/source/AppStates/BackupMenuState.cpp#L221-L309
- https://github.com/J-D-K/JKSM/blob/f883dff8fccbbb90851c3c6c2a16ea8bda250356/JKSM/source/FS/FS.cpp#L76-L89
- https://github.com/J-D-K/JKSM/blob/f883dff8fccbbb90851c3c6c2a16ea8bda250356/JKSM/source/FS/IO.cpp#L166-L172
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/cloudpoint_app/src/ctr_fs/fs_user/driver.rs#L133-L141
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/cloudpoint_app/src/ctr_fs/fs_user/driver/ffi.rs#L487-L500
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/cloudpoint_app/romfs/title_config.toml#L5-L9
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/source/save_backend.c#L1022-L1136
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/CLAUDE.md?plain=1#L15-L50
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/CLAUDE.md?plain=1#L131-L135
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/CLAUDE.md?plain=1#L574-L600
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/app.rsf#L66-L69
- https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/sysmodules/loader/source/hbldr.c#L284
- Source secondaire (forum, 2016) : https://projectpokemon.org/home/forums/topic/39325-how-to-properly-import-an-edited-save-file-in-sunmoon-tutorial/

---

## Question 4 — Clé d'identité : title ID

### Réponse courte

- Utilisez le title ID complet sur 64 bits (`0004000000xxxx00`) comme clé.
- Les 8 jeux Pokémon ont un seul title ID par jeu, pour toutes les régions et toutes les langues. La cartouche et la version eShop ont le même ID.
- Côté Azahar, la clé est dans le chemin : `title/<high>/<low>/` = `title/00040000/001b5000/`.
- Côté Checkpoint, le dossier n'a que l'unique ID (`0x01B50`) et un nom. Pour retrouver le title ID : `0x00040000` en haut, `uniqueId << 8` en bas.

| Jeu | Title ID | Unique ID (dossier Checkpoint) | Extdata ID |
|---|---|---|---|
| X | 0004000000055D00 | 0x0055D | 0x0055D |
| Y | 0004000000055E00 | 0x0055E | 0x0055D |
| Omega Ruby | 000400000011C400 | 0x011C4 | 0x011C5 |
| Alpha Sapphire | 000400000011C500 | 0x011C5 | 0x011C5 |
| Sun | 0004000000164800 | 0x01648 | 0x01648 |
| Moon | 0004000000175E00 | 0x0175E | 0x01648 |
| Ultra Sun | 00040000001B5000 | 0x01B50 | 0x01B50 |
| Ultra Moon | 00040000001B5100 | 0x01B51 | 0x01B50 |

### Détails

- PKSM vérifie la cartouche insérée contre une liste fixe de 8 IDs, un par jeu, sans variante régionale. [PKSM loader.cpp L1407-L1416] PKSM fonctionne avec les cartouches de toutes les régions. J'en déduis un ID unique par jeu. Ce point est une déduction, mais il concorde avec vos dossiers Checkpoint et Azahar.
- Extdata : `TitleQuirks::extdataIdFor()`. [titlequirks.cpp L57-L84]
- Pour d'autres jeux (hors Pokémon), les régions ont souvent des title IDs différents. NON VÉRIFIÉ pour un jeu précis. Dans ce cas, gardez une table de correspondance manuelle (comme les « game ID » de Citrahold, que l'utilisateur associe à un dossier). [citraholdUI ConfigManager.cpp L19-L34]
- Ne prenez pas le nom du dossier Checkpoint comme clé : il dépend du SMDH et peut changer. [titleprobe.cpp L68-L70]

### Sources

- https://github.com/FlagBrew/PKSM/blob/9cad15694c1e90cc058ddd285eb68d56c7ad9ac8/3ds/source/titles/loader.cpp#L1407-L1416
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/titlequirks.cpp#L57-L84
- https://github.com/FlagBrew/Checkpoint/blob/740f221a853db784401458fe74276977afcfa8b0/3ds/source/titleprobe.cpp#L68-L70
- https://github.com/azahar-emu/azahar/blob/15983ee9bf93e069474a27caf679f17aa2b6f5c7/src/core/file_sys/archive_source_sd_savedata.cpp#L25-L29
- https://github.com/regimensocial/citraholdUI/blob/c4947c13119e41ad540155d2dff6414ca900d2e5/ConfigManager.cpp#L19-L34

---

## Question 5 — Projets existants

### Réponse courte

- **DaeMoon** (MIT, C, libctru, serveur auto-hébergé) : c'est le plus proche de notre architecture. Son backend 3DS est un seul fichier C (`save_backend.c`, 1136 lignes avec beaucoup de commentaires). Licence MIT : réutilisation libre avec la mention de copyright. C'est le meilleur candidat.
- **Checkpoint** (GPL-3, C++) : la référence la plus éprouvée, mais licence contaminante et langage différent. Utilisez-le comme référence de comportement, pas comme code à copier.
- **Citrahold** (GPL-3 ; serveur Apache-2.0) : le client 3DS n'ouvre aucune archive. Il envoie des dossiers de la carte SD (par exemple des backups Checkpoint). Rien à réutiliser pour l'accès FS.
- **Cloudpoint** (MIT, Rust/ctru-rs) : séquence correcte (commit puis suppression de la secure value), mais en Rust. Bonne référence, pas réutilisable directement en C.

### Détails

**DaeMoon**

- Ouverture : `ARCHIVE_USER_SAVEDATA`, chemin binaire `{media, low, high}`. Media par défaut `MEDIATYPE_SD`. [save_backend.c L189-L206]
- Le client 3DS liste seulement `MEDIATYPE_SD` (pas la cartouche). [main.c L2231]
- Filtre : seulement les applications (`high == 0x00040000`). [save_backend.c L873-L878]
- Test d'existence de la sauvegarde = ouverture de l'archive. [save_backend.c L890-L906]
- Écriture : `FS_OPEN_WRITE | FS_OPEN_CREATE`, puis `FSFILE_SetSize(0)`, puis écriture avec `FS_WRITE_FLUSH`. [save_backend.c L426-L487], [save_backend.c L138-L161]
- Vidage avant restore : il collecte d'abord tous les chemins, puis supprime les fichiers un par un (les dossiers restent). [save_backend.c L489-L567]
- Commit : `FSUSER_ControlArchive(..., ARCHIVE_ACTION_COMMIT_SAVE_DATA, ...)` avec contrôle du résultat. [save_backend.c L263-L274]
- Secure value : lecture, écriture, suppression. [save_backend.c L1020-L1136]
- Sécurité : il refuse un backup d'archive vide, car sa restauration effacerait la sauvegarde. [CLAUDE.md L138-L139] Il traite une erreur de lecture de répertoire comme une erreur, pas comme une fin de liste. [save_backend.c L316-L326]
- État : le code indique qu'une restauration sur matériel n'a pas encore été exécutée à un moment du développement. [save_backend.c L240-L243] Plus loin, une trace montre `save/remove-all-done ok`, `save/commit`, `restore/core-done ok`. [CLAUDE.md L5-L18] Le comportement avec une sauvegarde d'une autre console reste ouvert selon l'auteur. [CLAUDE.md L20-L24]
- Licence MIT. [DaeMoon LICENSE]

**Citrahold**

- Description : « sync your saves between 3DSes and Emulators ». [Citrahold-3DS README L2]
- Le client 3DS navigue dans `/3ds/` et propose des sous-dossiers de sauvegarde. Il ne fait aucun appel `FSUSER_OpenArchive`. [directoryMenu.cpp L9-L16], [directoryMenu.cpp L66-L82]
- Le client PC copie des dossiers dans `sdmc/Nintendo 3DS/<0>/<0>/title` ou `.../extdata`, et garde les anciennes versions dans `oldSaves`/`oldExtdata`. [ConfigManager.cpp L19-L34], [ConfigManager.cpp L285-L302], [ConfigManager.cpp L393-L441]
- Licence GPL-3 (3DS et PC).

**Cloudpoint**

- Synchronise sauvegardes et extdata entre consoles « (and, in most cases, emulators) ». Serveur auto-hébergeable. Il garde toutes les versions envoyées. [README L27], [README L108-L110]
- Séquence de fin d'écriture : commit, puis `ctr_reset_secure_save_meta`. [driver.rs L133-L141]
- Liste d'exclusion de titres (NetPass, Super Mario Maker). [title_config.toml L1-L9]
- Licence MIT, mais Rust.

**Choix recommandé**

1. Écrivez votre propre module C (environ 300 à 450 lignes) avec les mêmes appels libctru.
2. Prenez DaeMoon (MIT) comme modèle de code. Copiez des fonctions si vous voulez, avec la mention MIT.
3. Prenez Checkpoint comme référence de comportement (ordre des étapes, refus du backup vide, nettoyage des fichiers d'hôte). Ne copiez pas son texte si vous voulez garder la liberté de licence.

### Sources

- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/source/save_backend.c#L138-L274
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/source/save_backend.c#L316-L326
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/source/save_backend.c#L426-L567
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/source/save_backend.c#L827-L947
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/source/main.c#L2231
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/CLAUDE.md?plain=1#L5-L24
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/platform/3ds/CLAUDE.md?plain=1#L138-L139
- https://github.com/mirusu400/DaeMoon/blob/d6d6ff390a292baf2f5aed42551d3f21245034e0/LICENSE
- https://github.com/regimensocial/Citrahold-3DS/blob/1c71efcacb23cd69f235dbc9492b099cf38e5fbb/README.md?plain=1#L2
- https://github.com/regimensocial/Citrahold-3DS/blob/1c71efcacb23cd69f235dbc9492b099cf38e5fbb/source/directoryMenu.cpp#L9-L82
- https://github.com/regimensocial/citraholdUI/blob/c4947c13119e41ad540155d2dff6414ca900d2e5/ConfigManager.cpp#L19-L34
- https://github.com/regimensocial/citraholdUI/blob/c4947c13119e41ad540155d2dff6414ca900d2e5/ConfigManager.cpp#L285-L441
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/README.md?plain=1#L27
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/README.md?plain=1#L106-L110
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/cloudpoint_app/src/ctr_fs/fs_user/driver.rs#L133-L141
- https://github.com/dwalker109/cloudpoint/blob/dd3ec875ac3236176b807f251efe94f22e15fd84/cloudpoint_app/romfs/title_config.toml#L1-L9

---

## Risques pour un outil qui ne doit jamais détruire une sauvegarde

1. **Vidage avant copie.** Checkpoint et DaeMoon vident l'archive avant d'écrire. Avant tout restore, faites un backup local complet de l'archive actuelle sur la carte SD, puis vérifiez ce backup (relecture + hash). Arrêtez si ce backup échoue.
2. **Backup vide ou partiel.** Une erreur de lecture de répertoire peut ressembler à une archive vide. Traitez toute erreur `FSDIR_Read` comme un échec. Refusez d'envoyer ou de restaurer un backup sans fichier. [DaeMoon save_backend.c L316-L326], [Checkpoint io.cpp L862-L867]
3. **Commit oublié ou ignoré.** Sans commit, rien n'est écrit. Contrôlez le `Result` du commit. Après le commit, rouvrez l'archive et comparez le hash de chaque fichier avec la source.
4. **Ordre secure value.** Supprimez la secure value seulement après un commit réussi et vérifié.
5. **Fichier hôte incomplet dans Azahar.** Azahar écrit pendant le jeu et ne vide le tampon qu'à la demande. Ne lisez ni n'écrivez `data/00000001/` pendant que `azahar.exe` tourne. Vérifiez qu'aucun autre processus `azahar.exe` n'existe (pas d'instance unique).
6. **`.metadata` manquant.** Ne supprimez jamais `00000001.metadata`. Ne créez pas un dossier de sauvegarde Azahar sans ce fichier. Préférez lancer le jeu une fois dans Azahar.
7. **Archive inexistante sur la 3DS.** Un jeu jamais lancé n'a pas d'archive. N'essayez pas de la formater vous-même (`FSUSER_FormatSaveData` ne s'applique qu'à son propre titre). Demandez à l'utilisateur de lancer le jeu une fois.
8. **Extdata partagée.** X/Y, OR/AS, Sun/Moon et US/UM partagent une extdata. Une synchronisation d'extdata pour un jeu modifie l'autre. Commencez sans extdata (seulement `main`).
9. **Mauvais titre.** Utilisez le title ID 64 bits comme clé, jamais le nom. Vérifiez la taille de `main` contre la taille PKHeX attendue pour ce title ID avant d'écrire.
10. **Horloge fausse.** La console a produit des dates en 2030. Pour la synchronisation à trois voies, utilisez les hashes et l'historique du serveur, pas les dates de modification.
11. **Fichiers d'hôte parasites.** Un dossier passé par un PC peut contenir `desktop.ini`, `Thumbs.db` ou `._*`. Filtrez-les avant le restore, comme Checkpoint. [io.cpp L831-L839]
12. **Écriture courte.** `FSFILE_Write` peut écrire moins d'octets que demandé (archive pleine). Traitez ce cas comme une erreur. [Checkpoint io.cpp L281-L285]
13. **Jeu en cours.** Ne restaurez pas une sauvegarde pendant que le jeu tourne sur la 3DS. DaeMoon note qu'il n'existe pas de moyen fiable de le détecter. [save_backend.c L973]
14. **Droits d'accès.** Un échec d'ouverture d'archive à cause des droits ressemble à « pas de sauvegarde ». Ne déduisez jamais « absence de sauvegarde » d'un échec d'ouverture pour décider de supprimer une version côté serveur.

## Estimation d'effort (côté 3DS, en C, hors interface et hors réseau existant)

| Partie | Lignes de C (estimation) |
|---|---|
| Liste des titres : `AM_GetTitleCount/List` (SD + cartouche via `FSUSER_GetCardType`), filtre `0x00040000`, test d'ouverture | 60-90 |
| Nom du titre via SMDH (optionnel ; une table fixe suffit pour 8 jeux Pokémon) | 0-80 |
| Ouverture/fermeture d'archive, conversion de chemins UTF-16 | 40-60 |
| Parcours récursif + backup vers un tampon ou la carte SD | 80-120 |
| Restore : vidage, copie, commit, suppression de la secure value | 80-120 |
| Sécurité : backup avant restore, relecture et hash après commit, refus du vide, filtre des fichiers d'hôte | 80-120 |
| **Total** | **environ 350-600 lignes** |

Pour comparaison, le fichier DaeMoon complet fait 1136 lignes avec beaucoup de commentaires et la lecture des noms SMDH. Sans SMDH et avec une table fixe des 8 jeux Pokémon, la version minimale est vers le bas de la fourchette.

---

# Partie 2 — Partie 2 — Pokémon GB / GBC / GBA sur 3DS, sauvegardes et forwarders HOME

Date de la recherche : 2026-11-01.
Méthode : code source cloné à un commit fixe, README officiels, 3dbrew. GBAtemp seulement en dernier recours, et signalé.
Convention : « NON VÉRIFIÉ » = affirmation non prouvée par une source primaire lue.

## Commits de référence

| Projet | Référence | Commit |
|---|---|---|
| open_agb_firm | tag `beta_2024-12-24` (dernière release) | `6b882e939526a14c9b82d3f18155f91bfc7e1c97` |
| open_agb_firm | master (README lu ici) | `401c3dff900f658d3edf77ababcfaf5dca54a0a8` |
| libn3ds (sous-module épinglé par le tag oaf) | — | `f6717f66858634b677ed695ee346a89db7684b43` |
| mGBA | master (0.11 en développement) | `52de910807c0178d6cfb088aa350952569e8f08f` |
| mGBA | dernière release stable | `0.10.5` |
| TWiLight Menu++ | tag `v27.25.1` | `8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc` |
| GameYob (fork DS-Homebrew) | branche par défaut | `c4d8672806b4a1cff8012fd40de4acf69b0094c2` |
| NTR_Forwarder | tag `SD-30` (dernière release) | `7f69da9f6953313af3a316d891bca455aa6fb1bd` |
| ndsForwarder (MechanicalDragon0687) | HEAD | `ad56edf6202dd49497d65e7275c76c5c7e11c536` |
| Luma3DS | HEAD | `cef6015b5d7005e51675855ff8b451299dcf5706` |
| GodMode9 | HEAD | `7cf777601f757472c7250c60bddd61ed1a0b639d` |
| PKHeX | HEAD | `de2e53d19466f72d186bd4ee189e6c18085c7ff7` |

Abréviations utilisées :
- **oaf** = open_agb_firm.
- **VC** = Virtual Console (émulateur officiel Nintendo dans un titre 3DS).
- **NSUI** = New Super Ultimate Injector (outil Windows qui fabrique des CIA VC).
- **RTC** = Real-Time Clock (horloge temps réel de la cartouche).
- **FIRM** = image de firmware 3DS lancée par le bootloader (B9S, puis Luma3DS).
- **CIA** = paquet d'installation de titre 3DS (icône HOME).

---

## 1. GBA sur 3DS

### Réponse courte

- **open_agb_firm** est la meilleure option pour Pokémon Gen 3. Il utilise le vrai matériel GBA de la 3DS. Il écrit un `.sav` brut sur la carte SD. Il règle la RTC du jeu sur l'horloge de la 3DS à chaque démarrage.
- oaf ne se lance pas depuis HOME avec B9S + Luma3DS. Il se lance au démarrage de la console, par le chainloader de Luma3DS. Un forwarder HOME exige de remplacer le bootloader FIRM0 : je ne le recommande pas.
- Les injects **GBA VC** (NSUI) donnent une icône HOME. Mais la sauvegarde vit dans une partition NAND et dans les données de sauvegarde chiffrées du titre. Un outil de sync ne peut pas lire ce fichier simplement.
- **mGBA 3DS** exige une New 3DS pour le GBA (FAQ officielle). Son format de sauvegarde est identique à mGBA PC. Il peut faire des forwarders CIA, mais seulement avec une build de développement.
- **TWiLight Menu++** lance le GBA avec GBARunner2 (hyperviseur en mode DS). La compatibilité est inférieure à oaf. Je ne le recommande pas.

### Détails

#### 1a. open_agb_firm

**Lancement**
- Le README dit : copie `open_agb_firm.firm` dans `/luma/payloads`, puis maintiens START au démarrage de la 3DS ([README](https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/README.md)).
- Dans Luma3DS, START seul ouvre le menu « Luma3DS chainloader » ([main.c L296-L300](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/main.c#L296-L300), [fs.c L274-L312](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/fs.c#L274-L312)).
- Un nom `<bouton>_<nom>.firm` lance le payload directement avec ce bouton, par exemple `x_open_agb_firm.firm` avec X ([fs.c L241-L270](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/fs.c#L241-L270), [fs.h L31](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/fs.h#L31)).
- oaf montre un navigateur de fichiers. Il mémorise le dernier dossier dans `lastdir.txt` ([open_agb_firm.c L163-L209](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/open_agb_firm.c#L163-L209)).
- Si `sdmc:/3ds/open_agb_firm/autoboot.txt` existe, oaf lance directement la ROM écrite dans ce fichier. Le contenu est un chemin, par exemple `sdmc:/roms/gba/Pokemon Emerald.gba`. oaf coupe au premier `\n`, `\r` ou `\t` ([open_agb_firm.c L284-L292](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/open_agb_firm.c#L284-L292), [fsutil.c L94-L104](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/fsutil.c#L94-L104)).
- Pour quitter : maintiens POWER. oaf écrit la sauvegarde, puis éteint la console ([main.c L39-L57](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/main.c#L39-L57)). Le README dit que le mode veille n'est pas complètement implémenté.

**Forwarder HOME**
- profi200 (auteur) : il y a deux moyens seulement. Le premier passe par le bootloader (PoC [OPEN_AGB_FIRM_Forwarder](https://github.com/ismailhasannnnnn/OPEN_AGB_FIRM_Forwarder)). Le second remplace AGB_FIRM, ce qui n'est pas supporté ([issue #168](https://github.com/profi200/open_agb_firm/issues/168), [issue #199](https://github.com/profi200/open_agb_firm/issues/199)).
- Le PoC exige fastboot3DS ou GodMode9 en FIRM0 à la place de B9S ([README du PoC](https://github.com/ismailhasannnnnn/OPEN_AGB_FIRM_Forwarder), même méthode que [That-Shortcut-Thingy](https://github.com/DullPointer/That-Shortcut-Thingy/tree/202c4068a1cd798ce40119823471eabd983dd53d)). Dernier push du PoC : 2022-11-21.
- Luma3DS sait charger un `agb.firm` externe ([firm.c L108-L123](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/firm.c#L108-L123)). Mais profi200 dit que oaf n'a pas d'accès NAND pour démarrer un titre VC. Un utilisateur dit que l'essai ne fonctionne pas ([issue #151](https://github.com/profi200/open_agb_firm/issues/151)). Statut : NON VÉRIFIÉ, non fonctionnel selon les rapports.
- Conclusion : pas de forwarder oaf sûr avec la configuration actuelle (B9S + Luma3DS).

**Emplacement et format de la sauvegarde**
- `useSavesFolder` vaut `true` par défaut ([config.c L61](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/config.c#L61)).
- Avec `true`, le chemin est `sdmc:/3ds/open_agb_firm/saves/<nom ROM sans .gba>.sav` ([config.h L30-L31](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/include/arm11/config.h#L30-L31), [open_agb_firm.c L212-L247](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/open_agb_firm.c#L212-L247)).
- Avec `false`, le `.sav` est à côté de la ROM.
- `saveSlot` 1 à 9 donne `<nom>.1.sav` … `<nom>.9.sav`. Le slot 0 donne `<nom>.sav` (même source).
- Le fichier `.ini` par jeu est `saves/<nom>.ini` (même source). Le code remplace les 4 derniers caractères : l'extension doit être `.gba` (4 caractères).
- Format : dump brut de la mémoire de sauvegarde, sans en-tête. Taille = taille du type de sauvegarde. Pokémon Gen 3 = Flash 1 Mbit = 131 072 octets ([lgy9.c L52-L77](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/arm9/drivers/lgy9.c#L52-L77)).
- Lecture : oaf lit `saveSize` octets seulement. Il ignore donc un footer RTC de mGBA. Si le fichier n'existe pas, oaf remplit la mémoire avec `0xFF` ([lgy9.c L89-L102](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/arm9/drivers/lgy9.c#L89-L102)).
- Écriture : seulement à la sortie (POWER), et seulement si le hash SHA-256 de la sauvegarde a changé ([lgy9.c L163-L188](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/arm9/drivers/lgy9.c#L163-L188), [lgy11.c L151-L153](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/arm11/drivers/lgy11.c#L151-L153)).
- L'écriture utilise `FA_OPEN_ALWAYS | FA_WRITE` sans troncature ([fsutil.c L41-L56](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/fsutil.c#L41-L56)). Conséquence : si le fichier contient un footer mGBA de 16 octets, oaf réécrit les 131 072 premiers octets et laisse le footer ancien intact. Un test utilisateur confirme que oaf n'écrit pas le footer ([issue #245](https://github.com/profi200/open_agb_firm/issues/245)).
- Options : `saveType` par jeu (avec variantes `_rtc`), `saveOverride`, `defaultSave`, `useGbaDb` ([README](https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/README.md)).

**RTC**
- À chaque lancement, oaf lit l'horloge MCU de la 3DS et l'écrit dans la RTC GBA matérielle ([lgy11.c L93-L117](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/arm11/drivers/lgy11.c#L93-L117)).
- oaf ne stocke pas la RTC dans le `.sav`. La RTC suit donc toujours l'heure de la 3DS.
- La détection automatique traite toutes les puces Flash comme « avec RTC » ([save_type.c L88-L101](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/save_type.c#L88-L101)).
- Problème rapporté : au premier lancement sans `.sav`, l'« Initial Time » d'une nouvelle partie Ruby/Sapphire vaut 1 jour (RTC vue à 2000-01-01). Aux lancements suivants, la RTC est à la date de la console ([issue #245](https://github.com/profi200/open_agb_firm/issues/245)). Un rapport similaire existe pour Emeraude : l'horloge du jeu est décalée après le premier redémarrage ([issue #297](https://github.com/profi200/open_agb_firm/issues/297), ouverte). La cause dans le code est NON VÉRIFIÉE.
- Contournement rapporté (#245) : crée la nouvelle partie dans mGBA PC, ou sauvegarde une fois avant de régler l'horloge, quitte, relance, puis règle l'horloge.

**Compatibilité**
- Exécution native : en théorie tous les jeux, sauf les limites matérielles listées : jeux > 32 MiB, matériel cartouche autre que RTC, câble link, pas de save states ([README](https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/README.md)).
- Les jeux Pokémon Gen 3 n'entrent dans aucune limite. Fonctionne sur Old 3DS et New 3DS (matériel GBA présent dans les deux).

#### 1b. Injects GBA VC (NSUI)

- NSUI est un outil fermé. Je n'ai pas de source primaire pour lui. Son comportement exact est NON VÉRIFIÉ.
- 3dbrew : AGB_FIRM écrit la mémoire de sauvegarde active dans la NAND **à la sortie**. Au redémarrage, NATIVE_FIRM vérifie la sauvegarde et la copie sur la SD ([3dbrew 3DS Virtual Console, section NAND Savegame](https://www.3dbrew.org/wiki/3DS_Virtual_Console)).
- La partition NAND AGB_FIRM SAVE est à l'offset `0x0B100000`, taille `0x30000` ([3dbrew Flash Filesystem](https://www.3dbrew.org/wiki/Flash_Filesystem)).
- Format « AGBSAVE » : en-tête de `0x200` octets (magic `.SAV`, AES-CMAC, title ID, CID de la carte SD, taille), puis la sauvegarde brute. La copie SD a un CMAC recalculé avec une clé issue de `movable.sed` ([3dbrew 3DS Virtual Console](https://www.3dbrew.org/wiki/3DS_Virtual_Console)).
- La copie SD est dans les données de sauvegarde chiffrées du titre (`/Nintendo 3DS/<id0>/<id1>/title/...`). Ce chemin précis est NON VÉRIFIÉ dans 3dbrew (la page dit seulement « out to SD »).
- Luma3DS force un démarrage SysNAND quand AGB_FIRM quitte, pour ne pas perdre la sauvegarde ([main.c L227-L236](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/main.c#L227-L236), note de version v3.7.1). Je n'ai trouvé aucun autre « GBA VC save fix » dans les notes de version de Luma3DS.
- Extraction : GodMode9 → lecteur `S:` → `agbsave.bin` → menu A → « Dump GBA VC save ». Il faut d'abord lancer le jeu sur la console ([README GodMode9](https://github.com/d0k3/GodMode9/blob/7cf777601f757472c7250c60bddd61ed1a0b639d/README.md)).
- GodMode9 écrit `0:/gm9/out/<TitleID>.gbavc.sav`. C'est la sauvegarde brute sans en-tête. GodMode9 inverse les octets des sauvegardes EEPROM ([nandutil.c L133-L157](https://github.com/d0k3/GodMode9/blob/7cf777601f757472c7250c60bddd61ed1a0b639d/arm9/source/utils/nandutil.c#L133-L157)). Pour Flash/SRAM, le format est identique à un `.sav` standard ([mGBA issue #878](https://github.com/mgba-emu/mgba/issues/878), commentaire utilisateur).
- L'injection inverse exige la sauvegarde dans le presse-papiers de GodMode9 (même README).
- RTC : 3dbrew liste des types de sauvegarde « + RTC » (0x4, 0x6, 0x8, 0xA, 0xC). La source de l'heure dans AGB_FIRM et le support RTC par NSUI sont NON VÉRIFIÉS.
- Bilan pour la sync : chaque transfert exige un redémarrage dans GodMode9. Un outil 3DS normal ne peut pas faire cela de façon automatique.

#### 1c. mGBA (port 3DS)

- Performance : la FAQ officielle dit « New 3DS is required for GBA games » ([mgba.io FAQ](https://mgba.io/faq.html)). Sur Old 3DS, le GBA n'est pas jouable à pleine vitesse.
- Audio : la FAQ dit qu'il faut le dump du firmware DSP (même source).
- Dossier de configuration 3DS : `/mGBA` ([config.c L264-L265](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/core/config.c#L264-L265), `project(mGBA)` dans CMakeLists).
- Sauvegarde par défaut : à côté de la ROM, `<nom ROM sans extension>.sav`. Le dossier `save` vaut le dossier de base si `savegamePath` est vide ([directories.c L62-L66](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/core/directories.c#L62-L66), [directories.c L112-L119](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/core/directories.c#L112-L119), [core.c L257-L268](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/core/core.c#L257-L268)).
- Écriture : quelques images après la dernière écriture du jeu (`GBASavedataClean`), puis écriture du footer RTC ([savedata.c L586-L604](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gba/savedata.c#L586-L604)).
- Format RTC GBA : voir section 4. Le code est le même que mGBA PC, donc le fichier est identique octet pour octet.
- Forwarders : voir section 3c. Ils existent seulement à partir de la 0.11 (en développement).
- Point faible du code : `strncpy(initialPath, argv[1], sizeof(PATH_MAX))` copie 4 octets seulement ([3ds/main.c L829-L837](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/3ds/main.c#L829-L837)). Le lancement par argv depuis un autre lanceur est donc probablement cassé. Le lancement par `APT_ReceiveDeliverArg` n'a pas ce défaut (même source). Effet réel NON VÉRIFIÉ.

#### 1d. TWiLight Menu++ (GBA)

- Le lanceur `.gba` choisit dans cet ordre : cartouche Slot-2 (DS seulement), puis GBARunner3 s'il existe dans `/_nds/TWiLightMenu/emulators/GBARunner3.nds`, puis GBARunner2 ([config.gba.ini](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/7zfile/Virtual%20Console%20(3DS,%20Full)/_nds/TWiLightMenu/extras/config.gba.ini), [launchPlanner.cpp L387-L418](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/source/launch/launchPlanner.cpp#L387-L418)).
- Sur 3DS depuis la SD, TWiLight lance `GBARunner2_arm7dldi_3ds.nds` (ou `_nodsp_3ds`) par nds-bootstrap-hb ([launchPlanner.cpp L111-L125](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/source/launch/launchPlanner.cpp#L111-L125)).
- GBARunner2 est un hyperviseur qui fait tourner le GBA en mode DS ([README GBARunner2](https://github.com/Gericom/GBARunner2/blob/6e4ce456597cabaaa45aac83bbd2f4c90580dca8/readme.md)). Il a du code RTC (`arm9/source/emu/romGpioRtc.vram.cpp`). La liste de compatibilité DSi/3DS est sur le wiki GBAtemp. Le wiki m'a renvoyé HTTP 403 : la compatibilité Pokémon est NON VÉRIFIÉE.
- GBARunner3 n'a pas de release publique. Le README du dépôt est vide ([GBARunner3](https://github.com/Gericom/GBARunner3/tree/ecaa817815d9761745606592f04affe5ee9c3731)).
- Emplacement de la sauvegarde GBARunner2 : NON VÉRIFIÉ (probablement à côté de la ROM).

#### Recommandation Gen 3

1. Utilise **open_agb_firm** pour Ruby, Sapphire, Emerald, FireRed, LeafGreen.
2. Pour une nouvelle partie Ruby/Sapphire/Emerald, crée la partie dans mGBA PC, puis copie le `.sav` sur la 3DS. Cette méthode évite le problème RTC du premier lancement (#245, #297).
3. Accepte que le lancement passe par un redémarrage (START ou bouton dédié au boot).

### Sources (section 1)

- [open_agb_firm README @401c3df](https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/README.md)
- [open_agb_firm.c @6b882e9](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/open_agb_firm.c), [main.c](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/main.c), [config.c](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/config.c), [config.h](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/include/arm11/config.h), [save_type.c](https://github.com/profi200/open_agb_firm/blob/6b882e939526a14c9b82d3f18155f91bfc7e1c97/source/arm11/save_type.c)
- [libn3ds lgy9.c @f6717f6](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/arm9/drivers/lgy9.c), [lgy11.c](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/arm11/drivers/lgy11.c), [fsutil.c](https://github.com/profi200/libn3ds/blob/f6717f66858634b677ed695ee346a89db7684b43/source/fsutil.c)
- Issues oaf : [#151](https://github.com/profi200/open_agb_firm/issues/151), [#168](https://github.com/profi200/open_agb_firm/issues/168), [#199](https://github.com/profi200/open_agb_firm/issues/199), [#245](https://github.com/profi200/open_agb_firm/issues/245), [#297](https://github.com/profi200/open_agb_firm/issues/297)
- [Luma3DS main.c](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/main.c), [fs.c](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/fs.c), [firm.c](https://github.com/LumaTeam/Luma3DS/blob/cef6015b5d7005e51675855ff8b451299dcf5706/arm9/source/firm.c)
- [3dbrew 3DS Virtual Console](https://www.3dbrew.org/wiki/3DS_Virtual_Console), [3dbrew Flash Filesystem](https://www.3dbrew.org/wiki/Flash_Filesystem)
- [GodMode9 README](https://github.com/d0k3/GodMode9/blob/7cf777601f757472c7250c60bddd61ed1a0b639d/README.md), [nandutil.c](https://github.com/d0k3/GodMode9/blob/7cf777601f757472c7250c60bddd61ed1a0b639d/arm9/source/utils/nandutil.c)
- [mGBA FAQ](https://mgba.io/faq.html), [mGBA issue #878](https://github.com/mgba-emu/mgba/issues/878)
- [TWiLight launchPlanner.cpp @8d3a32f](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/source/launch/launchPlanner.cpp)

---

## 2. GB / GBC sur 3DS

### Réponse courte

- **mGBA 3DS** est la meilleure option pour la sync. Il utilise le même code que mGBA PC, donc le même format `.sav` et le même footer RTC de 48 octets.
- **TWiLight Menu++** lance GB/GBC avec **GameYob** en mode DS. GameYob met le `.sav` à côté de la ROM. Son footer RTC a la même taille que celui de mGBA, mais son contenu n'est pas compatible pour l'horloge (détails plus bas).
- Les injects **GB/GBC VC** donnent une icône HOME. La sauvegarde `sav.dat` vit dans les données de sauvegarde du titre. L'extraction exige un outil comme Checkpoint (NON VÉRIFIÉ pour ces titres). Le support RTC des injects NSUI est NON VÉRIFIÉ.
- Pour Gen 1 (Red/Blue/Yellow, sans RTC), les trois options marchent. Pour Gen 2 (Gold/Silver/Crystal, avec RTC), choisis mGBA 3DS.

### Détails

#### TWiLight Menu++ + GameYob

- Le lanceur `.gb`/`.gbc` est `/_nds/TWiLightMenu/emulators/gameyob.nds`, en mode DS ([config.gb.ini](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/7zfile/Virtual%20Console%20(3DS,%20Full)/_nds/TWiLightMenu/extras/config.gb.ini), [bootstrappaths.h L27-L28](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/include/common/bootstrappaths.h#L27-L28)).
- L'argument par défaut est `ARG=%PATH%`, donc le chemin complet de la ROM ([customLaunchers.h L9-L22](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/include/common/customLaunchers.h#L9-L22)).
- Le paquet TWiLight prévoit le dossier `roms/gb/` pour les ROM GB/GBC/SGB.
- J'ai lu le fork [DS-Homebrew/GameYob](https://github.com/DS-Homebrew/GameYob/tree/c4d8672806b4a1cff8012fd40de4acf69b0094c2). Le binaire exact livré par TWiLight v27.25.1 vient probablement de ce fork : NON VÉRIFIÉ.
- Chemin de sauvegarde : chemin de la ROM avec `.sav` à la place de l'extension, donc `sd:/roms/gb/<nom>.sav`. Pas de sous-dossier `saves/` ([inputhelper.cpp L740-L744](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/inputhelper.cpp#L740-L744), [main.cpp L216-L222](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/main.cpp#L216-L222)).
- Taille : `numRamBanks × 0x2000`, plus `sizeof(clockStruct)` pour **tout** MBC3 et HuC3. Red/Blue sont donc aussi concernés s'ils sont MBC3 ([inputhelper.cpp L937-L975](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/inputhelper.cpp#L937-L975)).
- `clockStruct` = 5 `int` (s, m, h, d, ctrl) + 5 `int` latch + `time_t last` ([mmu.h L18-L32](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/include/mmu.h#L18-L32)). Taille 48 octets si `time_t` fait 64 bits (devkitARM récent), 44 sinon : NON VÉRIFIÉ pour le binaire livré.
- Écriture : autosave par secteur après 120 images d'écritures SRAM, et sauvegarde complète ailleurs ([inputhelper.cpp L981-L1050](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/inputhelper.cpp#L981-L1050)).
- Différences RTC avec mGBA (déduites du code, comportement réel NON VÉRIFIÉ) :
  - GameYob ne met jamais à jour les champs `latch[5]`. Il les relit et les réécrit tels quels.
  - mGBA lit **seulement** les champs latch et `unixTime` ([mbc.c L648-L665](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c#L648-L665)).
  - Donc un fichier GameYob ouvert dans mGBA donne une horloge fausse.
  - GameYob stocke `last = rawTime − 2 h` (« the same as lameboy »), sur l'heure locale de la DS ([mmu.cpp L668-L693](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/mmu.cpp#L668-L693)). mGBA stocke `time(0)` ([mbc/mbc.c L14-L26](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc/mbc.c#L14-L26)).
  - GameYob met le jour sur 9 bits dans `d`. mGBA sépare `days` (8 bits) et `daysHi`.
  - Un outil de sync pourrait convertir ces champs. Le travail et le risque sont réels.

#### Injects GB/GBC VC

- 3dbrew décrit les fichiers de sauvegarde VC NES/GB/GBC : `sav.dat` (sauvegarde du jeu), `rsm1.dat`/`rsm2.dat` (états émulateur), `SecureValue` ([3dbrew 3DS Virtual Console](https://www.3dbrew.org/wiki/3DS_Virtual_Console)).
- Le format exact de `sav.dat` (brut ou avec en-tête) est NON VÉRIFIÉ.
- Le moment de l'écriture (en jeu, à la suspension, à la fermeture HOME) est NON VÉRIFIÉ.
- Le support de la RTC MBC3 dans les injects NSUI est NON VÉRIFIÉ.
- Extraction : `sav.dat` est dans les données de sauvegarde du titre. Un outil comme Checkpoint peut les exporter : NON VÉRIFIÉ pour les titres injectés.

#### mGBA 3DS (GB/GBC)

- Même chemin et même logique d'écriture que pour le GBA (section 1c).
- Footer RTC seulement pour les cartouches de type `0x0F` et `0x10` (MBC3+TIMER) ([mbc.c L375-L382](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c#L375-L382), [gb.c L257-L292](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/gb.c#L257-L292)).
- Types de cartouche Pokémon (octet `0x147` de la ROM) : Gold/Silver/Crystal `0x10`, Red/Blue `0x13`, Yellow `0x1B`. Ces valeurs viennent de ma connaissance, pas d'une source lue : NON VÉRIFIÉ. Vérifie l'octet `0x147` sur les ROM.
- Performance GB/GBC sur Old 3DS : la FAQ dit seulement que la New 3DS est requise « pour les jeux GBA ». La vitesse GB sur Old 3DS est NON VÉRIFIÉE.
- Risque de fuseau horaire : la 3DS n'a pas de fuseau horaire. `time(0)` sur 3DS donne probablement l'heure locale comme si c'était UTC. Sur PC, `time(0)` donne l'UTC réel. Le footer `unixTime` peut donc différer de 1 h ou 2 h (heure de Paris) entre les deux machines. Effet : l'horloge Gen 2 peut avancer ou reculer de 1-2 h à chaque transfert. NON VÉRIFIÉ : teste avant de jouer longtemps.

#### Recommandation Gen 1 / Gen 2

1. Pour Gold/Silver/Crystal, utilise mGBA 3DS (3DSX ou CIA de la release 0.10.5, ou forwarder 0.11-dev).
2. Pour Red/Blue/Yellow, mGBA 3DS est aussi le plus simple. GameYob reste possible : le format SRAM est brut et les 32 768 premiers octets sont identiques.
3. Évite GameYob pour Gen 2 si la sync avec mGBA PC est un objectif.

### Sources (section 2)

- [TWiLight config.gb.ini @8d3a32f](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/7zfile/Virtual%20Console%20(3DS,%20Full)/_nds/TWiLightMenu/extras/config.gb.ini), [customLaunchers.h](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/include/common/customLaunchers.h), [bootstrappaths.h](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/include/common/bootstrappaths.h)
- [GameYob inputhelper.cpp @c4d8672](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/inputhelper.cpp), [mmu.h](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/include/mmu.h), [mmu.cpp](https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/mmu.cpp)
- [mGBA src/gb/mbc.c @52de910](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c), [src/gb/gb.c](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/gb.c), [src/gb/mbc/mbc.c](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc/mbc.c)
- [3dbrew 3DS Virtual Console](https://www.3dbrew.org/wiki/3DS_Virtual_Console)
- [mGBA FAQ](https://mgba.io/faq.html)

---

## 3. Forwarders pour le menu HOME

### Réponse courte

- **DS** : `ndsForwarder` (app 3DS) + le « nds-bootstrap forwarder pack » (NTR_Forwarder). Le forwarder installe un titre DSiWare qui lance `sd:/_nds/ntr-forwarder/sdcard.nds` avec le chemin de la ROM. Par défaut, la sauvegarde va dans `<dossier ROM>/saves/<nom>.sav`. C'est le même chemin que le défaut de TWiLight : `sd:/roms/nds/saves/`.
- Les réglages du forwarder **ne suivent pas** ceux de TWiLight. Le forwarder lit `sd:/_nds/ntr_forwarder.ini` et ses propres réglages par jeu.
- **GB/GBC/GBA par TWiLight forwarder** : non. ndsForwarder accepte seulement `.nds`, `.srl`, `.ids`. NTR_Forwarder lit un en-tête NDS.
- **mGBA forwarders** : oui, avec mGBA Qt 0.11-dev (menu Tools → « Create forwarder... »). Un CIA par jeu, la ROM est dans le romfs. La sauvegarde va dans `sdmc:/mGBA/forwarders/<nom ROM>.sav`.
- **open_agb_firm forwarders** : pas possible de façon sûre avec B9S + Luma3DS (section 1a).
- **3DS** : les jeux 3DS installés (CIA ou cartouche) ont déjà leur icône HOME.

### Détails

#### 3a. Forwarders DS (TWiLight / nds-bootstrap)

**Fonctionnement**
- Le wiki DS-Homebrew décrit la méthode : installe Universal Updater, puis le paquet « NDSForwarder » et le « nds-bootstrap forwarder pack ». Lance « NDS Forwarder Generator » depuis le Homebrew Launcher. Choisis le jeu, confirme ([wiki DS-Homebrew Forwarders](https://wiki.ds-homebrew.com/ds-index/forwarders)).
- Si `/_nds/ntr-forwarder/sdcard.nds` manque, le forwarder affiche une erreur (même source).
- ndsForwarder remplace l'ancienne app Java Forwarder3-DS. Il crée et installe les forwarders sur le HOME ([README ndsForwarder](https://github.com/MechanicalDragon0687/ndsForwarder/blob/ad56edf6202dd49497d65e7275c76c5c7e11c536/README.md)).
- Limite : 40 titres DSiWare (`MAX_DSIWARE 40`) ([menu.cpp L18](https://github.com/MechanicalDragon0687/ndsForwarder/blob/ad56edf6202dd49497d65e7275c76c5c7e11c536/source/menu.cpp#L18), contrôle en [L198-L201](https://github.com/MechanicalDragon0687/ndsForwarder/blob/ad56edf6202dd49497d65e7275c76c5c7e11c536/source/menu.cpp#L198-L201)).
- Extensions acceptées : `.nds`, `.srl`, `.ids` ([menu.cpp L76-L80](https://github.com/MechanicalDragon0687/ndsForwarder/blob/ad56edf6202dd49497d65e7275c76c5c7e11c536/source/menu.cpp#L76-L80)).
- Le chemin de la ROM est fixé dans le forwarder. Si tu déplaces la ROM, le forwarder affiche « Please recreate the forwarder with the correct ROM path » ([NTR_Forwarder main.cpp L486-L495](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/main.cpp#L486-L495)).

**Fichiers sur la SD (lus dans le code NTR_Forwarder SD-30)**
- `sd:/_nds/ntr-forwarder/sdcard.nds` (le lanceur).
- `sd:/_nds/ntr_forwarder.ini` (réglages globaux du forwarder) ([main.cpp L500-L504](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/main.cpp#L500-L504)).
- `sd:/_nds/ntr-forwarder/gamesettings/<fichier>.ini` (réglages par jeu, touche Y au lancement) ([perGameSettings.cpp L24](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/perGameSettings.cpp#L24)).
- `sd:/_nds/nds-bootstrap-release.nds` ou `-nightly.nds`, et `/_nds/nds-bootstrap.ini` ([main.cpp L985-L990](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/main.cpp#L985-L990)). TWiLight utilise les mêmes chemins ([bootstrappaths.h L6-L13](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/include/common/bootstrappaths.h#L6-L13)). Le pack forwarder peut donc remplacer le nds-bootstrap de TWiLight : NON VÉRIFIÉ (contenu exact du 7z non lu).

**Emplacement de la sauvegarde (point crucial)**
- `SAVE_LOCATION` dans `[NTR-FORWARDER]` de `ntr_forwarder.ini`, défaut `0` ([main.cpp L502](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/main.cpp#L502)) :
  - `0` → `<dossier ROM>/saves/<nom>.sav`
  - `1` → `<dossier ROM>/<nom>.sav`
  - `2` → `sd:/_nds/TWiLightMenu/saves/<nom>.sav` (repli sur `0` si `/_nds/TWiLightMenu` n'existe pas)
  ([main.cpp L612-L645](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/main.cpp#L612-L645), [L520-L522](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/main.cpp#L520-L522)).
- TWiLight a le même enum : `ESavesFolder = 0`, `EGamesFolder = 1`, `ETWLMFolder = 2`, défaut `0` ([twlmenusettings.h L241-L246](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/include/common/twlmenusettings.h#L241-L246), [twlmenusettings.cpp L85](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/source/common/twlmenusettings.cpp#L85), [L283](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/source/common/twlmenusettings.cpp#L283), [main.cpp L692-L700](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/romsel_dsimenutheme/arm9/source/main.cpp#L692-L700)).
- Mais le forwarder lit `sd:/_nds/ntr_forwarder.ini`, pas `sd:/_nds/TWiLightMenu/settings.ini`. Si tu changes la valeur dans TWiLight, change-la aussi dans `ntr_forwarder.ini`.
- Le numéro de slot (`saveNo`) vient des réglages par jeu du forwarder. Slot > 0 donne `<nom>.sav<N>` (même source). Les réglages par jeu de TWiLight (`/_nds/TWiLightMenu/gamesettings/`) ne s'appliquent pas.
- Conséquence pour l'outil de sync : avec les défauts, un jeu DS lancé par TWiLight et le même jeu lancé par forwarder utilisent le même fichier `sd:/roms/nds/saves/<nom>.sav`. Aucun changement n'est nécessaire.

#### 3b. Forwarders TWiLight pour GB/GBC/GBA

- Non avec ndsForwarder et NTR_Forwarder (extensions et en-tête NDS, voir 3a).
- Le fork [I-asked/NDSForwarder-DSi-VC](https://github.com/I-asked/NDSForwarder-DSi-VC) annonce un « VC forwarding support », mais pour DSi SDNAND et flashcards, pas pour le HOME 3DS. Son applicabilité à la 3DS est NON VÉRIFIÉE.

#### 3c. Forwarders mGBA (GB/GBC/GBA)

- Le changelog 0.11.0 (« Future ») annonce « Forwarder support for 3DS and Vita » ([CHANGES L1-L3](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/CHANGES#L1-L3)). La release 0.10.5 n'a pas ce code (pas de `mGUIGetRom`/`romfsInit` dans `src/platform/3ds/main.c` au tag 0.10.5).
- Menu Qt : Tools → « Create forwarder... » ([Window.cpp L1805](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/Window.cpp#L1805)).
- L'outil télécharge le [forwarder-kit](https://github.com/mgba-emu/forwarder-kit/releases/tag/v1) (win32/win64/macOS) ([ForwarderController.cpp L77](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/ForwarderController.cpp#L77)). Il utilise `3dstool` et `bannertool` ([ForwarderGenerator3DS.cpp L141-L189](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/ForwarderGenerator3DS.cpp#L141-L189)).
- La base « stable » est refusée (« TODO: remove this before cutting release »). Choisis la base « dev » ou un fichier de base local ([ForwarderView.cpp L137-L140](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/ForwarderView.cpp#L137-L140)).
- La ROM est copiée dans le romfs du CIA, avec un fichier `filename` ([ForwarderGenerator3DS.cpp L141-L165](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/ForwarderGenerator3DS.cpp#L141-L165)). Le title ID dérive du hash de la ROM ([L92-L116](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/ForwarderGenerator3DS.cpp#L92-L116)).
- Au lancement, le forwarder règle `savegamePath`, `savestatePath`, `screenshotPath`, `cheatsPath` sur `<config>/forwarders` ([gui-runner.c L795-L830](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/feature/gui/gui-runner.c#L795-L830), [3ds/main.c L1090-L1104](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/3ds/main.c#L1090-L1104)).
- Chemin final : `sdmc:/mGBA/forwarders/<nom du fichier ROM sans extension>.sav`.
- Le format est identique à mGBA PC (même code, section 4).
- Inconvénients : la ROM est dupliquée dans chaque CIA. Pour la GBA, il faut une New 3DS.

#### 3d. Forwarders open_agb_firm

- Pas de méthode sûre avec B9S + Luma3DS (section 1a, issues [#168](https://github.com/profi200/open_agb_firm/issues/168), [#199](https://github.com/profi200/open_agb_firm/issues/199)).
- AVERTISSEMENT : le PoC exige de remplacer FIRM0 (B9S) par fastboot3DS ou GodMode9. Une erreur sur FIRM0 peut rendre la console inutilisable sans ntrboot. Ne fais pas cette opération pour une simple icône.

#### Procédure utilisateur recommandée

**DS (forwarders HOME)**
1. Ouvre Universal Updater sur la 3DS.
2. Installe « NDSForwarder », puis le « nds-bootstrap forwarder pack ».
3. Lance « NDS Forwarder Generator » depuis le Homebrew Launcher.
4. Choisis `sd:/roms/nds/<jeu>.nds`, puis confirme.
5. Vérifie que `sd:/_nds/ntr_forwarder.ini` a `SAVE_LOCATION=0` (ou la même valeur que TWiLight).
6. Ne déplace plus la ROM après la création du forwarder.

**GBA (open_agb_firm)**
1. Sur Windows, télécharge `open_agb_firm_beta_20241224.7z` depuis les [releases](https://github.com/profi200/open_agb_firm/releases/tag/beta_2024-12-24).
2. Copie `open_agb_firm.firm` dans `sd:/luma/payloads/`. Pour un lancement direct avec X, renomme-le `x_open_agb_firm.firm`.
3. Copie le dossier `3ds` à la racine de la SD (il contient `3ds/open_agb_firm/gba_db.bin`).
4. Copie les ROM dans `sd:/roms/gba/`. Garde l'extension `.gba`.
5. Éteins la 3DS. Maintiens X (ou START pour le menu) et allume.
6. AVERTISSEMENT : ne retire pas la batterie ou la carte SD pendant le jeu. oaf écrit la sauvegarde seulement à la sortie par POWER.
7. Pour quitter, maintiens POWER. La console s'éteint après l'écriture.
8. Option : crée `sdmc:/3ds/open_agb_firm/autoboot.txt` avec le chemin d'un jeu pour sauter le navigateur. Supprime ce fichier pour revoir le navigateur.

**GB/GBC (et GBA sur New 3DS) avec mGBA**
1. Pour jouer sans icône : copie `mGBA-0.10.5-3ds.7z` (3DSX ou CIA) depuis les [releases mGBA](https://github.com/mgba-emu/mgba/releases/tag/0.10.5). Les saves vont à côté de la ROM.
2. Pour une icône HOME : installe une build de développement de mGBA sur Windows ([mgba.io downloads](https://mgba.io/downloads.html)).
3. Ouvre Tools → « Create forwarder... ». Choisis la ROM, le titre, la plateforme 3DS et la base « dev ».
4. Copie le CIA sur la SD. Installe-le avec FBI.
5. La sauvegarde est dans `sdmc:/mGBA/forwarders/<nom ROM>.sav`.

### Sources (section 3)

- [wiki DS-Homebrew Forwarders](https://wiki.ds-homebrew.com/ds-index/forwarders)
- [ndsForwarder @ad56edf](https://github.com/MechanicalDragon0687/ndsForwarder/tree/ad56edf6202dd49497d65e7275c76c5c7e11c536)
- [NTR_Forwarder main.cpp @SD-30](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/main.cpp), [perGameSettings.cpp](https://github.com/RocketRobz/NTR_Forwarder/blob/7f69da9f6953313af3a316d891bca455aa6fb1bd/SD_Card/sd/arm9/source/perGameSettings.cpp), [release SD-30](https://github.com/RocketRobz/NTR_Forwarder/releases/tag/SD-30)
- [TWiLight twlmenusettings.h @8d3a32f](https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/universal/include/common/twlmenusettings.h)
- [mGBA ForwarderView.cpp @52de910](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/ForwarderView.cpp), [ForwarderGenerator3DS.cpp](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/platform/qt/ForwarderGenerator3DS.cpp), [gui-runner.c](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/feature/gui/gui-runner.c), [forwarder-kit](https://github.com/mgba-emu/forwarder-kit)
- [OPEN_AGB_FIRM_Forwarder](https://github.com/ismailhasannnnnn/OPEN_AGB_FIRM_Forwarder)

---

## 4. Chemins et formats pour l'outil de sync

### Réponse courte

- **GBA / oaf** : `sd:/3ds/open_agb_firm/saves/<nom ROM>.sav`, 131 072 octets bruts pour Pokémon Gen 3. Compatible avec mGBA PC. mGBA ajoute un footer RTC de 16 octets (fichier de 131 088 octets). oaf ignore ce footer et ne le met pas à jour.
- **GB/GBC / mGBA 3DS** : à côté de la ROM, ou `sd:/mGBA/forwarders/<nom ROM>.sav` pour un forwarder. Identique à mGBA PC : 32 768 octets, plus 48 octets RTC pour Gold/Silver/Crystal (32 816 octets).
- **GB/GBC / GameYob** : `sd:/roms/gb/<nom>.sav`. SRAM brute compatible. Footer de 44/48 octets pour tout MBC3, contenu RTC non compatible avec mGBA.
- **Règle pour le hash de contenu** : compare seulement les N premiers octets (taille SRAM/Flash réelle). Ne compare pas le footer.

### Détails

#### Footer RTC GBA de mGBA (vérifié dans le code)

- Structure `GBASavedataRTCBuffer` : `uint8_t time[7]` (BCD : année, mois, jour, jour de semaine, heure, minute, seconde), `uint8_t control`, `uint64_t lastLatch` (little-endian, temps Unix). Total 16 octets ([savedata.h L90-L94](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/include/mgba/internal/gba/savedata.h#L90-L94)).
- Position : à l'offset `GBASavedataSize & ~0xFF`, donc juste après la sauvegarde (131 072 pour Flash 1M) ([savedata.c L606-L638](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gba/savedata.c#L606-L638)).
- mGBA écrit le footer seulement si le jeu a une RTC (`HW_RTC`).
- Ruby/Sapphire/Emerald ont `HW_RTC`. FireRed/LeafGreen ont `HW_NONE`. Tous sont `GBA_SAVEDATA_FLASH1M` ([overrides.c L101-L147](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gba/overrides.c#L101-L147)).
- Lecture : si le footer manque (fichier trop court), mGBA ne change pas la RTC et garde l'heure système. Si le footer existe, mGBA calcule un décalage `lastLatch − heure stockée` ([savedata.c L644-L712](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gba/savedata.c#L644-L712)).

#### Footer RTC GB (MBC3) de mGBA (vérifié dans le code)

- Structure `GBMBCRTCSaveBuffer` : 10 × `uint32_t` (sec, min, hour, days, daysHi, puis les 5 valeurs latch) + `uint64_t unixTime`. Total 48 octets, little-endian ([mbc.h L34-L46](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/include/mgba/internal/gb/mbc.h#L34-L46)).
- Position : à l'offset `sramSize` ([mbc.c L640-L645](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c#L640-L645), [L667-L685](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c#L667-L685)).
- mGBA accepte aussi un footer de 44 octets (`unixTime` sur 4 octets) : le test est `< sizeof − 4` ([mbc.c L648-L657](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c#L648-L657)).
- Seulement pour cartouche `0x0F`/`0x10` ([mbc.c L375-L382](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c#L375-L382)).
- PKHeX accepte les fichiers Gen 1-3 avec un footer de `0x0C` à `0x30` octets après 0x8000 / 0x10000 / 0x20000 ([SaveHandlerFooterRTC.cs](https://github.com/kwsch/PKHeX/blob/de2e53d19466f72d186bd4ee189e6c18085c7ff7/PKHeX.Core/Saves/Util/Recognition/SaveHandlerFooterRTC.cs)). PKHeX est donc un bon outil pour contrôler un fichier synchronisé.

#### Règles proposées pour l'outil de sync

1. Calcule le hash sur les `N` premiers octets seulement : `N = 131072` (GBA Pokémon), `N = 32768` (GB/GBC Pokémon, versions occidentales). Les versions japonaises Gen 2 peuvent avoir 65 536 octets (`SIZE_G2RAW_J = 0x10000` dans [SaveUtil.cs](https://github.com/kwsch/PKHeX/blob/de2e53d19466f72d186bd4ee189e6c18085c7ff7/PKHeX.Core/Saves/Util/SaveUtil.cs)).
2. PC → 3DS (oaf) : copie le fichier complet. oaf lit 131 072 octets et ignore le reste.
3. 3DS (oaf) → PC : copie le fichier complet. Le footer éventuel est l'ancien footer mGBA. Un commentaire utilisateur dans #245 dit que le footer ne joue pas de rôle pour Gen 3, car les deux côtés suivent l'horloge réelle : NON VÉRIFIÉ par moi. Option plus propre : tronque à 131 072 octets ; mGBA utilise alors l'heure système.
4. mGBA 3DS ↔ mGBA PC : copie le fichier complet, footer inclus. Teste le décalage de fuseau horaire (section 2) sur Gold/Silver/Crystal.
5. GameYob → mGBA (Gen 2) : ne copie pas tel quel. Il faut convertir les champs RTC, ou accepter une horloge fausse.
6. Ne dépends pas de la date de modification pour oaf. oaf réécrit le fichier seulement si le contenu a changé, et seulement à l'extinction.

### Sources (section 4)

- [mGBA savedata.h @52de910](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/include/mgba/internal/gba/savedata.h), [savedata.c](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gba/savedata.c), [overrides.c](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gba/overrides.c), [mbc.h](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/include/mgba/internal/gb/mbc.h), [mbc.c](https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/src/gb/mbc.c)
- [libn3ds lgy9.c / fsutil.c @f6717f6](https://github.com/profi200/libn3ds/tree/f6717f66858634b677ed695ee346a89db7684b43/source)
- [PKHeX @de2e53d](https://github.com/kwsch/PKHeX/tree/de2e53d19466f72d186bd4ee189e6c18085c7ff7/PKHeX.Core/Saves/Util)
- [open_agb_firm issue #245](https://github.com/profi200/open_agb_firm/issues/245)

---

## Tableau de recommandation

| Système | Lanceur 3DS | Chemin de sauvegarde 3DS | Compatibilité mGBA PC | Effort / risques |
|---|---|---|---|---|
| GBA Gen 3 (R/S/E, FR/LG) | open_agb_firm (boot Luma : X ou START) | `sd:/3ds/open_agb_firm/saves/<nom>.sav`, 131 072 o | Oui sur les 131 072 o. Footer mGBA 16 o ignoré par oaf, jamais mis à jour | Faible. Pas d'icône HOME. Save écrite seulement à la sortie POWER. Bug RTC au 1er lancement (#245, #297) : crée la partie sur PC |
| GBA (alternative) | mGBA 3DS / forwarder 0.11-dev | à côté de la ROM, ou `sd:/mGBA/forwarders/<nom>.sav` | Identique (footer 16 o inclus) | New 3DS obligatoire. Base forwarder « dev » seulement. Fuseau horaire à tester |
| GBA (déconseillé) | Inject VC NSUI | NAND AGBSAVE + savedata chiffrée du titre ; dump GodMode9 `0:/gm9/out/<TID>.gbavc.sav` | Oui après dump (Flash brut) | Élevé : redémarrage GodMode9 à chaque transfert. RTC NON VÉRIFIÉE |
| GBA (déconseillé) | TWiLight → GBARunner2 | NON VÉRIFIÉ | NON VÉRIFIÉ | Compatibilité inférieure, non vérifiée |
| GB/GBC Gen 2 (Or/Argent/Cristal) | mGBA 3DS / forwarder 0.11-dev | à côté de la ROM, ou `sd:/mGBA/forwarders/<nom>.sav`, 32 768 + 48 o | Identique | Moyen : build dev pour l'icône. Décalage RTC de fuseau possible (NON VÉRIFIÉ) |
| GB/GBC Gen 1 (Rouge/Bleu/Jaune) | mGBA 3DS (ou TWiLight GameYob) | mGBA : idem ; GameYob : `sd:/roms/gb/<nom>.sav` | mGBA : identique. GameYob : SRAM 32 768 o identique, footer à ignorer | Faible |
| GB/GBC (déconseillé pour Gen 2) | TWiLight GameYob | `sd:/roms/gb/<nom>.sav`, SRAM + 44/48 o | SRAM oui ; RTC non (latch non mis à jour, `last` = heure locale − 2 h) | Conversion RTC nécessaire dans l'outil |
| DS (Gen 4-5) | TWiLight ou forwarder ndsForwarder + NTR_Forwarder | `sd:/roms/nds/saves/<nom>.sav` (défaut `SAVE_LOCATION=0` des deux) | (melonDS, hors sujet ici) | Faible. Max 40 forwarders. Réglages forwarder séparés (`ntr_forwarder.ini`). Ne déplace pas les ROM |
| 3DS (Gen 6-7) | HOME natif | savedata du titre (hors sujet ici) | — | — |

## Points NON VÉRIFIÉS principaux

- Cause exacte du bug RTC au premier lancement dans oaf (#245, #297).
- Comportement de NSUI (GBA et GB/GBC) : RTC, format `sav.dat`, moment d'écriture.
- Compatibilité GBARunner2 en mode DSi/3DS pour Pokémon (wiki GBAtemp inaccessible, HTTP 403).
- Binaire GameYob exact livré par TWiLight v27.25.1 et taille de `time_t` (44 ou 48 octets).
- Décalage de fuseau horaire du footer `unixTime` entre mGBA 3DS et mGBA PC.
- Vitesse de mGBA en GB/GBC sur Old 3DS.
- Contenu exact du 7z « nds-bootstrap forwarder pack » (remplace-t-il le nds-bootstrap de TWiLight ?).
- Octets de type de cartouche `0x147` des ROM Pokémon Gen 1/2 (valeurs données de mémoire).

---

# Partie 3 — Partie 3 — PC : mGBA, Azahar, compatibilité des formats GB/GBC/GBA

Sources figées (commit exact) :

| Projet | Réf | Commit |
|---|---|---|
| mGBA | tag `0.10.5` (dernière stable) | `26b7884bc25a5933960f3cdcd98bac1ae14d42e2` |
| Azahar | tag `2126.2` (dernière stable ; `2127.0-alpha*` existent) | `219d743a1e698101fe1ab8bef43e6563126c3c27` |
| open_agb_firm | `master` | `401c3dff900f658d3edf77ababcfaf5dca54a0a8` |
| libn3ds (sous-module figé par open_agb_firm) | — | `ccbb6a94d60bea2e6a1901263251636b192bad3f` |
| GameYob (DS-Homebrew) | `master` | `c4d8672806b4a1cff8012fd40de4acf69b0094c2` |
| TWiLight Menu++ | `master` | `8a90785b79c8e1b5e8ebfb3010eb5ee552bef70f` |

Préfixes de liens utilisés ci-dessous :
- `MG` = `https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/`
- `AZ` = `https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/`
- `OAF` = `https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/`
- `N3DS` = `https://github.com/profi200/libn3ds/blob/ccbb6a94d60bea2e6a1901263251636b192bad3f/`
- `GY` = `https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/`

Les liens complets sont écrits en entier dans chaque section « Sources ».

Toutes les affirmations viennent d'une lecture du code. Aucune n'a été testée en exécution. Une affirmation marquée « NON VÉRIFIÉ » n'a pas de source primaire consultée.

---

## 1. mGBA (PC, Qt, 0.10.5)

### Réponse courte

- **(a) Nom du fichier.** Avec un `.zip`, mGBA nomme la sauvegarde d'après **le nom du zip**, sans l'extension : `Pokemon Crystal.zip` → `Pokemon Crystal.sav`. Le nom du fichier interne au zip n'est pas utilisé. Le fichier va dans `savegamePath` si cette clé existe, sinon dans le dossier du ROM.
- **(a) Par système / par jeu.** Le `config.ini` n'a pas de section par plateforme (GB vs GBA) ni de chemin de sauvegarde par jeu. **Mais** la ligne de commande accepte `-C savegamePath=<dossier>` : cette surcharge a la priorité la plus haute et ne s'écrit pas dans `config.ini`. Le launcher peut donc choisir un dossier par système à chaque lancement.
- **(b) Écriture.** Sous Windows, le `.sav` est un **fichier mappé en mémoire** (`CreateFileMapping` + `MapViewOfFile`, en lecture/écriture). Chaque écriture du jeu modifie directement les pages du fichier. mGBA appelle `FlushViewOfFile` environ 15 images (~0,25 s) après la dernière écriture du jeu, puis à la fermeture. Un kill ne tronque pas le fichier. Les pages modifiées restent dans le cache de Windows. Le risque principal d'un kill est un trailer RTC périmé.
- **(c) Trailer RTC.** GB MBC3+TIMER : **48 octets** ajoutés après la SRAM (10 × `uint32` LE + 1 × `uint64` LE). Le même agencement que VBA/BGB (indice indirect, voir détails). mGBA lit aussi un trailer de **44 octets** (ancien horodatage 32 bits). GBA avec RTC : **16 octets** (7 octets BCD + 1 octet `control` + `uint64` LE), placés à l'offset `taille & ~0xFF`. Sans trailer, mGBA charge la sauvegarde et démarre l'horloge à zéro (GB) ou sans décalage (GBA). Un fichier plus petit est **agrandi** (remplissage `0xFF`). Un fichier plus grand **n'est jamais tronqué**.
- **(d) Ligne de commande.** `mGBA.exe [options] <rom>` charge et démarre le jeu, dans **un seul processus**. Une seule exception : la mise à jour intégrée lance un autre exécutable à la sortie.
- **(e) Autres fichiers.** États `.ss0` à `.ss9` (dossier `savestatePath`, sinon dossier du ROM). Captures `.png`. Fichier `.cheats` (sauvegarde auto des cheats active par défaut). `.sa2`, `.sa3`… pour les joueurs 2+ en multijoueur. Aucun fichier `.sav.bak` : mGBA ne fait pas de copie de secours.

### Détails

**(a) Nom du fichier quand le ROM est dans un zip**

1. `main.cpp` lit les arguments, puis `Window::argumentsPassed()` appelle `CoreManager::loadGame(args->fname)`.
2. Pour un fichier lisible, `loadGame(path)` ouvre le zip avec `VDirOpenArchive`. La fonction prend le premier fichier compatible (`VDirFindFirst` + `mCoreIsCompatible`). Elle copie ce fichier en mémoire.
3. Ensuite, `loadGame(vf, info.fileName(), dir.canonicalPath())` reçoit **le nom du zip** (`info` décrit le chemin passé en argument).
4. `separatePath(bytes, nullptr, core->dirs.baseName, nullptr)` coupe ce nom au dernier `.`. Résultat : `baseName` = nom du zip sans extension.
5. `mCoreAutoloadSave` ouvre `baseName + ".sav"` dans `core->dirs.save` avec `O_CREAT | O_RDWR`.
6. `dirs.save` vient de `mCoreLoadForeignConfig` → `mDirectorySetMapOptions`, appelé **avant** `mDirectorySetAttachBase`. `AttachBase` ne remplace `dirs.save` que si cette valeur est vide. Donc `savegamePath` gagne sur le dossier du ROM.

Conséquences :
- Si le zip contient plusieurs ROM, mGBA prend le premier compatible.
- Si LaunchBox extrait le zip avant le lancement, mGBA reçoit le nom du fichier interne. Le `.sav` change alors de nom. Gardez l'extraction désactivée pour mGBA, ou adaptez le calcul du nom.
- Le support zip dépend des options de compilation (`VDirOpenArchive`). NON VÉRIFIÉ dans la config de build Windows officielle. Le fonctionnement actuel avec LaunchBox suggère que le support est présent.

**Repli silencieux** : `mDirectorySetMapOptions` essaie `VDirOpen(savegamePath)`, puis `VDirCreate`. Si les deux échouent, `dirs.save` ne change pas. La sauvegarde va alors **dans le dossier du ROM**, sans message. Si le dossier n'existe pas, mGBA le **crée** (une faute de frappe crée un dossier vide et un `.sav` neuf).

**(a) Par système / par jeu**

- `_lookupValue` cherche une clé dans cet ordre : surcharges (section du port `ports.qt`, puis globale), config (port, puis globale), valeurs par défaut. Il n'y a pas de niveau « plateforme » ni « jeu ».
- `mCoreConfigMap` lit `savegamePath` avec cette fonction.
- `-C OPTION=VALUE` insère la paire dans `configOverrides`. `mArgumentsApply` la passe à `mCoreConfigSetOverrideValue` (table des surcharges, priorité maximale).
- Le Qt `ConfigController::parseArguments` appelle `mArgumentsApply` sur la config que `CoreManager` utilise (`m_manager.setConfig(m_configController->config())`).
- **Attention** : l'aide affiche `-C, --config`, mais la table `_options` ne contient pas `config`. La forme longue `--config` donne une option inconnue. `mArgumentsParse` renvoie alors `false`, et `main` affiche l'usage puis **quitte avec le code 1 sans lancer le jeu**. Utilisez seulement `-C`.

Exemple pour le launcher (non testé) :
`mGBA.exe -C savegamePath=C:/Saves/GBA "C:\Roms\gba\Pokemon - Emerald Version (USA, Europe).zip"`

**(b) Quand le `.sav` est écrit**

- Ouverture du fichier : `VFileOpen` → `VFileOpenFD` (`_wopen`) sous Windows.
- Mappage : `_vfdMap` utilise `CreateFileMapping(..., PAGE_READWRITE, ...)` et `MapViewOfFile(..., FILE_MAP_WRITE, ...)` quand `MAP_WRITE` est demandé. GB (`GBResizeSram`) et GBA (`GBASavedataInit*`) demandent `MAP_WRITE`.
- Une écriture du jeu en SRAM/Flash/EEPROM met `dirty |= mSAVEDATA_DIRT_NEW`.
- À chaque fin d'image : `GBFrameEnded` → `GBSramClean`, `GBAFrameEnded` → `GBASavedataClean`. `mSavedataClean` renvoie `true` quand plus de `mSAVEDATA_CLEANUP_THRESHOLD` = 15 images passent sans nouvelle écriture.
- À ce moment : GB écrit le trailer RTC, puis `sync` ; GBA fait `sync`, puis écrit le trailer RTC.
- `_vfdSync` sous Windows : met à jour la date du fichier, puis `FlushViewOfFile`.
- À la fermeture : `GBSramDeinit` fait `unmap` (`FlushViewOfFile` + `UnmapViewOfFile`) et écrit le trailer RTC. `GBASavedataDeinit` fait `unmap` sans réécrire le trailer.

**Kill et troncature** (déduction du code + doc Microsoft, NON VÉRIFIÉ par test) :
- La taille du fichier est fixée par `truncate` **avant** le mappage. Un kill ne réduit pas la taille.
- Les octets du jeu sont dans une vue partagée. Microsoft documente que les pages modifiées d'une vue démappée sont écrites « paresseusement » plus tard. Un kill ne perd donc pas les écritures déjà faites. Une coupure de courant peut les perdre.
- Microsoft documente aussi qu'un fichier mappé et un accès `ReadFile`/`WriteFile` « ne sont pas nécessairement cohérents ». Sur un disque local NTFS, la copie après la fin du processus lit en pratique les bonnes données (NON VÉRIFIÉ). Sur un **disque réseau**, Microsoft dit que la cohérence n'est pas garantie. Gardez `savegamePath` sur un disque local.
- Le trailer RTC est écrit avec `write()` (pas par la vue). Pour l'écrire au-delà de la fin du fichier, mGBA **démappe, écrit, puis remappe**. Un kill pendant cette fenêtre très courte peut laisser un trailer incomplet. La SRAM reste intacte.

**(c) Format exact des trailers**

GB (`struct GBMBCRTCSaveBuffer`, 48 octets, petit-boutiste) :

| Offset après SRAM | Champ | Type |
|---|---|---|
| 0x00 | `sec` | u32 |
| 0x04 | `min` | u32 |
| 0x08 | `hour` | u32 |
| 0x0C | `days` (8 bits bas) | u32 |
| 0x10 | `daysHi` (bit 0 = jour bit 8, bit 6 = halt, bit 7 = carry) | u32 |
| 0x14–0x24 | `latchedSec` … `latchedDaysHi` | 5 × u32 |
| 0x28 | `unixTime` | u64 |

- Écriture (`GBMBCRTCWrite`) : les 5 premiers champs = registres avancés jusqu'à « maintenant » (`_latchRtc` sur une copie). Les 5 suivants = registres internes `gb->memory.rtcRegs`. `unixTime` = `gb->memory.rtcLastLatch`, c.-à-d. l'instant où les registres « latched » étaient justes.
- Lecture (`GBMBCRTCRead`) : mGBA lit **seulement** les 5 champs `latched*` et `unixTime`. Il ignore les 5 premiers. Il accepte un trailer d'au moins `sizeof - 4` = 44 octets.
- `rtcRegs` est `uint8_t[5]` : `LOAD_32LE` dans un `uint8_t` garde l'octet bas.
- Le trailer est écrit seulement si `mbcType == GB_MBC3_RTC`, c.-à-d. type de cartouche `0x0F` ou `0x10`. Les types `0x11`–`0x13` (MBC3 sans horloge, par ex. Pokémon Rouge/Bleu) n'ont pas de trailer.
- Vos tailles observées collent : Or/Argent/Cristal = 32768 + 48 = 32816 ; Rouge/Bleu/Jaune = 32768.
- Compatibilité VBA-M/BGB : le code mGBA ne cite pas VBA. GameYob, qui écrit le même agencement, commente ses 5 champs `latch` « For VBA compatibility ». Compatibilité exacte avec VBA-M : NON VÉRIFIÉ (source VBA-M non lue).

GBA (`struct GBASavedataRTCBuffer`, 16 octets) :

| Offset | Champ |
|---|---|
| +0x0 | `time[7]` en BCD : année, mois, jour, jour de semaine, heure, minute, seconde (indices 0,1,2,4,5,6 lus par `_unBCD` ; l'indice 3 n'est pas lu) |
| +0x7 | `control` (la valeur `0x01` est ignorée à la lecture, pour les vieux fichiers FlashGBX) |
| +0x8 | `lastLatch` u64 LE |

- Position : `GBASavedataSize(savedata) & ~0xFF`, donc juste après 128 KiB pour une Flash 1M. Émeraude = 131072 + 16 = 131088.
- Écrit seulement si la cartouche a `HW_RTC`.
- À la lecture, `offset = lastLatch - heure_stockée`. mGBA garde ainsi un décalage entre l'horloge du jeu et l'horloge du PC.

Fichier sans trailer ou de taille différente :
- GB : `GBMBCRTCRead` lit moins de 44 octets et sort sans rien changer. L'horloge part des valeurs remises à zéro par `GBMBCReset`. Au premier `clean`, mGBA ajoute le trailer.
- GBA : `GBASavedataRTCRead` lit moins de 16 octets et sort. Au premier `clean` d'un jeu RTC, mGBA ajoute le trailer (le test `(size & 0xFF) != sizeof(buffer)` déclenche l'écriture au-delà de la fin).
- Fichier trop petit : GB `GBResizeSram` agrandit le fichier, recopie un éventuel pied (`vfSize & 0xFF`) à la nouvelle fin, et remplit le reste de `0xFF`. GBA `GBASavedataInitFlash/EEPROM/SRAM` font `truncate` à la taille du type, puis remplissent de `0xFF`.
- Fichier trop grand : aucun chemin ne réduit la taille. Les octets en trop restent.

**(d) Ligne de commande et processus**

- `wmain` convertit les arguments UTF-16 en UTF-8, puis appelle `main`.
- `main` → `parseArguments` → `GBAApp` → `Window::argumentsPassed()` → `loadGame(args->fname)`.
- `mArgumentsParse` accepte **un seul** chemin positionnel (`argc > 1` après options → échec).
- Le processus reste vivant tant que la fenêtre est ouverte (`application.exec()`). Si l'utilisateur ferme le jeu sans fermer la fenêtre, le processus continue. Le push du launcher attend alors.
- Exception : après une mise à jour acceptée, `invokeOnExit` lance `updater.exe` ou `update.exe` avec `_execl`. Le processus surveillé se termine, un autre continue. Désactivez les mises à jour automatiques.
- NON VÉRIFIÉ : comportement exact de `_execl` sous Windows (création d'un nouveau processus).

**(e) Autres fichiers**

- États : `<baseName>.ss<N>` dans `dirs.state` (`savestatePath`, sinon dossier du ROM). Ouverture en écriture avec `O_TRUNC`.
- **`autoload` vaut `true` par défaut** : au reset, mGBA charge l'état du slot 0 (`.ss0`) s'il existe. Les drapeaux de chargement par défaut sont `SCREENSHOT | RTC`, donc l'état **ne remplace pas** le `.sav`. Mais la RAM du jeu revient à l'état ancien. Si le joueur sauvegarde ensuite en jeu, il écrase le `.sav` récent avec une progression ancienne.
- `autosave` vaut `false` par défaut. S'il est actif, mGBA écrit `.ss0` toutes les 600 images et à l'arrêt.
- `.cheats` : `cheatAutosave` est actif si la clé est absente.
- `.sa2`… : `savePlayerId > 1` change le suffixe.
- Captures : `<baseName>-N.png` dans `screenshotPath`.
- Pas de `.sav.bak` (aucune occurrence dans le code lu).
- **Création à vide** : `mCoreAutoloadSave` ouvre toujours avec `O_CREAT`. Un jeu lancé crée un `.sav` même sans sauvegarde existante. En GB, la taille passe tout de suite à la taille SRAM, remplie de `0xFF`. En GBA, le fichier reste à 0 octet jusqu'au premier accès du jeu à la sauvegarde.

### Sources

- Nom du zip → `baseName` (Qt) : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/CoreManager.cpp#L60-L81 et #L109-L118
- Ordre config puis base : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/CoreManager.cpp#L96-L118
- `separatePath` coupe au dernier `.` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/util/vfs.c#L161-L194
- Chemin non-Qt (GUI 3DS, SDL) : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/directories.c#L86-L110
- `AttachBase` ne remplace pas `save` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/directories.c#L62-L79
- `savegamePath`, création et repli silencieux : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/directories.c#L121-L133
- Ouverture `.sav` avec `O_CREAT`, `.saN` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/core.c#L223-L234
- Ordre de recherche des clés : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/config.c#L49-L78
- Lecture de `savegamePath` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/config.c#L436
- Surcharges dans la section du port : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/config.c#L379-L381
- `-C` (et absence de `--config` dans `_options`) : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/feature/commandline.c#L26-L42 , #L71-L74 , #L135-L137 , #L169-L190 , #L193-L211 , #L296
- Qt applique les surcharges : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/ConfigController.cpp#L180-L193
- Même config pour le cœur : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/GBAApp.cpp#L62
- `main`, échec d'analyse = code 1, `invokeOnExit` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/main.cpp#L72-L84 et #L120-L141 ; `wmain` #L147-L159
- Chargement du ROM passé en argument : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/Window.cpp#L225-L227
- Mise à jour : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/GBAApp.cpp#L316-L332
- `VFileOpen` → `VFileOpenFD` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/util/vfs.c#L94-L96
- Mappage Windows, `FlushViewOfFile`, `sync` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/util/vfs/vfs-fd.c#L156-L197 et #L246-L282
- Seuil de 15 images : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/include/mgba/internal/defines.h#L9-L29
- Appels par image : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gb/gb.c#L1154-L1155 ; https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gba/gba.c#L888-L890
- GB : deinit, chargement, agrandissement, clean : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gb/gb.c#L218-L383
- GB : détection MBC3 vs MBC3+RTC : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gb/mbc.c#L356-L363
- GB : reset RTC puis lecture trailer : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gb/mbc.c#L530-L552
- GB : `_latchRtc` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gb/mbc.c#L586-L626
- GB : `_appendSaveSuffix`, `GBMBCRTCRead`, `GBMBCRTCWrite` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gb/mbc.c#L2205-L2264
- GB : structure 48 octets : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/include/mgba/internal/gb/mbc.h#L34-L46
- GB : `rtcRegs` en `uint8_t` : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/include/mgba/internal/gb/memory.h#L330-L331
- GBA : init, deinit, taille, chargement : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gba/savedata.c#L46-L100 , #L164-L221 , #L261-L289
- GBA : clean, écriture et lecture RTC : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/gba/savedata.c#L571-L698
- GBA : structure 16 octets : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/include/mgba/internal/gba/savedata.h#L97-L101
- États `.ssN` (`O_TRUNC`) : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/core.c#L322-L338
- `autoload`/`autosave`, drapeaux, 600 images : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/qt/CoreController.cpp#L35 , #L41-L56 , #L107-L126 , #L283-L284
- `.cheats` en sauvegarde auto : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/core/core.c#L258-L276
- Doc Microsoft, écriture paresseuse des pages : https://learn.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-unmapviewoffile (section Remarks)
- Doc Microsoft, cohérence mappage vs `ReadFile`, fichiers distants : https://learn.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-createfilemappingw (section Remarks)

---

## 2. Azahar (2126.2)

### Réponse courte

- **(a)** `azahar.exe "<chemin>"` démarre le jeu. Le chemin doit être **le dernier argument**. Options utiles : `-f`/`--fullscreen`, `-w`/`--windowed`. Un seul processus Qt. Le code lu ne contient aucun mécanisme d'instance unique.
- **(b)** La sauvegarde est un **dossier ordinaire** sur le disque du PC. Chaque écriture du jeu passe tout de suite par `fwrite`. Si le jeu demande un flush, Azahar appelle `fflush`. Sinon, les données restent dans le tampon `stdio` jusqu'à la fermeture du fichier. Il n'y a pas d'écriture différée « à la sortie ». Un kill peut perdre le tampon non vidé et laisser un fichier à moitié écrit.
- **(c)** Chemin : `<sdmc_directory>/Nintendo 3DS/00000000000000000000000000000000/00000000000000000000000000000000/title/<high 8 hex>/<low 8 hex>/data/00000001/`. id0 et id1 sont **des constantes à 32 zéros**, pas « par défaut » : elles ne changent jamais. Le fichier de format est **`data/00000001.metadata`**, à côté du dossier, pas dedans. Il fait 16 octets (`ArchiveFormatInfo`). Hex en minuscules.
- **(d)** Azahar **n'accepte pas** les `.zip`. Formats reconnus : `.cci/.3ds/.zcci`, `.cxi/.app/.zcxi`, `.3dsx/.z3dsx`, `.elf/.axf`. Un `.cia` ne démarre pas : Azahar demande de l'installer. Les ROM chiffrées sont refusées. Le launcher doit donc extraire le zip (ou LaunchBox le fait). Title ID : lire l'en-tête NCCH. Pour un `.3ds/.cci` : offset NCCH = `u32 LE @ 0x120` × 0x200, puis `program_id` = `u64 LE @ NCCH + 0x118`.

### Détails

**(a) Ligne de commande**

- `main` (citra_meta) teste d'abord les options CLI `-c`, `-x`, `-o` (compression Z3DS). Si une de ces options est présente, Azahar compresse ou décompresse puis quitte. Sinon, il lance l'interface Qt.
- Dans le constructeur `GMainWindow` :
  - avec un seul argument qui ne commence pas par `-`, cet argument est le jeu ;
  - sinon, seul **le dernier** argument sans `-` est pris comme chemin.
- Le jeu démarre par `BootGame(game_path)` à la fin du constructeur.
- Aide officielle : `-d`, `-f`, `-g`, `-h`, `-i`, `-p`, `-r`, `-a`, `-v`, `-w`, `-c`, `-x`, `--room`.
- Comme pour mGBA, le processus vit tant que la fenêtre est ouverte.
- Recherche de `QLocalServer`, `QSharedMemory`, `SingleInstance`, `startDetached`, `ShellExecute` : aucun résultat dans `src/citra_qt`, `src/common`, `src/core`. Absence d'instance unique : vraie pour ce grep, NON VÉRIFIÉ au-delà.

**(b) Écriture des sauvegardes**

- `ArchiveSource_SDSaveData::Open` renvoie une `SaveDataArchive` sur le dossier hôte.
- `SaveDataArchive::OpenFile` ouvre le fichier hôte en `"r+b"` (écriture) ou `"rb"`.
- `FS_USER File::Write` : si le backend n'autorise pas les lectures en cache, l'écriture est synchrone. Sinon elle tourne dans `RunAsync`. Dans les deux cas, le jeu attend la réponse avant de continuer.
- `DiskFile::Write` : `Seek`, `WriteBytes`, puis `Flush()` si le drapeau `flush` est présent. `IOFile::Flush` = `fflush`. `DiskFile::SetSize` fait toujours un flush.
- Le réglage `async_fs_operations` vaut `true` par défaut. Il ne retarde pas l'écriture au-delà de la réponse au jeu (lecture de `file.cpp`).
- Fermeture des fichiers à l'arrêt de l'émulation : NON VÉRIFIÉ (chemin d'arrêt non lu).
- Le dossier `data/00000001/` contient les **fichiers internes** de la sauvegarde (ce que le jeu voit). Ce n'est pas le conteneur chiffré d'une vraie carte SD 3DS. Format exact sur console : NON VÉRIFIÉ ici.

**(c) Arborescence**

- `SYSTEM_ID` (id0) et `SDCARD_ID` (id1) : 32 zéros, `static constexpr`.
- `GetSaveDataContainerPath` = `{sdmc}Nintendo 3DS/{id0}/{id1}/title/`.
- `GetSaveDataPath` = `{...}{high:08x}/{low:08x}/data/00000001/`.
- `GetSaveDataMetadataPath` = `{...}{high:08x}/{low:08x}/data/00000001.metadata`.
- `ArchiveFormatInfo` (16 octets, LE) : `u32 total_size`, `u32 number_directories`, `u32 number_files`, `u8 duplicate_data`, 3 octets de bourrage.
- Dossier absent → `Open` renvoie `ResultNotFormatted`. Le jeu formate alors la sauvegarde.
- **`Format` fait `DeleteDirRecursively` sur `data/00000001/`**, recrée le dossier, puis écrit le `.metadata`.
- `.metadata` absent → `GetFormatInfo` renvoie `ResultNotFormatted`. Réaction du jeu : NON VÉRIFIÉ (certains jeux peuvent reformater).
- Cartouche ou CIA : la factory `SaveData` utilise toujours `sd_savedata_source`. Donc un dump `.3ds` a aussi sa sauvegarde sous ce chemin SDMC.
- `UpdateUserPath` ajoute le séparateur final. Mais si `sdmc_directory` **n'est pas un dossier existant**, la fonction écrit une erreur dans le journal et **garde le chemin par défaut** (`<user>/sdmc/`). Le chemin personnalisé ne s'applique que si `use_custom_storage` est vrai.
- États de l'émulateur : `<user>/states/`, hors du SDMC.

**(d) Zip, CIA et title ID**

- `GuessFromExtension` ne connaît pas `.zip`. `IdentifyFile` teste les magics NCSD/NCCH à 0x100. Un zip donne `ErrorGetLoader` et la boîte « Invalid application format ».
- `.cia` : `BootGame` affiche « CIA must be installed before usage » et ne démarre pas.
- ROM chiffrée : `ErrorLoader_ErrorEncrypted` → refus.
- Le launcher doit extraire le zip dans un dossier temporaire et passer le `.3ds`/`.cci` extrait. Si LaunchBox extrait déjà, le launcher reçoit le chemin extrait. Option LaunchBox exacte : NON VÉRIFIÉ.

Program ID utilisé pour la sauvegarde :
1. `AppLoader_NCCH::Load` lit `ReadProgramId` et crée le `CodeSet` avec cette valeur.
2. `FS_USER` met `slot->program_id = codeset->program_id`.
3. `ReadProgramId` lit `base_ncch.ncch_header.program_id` (pas la mise à jour).

Offsets (struct Azahar, tailles vérifiées par `static_assert`) :

| Fichier | Lecture |
|---|---|
| `.cxi` / `.app` (NCCH seul) | magic `"NCCH"` @ 0x100 ; `program_id` = u64 LE @ **0x118** |
| `.3ds` / `.cci` (NCSD) | magic `"NCSD"` @ 0x100 ; `media_id` u64 @ 0x108 ; table des partitions @ 0x120 (8 × {u32 offset, u32 taille} en unités de 0x200). Partition 0 : `off = u32 LE @ 0x120 × 0x200`. Puis magic `"NCCH"` @ `off + 0x100` et `program_id` @ **`off + 0x118`** |
| `.zcci` / `.zcxi` (Z3DS) | en-tête compressé. Lecture directe impossible. Format Z3DS : NON VÉRIFIÉ |

Ensuite : `high = program_id >> 32`, `low = program_id & 0xFFFFFFFF`, en hex minuscules sur 8 caractères.

NON VÉRIFIÉ : le `media_id` NCSD est souvent égal au program ID de la partition 0. Azahar utilise le program ID NCCH. Lisez celui-ci.

### Sources

- Dispatch CLI / Qt : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_meta/main.cpp#L53-L83
- Options CLI de compression : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_cli/citra_cli.h#L8
- Texte d'aide : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_meta/common_strings.h#L11-L31
- Analyse des arguments : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_qt/citra_qt.cpp#L268-L398
- Démarrage du jeu : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_qt/citra_qt.cpp#L543-L544
- Erreurs de chargement (format, chiffré) : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_qt/citra_qt.cpp#L1379-L1456
- CIA à installer : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_qt/citra_qt.cpp#L1481-L1491
- Extensions reconnues : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/loader/loader.cpp#L48-L67
- Pas de loader CIA : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/loader/loader.cpp#L99-L148 et #L150-L183
- id0 / id1 : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/hle/service/fs/archive.h#L21-L24
- Chemins, `.metadata`, `Open`, `Format`, `GetFormatInfo` : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/archive_source_sd_savedata.cpp#L21-L139
- `ArchiveFormatInfo` 16 octets : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/archive_backend.h#L111-L118
- Factory SaveData → source SD : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/archive_savedata.cpp#L20-L33
- Ouverture des fichiers hôtes : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/savedata_archive.cpp#L87
- `File::Write` (sync et async) : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/hle/service/fs/file.cpp#L177-L263
- `DiskFile::Write`, `SetSize`, `Close` : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/disk_archive.cpp#L30-L54
- `IOFile::Close` / `Flush` : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/common/file_util.cpp#L1403-L1437
- `async_fs_operations` par défaut : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/common/settings.h#L497-L499
- Lecture `sdmc_directory` / `use_custom_storage` : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/citra_qt/configuration/config.cpp#L491-L510
- `UpdateUserPath` (repli si dossier absent) : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/common/file_util.cpp#L1081-L1091
- Dossier `states` : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/common/file_util.cpp#L1033
- Structs NCSD / NCCH : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/ncch_container.h#L20-L55
- Ouverture NCSD → partition NCCH : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/ncch_container.cpp#L209-L235
- `ReadProgramId` : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/file_sys/ncch_container.cpp#L693-L703
- `CodeSet` avec program ID : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/loader/ncch.cpp#L129-L140 et #L359-L360
- `FS_USER` program ID de session : https://github.com/azahar-emu/azahar/blob/219d743a1e698101fe1ab8bef43e6563126c3c27/src/core/hle/service/fs/fs_user.cpp#L1090

---

## 3. Compatibilité GB/GBC/GBA entre mGBA (PC) et la 3DS

### Réponse courte

| Paire | Données de jeu | Trailer RTC | Conversion |
|---|---|---|---|
| mGBA PC ↔ **mGBA 3DS** | identiques (même code cœur) | identique | aucune |
| mGBA PC GBA ↔ **open_agb_firm** | identiques pour SRAM/Flash ; **EEPROM différente** | open_agb_firm l'ignore et ne l'écrit pas ; il garde les octets en trop | Flash/SRAM : aucune. EEPROM : conversion d'ordre des octets |
| mGBA PC GB ↔ **GameYob** (TWiLight Menu++) | identiques | 48 (ou 44) octets, même agencement, **sens différent des champs** | conseillée pour les jeux MBC3+RTC |
| Anciens `.rtc` (23 octets) | — | lus par aucun des trois | aucune ; ne pas synchroniser |

### Détails

**mGBA 3DS**
- Le port 3DS passe par `gui-runner` : `mDirectorySetOpenPath` (nom = zip sans extension), `mCoreLoadForeignConfig`, `mCoreAutoloadSave`. Le cœur `src/gb` et `src/gba` est le même. Le format du `.sav` et des trailers est donc le même.
- Le VFS 3DS ne mappe pas le fichier : `_vf3dMap` lit dans un tampon, `_vf3dSync`/`_vf3dUnmap` réécrivent avec `FS_WRITE_FLUSH`. L'écriture sur la carte SD arrive ~15 images après la dernière écriture du jeu.
- L'interface du port ne propose pas `savegamePath` (aucune occurrence dans `gui-config.c`). Par défaut, le `.sav` est à côté du ROM. Emplacement du `config.ini` du port 3DS : NON VÉRIFIÉ.

**open_agb_firm (GBA natif)**
- Chemin : travail dans `sdmc:/3ds/open_agb_firm`, sauvegardes dans `saves/`. `useSavesFolder` vaut `true` par défaut. Nom = nom du ROM avec les **4 derniers caractères** remplacés par `.sav` (suppose `.gba`). Slots 1–9 → `.N.sav`.
- Taille : fixée par le type de sauvegarde (`sizeShiftLut`). Flash 1M = 2^17 = 131072.
- Lecture au démarrage : `fsQuickRead(savePath, ..., saveSize)` lit **les `saveSize` premiers octets**. Un fichier mGBA de 131088 octets est accepté ; le trailer est ignoré. Fichier absent → remplissage `0xFF`.
- Écriture : seulement dans `LGY_backupGbaSave`, appelé par `LGY11_deinit` à la sortie (bouton POWER), et seulement si le SHA-256 a changé. `fsQuickWrite` ouvre avec `FA_OPEN_ALWAYS | FA_WRITE` **sans troncature**. Les octets au-delà de `saveSize` (ancien trailer mGBA) restent tels quels.
- RTC : open_agb_firm règle l'horloge GBA matérielle depuis l'horloge de la 3DS au démarrage. Aucune donnée RTC dans le `.sav`.
- Conséquence pour Émeraude : copier tel quel dans les deux sens marche. mGBA relit un trailer éventuellement ancien et garde son ancien décalage d'horloge. Option propre : enlever le trailer vers la 3DS et le garder côté PC.
- EEPROM : le README dit que la plupart des émulateurs écrivent l'EEPROM « différemment » et renvoie à l'outil d'exelotl. Algorithme exact de conversion : NON VÉRIFIÉ (code de l'outil non lu). Que mGBA soit dans la catégorie « la plupart des émulateurs » : NON VÉRIFIÉ.
- Sleep : « not fully implemented » (README). Une sortie sans POWER (batterie vide, plantage) perd **toute** la session.

**GameYob (émulateur GB de TWiLight Menu++)**
- TWiLight Menu++ embarque `sd:/_nds/TWiLightMenu/emulators/gameyob.nds` (addon « Virtual Console »).
- Nom : chemin du ROM, extension remplacée par `.sav` (à côté du ROM). Une redirection par TWiLight vers un autre dossier : NON VÉRIFIÉ. GameYob lit-il les `.zip` : NON VÉRIFIÉ.
- Contenu : `numRamBanks × 0x2000` octets de SRAM, puis `clockStruct` pour **tous** les MBC3 et HuC3. GameYob classe `0x0F`–`0x13` en MBC3. Donc Pokémon Rouge/Bleu (MBC3 sans horloge) reçoivent aussi un trailer chez GameYob, mais pas chez mGBA.
- Fichier trop court → GameYob l'**agrandit avec des zéros** (pas `0xFF`).
- `clockStruct` = `int s, m, h, d, ctrl` + `int latch[5]` (« For VBA compatibility ») + `time_t last`. Taille = 44 ou 48 octets selon la taille de `time_t` du devkitARM utilisé (NON VÉRIFIÉ ; mGBA accepte les deux).
- Différences de sens avec mGBA :
  - GameYob lit et écrit **seulement** `s,m,h,d,ctrl` et `last`. Il ne met jamais à jour `latch[5]`.
  - mGBA lit **seulement** les 5 champs `latched*` et `unixTime`.
  - Résultat d'un aller-retour PC → 3DS → PC sans conversion : mGBA relit les `latched*` de **l'ancienne** session PC avec le `last` de GameYob. L'horloge du jeu peut reculer.
  - GameYob `d` contient le jour sur 9 bits ; mGBA stocke 8 bits dans `days` et le bit 8 dans `daysHi`.
  - GameYob calcule `now = rawTime - 2 h` (« same as lameboy »). mGBA utilise l'heure Unix du PC. Le fuseau de `rawTime` : NON VÉRIFIÉ.
- Conversion proposée (déduite du code, à tester) :
  - GameYob → mGBA : `latched[0..4] = {s, m, h, d & 0xFF, ctrl}` ; `unixTime = last` (+ correction 2 h et fuseau, NON VÉRIFIÉ) ; les 5 premiers champs peuvent recevoir les mêmes valeurs.
  - mGBA → GameYob : `{s,m,h} = latched[0..2]` ; `d = latchedDays | ((latchedDaysHi & 1) << 8)` ; `ctrl = latchedDaysHi` ; `last = unixTime` (même correction).
  - SRAM (`N × 0x2000` premiers octets) : copie directe dans les deux sens.

**Anciens fichiers `.rtc` de 23 octets**
- Aucune occurrence de `.rtc` dans le code mGBA ni GameYob lus. Ces fichiers ne sont lus par aucun des trois émulateurs.
- Émulateur d'origine et format de ces 23 octets : NON VÉRIFIÉ.
- Conseil : gardez-les en archive. Ne les synchronisez pas. Ne les supprimez pas automatiquement.

**Conversions dont l'outil a besoin**
1. Comparer et copier seulement la **partie données** (taille SRAM/Flash du type). Ne jamais conclure à une corruption sur une différence de taille de 16, 44 ou 48 octets.
2. GBA vers open_agb_firm : rien d'obligatoire. Option : enlever les 16 octets de trailer.
3. GBA depuis open_agb_firm vers PC : copier les N premiers octets ; garder ou enlever le trailer PC (mGBA le recrée).
4. GBA EEPROM : conversion d'ordre des octets (NON VÉRIFIÉ, voir README open_agb_firm).
5. GB MBC3+RTC vers/depuis GameYob : réécrire le trailer (voir ci-dessus).
6. GB vers/depuis mGBA 3DS : aucune conversion.
7. `.rtc` séparés : ignorer.

### Sources

- Port 3DS = `gui-runner` (même chargement) : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/feature/gui/gui-runner.c#L400-L447
- VFS 3DS (tampon + écriture avec flush) : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/platform/3ds/3ds-vfs.c#L137-L179
- Config GUI (pas de `savegamePath`) : https://github.com/mgba-emu/mgba/blob/26b7884bc25a5933960f3cdcd98bac1ae14d42e2/src/feature/gui/gui-config.c
- open_agb_firm, dossiers : https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/include/arm11/config.h#L30-L31
- open_agb_firm, nom `.sav` et slots : https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/source/arm11/open_agb_firm.c#L212-L247
- open_agb_firm, séquence de démarrage : https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/source/arm11/open_agb_firm.c#L276-L352 et #L372-L378
- open_agb_firm, boucle et sortie par POWER : https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/source/arm11/main.c#L39-L57
- README (useSavesFolder, Known Issues, EEPROM Fixer) : https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/README.md#L69-L70 , #L200-L206 , #L221-L224
- libn3ds, taille, lecture, hash, sauvegarde : https://github.com/profi200/libn3ds/blob/ccbb6a94d60bea2e6a1901263251636b192bad3f/source/arm9/drivers/lgy9.c#L52-L106 et #L163-L191
- libn3ds, `fsQuickRead` / `fsQuickWrite` (`FA_OPEN_ALWAYS`) : https://github.com/profi200/libn3ds/blob/ccbb6a94d60bea2e6a1901263251636b192bad3f/source/fsutil.c#L27-L56
- libn3ds, RTC depuis l'horloge 3DS, sauvegarde au deinit : https://github.com/profi200/libn3ds/blob/ccbb6a94d60bea2e6a1901263251636b192bad3f/source/arm11/drivers/lgy11.c#L95-L157
- TWiLight Menu++ → GameYob : https://github.com/DS-Homebrew/TWiLightMenu/blob/8a90785b79c8e1b5e8ebfb3010eb5ee552bef70f/universal/include/common/bootstrappaths.h#L27-L28 ; https://github.com/DS-Homebrew/TWiLightMenu/blob/8a90785b79c8e1b5e8ebfb3010eb5ee552bef70f/title/arm9/source/main.cpp#L2852-L2859
- GameYob, `clockStruct` : https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/include/mmu.h#L18-L32
- GameYob, nom `.sav` : https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/inputhelper.cpp#L741-L744
- GameYob, MBC3 = `0x0F`–`0x13` : https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/inputhelper.cpp#L783-L784
- GameYob, chargement / agrandissement / sauvegarde complète / autosave : https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/inputhelper.cpp#L937-L1038
- GameYob, registres RTC et `writeClockStruct` : https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/mmu.cpp#L236-L242 et #L330-L358
- GameYob, `latchClock` (−2 h, jour 9 bits) : https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/arm9/source/mmu.cpp#L668-L705

---

## Pièges pour un outil qui ne doit jamais détruire une sauvegarde

1. **Ne tuez jamais l'émulateur.** mGBA peut laisser un trailer RTC incomplet. Azahar peut perdre un tampon `stdio` et laisser un fichier à moitié écrit. Attendez la fin normale du processus.
2. **Le processus vit tant que la fenêtre vit.** Si le joueur ferme le jeu mais pas la fenêtre, le push n'a pas lieu. N'ajoutez pas de délai d'attente qui tue le processus.
3. **Mise à jour mGBA** : `invokeOnExit` lance un autre exécutable à la sortie. Désactivez les mises à jour, ou vérifiez qu'aucun processus `updater.exe` ne tourne avant le push.
4. **Fichiers créés à vide.** mGBA crée le `.sav` avec `O_CREAT` à chaque lancement. En GB, il le remplit tout de suite de `0xFF`. Ne poussez jamais vers la 3DS un fichier de 0 octet ou rempli de `0xFF` si la 3DS a une vraie sauvegarde. Gardez toujours une copie horodatée avant d'écraser.
5. **Repli silencieux de dossier.** Si `savegamePath` (mGBA) ou `sdmc_directory` (Azahar) est absent ou inaccessible, l'émulateur écrit ailleurs sans message : dossier du ROM pour mGBA, `<user>/sdmc/` pour Azahar. Vérifiez que le dossier existe **avant** le lancement. Après la sortie, cherchez un `.sav` plus récent à côté du ROM.
6. **mGBA crée un dossier mal tapé.** `VDirCreate` crée le dossier s'il manque. Une faute de frappe donne un dossier vide, un jeu « neuf », puis un `.sav` neuf. Validez le chemin dans le launcher.
7. **`--config` fait quitter mGBA.** Utilisez seulement `-C savegamePath=...`. La forme longue fait sortir mGBA avec le code 1, sans jeu. Le launcher ne doit pas faire de push après un code 1 immédiat.
8. **Nom du `.sav` lié au nom du zip.** Renommer un zip, ou activer l'extraction LaunchBox, change le nom du `.sav`. L'outil doit trouver un fichier « orphelin » et ne pas créer une sauvegarde vide à côté.
9. **État auto `.ss0`.** `autoload` vaut `true` par défaut. Un `.ss0` ancien restaure une RAM ancienne. Une sauvegarde en jeu peut alors écrire une progression ancienne dans le `.sav`. Ne synchronisez pas les `.ssN`. Vérifiez `autoload`/`autosave` dans `config.ini`.
10. **Disque réseau.** Microsoft ne garantit pas la cohérence d'un fichier mappé sur un disque distant. Gardez `savegamePath` sur un disque local, même si le NAS sert de stockage final.
11. **Tailles différentes ≠ corruption.** 32768 vs 32816 (GB), 131072 vs 131088 (GBA), et 32768 + 44/48 (GameYob sur MBC3 sans horloge) sont normaux. Comparez et hachez la **partie données** seulement.
12. **Ne réduisez jamais un fichier sans savoir.** Un fichier plus grand que prévu peut contenir un trailer RTC. Une troncature aveugle efface l'horloge. Un fichier plus petit est agrandi par l'émulateur avec `0xFF` (mGBA) ou `0x00` (GameYob) : ces octets ne sont pas des données réelles.
13. **RTC GB GameYob ↔ mGBA.** Sans conversion, l'horloge de Pokémon Or/Argent/Cristal peut reculer après un aller-retour. Convertissez le trailer, ou au minimum signalez le problème.
14. **EEPROM GBA et open_agb_firm.** L'ordre des octets diffère. Une copie brute donne une sauvegarde illisible sans erreur visible. Détectez le type (taille 512 ou 8192) et convertissez.
15. **open_agb_firm n'écrit qu'à la sortie par POWER.** Une session sans sortie propre ne produit aucun fichier nouveau. L'outil ne doit pas conclure « pas de changement » avec certitude.
16. **Azahar `.metadata` obligatoire.** Créez `data/00000001.metadata` (16 octets) en même temps que `data/00000001/`. Sans ce fichier, `GetFormatInfo` échoue. Un jeu qui formate déclenche `DeleteDirRecursively` sur le dossier poussé.
17. **Azahar : dossier présent mais incomplet.** Si le dossier manque, le jeu formate. Écrivez d'abord dans un dossier temporaire, puis renommez en une seule opération. N'écrivez jamais fichier par fichier dans le dossier final pendant qu'Azahar tourne.
18. **Title ID : lisez le NCCH, pas le nom du fichier.** Lisez `program_id` à `NCCH + 0x118`. Écrivez high/low en hex minuscules sur 8 caractères. Pour un `.zcci` compressé, l'en-tête n'est pas lisible directement.
19. **Zip Azahar.** Extrayez dans un dossier temporaire propre au lancement. Supprimez seulement ce dossier temporaire, jamais un dossier de sauvegarde. Vérifiez le chemin avant toute suppression.
20. **Écriture atomique côté outil.** Écrivez chaque fichier poussé dans un fichier temporaire du même dossier, puis remplacez-le. Ne lancez pas l'émulateur avant la fin du remplacement.
