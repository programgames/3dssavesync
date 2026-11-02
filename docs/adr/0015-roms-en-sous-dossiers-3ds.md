# ROM en sous-dossiers sur la 3DS : la Sauvegarde là où le lanceur l'écrit

Avant, le homebrew ne lisait que la racine de chaque dossier des ROM (décision Q10). Il lit maintenant les sous-dossiers, sur 4 niveaux au plus. Il ne lit jamais les dossiers `saves`, les dossiers cachés (`.…`) ou qui commencent par `_` (`_nds` de TWiLight Menu++), ni `3ds`, `luma`, `mGBA` et `Nintendo 3DS` (utile si le dossier des ROM est la racine de la carte SD).

Sauvegarde d'une ROM `<roms>/<sous-dossier>/X.nds` (ou `.gb`, `.gbc`, `.gba`), selon le réglage « Sauvegardes » de son Système (`docs/research-v2.md` : `SAVE_LOCATION` de TWiLight Menu++ et de NTR_Forwarder, GameYob, `useSavesFolder` d'open_agb_firm) :

| Réglage « Sauvegardes » | Sauvegarde | Lanceur |
|---|---|---|
| `<roms>/saves` | `<roms>/<sous-dossier>/saves/X.sav` | TWiLight Menu++ et forwarders DS (`SAVE_LOCATION=0`, défaut) |
| vide, ou égal à `<roms>` | `<roms>/<sous-dossier>/X.sav` | GameYob (GB), open_agb_firm `useSavesFolder=false`, `SAVE_LOCATION=1` |
| autre dossier | `<saves>/X.sav` | open_agb_firm `useSavesFolder=true`, `SAVE_LOCATION=2` (`_nds/TWiLightMenu/saves`) |

Une ROM de la racine garde le même chemin qu'avant. Le forwarder mGBA ne change pas (`sdmc:/mGBA/forwarders/<nom>.sav`). La comparaison entre `saves` et `roms` ignore la casse (carte SD en FAT).

Deux ROM qui donnent le même fichier de Sauvegarde (même nom dans deux sous-dossiers, avec un dossier de sauvegardes commun) sont un Doublon, comme deux ROM de même Clé. Deux Copies de sécurité de même nom dans la même seconde reçoivent la Clé dans leur nom.

## Conséquences

- Chemins déduits du code des lanceurs, non vérifiés sur la 3DS pour une ROM en sous-dossier.
- La limite de 512 jeux (`MAX_JEUX`) compte aussi les ROM des sous-dossiers ; au-delà, les ROM sont ignorées et signalées.
