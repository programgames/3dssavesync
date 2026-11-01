# GB, GBC et GBA : synchroniser la SRAM seule, garder les lanceurs déjà utilisés

Relevé du 2026-11-01 : sur la 3DS, le GBA tourne déjà sous open_agb_firm (`useSavesFolder=false`, donc `.sav` brut à côté de la ROM dans `sd:/roms/gba/`) et le GB/GBC sous TWiLight (`.sav` à côté de la ROM dans `sd:/roms/gb/`). Sur le PC, mGBA. Ce sont des fichiers ordinaires, comme le `.sav` DS : la v2 les synchronise de la même façon, sans changer d'émulateur.

Le seul écart de format est l'horloge temps réel : mGBA ajoute un Trailer d'horloge à la fin du `.sav` (48 octets GB MBC3+TIMER, 16 octets GBA), open_agb_firm prend l'horloge de la 3DS, TWiLight n'en écrit pas. Règle :

- la SRAM est la partie « jeu » : `taille - (taille mod 1024)` (les trailers font moins de 1 Kio) ;
- le Hash, la taille comparée et la Version envoyée portent **sur la SRAM seule** ;
- à la Récupération, le Client écrit la SRAM reçue **puis remet son propre Trailer d'horloge** s'il en avait un : chaque machine garde son horloge.

## Options écartées

- **mGBA 3DS partout** (forwarders avec icône HOME) : fichiers identiques au PC, mais change l'émulateur de la 3DS, cache la ROM dans chaque CIA (pas de Clé lisible) et demande mGBA 0.11-dev. Gardé comme étape facultative.
- **Convertir les trailers entre émulateurs** : formats différents (GameYob ne met pas à jour les champs que mGBA lit), gain faible, risque réel. Chaque machine garde son horloge à la place.
- **Injects Virtual Console** : sauvegarde GBA dans la NAND (AGBSAVE), extractible seulement avec GodMode9.

## Conséquences

- Or, Argent, Cristal (horloge) : l'heure du jeu peut différer d'une machine à l'autre ; la partie, elle, est la même.
- open_agb_firm n'écrit le `.sav` qu'à l'extinction par POWER : synchroniser au démarrage suivant.
