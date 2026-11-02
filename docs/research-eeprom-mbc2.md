# Recherche — EEPROM GBA (ticket #7) et GB MBC2 (ticket #8)

Date : 2026-11-02. Méthode : lecture du code source à un commit figé et des pages des auteurs. Aucun émulateur n'a été lancé.

Légende :
- **NON TROUVÉ** = aucune source primaire trouvée.
- **NON VÉRIFIÉ** = déduction sans test.
- « déduction » = conclusion tirée du code cité, sans exécution.

Point de départ : [research-v3-emulateurs.md](research-v3-emulateurs.md) §4.7, §8.2, §8.3.

## Sources figées

| Projet | Commit |
|---|---|
| open_agb_firm | `401c3dff900f658d3edf77ababcfaf5dca54a0a8` |
| libn3ds (sous-module `libraries/libn3ds` de ce commit d'open_agb_firm) | `ccbb6a94d60bea2e6a1901263251636b192bad3f` |
| gba-eeprom-save-fix (exelotl) | `fdc61d84c6b7779dd39786a70ee98739c9568001` |
| mGBA | `52de910807c0178d6cfb088aa350952569e8f08f` |
| SameBoy | `3e271573e9fcd07d4093ca327cfbed2a7797b7d3` |
| VisualBoyAdvance-M | `beab4e0400160e645fc1b00fce275c74ed34d068` |
| GameYob (fork DS-Homebrew, branche `v0.5-hotfix-pocketcam`) | `c4d8672806b4a1cff8012fd40de4acf69b0094c2` |
| GameYob (amont Stewmath, ex-Drenn1, `master`) | `45599fbb2f08b2f8042e7f9f47578ef078f2e889` |
| TWiLight Menu++ | `8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc` |

Préfixes de liens :
- `OAF` = `https://github.com/profi200/open_agb_firm/blob/401c3dff900f658d3edf77ababcfaf5dca54a0a8/`
- `N3DS` = `https://github.com/profi200/libn3ds/blob/ccbb6a94d60bea2e6a1901263251636b192bad3f/`
- `EX` = `https://github.com/exelotl/gba-eeprom-save-fix/blob/fdc61d84c6b7779dd39786a70ee98739c9568001/`
- `MG` = `https://github.com/mgba-emu/mgba/blob/52de910807c0178d6cfb088aa350952569e8f08f/`
- `SB` = `https://github.com/LIJI32/SameBoy/blob/3e271573e9fcd07d4093ca327cfbed2a7797b7d3/`
- `VBM` = `https://github.com/visualboyadvance-m/visualboyadvance-m/blob/beab4e0400160e645fc1b00fce275c74ed34d068/`
- `GY` = `https://github.com/DS-Homebrew/GameYob/blob/c4d8672806b4a1cff8012fd40de4acf69b0094c2/`
- `UGY` = `https://github.com/Stewmath/GameYob/blob/45599fbb2f08b2f8042e7f9f47578ef078f2e889/`
- `TWL` = `https://github.com/DS-Homebrew/TWiLightMenu/blob/8d3a32ff0b5cb7e588b9d030ab6e88bc2412f1bc/`
- BGB : https://bgb.bircd.org/mbc2save.html et https://bgb.bircd.org/manual.html (consultées le 2026-11-02)

---

## Synthèse

- **EEPROM GBA** : open_agb_firm écrit 512 o (4 Kbit) ou 8192 o (64 Kbit), sans pied de page. mGBA écrit les mêmes tailles, plus 16 o d'horloge si le jeu a une RTC. L'ordre des octets diffère : il faut inverser les 8 octets de chaque bloc de 8. mGBA a lui-même ce convertisseur (`SaveConverter`), avec la même opération que l'outil d'exelotl.
- **Taille 512 o ou 8 Kio côté GBA** : chez open_agb_firm, mGBA et VBA-M, ces tailles viennent seulement de l'EEPROM. SRAM = 32 Kio (64 Kio possible chez mGBA), Flash = 64 ou 128 Kio.
- **Détection GBA** : mGBA ne lit pas les chaînes `EEPROM_V` etc. Il détecte le type au premier accès du jeu (plus une table de codes de jeu). open_agb_firm et VBA-M cherchent les chaînes dans la ROM.
- **MBC2** : quatre agencements différents. mGBA = 256 o (deux valeurs par octet). SameBoy, VBA-M, BGB = 512 o (une valeur par octet), mais le quartet haut diffère. GameYob = 8 Kio, une valeur par octet au début, quartet haut à 0. Aucun émulateur n'ajoute d'horloge pour MBC2.
- **Conséquence** : pour MBC2, le Hash du fichier brut ne peut pas être égal entre machines. Il faut comparer les 512 quartets bas, après normalisation.

---

## A. EEPROM GBA

### A.1 open_agb_firm

**Taille du fichier.**
- Le type de sauvegarde vient de `gba_db.bin`, d'un choix manuel, ou de la détection par chaînes (A.3). Code : `OAF source/arm11/open_agb_firm.c#L310-L328`.
- Les types EEPROM sont `SAVE_TYPE_EEPROM_8k` (0x0), `_8k_2` (0x1), `_64k` (0x2), `_64k_2` (0x3). Les variantes `_2` servent aux ROM de plus de 16 Mio. Source : `N3DS include/drivers/lgy_common.h#L42-L45`.
- La taille vaut `BIT(sizeShift) & ~BIT(0)`, avec la table `{9, 9, 13, 13, 16, …, 17, 15, 0}`. Source : `N3DS source/arm9/drivers/lgy9.c#L57-L63`.
  - Types 0x0 et 0x1 : 2^9 = **512 octets** (4 Kbit). Le nom « eeprom_8k » du README désigne donc 512 octets (`OAF README.md#L169-L172`).
  - Types 0x2 et 0x3 : 2^13 = **8192 octets** (64 Kbit).
  - SRAM (0xE) : 2^15 = 32768 octets. Flash : 2^16 ou 2^17.
- Une taille forcée est possible (16 bits hauts du type), par exemple pour un patch EEPROM → Flash de Boktai (`N3DS source/arm9/drivers/lgy9.c#L57-L58`).

**Lecture et écriture.**
- Au lancement : `fsQuickRead(savePath, LGY9_SAVE_LOC, saveSize)`. Si le fichier manque, la zone est remplie de `0xFF`. Source : `N3DS source/arm9/drivers/lgy9.c#L80-L106`.
- À la sauvegarde : `fsQuickWrite(g_savePath, LGY9_SAVE_LOC, saveSize)`, seulement si le SHA-256 a changé. Source : `N3DS source/arm9/drivers/lgy9.c#L163-L191`.
- `fsQuickWrite` ouvre avec `FA_OPEN_ALWAYS | FA_WRITE` et écrit depuis l'offset 0. Il ne tronque pas le fichier. Source : `N3DS source/fsutil.c#L41-L56`.
  - Déduction : un fichier existant plus long (par exemple 8208 o de mGBA) garde ses octets de fin. Seuls les `saveSize` premiers octets changent.
  - Déduction : un fichier plus court est lu en partie. Le code ne remplit de `0xFF` que si le fichier manque (`RES_FR_NO_FILE`). Le reste de la zone garde un contenu inconnu. NON VÉRIFIÉ sur machine.

**Pied de page.** Aucun. Le fichier contient exactement la zone de sauvegarde (`lgy9.c#L183`). L'horloge GBA vient du matériel 3DS (`LGY_setGbaRtc`, `N3DS source/arm9/drivers/lgy9.c#L108-L130`), pas du fichier.

**Ordre des octets.**
- open_agb_firm ne réordonne rien dans son code. Il copie la zone telle que le matériel « legacy » de la 3DS la remplit (`lgy9.c#L91`, `#L183`).
- Le README dit que la plupart des émulateurs écrivent l'EEPROM autrement. Il renvoie à l'outil d'exelotl, valable dans les deux sens. Source : `OAF README.md#L221-L224`, `#L245-L246`.
- L'agencement exact dans le matériel : NON TROUVÉ (pas de documentation primaire du format interne LGY). La correspondance avec mGBA passe par l'outil d'exelotl (A.2).

### A.2 mGBA

**Taille du fichier.**
- `GBA_SIZE_EEPROM = 0x2000` (8192 o), `GBA_SIZE_EEPROM512 = 0x200` (512 o). Source : `MG include/mgba/internal/gba/memory.h#L74-L75`.
- Au premier accès EEPROM, le type devient `GBA_SAVEDATA_EEPROM512`. Un fichier plus court est agrandi à 512 o et complété par `0xFF`. Source : `MG src/gba/savedata.c#L299-L325`.
- Si le jeu lit ou écrit une adresse d'octet ≥ 0x200, `_ensureEeprom` passe en 8 Kio et agrandit le fichier (`0xFF`). Source : `MG src/gba/savedata.c#L485-L505`, appels `#L532`, `#L572`.
- mGBA ne réduit jamais un fichier plus long (`truncate` seulement si `end < eepromSize`, `#L316-L318`).

**Pied de page.** Si la cartouche a une RTC (`HW_RTC`), mGBA écrit 16 octets à l'offset `size & ~0xFF`. Structure : 7 octets d'heure, 1 octet de contrôle, 8 octets `lastLatch`. Sources : `MG src/gba/savedata.c#L606-L638` ; `MG include/mgba/internal/gba/savedata.h#L90-L94`.
- Donc : 512 + 16 = 528 o, ou 8192 + 16 = 8208 o pour un jeu EEPROM avec RTC. Sans RTC : pas de pied de page.

**Ordre des octets dans mGBA.**
- Écriture : le bit d'adresse `writeAddress` va dans l'octet `writeAddress >> 3`, au bit `7 - (writeAddress & 7)`. L'adresse de bloc est décalée de 6 (`<< 6`), donc le bloc n occupe les octets 8n à 8n+7. Le premier bit transmis va dans le bit 7 de l'octet 8n. Source : `MG src/gba/savedata.c#L524-L540`.
- Lecture : même formule (`data[address] >> (7 - (step & 7))`). Source : `MG src/gba/savedata.c#L560-L584`.

**Conversion confirmée par mGBA lui-même.**
- `SaveConverter` (interface Qt) propose pour une EEPROM de 512 o ou 8 Kio (± 16) deux variantes : « little endian » et « big endian ». Source : `MG src/platform/qt/SaveConverter.cpp#L218-L227`.
- mGBA nomme son propre format « little endian » : une EEPROM extraite d'un savestate mGBA reçoit `Endian::LITTLE`. Source : `MG src/platform/qt/SaveConverter.cpp#L180-L183`.
- La conversion fait `LOAD_64LE` puis `STORE_64BE` sur chaque mot de 8 octets. Cette opération inverse les 8 octets de chaque bloc. La boucle s'arrête à `size & ~0xFF` : le pied de page RTC est retiré. Source : `MG src/platform/qt/SaveConverter.cpp#L674-L688`.
- L'outil d'exelotl fait la même inversion (`a b c d e f g h` → `h g f e d c b a`). Il accepte tout fichier dont la taille est une puissance de 2 et ≥ 32. Il ne vérifie pas que le fichier est une EEPROM. Source : `EX index.html#L48-L50`, `#L52-L69`.
- Conclusion : **conversion open_agb_firm ↔ mGBA = inversion des 8 octets de chaque bloc de 8.** C'est le format « big endian » de mGBA. La conversion est son propre inverse. Les sources sont le README d'open_agb_firm, l'outil d'exelotl et le code de mGBA. Aucun test sur machine.

### A.3 Un fichier de 512 o ou 8 Kio peut-il être autre chose qu'une EEPROM ?

| Émulateur | SRAM | Flash 64 K | Flash 128 K | EEPROM 4 Kbit | EEPROM 64 Kbit | Source |
|---|---|---|---|---|---|---|
| open_agb_firm | 32768 | 65536 | 131072 | 512 | 8192 | `N3DS source/arm9/drivers/lgy9.c#L59-L62` |
| mGBA | 32768 (ou 65536 pour `SRAM512`) | 65536 | 131072 | 512 (+16 RTC) | 8192 (+16 RTC) | `MG include/mgba/internal/gba/memory.h#L70-L75` ; `MG src/gba/savedata.c#L169-L192`, `#L327-L350` |
| VBA-M | 32768 | `g_flashSize` (64 K ou 128 K) | idem | `eepromSize` (512) | 8192 | `VBM src/core/gba/gba.cpp#L1801-L1832` ; `VBM src/core/gba/gbaEeprom.cpp#L16-L20` |

- Réponse : chez ces trois logiciels, **512 o et 8192 o = EEPROM seulement**.
- Exceptions connues :
  - mGBA ne réduit jamais un fichier (`MG src/gba/savedata.c#L316-L318`, `#L340-L342`). Un ancien fichier trop long garde sa taille.
  - mGBA agrandit un fichier trop court (`0xFF`), mais ne choisit pas le type d'après la taille du fichier (`MG src/gba/savedata.c#L316-L323`). Comportement de VBA-M au chargement : non lu.
  - Un fichier de 512 o « GB MBC2 » existe aussi (partie B). Le système (GB ou GBA) vient de la ROM, pas de la taille.

**Détection du type par mGBA.**
- mGBA ne cherche **pas** les chaînes `EEPROM_V`, `SRAM_V`, `FLASH_V`, `FLASH512_V`, `FLASH1M_V`. Recherche dans `src/` et `include/` au commit figé : aucune occurrence.
- Ordre de détection :
  1. Table `_overrides` par code de jeu (4 caractères), puis code commençant par `F` (Classic NES Series) → `GBA_SAVEDATA_EEPROM`. Source : `MG src/gba/overrides.c#L263-L288`, application `#L334-L337`.
  2. Sinon, type `GBA_SAVEDATA_AUTODETECT` (`MG src/gba/savedata.c#L46-L47`). Le type est fixé au premier accès du jeu :
     - écriture dans la zone `0x0D…` (ROM2_EX) → EEPROM : `MG src/gba/memory.c#L986-L993` ;
     - écriture 8 bits à `0x0E005555` → Flash, sinon SRAM : `MG src/gba/memory.c#L1062-L1072` ;
     - lecture dans la zone SRAM → SRAM : `MG src/gba/memory.c#L717-L723`.
  3. EEPROM 512 o d'abord, puis 8 Kio si une adresse haute est utilisée (A.2).

**Détection par chaînes (pour comparaison).**
- open_agb_firm : cherche `EEPR`, `FLAS`, `SRAM` puis compare à 25 chaînes (`EEPROM_V111` … `SRAM_V113`). `EEPROM_V121` et `EEPROM_V124` → 64 Kbit ; les autres versions EEPROM → 4 Kbit. Source : `OAF source/arm11/save_type.c#L48-L136`. Les codes de jeu se terminant par `1`, `2`, `3`, `4`, `F`, `S` ont un type forcé : `#L33-L46`.
- VBA-M : `flashDetectSaveType` cherche `EEPROM_`, `SRAM_`, `FLASH1M_`, `FLASH512_`, `FLASH`, et `SIIRTC_V` pour la RTC. Source : `VBM src/core/gba/gbaFlash.cpp#L33-L79`.
- Conséquence : open_agb_firm devine la taille EEPROM d'après la version du SDK. mGBA la déduit de l'adresse utilisée. Les deux peuvent donc donner des tailles différentes pour un même jeu. NON VÉRIFIÉ sur un jeu réel.

### A.4 Tableau récapitulatif — EEPROM GBA

| | open_agb_firm | mGBA |
|---|---|---|
| Taille 4 Kbit | 512 | 512 (528 avec RTC) |
| Taille 64 Kbit | 8192 | 8192 (8208 avec RTC) |
| Pied de page | aucun | 16 o RTC si `HW_RTC` |
| Ordre dans un bloc de 8 | inverse de mGBA (« big endian » au sens de mGBA) | octet 8n = premier octet transmis (« little endian » au sens de mGBA) |
| Fichier absent | zone remplie de `0xFF` | fichier créé, rempli de `0xFF` |
| Fichier trop long | gardé, seul le début est réécrit | gardé |
| Conversion | inverser les 8 octets de chaque bloc, sur `taille & ~0xFF` octets | idem |

---

## B. GB MBC2

Rappel matériel (page BGB) : 512 valeurs de 4 bits, aux adresses `$A000-$A1FF`. Le quartet haut se lit toujours à 1 (`$F0`). Source : https://bgb.bircd.org/mbc2save.html.

### B.1 mGBA

- Taille : `sramSize = 0x100` → **256 octets**. Source : `MG src/gb/mbc.c#L441-L446`, puis `GBResizeSram(gb, sramSize)` à `#L588`.
- Agencement : **deux valeurs par octet**. Adresse paire → quartet **bas**. Adresse impaire → quartet **haut**. Octet i = valeur(2i) | valeur(2i+1) << 4. Source : `MG src/gb/mbc/mbc.c#L123-L159` (`shift = (address & 1) * 4`, `address &= 0x1FF`, `sramBank[address >> 1]`).
- Lecture par le jeu : `(sramBank[address >> 1] >> shift) | 0xF0`. Source : `MG src/gb/mbc/mbc.c#L167-L174`.
- Fichier neuf ou trop court : agrandi et rempli de `0xFF`. Fichier plus long : non réduit, seuls les 256 premiers octets sont lus. Source : `MG src/gb/gb.c#L298-L338`.
  - Déduction : un fichier de 512 o (SameBoy, VBA-M, BGB) chargé dans mGBA donne de mauvaises valeurs. Les 256 premiers octets sont lus comme des paires.
- Horloge : aucune. Le pied de page GB existe seulement pour `GB_MBC3_RTC`, `GB_HuC3`, `GB_TAMA5`. Source : `MG src/gb/gb.c#L283-L294`.
- mGBA nomme ce format « packed MBC2, little endian ». Son `SaveConverter` convertit 256 ↔ 512. Vers 512 o, il écrit `0xF0 | quartet`. Depuis 512 o, il ne garde que `& 0xF`. Source : `MG src/platform/qt/SaveConverter.cpp#L250-L256`, `#L607-L620`, `#L700-L734`.

### B.2 SameBoy

- Taille : `mbc_ram_size = 0x200` → **512 octets**. Source : `SB Core/mbc.c#L210-L213`.
- Agencement : **une valeur par octet**, octet i = adresse `$A000 + i`. Index `(addr & 0x1FFF) & (mbc_ram_size - 1)`, donc miroir tous les 512 o. Source : `SB Core/memory.c#L454`, `#L1316`.
- Quartet haut **en écriture** : SameBoy range l'octet entier écrit par le jeu, sans masque. Source : `SB Core/memory.c#L1316` (aucun cas MBC2 dans `write_mbc_ram`, `#L1243-L1318`).
  - Donc : le quartet haut du fichier vaut ce que le jeu a écrit. Il n'est pas fixé.
- Quartet haut **en lecture** : `ret |= 0xF0` pour MBC2. Le quartet haut du fichier est donc ignoré. Source : `SB Core/memory.c#L455-L457`.
- Valeur initiale : `0xFF`. Source : `SB Core/mbc.c#L243-L247`.
- Chargement : `fread` de `mbc_ram_size` octets. Un fichier plus long est accepté, la fin est ignorée. Source : `SB Core/gb.c#L1080-L1091`.
- Horloge : seulement si `has_rtc` (MBC3, HuC3, TPP1). Pas pour MBC2. Source : `SB Core/gb.c#L827-L844`, `#L896-L912`.
- Note : SameBoy sauvegarde seulement si `has_battery`, donc type `0x06` (`SB Core/mbc.c#L14-L15` ; `SB Core/gb.c#L900`).

### B.3 VBA-M

- Taille : `ram_size_ = k512B` → **512 octets**. Source : `VBM src/core/gb/gbCartData.cpp#L270-L283`.
- Le fichier contient seulement les vecteurs d'E/S : `{gbRam, ramSize}`. MBC2 n'ajoute rien. Source : `VBM src/core/gb/gb.cpp#L454-L494` ; écriture `#L150-L164`.
- Agencement : **une valeur par octet**, offset `address & 0x01FF`. Source : `VBM src/core/gb/gbMemory.cpp#L187-L199`.
- Quartet haut **en écriture** : toujours `0xF` (`0xF0 | (value & 0x0F)`). Source : `VBM src/core/gb/gbMemory.cpp#L197`.
- Quartet haut **en lecture** : VBA-M rend l'octet tel quel, sans `| 0xF0`. Source : `VBM src/core/gb/gbMemory.cpp#L208-L216`.
  - Déduction : un fichier dont le quartet haut vaut 0 (GameYob) donne `$0n` au jeu jusqu'à la prochaine écriture.
- Valeur initiale : `0xFF` (`gbRamFill`). Source : `VBM src/core/gb/gb.cpp#L356`, `#L446`.
- Chargement : la taille lue doit être exacte. Sinon VBA-M affiche « Battery file's size incompatible » et **désactive la sauvegarde**. Source : `VBM src/core/gb/gb.cpp#L166-L200`.
  - Déduction : un fichier de 256 o (mGBA) ou de 8 Kio (GameYob) est refusé.
- Horloge : aucune pour MBC2 (`case kMbc2: // Do nothing more`). Source : `VBM src/core/gb/gb.cpp#L480-L493`.

### B.4 BGB

- Taille : **512 octets**, image directe de `$A000-$A1FF`. Source : https://bgb.bircd.org/mbc2save.html.
- Agencement : **une valeur par octet**.
- Quartet haut dans le fichier : « undefined » selon l'auteur. Valeur exacte écrite : NON TROUVÉ (BGB n'a pas de code source public).
- Lecture : BGB ignore les données au-delà de la taille attendue. Les anciens fichiers de 8192 o (BGB 1.02) fonctionnent encore. Source : même page.
  - Déduction : un fichier GameYob de 8 Kio serait accepté par BGB. NON VÉRIFIÉ.
- Traitement du quartet haut à la lecture : NON TROUVÉ.
- Horloge : seulement pour MBC3 (48 octets, format VBA). Source : https://bgb.bircd.org/manual.html (section « SAV (battery) file format »).

### B.5 GameYob (TWiLight Menu++)

**Quel GameYob ?**
- TWiLight Menu++ lance `/_nds/TWiLightMenu/emulators/gameyob.nds`. Source : `TWL universal/include/common/bootstrappaths.h#L27-L28`.
- Le README crédite « Stewmath/GameYob ». Source : `TWL README.md#L114`.
- Le binaire est livré dans le dépôt (`7zfile/Virtual Console (…)/_nds/TWiLightMenu/emulators/gameyob.nds`, 429568 o). Il contient la chaîne `/gameyobds.ini`. Le fork DS-Homebrew au commit figé utilise `"/_nds/gameyobds.ini"` (`GY arm9/include/inputhelper.h#L21`).
- Conclusion : la version exacte du binaire est **NON VÉRIFIÉE**. Il semble plus ancien que le commit figé (CHANGELOG `0.5.3-mavica`). Le code MBC2 est le même dans le fork et dans l'amont, sauf une limite d'adresse (voir plus bas).

**Taille du fichier : 8192 octets.**
- `numRamBanks = 1` pour MBC2, quelle que soit la valeur de `0x149`. Source : `GY arm9/source/inputhelper.cpp#L905-L926`.
- Taille demandée : `numRamBanks * 0x2000` = 8192. Pas d'horloge pour MBC2 (seulement MBC3 et HuC3). Source : `GY arm9/source/inputhelper.cpp#L932-L941`.
- Fichier absent ou trop court : GameYob l'agrandit avec des **`0x00`** (`fputc(0)`). Source : `GY arm9/source/inputhelper.cpp#L950-L967`.
- Fichier plus long : non réduit ; GameYob lit et écrit les 8192 premiers octets. Source : `GY arm9/source/inputhelper.cpp#L969`, `#L988-L990`.
- Sauvegarde complète ou par secteurs modifiés (`AUTOSAVE_SECTOR_SIZE`), toujours dans les 8192 premiers octets. Source : `GY arm9/source/inputhelper.cpp#L981-L1002`, `#L1007-L1030`.
- GameYob sauvegarde aussi pour le type `0x05` (sans pile) : il ne teste pas la pile. Déduction de `#L925-L939`.

**Agencement : une valeur par octet, au début du fichier.**
- Écriture : `writeSram(addr & 0x1fff, val & 0xf)`. Source : `GY arm9/source/mmu.cpp#L270-L294`.
  - Octet i = valeur de l'adresse `$A000 + i`.
  - Quartet haut **toujours 0** après une écriture du jeu.
  - **Pas de miroir** à 512 o : une écriture à `$A200` va à l'octet 0x200. Le fork DS-Homebrew n'a pas de limite d'adresse.
  - L'amont Stewmath ajoute `addr < 0xa200` : les écritures au-delà de 512 sont ignorées. Source : `UGY platform/common/mbc.cpp#L167-L171`.
- Lecture : pas de fonction de lecture pour MBC2 (`mbcReads[2] = NULL`). Le jeu lit l'octet brut de `externRam`, sans `| 0xF0` et sans miroir. Sources : `GY arm9/source/mmu.cpp#L215-L217`, `#L724-L734`, `#L118-L125`.
- Les octets `0x200` à `0x1FFF` : `0x00` pour un fichier créé par GameYob. Ils restent à 0 si le jeu n'écrit pas au-delà de `$A1FF`. Déduction.
- Les 512 premiers octets d'un fichier neuf valent `0x00` (et non `0xFF` comme chez les autres émulateurs). Déduction de `#L950-L967`.

### B.6 Reconnaître un jeu MBC2 depuis l'en-tête

- Octet `0x147` (type de cartouche) : `0x05` = MBC2, `0x06` = MBC2 + pile. L'octet `0x149` (taille RAM) est ignoré.
- Code :
  - mGBA : `struct GBCartridge` est placée à `rom[0x100]` ; son champ `type` tombe à `0x147`. `case 5: case 6: mbcType = GB_MBC2`. Sources : `MG include/mgba/internal/gb/gb.h#L139-L155` ; `MG src/gb/mbc.c#L317-L318`, `#L354-L369`.
  - SameBoy : `cartridge_type = &GB_cart_defs[rom[0x147]]` ; entrées `[5]` et `[6]` = `GB_MBC2`. Sources : `SB Core/mbc.c#L160` ; `SB Core/mbc.c#L13-L15`.
  - VBA-M : `case 0x05` et `case 0x06` → `kMbc2`, `ram_size_ = k512B`, pile pour `0x06`. Source : `VBM src/core/gb/gbCartData.cpp#L270-L283` (lecture de `cartridge_type` à `#L255`).
  - GameYob : `mapper = romSlot0[0x147]` puis `case 5: case 6: MBC = MBC2`. Source : `GY arm9/source/inputhelper.cpp#L748-L749`, `#L777-L778`.
- Remarque : mGBA teste d'abord un en-tête GBX et les mappers non officiels (`MG src/gb/mbc.c#L327-L352`). Pour une ROM `.gb` normale, l'octet `0x147` décide.

### B.7 Tableau récapitulatif — MBC2

| Émulateur | Taille | Valeurs par octet | Adresse paire | Quartet haut écrit | Quartet haut à la lecture du fichier | Fichier neuf | Horloge |
|---|---|---|---|---|---|---|---|
| mGBA | 256 | 2 | quartet bas | — (pas de quartet libre) | — | `0xFF` | non |
| SameBoy | 512 | 1 | — | valeur brute écrite par le jeu | ignoré (`\| 0xF0`) | `0xFF` | non |
| VBA-M | 512 (exact, sinon refus) | 1 | — | `0xF` | rendu tel quel au jeu | `0xFF` | non |
| BGB | 512 (accepte plus long) | 1 | — | « undefined » | NON TROUVÉ | NON TROUVÉ | non |
| GameYob | 8192 (valeurs dans les 512 premiers octets) | 1 | — | `0x0` | rendu tel quel au jeu | `0x00` | non |

### B.8 Conséquences pour 3dssavesync (déduction)

- Le Hash du fichier brut ne peut pas servir pour MBC2. Même entre SameBoy et VBA-M (512 o tous les deux), le quartet haut diffère.
- Forme canonique possible : 512 quartets bas. Lecture :
  - 256 o (mGBA) : octet i → `b & 0xF`, puis `b >> 4`.
  - 512 o (SameBoy, VBA-M, BGB) : `b & 0xF` pour chaque octet.
  - 8192 o (GameYob) : `b & 0xF` pour les 512 premiers octets. Le reste est ignoré, sauf si un jeu écrit au-delà de `$A1FF`.
- Écriture vers chaque cible :
  - mGBA : 256 o packés, adresse paire en quartet bas.
  - SameBoy, VBA-M, BGB : 512 o, `0xF0 | v` (forme du `SaveConverter` de mGBA, acceptée par les trois).
  - GameYob : 8192 o, `v` (ou `0xF0 | v`) dans les 512 premiers octets, `0x00` ensuite. NON VÉRIFIÉ sur la 3DS.
- Une sauvegarde 3DS qui vaut 8192 o ne permet pas, seule, de savoir que le jeu est MBC2. Il faut l'octet `0x147` de la ROM (B.6).

## Bonus pour §8.3 (petites SRAM)

- mGBA : `ramSize` d'en-tête `1` tombe dans `default`, donc **0x2000 (8 Kio)**, comme `2`. Source : `MG src/gb/mbc.c#L331-L338`.
- SameBoy : `ram_sizes[1] = 0x800` (2 Kio). Source : `SB Core/mbc.c#L223-L228`.
- GameYob : 8 Kio pour `1` et `2` (`GY arm9/source/inputhelper.cpp#L910-L913`).
- Déduction : pour `0x149 = 1`, mGBA et GameYob donnent 8 Kio ; SameBoy donne 2 Kio. Ce point ne concerne pas MBC2.

## Points NON TROUVÉS ou incertains

1. Format interne de l'EEPROM dans le matériel « legacy » de la 3DS : NON TROUVÉ. La correspondance repose sur le README d'open_agb_firm et l'outil d'exelotl, pas sur du code d'open_agb_firm.
2. open_agb_firm avec un fichier EEPROM plus court que prévu : contenu du reste de la zone inconnu. NON VÉRIFIÉ.
3. Écart de taille EEPROM entre open_agb_firm (version SDK) et mGBA (adresse utilisée) sur un jeu réel : NON VÉRIFIÉ.
4. BGB : valeur exacte du quartet haut écrit, valeur d'un fichier neuf, et masque à la lecture : NON TROUVÉ (pas de code source).
5. Version exacte du `gameyob.nds` livré par TWiLight Menu++ : NON VÉRIFIÉE. Le code MBC2 est le même dans le fork et l'amont, sauf la limite `addr < 0xa200`.
6. Comportement réel d'un jeu MBC2 sous GameYob avec un quartet haut à `0xF` : NON VÉRIFIÉ.
