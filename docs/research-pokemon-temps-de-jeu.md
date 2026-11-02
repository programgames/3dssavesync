# Recherche : temps de jeu et nom du dresseur dans les sauvegardes Pokémon Gen 1 à 5

Date : 2026-11-02.

Source unique : le code de PKHeX, dépôt `kwsch/PKHeX`, branche `master`.
Commit lu : `0660d1e1f4924b8478daec73281bb8071a82ad19` (1 novembre 2026).
URL de base des fichiers bruts : `https://raw.githubusercontent.com/kwsch/PKHeX/master/`.
Les chemins ci-dessous sont relatifs à cette base. Les numéros de ligne correspondent à ce commit.

Conventions :

- « LE » = petit-boutiste (little-endian). « BE » = gros-boutiste (big-endian).
- `u8` = 1 octet. `u16` = 2 octets. `u32` = 4 octets.
- Toutes les positions sont en hexadécimal. Elles sont absolues dans le fichier brut, sauf mention contraire.
- « INT » = versions internationales (dont FR/EUR). « JPN » = japonaise. « KOR » = coréenne.

## 1. Tableau récapitulatif

| Gén. | Taille brute | Reconnaissance (PKHeX) | Temps de jeu | Nom du dresseur | Sources principales |
|---|---|---|---|---|---|
| 1 RBJ | `0x8000` (32 Kio) | Listes Pokémon valides à `0x2F2C` et `0x30C0` (INT). Jaune : octet `0x29C3` = `0x54`, sinon `0x271C` ≠ 0. | INT `0x2CED` : heures `u8`, `+1` drapeau « maxed », `+2` minutes, `+3` secondes, `+4` frames. JPN `0x2CA0`. | `0x2598`, 7 caractères max (INT), table Gen 1, fin `0x50`. | `Saves/SAV1.cs`, `Saves/Substructures/Gen12/SAV1Offsets.cs`, `PKM/Strings/StringConverter1.cs` |
| 2 OAC | `0x8000` (PKHeX accepte aussi `0x10000`) | Listes valides : GS INT `0x288A`/`0x2D6C`, Cristal INT `0x2865`/`0x2D10`. | GS INT `0x2053`, Cristal INT `0x2052` : heures `u16` **BE**, `+2` minutes, `+3` secondes. | `0x200B`, 7 caractères max (INT), table Gen 2 (table FR propre), fin `0x50`. | `Saves/SAV2.cs`, `Saves/Substructures/Gen12/SAV2Offsets.cs`, `PKM/Strings/StringConverter2.cs` |
| 3 RSE/RFVF | `0x20000` (128 Kio) | Une moitié a les 14 ID de secteur 0 à 13. Version par le `u32` à `0xAC` de la section 0. | Section 0 : heures `u16` LE à `+0x0E`, minutes `+0x10`, secondes `+0x11`, frames `+0x12`. Identique RS/E/RFVF. | Section 0 : `+0x00`, 8 octets, 7 caractères max (INT), table Gen 3, fin `0xFF`. | `Saves/SAV3.cs`, `Saves/Blocks/Gen3/SaveBlock3Small*.cs`, `PKM/Strings/StringConverter3.cs` |
| 4 DPPt/HGSS | `0x80000` (512 Kio) | Pied du bloc général de la 2e partition (`0x40000 + taille`) : taille = `0xC100` (DP), `0xCF2C` (Pt), `0xF628` (HGSS) et magie `0x20060623` (ou `0x20070903` KOR). | Bloc général actif + `Trainer1 + 0x22` : heures `u16` LE, `+0x24` minutes, `+0x25` secondes. `Trainer1` = `0x64` (DP, HGSS) ou `0x68` (Pt). | Bloc général actif + `Trainer1` : 16 octets, 8 caractères 16 bits, table Gen 4 propre, fin `0xFFFF`. | `Saves/SAV4.cs`, `Saves/SAV4DP.cs`, `Saves/SAV4Pt.cs`, `Saves/SAV4HGSS.cs`, `PKM/Strings/StringConverter4*.cs` |
| 5 NB/N2B2 | `0x80000` (512 Kio) | CRC16-CCITT valide : NB sur `0x23F00` (longueur `0x8C`) stocké à `0x23F9A` ; N2B2 sur `0x25F00` (longueur `0x94`) stocké à `0x25FA2`. | Heures `u16` LE à `0x19424`, minutes `0x19426`, secondes `0x19427`. Identique NB/N2B2. | `0x19404`, 16 octets, UTF-16 LE, fin `0xFFFF`. Identique NB/N2B2. | `Saves/SAV5.cs`, `Saves/Substructures/Gen5/PlayerData5.cs`, `Saves/Access/SaveBlockAccessor5*.cs` |

## 2. Tailles de fichier et en-têtes/pieds d'émulateur

### 2.1 Tailles brutes reconnues

Source : `PKHeX.Core/Saves/Util/SaveUtil.cs`, lignes 77-94.

| Constante | Valeur | Usage |
|---|---|---|
| `SIZE_G1RAW` | `0x8000` | Gen 1 |
| `SIZE_G2RAW_U` | `0x8000` | Gen 2 INT |
| `SIZE_G2RAW_J` | `0x10000` | Gen 2 (taille de création par défaut de `SAV2`) |
| `SIZE_G3RAW` | `0x20000` | Gen 3 |
| `SIZE_G3RAWHALF` | `0x10000` | Gen 3 « demi-taille » (émulateur mal configuré) |
| `SIZE_G4RAW` | `0x80000` | Gen 4 |
| `SIZE_G5RAW` | `0x80000` | Gen 5 |

`IsSizeGen2` accepte `0x8000` et `0x10000` (`SaveUtil.cs`, ligne 149).
`IsG3` exige exactement `0x20000` (`SaveUtil.cs`, ligne 284).

### 2.2 Ordre de lecture d'un fichier

Source : `SaveUtil.TryGetSaveFile`, `SaveUtil.cs`, lignes 494-508.

1. PKHeX essaie les lecteurs spéciaux (`ZipReader`).
2. PKHeX essaie le fichier brut, sans retrait d'octets.
3. Si l'étape 2 échoue, PKHeX essaie chaque « handler » d'émulateur. Le handler retire l'en-tête ou le pied. PKHeX analyse ensuite la partie restante.

Liste des handlers : `SaveUtil.cs`, lignes 117-124.

### 2.3 Handlers d'émulateur

| Handler | Fichier | Règle |
|---|---|---|
| `SaveHandlerFooterRTC` | `Saves/Util/Recognition/SaveHandlerFooterRTC.cs` | Gen 1 à 3. Taille du pied = `taille & 0x3F`. Le pied doit être pair et entre `0x0C` et `0x30`, ou égal à `0x07` (FlashGBX). La taille sans pied doit être `0x20000`, `0x10000` ou `0x8000` (lignes 13-31). PKHeX coupe le pied à la fin (lignes 33-41). |
| `SaveHandlerDeSmuME` | `Saves/Util/Recognition/SaveHandlerDeSmuME.cs` | Taille `0x80000 + 0x7A`. Le fichier finit par `\|-DESMUME SAVE-\|`. PKHeX coupe les `0x7A` derniers octets (lignes 10-27). |
| `SaveHandlerARDS` | `Saves/Util/Recognition/SaveHandlerARDS.cs` | Fichier `.duc`. Taille `0x80000 + 0xA4`. PKHeX coupe un en-tête de `0xA4` octets, sans vérification (lignes 10-21). |
| `SaveHandlerNSO` | `Saves/Util/Recognition/SaveHandlerNSO.cs` | Nintendo Switch Online. Le fichier commence par `SRAM`. Taille de base `0x8000` ou `0x10000`, plus `0x68` à `0xA0` octets d'en-tête (lignes 85-109). |
| `SaveHandlerGCI` | `Saves/Util/Recognition/SaveHandlerGCI.cs` | Dolphin (GameCube). Hors sujet ici. |

Conséquence pour les pieds RTC Gen 2 et Gen 3 :

- 16 octets (`0x10`) : accepté.
- 44 octets (`0x2C`) : accepté.
- 48 octets (`0x30`) : accepté.
- Un fichier Gen 2 de `0x8000 + pied` passe aussi, car `0x8000` = `SIZE_G1RAW`.

Règle simple pour notre outil : si `taille & 0x3F` est un pied plausible, lis seulement les `taille - (taille & 0x3F)` premiers octets.

### 2.4 Langue et sous-version en Gen 1 à 3

Source : `Saves/Util/SaveLanguage.cs`.

- Gen 1 à 3 ne stockent pas la langue INT dans les données lues par PKHeX.
- PKHeX déduit la langue et la version depuis le **nom du fichier** (`InferFrom1`, `InferFrom2`, `InferFrom3`, lignes 153-317).
- Exemples : « roug » → Rouge FR, « bleu » → Bleu FR, « jaun » → Jaune FR, « rubis » → Rubis FR, « vert » → Vert Feuille FR.
- Sans indice, PKHeX prend l'anglais (`OverrideLanguageGen1` à `Gen3FRLG`, lignes 13-20).
- PKHeX ne distingue pas Rouge et Bleu, Or et Argent, Rubis et Saphir, Rouge Feu et Vert Feuille avec les données seules.
- `SaveUtil.TryGetSaveFile(path)` appelle `SaveLanguage.TryRevise` pour les Gen ≤ 3 (`SaveUtil.cs`, lignes 466-467).

## 3. Génération 1 (Rouge, Bleu, Jaune)

### 3.1 Reconnaissance

Source : `SaveUtil.cs`, lignes 206-246.

1. Vérifie la taille : `0x8000`.
2. Teste d'abord la version JPN : listes valides à `0x2ED5` et `0x302D`, avec 30 Pokémon max.
3. Sinon, teste la version INT : listes valides à `0x2F2C` (équipe) et `0x30C0` (boîte courante), avec 20 Pokémon max (`IsG1INT`, ligne 206).
4. Une liste est valide si `count = data[offset]`, `count ≤ max`, et `data[offset + 1 + count] == 0xFF` (`IsListValidG12`, lignes 223-227).

Jaune ou Rouge/Bleu : `SAV1.IsYellowINT`, `Saves/SAV1.cs`, lignes 569-578.

- Lis l'octet du starter à `0x29C3` (INT) ou `0x29B9` (JPN).
- Si cet octet ≠ 0 : Jaune si l'octet vaut `0x54` (Pikachu).
- Si cet octet = 0 : Jaune si l'amitié de Pikachu ≠ 0, à `0x271C` (INT) ou `0x2712` (JPN).

### 3.2 Temps de jeu

Source : `SAV1Offsets.PlayTime` (`Saves/Substructures/Gen12/SAV1Offsets.cs`, lignes 29 et 57) et `SAV1.PlayedHours`… (`Saves/SAV1.cs`, lignes 324-363).

| Champ | INT | JPN | Format |
|---|---|---|---|
| Heures | `0x2CED` | `0x2CA0` | `u8` |
| Drapeau « maxed » | `0x2CEE` | `0x2CA1` | `u8`, ≠ 0 = compteur bloqué |
| Minutes | `0x2CEF` | `0x2CA2` | `u8` |
| Secondes | `0x2CF0` | `0x2CA3` | `u8` |
| Frames | `0x2CF1` | `0x2CA4` | `u8` |

Le format a bien un octet « maxed » : `PlayedMaximum` lit `PlayTime + 1` (ligne 343).
Quand PKHeX écrit 255 heures ou plus, il met 255, met le drapeau à 1 et remet minutes, secondes et frames à 0 (lignes 331-336).

### 3.3 Nom du dresseur

Source : `SAV1Offsets.OT => 0x2598` (ligne 64), `SAV1.OT` (`SAV1.cs`, lignes 268-272), `MaxStringLengthTrainer` (ligne 242).

- Position : `0x2598`, pour INT et JPN.
- Longueur lue : 7 octets (INT), 5 octets (JPN). Taille du tampon : 11 octets (INT), 6 (JPN) (`PKM/Shared/GBPKML.cs`, lignes 12-13).
- Fin de chaîne : `0x50` (`StringConverter1.TerminatorCode`, ligne 11). Un octet qui donne `NUL` dans la table arrête aussi la lecture.
- Un premier octet `0x5D` signifie « nom d'échange en jeu » (ligne 116).

Table de caractères : `PKM/Strings/StringConverter1.cs`, `TableEN` (lignes 228-246).

- PKHeX utilise **une seule table pour toutes les langues INT**, dont le français. Il n'existe pas de table FR séparée en Gen 1.
- Le fichier `StringConverter12` n'existe plus sur `master`. Les fichiers actuels sont `StringConverter1.cs`, `StringConverter2.cs` et `StringConverter12Transporter.cs`.
- Valeurs utiles : `A`–`Z` = `0x80`–`0x99`, `a`–`z` = `0xA0`–`0xB9`, `0`–`9` = `0xF6`–`0xFF`, espace = `0x7F`.
- Accents : `à` `0xBA`, `è` `0xBB`, `é` `0xBC`, `ù` `0xBD`, `À` `0xBE`, `È` `0xC6`, `É` `0xC7`.

### 3.4 Somme de contrôle (pour valider la lecture)

Source : `SAV1.cs`, lignes 252-263.

- Octet de contrôle à `0x3523` (INT) ou `0x3594` (JPN).
- Valeur = `~(somme sur 8 bits des octets de 0x2598 à ChecksumOfs - 1)`.

## 4. Génération 2 (Or, Argent, Cristal)

### 4.1 Reconnaissance

Source : `SaveUtil.cs`, lignes 208-212 et 252-275.

1. Vérifie la taille : `0x8000` ou `0x10000`.
2. Teste dans cet ordre. La première règle vraie gagne.

| Ordre | Version | Listes testées | Max |
|---|---|---|---|
| 1 | Or/Argent INT | `0x288A` et `0x2D6C` | 20 |
| 2 | Cristal INT | `0x2865` et `0x2D10` | 20 |
| 3 | Or/Argent JPN | `0x2D10` et `0x283E` | 30 |
| 4 | Cristal JPN | `0x2D10` et `0x281A` | 30 |
| 5 | Or/Argent KOR | `0x2DAE` et `0x28CC` | 20 |

Il n'y a pas de Cristal coréen (`SAV2Offsets.cs`, ligne 199).
Or et Argent ne se distinguent pas avec les données (voir 2.4).

### 4.2 Temps de jeu

Source : `SAV2Offsets.TimePlayed` (`Saves/Substructures/Gen12/SAV2Offsets.cs`, lignes 72, 96, 131, 204) et `SAV2.PlayedHours`… (`Saves/SAV2.cs`, lignes 372-387).

| Version | Heures (`u16` **BE**) | Minutes (`u8`) | Secondes (`u8`) |
|---|---|---|---|
| Or/Argent INT | `0x2053` | `0x2055` | `0x2056` |
| Cristal INT | `0x2052` | `0x2054` | `0x2055` |
| Or/Argent/Cristal JPN | `0x2034` | `0x2036` | `0x2037` |
| Or/Argent KOR | `0x204D` | `0x204F` | `0x2050` |

PKHeX lit les heures avec `ReadUInt16BigEndian` (ligne 374). PKHeX ne lit pas d'octet « maxed » en Gen 2.

### 4.3 Nom du dresseur

Source : `SAV2Offsets.Trainer1 = 0x2009` (ligne 10) et `SAV2.OT` (`SAV2.cs`, lignes 316-319).

- Position : `Trainer1 + 2` = `0x200B`, pour toutes les versions.
- `0x2009` contient l'ID dresseur (`u16` BE, `SAV2.cs`, ligne 366).
- Longueur lue : 7 octets (INT), 5 octets (JPN), 10 octets (KOR, 2 octets par caractère) (ligne 273 et ligne 318).
- Fin de chaîne : `0x50`.

Tables de caractères : `PKM/Strings/StringConverter2.cs`.

- `GetDict` choisit la table selon la langue (lignes 98-104) : 1 → `TableJP`, 3 ou 5 → `TableFRE`, 4 ou 7 → `TableITA`, sinon `TableEN`.
- `LanguageID` : 3 = français, 5 = allemand (`Game/Enums/LanguageID.cs`, lignes 27 et 37).
- `TableFRE` (lignes 248-266) est la table « français et allemand ».
- Différences FR par rapport à `TableEN` : `ß` `0xBE`, `ç` `0xBF`, `ë` `0xC6`, `ï` `0xC7`, `â` `0xC8`, `ô` `0xC9`, `û` `0xCA`, `ê` `0xCB`, `î` `0xCC`.
- Valeurs communes : `A`–`Z` `0x80`–`0x99`, `a`–`z` `0xA0`–`0xB9`, `à` `0xBA`, `è` `0xBB`, `é` `0xBC`, `ù` `0xBD`, `0`–`9` `0xF6`–`0xFF`.
- Avertissement : la détection INT donne la langue 0 (`SaveTypeInfo`, langue par défaut). PKHeX choisit ensuite la langue avec le nom du fichier (voir 2.4). Pour une cartouche FR, utilise directement `TableFRE`.

### 4.4 Somme de contrôle

Source : `SAV2.cs`, lignes 281-294, et `SAV2Offsets.cs`.

- Somme `u16` des octets de `0x2009` à `AccumulatedChecksumEnd` inclus.
- Or/Argent INT : fin `0x2D68`, stockage `u16` LE à `0x2D69` et `0x7E6D`.
- Cristal INT : fin `0x2B82`, stockage à `0x2D0D` et `0x1F0D`.
- PKHeX lit seulement la copie principale. Il recopie les zones vers la copie de secours à l'écriture (`SAV2.cs`, lignes 187-218).

## 5. Génération 3 (Rubis, Saphir, Émeraude, Rouge Feu, Vert Feuille)

### 5.1 Structure

Source : `Saves/SAV3.cs`, lignes 29-153.

- Taille de secteur : `0x1000`. Données utiles par secteur : `0xF80` (`SIZE_SECTOR_USED`, ligne 40).
- Une sauvegarde principale = 14 secteurs (`COUNT_MAIN`, ligne 41) = `0xE000` octets.
- Moitié 0 : `0x00000`–`0x0DFFF`. Moitié 1 : `0x0E000`–`0x1BFFF`.
- Secteurs « extra » fixes (Temple de la Renommée, e-Reader) : `0x1C000`, `0x1D000`, `0x1E000`, `0x1F000` (lignes 250-251 et 322-329).
- Les secteurs tournent dans chaque moitié. La position physique du secteur d'ID 0 change.

Pied de chaque secteur (positions relatives au début du secteur) :

| Position | Type | Contenu | Source |
|---|---|---|---|
| `0xFF4` | `u16` LE | ID de section (0 à 13) | `SAV3.cs`, lignes 85 et 129 |
| `0xFF6` | `u16` LE | Somme de contrôle | `SAV3.cs`, lignes 239 et 305 |
| `0xFF8` | `u32` | Non lu par PKHeX | — |
| `0xFFC` | `u32` LE | Compteur de sauvegarde | `SAV3BlockDetection.cs`, ligne 21 |

Avertissement : PKHeX ne lit pas la valeur de signature `0x08012025`. Une recherche de `08012025` et `0x0801` dans `PKHeX.Core/Saves` ne donne aucun résultat. La position `0xFF8` de cette signature ne vient donc pas de PKHeX.

Somme de contrôle : `Checksums.CheckSum32` (`Saves/Util/Checksums.cs`, lignes 128-139).

1. Additionne les 0xF80 premiers octets du secteur par mots `u32` LE, dans un `u32`.
2. Calcule `(u16)(somme + (somme >> 16))`.

### 5.2 Algorithme de choix de la moitié active

Source : `SAV3.IsAllMainSectorsPresent` (lignes 119-138), `SAV3.GetActiveSlot` (lignes 140-153), `Saves/Substructures/Gen3/SAV3BlockDetection.cs` (lignes 19-43).

1. Pour chaque moitié `m` (0 et 1), lis l'ID `u16` à `m*0xE000 + k*0x1000 + 0xFF4`, pour `k` = 0 à 13.
2. Si un ID est ≥ 14, la moitié est invalide.
3. Si les 14 ID de 0 à 13 sont tous présents, la moitié est valide. Note la position `S0[m]` du secteur d'ID 0.
4. Si une seule moitié est valide, prends-la. Si aucune n'est valide, prends la moitié 0.
5. Si les deux sont valides, lis le compteur `u32` LE à `S0[m] + 0xFFC` pour chaque moitié.
6. Si un compteur vaut `0xFFFFFFFF` et l'autre ne vaut pas `0xFFFFFFFE`, prends l'autre moitié.
7. Sinon, prends la moitié au plus grand compteur. En cas d'égalité, prends la moitié 0.
8. Pour un fichier de `0x10000` octets, PKHeX prend toujours la moitié 0 (ligne 142).

PKHeX ne vérifie pas les sommes de contrôle pour choisir la moitié.

### 5.3 Section 0 (« small block ») : temps de jeu et nom

Source : `Saves/Blocks/Gen3/SaveBlock3SmallRS.cs`, `SaveBlock3SmallE.cs`, `SaveBlock3SmallFRLG.cs`, lignes 11-19 dans les trois fichiers.

Les positions sont **identiques** pour RS, E et RFVF. Elles sont relatives à `S0` (début du secteur d'ID 0 de la moitié active).

| Champ | Position | Format |
|---|---|---|
| Nom du dresseur | `S0 + 0x00` | 8 octets, table Gen 3 |
| Sexe | `S0 + 0x08` | `u8` |
| ID / ID secret | `S0 + 0x0A` / `S0 + 0x0C` | `u16` LE |
| Heures | `S0 + 0x0E` | `u16` LE |
| Minutes | `S0 + 0x10` | `u8` |
| Secondes | `S0 + 0x11` | `u8` |
| Frames | `S0 + 0x12` | `u8` |

### 5.4 Encodage du nom

Source : `SAV3.OT` (`SAV3.cs`, lignes 350-362), constructeur (lignes 72-75), `PKM/Strings/StringConverter3.cs`.

- Longueur lue : 7 octets (INT), 5 octets (JPN).
- Version JPN si le `u16` LE à `S0 + 0x06` vaut 0 (ligne 75).
- Fin de chaîne : `0xFF` (`TerminatorByte`, ligne 10).
- PKHeX utilise `G3_EN` pour toutes les langues non japonaises (ligne 48). Il n'y a pas de table FR séparée.
- Seuls les guillemets dépendent de la langue : `0xB1` = `«` et `0xB2` = `»` en français (lignes 154-168).
- Valeurs utiles (`G3_EN`, lignes 190-211) : espace `0x00`, `A`–`Z` `0xBB`–`0xD4`, `a`–`z` `0xD5`–`0xEE`, `0`–`9` `0xA1`–`0xAA`.
- Accents : `À` `0x01`, `Ç` `0x04`, `È` `0x05`, `É` `0x06`, `à` `0x16`, `ç` `0x19`, `è` `0x1A`, `é` `0x1B`, `ê` `0x1C`, `ë` `0x1D`, `î` `0x20`, `ï` `0x21`, `ô` `0x24`, `ù` `0x26`, `û` `0x28`.

### 5.5 Distinguer RS, E et RFVF

Source : `SaveUtil.GetVersionG3SAV`, `SaveUtil.cs`, lignes 302-321.

1. Lis le `u32` LE à `S0 + 0xAC`.
2. Si la valeur vaut 1 : Rouge Feu / Vert Feuille.
3. Si la valeur vaut 0 : Rubis / Saphir.
4. Sinon : Émeraude si un octet non nul existe dans `S0 + 0x890` à `S0 + 0xF2C`. Sinon Rubis / Saphir.

Avertissement : PKHeX applique cette règle au secteur 0 de la **première** moitié valide trouvée (`IsG3`, lignes 288-293), pas forcément la moitié active.

Pour le temps de jeu et le nom, cette distinction est inutile. Les positions de la section 0 sont les mêmes.

## 6. Génération 4 (Diamant, Perle, Platine, HeartGold, SoulSilver)

### 6.1 Structure

Source : `Saves/SAV4.cs`, lignes 20 et 53-67.

- Deux partitions de `0x40000` : partition 0 à `0x00000`, partition 1 à `0x40000`.
- Chaque partition contient un bloc « général » à son début, puis un bloc « stockage » (boîtes).
- PKHeX choisit le bloc général actif et le bloc stockage actif séparément.

| Jeu | Taille bloc général | Début bloc stockage | Taille pied | `Trainer1` | Sources |
|---|---|---|---|---|---|
| DP | `0xC100` | `0xC100` | `0x14` | `0x64` | `SAV4DP.cs` lignes 18, 36-37, 58 ; `SAV4Sinnoh.cs` ligne 12 |
| Pt | `0xCF2C` | `0xCF2C` | `0x14` | `0x68` | `SAV4Pt.cs` lignes 18, 33-34, 62 ; `SAV4Sinnoh.cs` ligne 12 |
| HGSS | `0xF628` | `0xF700` | `0x10` | `0x64` | `SAV4HGSS.cs` lignes 20, 34-38, 70 |

### 6.2 Pied du bloc général

Positions relatives à la fin `F` du bloc (`F` = début + taille) :

| Position | Type | Contenu | Source |
|---|---|---|---|
| `F - 0x14` | `u32` LE | Compteur « majeur » | `SAV4.GetActiveBlock` (lignes 180-184), `SAV4BlockDetection.cs` ligne 22 |
| `F - 0x10` | `u32` LE | Compteur « mineur » | `SAV4BlockDetection.cs` ligne 29 |
| `F - 0x0C` | `u32` LE | Taille du bloc | `SaveUtil.IsValidGeneralFooter2`, ligne 398 |
| `F - 0x08` | `u32` LE | Magie : `0x20060623` (JPN/INT) ou `0x20070903` (KOR) | `SAV4.cs` lignes 117-119 ; `SaveUtil.cs` ligne 401 |
| `F - 0x02` | `u16` LE | CRC16-CCITT du bloc sans le pied | `SAV4.cs` lignes 113-115 |

### 6.3 Reconnaissance de DP, Pt et HGSS

Source : `SaveUtil.cs`, lignes 171-173, 377-379 et 389-403.

1. Vérifie la taille : `0x80000`.
2. Pour chaque jeu, dans l'ordre DP, Pt, HGSS, prends le bloc général de la **partition 1** : début `0x40000`, taille `G`.
3. Le jeu est reconnu si le `u32` LE à `0x40000 + G - 0x0C` vaut `G`, et si le `u32` LE à `0x40000 + G - 0x08` vaut `0x20060623` ou `0x20070903`.

Commentaire de PKHeX (ligne 391) : le jeu écrit la première sauvegarde dans la seconde moitié. La partition 1 suffit donc pour la détection.

Valeurs concrètes à tester :

| Jeu | Taille à `0x40000 + ...` | Magie à `0x40000 + ...` |
|---|---|---|
| DP | `0x4C0F4` = `0x0000C100` | `0x4C0F8` |
| Pt | `0x4CF20` = `0x0000CF2C` | `0x4CF24` |
| HGSS | `0x4F61C` = `0x0000F628` | `0x4F620` |

Sous-version :

- HGSS : octet `ROMCode` à `Trainer1 + 0x1C` du bloc général actif. 7 = HeartGold, 8 = SoulSilver (`SAV4HGSS.cs` ligne 30 ; `SAV4.cs` lignes 317-321 ; `Game/Enums/GameVersion.cs` lignes 66 et 71).
- DP : PKHeX ne distingue pas Diamant et Perle. `Version` vaut toujours `DP` (`SAV4DP.cs`, ligne 31). Le commentaire de `ROMCode` dit « Unused by D/P » (`SAV4.cs`, ligne 317).
- Pt : `Version` vaut toujours `Pt` (`SAV4Pt.cs`, ligne 28).

### 6.4 Algorithme de choix du bloc général actif

Source : `SAV4.GetActiveBlock` (lignes 180-184) et `SAV4BlockDetection.CompareFooters` (lignes 19-50).

1. Lis le compteur majeur `u32` LE à `G - 0x14` (partition 0) et à `0x40000 + G - 0x14` (partition 1).
2. Si un compteur vaut `0xFFFFFFFF` et l'autre ne vaut pas `0xFFFFFFFE`, prends l'autre partition.
3. Sinon, si les compteurs diffèrent, prends la partition au plus grand compteur.
4. Si les compteurs sont égaux, compare les compteurs mineurs à `G - 0x10` avec les mêmes règles.
5. En cas d'égalité totale, prends la partition 0.
6. Base du bloc général actif : `B` = `0x00000` ou `0x40000`.

PKHeX ne vérifie pas le CRC pour choisir le bloc.

Avertissement : pour HGSS, le pied fait `0x10` octets. Le compteur « majeur » à `F - 0x14` est donc hors du pied. PKHeX utilise quand même cette position (ligne 182). Voir les points incertains.

### 6.5 Temps de jeu et nom

Source : `SAV4.cs`, lignes 261-365.

| Champ | Position | DP / HGSS | Pt | Format |
|---|---|---|---|---|
| Nom | `B + Trainer1` | `B + 0x64` | `B + 0x68` | 16 octets |
| ID / ID secret | `B + Trainer1 + 0x10` / `+ 0x12` | `B + 0x74` / `0x76` | `B + 0x78` / `0x7A` | `u16` LE |
| Langue | `B + Trainer1 + 0x19` | `B + 0x7D` | `B + 0x81` | `u8` (3 = FR) |
| ROMCode | `B + Trainer1 + 0x1C` | `B + 0x80` | `B + 0x84` | `u8` |
| Heures | `B + Trainer1 + 0x22` | `B + 0x86` | `B + 0x8A` | `u16` LE |
| Minutes | `B + Trainer1 + 0x24` | `B + 0x88` | `B + 0x8C` | `u8` |
| Secondes | `B + Trainer1 + 0x25` | `B + 0x89` | `B + 0x8D` | `u8` |

### 6.6 Encodage du nom (Gen 4)

Source : `PKM/Strings/StringConverter4.cs` (lignes 12-42) et `PKM/Strings/StringConverter4Util.cs` (lignes 16-28, 116, 155-188).

- Le nom est une suite de valeurs `u16` LE. Ce n'est **pas** de l'UTF-16.
- Fin de chaîne : `0xFFFF`. La valeur 0 arrête aussi la lecture.
- Chaque valeur est un index dans `TableINT`. Les valeurs à partir de `0x400` utilisent `TableKOR`.
- Valeurs utiles : `0`–`9` `0x121`–`0x12A`, `A`–`Z` `0x12B`–`0x144`, `a`–`z` `0x145`–`0x15E`, espace `0x1DE`.
- Accents : `À` `0x15F`, `Ç` `0x166`, `È` `0x167`, `É` `0x168`, `à` `0x17F`, `â` `0x181`, `ç` `0x186`, `è` `0x187`, `é` `0x188`, `ê` `0x189`, `ë` `0x18A`, `î` `0x18D`, `ï` `0x18E`, `ô` `0x193`, `ù` `0x198`, `û` `0x19A`.

## 7. Génération 5 (Noir, Blanc, Noir 2, Blanc 2)

### 7.1 Reconnaissance

Source : `SaveUtil.cs`, lignes 78-79, 174-175, 381-382 et 405-411.

1. Vérifie la taille : `0x80000`.
2. Teste NB : calcule le CRC16-CCITT de `0x8C` octets à partir de `0x23F00`. Compare au `u16` LE à `0x23F9A`.
3. Si le test NB échoue, teste N2B2 : CRC16-CCITT de `0x94` octets à partir de `0x25F00`. Compare au `u16` LE à `0x25FA2`.

Calcul : `0x23F00` = `SIZE_G5BW (0x24000) - 0x100`. La valeur stockée est à `footer[^2]`, avec `footer` = `0x8C + 0x10` octets.
Ce bloc est le bloc 69 « Checksums » de `SaveBlockAccessor5BW.cs` (ligne 82) : `new(0x23F00, 0x008C, 0x23F9A, 0x23F9A)`.

CRC16-CCITT : `Checksums.CRC16_CCITT` (`Saves/Util/Checksums.cs`, lignes 16-28). Valeur initiale `0xFFFF`. Pas d'inversion finale.

Sous-version : octet `Game` à `0x1941F` (`PlayerData5.cs`, lignes 61-65 ; `SAV5.cs`, ligne 105). 20 = Blanc, 21 = Noir, 22 = Blanc 2, 23 = Noir 2 (`GameVersion.cs`, lignes 83-98).

### 7.2 Temps de jeu et nom

Source : bloc 27 « Trainer Data » à `0x19400`, identique pour NB (`SaveBlockAccessor5BW.cs`, ligne 40 : `new(0x19400, 0x0068, ...)`) et N2B2 (`SaveBlockAccessor5B2W2.cs`, ligne 41 : `new(0x19400, 0x00b0, ...)`). Champs : `Saves/Substructures/Gen5/PlayerData5.cs`.

| Champ | Position relative | Position absolue | Format | Ligne |
|---|---|---|---|---|
| Nom | `+0x04` | `0x19404` | 16 octets, UTF-16 LE | 11 |
| ID / ID secret | `+0x14` / `+0x16` | `0x19414` / `0x19416` | `u16` LE | 25-35 |
| Langue | `+0x1E` | `0x1941E` | `u8` (3 = FR) | 55-59 |
| Version | `+0x1F` | `0x1941F` | `u8` | 61-65 |
| Heures | `+0x24` | `0x19424` | `u16` LE | 75-79 |
| Minutes | `+0x26` | `0x19426` | `u8` | 81-85 |
| Secondes | `+0x27` | `0x19427` | `u8` | 87-91 |

Encodage : `PKM/Strings/StringConverter5.cs`, lignes 11-39.

- Chaque `u16` LE est un caractère UTF-16.
- Fin de chaîne : `0xFFFF`. La valeur 0 arrête aussi la lecture.
- PKHeX remplace seulement deux symboles de sexe par `♂`/`♀` (`NormalizeGenderSymbol`).

PKHeX lit ces positions directement dans le fichier. `SAV5` ne fait pas de choix entre deux copies (`SAV5.cs`, lignes 44-53 et 97-109).

## 8. Points incertains

- Signature Gen 3 `0x08012025` : PKHeX ne la lit pas. Sa position `0xFF8` n'est pas confirmée par cette source.
- Gen 3 demi-taille (`0x10000`) : `GetActiveSlot` la gère, mais `IsG3` exige `0x20000`. La détection automatique de PKHeX échoue donc sur ce cas, sauf via un autre chemin non trouvé.
- Gen 2 `0x10000` : PKHeX accepte cette taille. Le code ne dit pas quel émulateur ou quelle version produit cette taille.
- Gen 2 : PKHeX lit les heures en `u16` BE à `TimePlayed`. Le code ne lit pas d'octet avant `TimePlayed`. L'existence d'un drapeau « maxed » en Gen 2 n'est pas vérifiée ici.
- Gen 4 HGSS : PKHeX lit le compteur majeur à `F - 0x14`, hors du pied de `0x10` octets. Le sens exact de ce champ en HGSS n'est pas documenté dans le code.
- Gen 4 : la détection repose sur la partition 1 seulement. Un fichier où seule la partition 0 est valide n'est pas reconnu.
- Gen 4 : PKHeX ne distingue pas Diamant et Perle.
- Gen 5 : PKHeX ne lit qu'une copie. L'existence d'une copie de secours dans le fichier n'est pas traitée dans le code lu.
- Gen 1 à 3 : la langue et la sous-version (Rouge/Bleu, Or/Argent, Rubis/Saphir, Rouge Feu/Vert Feuille) viennent du nom de fichier, pas des données.
- melonDS et mGBA : PKHeX n'a pas de handler nommé pour eux. Seul le handler générique RTC (Gen 1 à 3) et DeSmuME (`.dsv`) existent.
