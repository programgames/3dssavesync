# gpSP et Beetle GBA : la taille de la SRAM vient de la Version du serveur

Les cœurs RetroArch gpSP et Beetle GBA écrivent un `.srm` de taille fixe, quel que soit le type de sauvegarde du jeu (ticket #4, `docs/research-v3-emulateurs.md` §4.6) :

- gpSP : 131072 octets (128 Kio). La SRAM est au début ; le reste garde sa valeur d'origine (0xFF, ou 0x00 si le fichier chargé était tout à 0x00).
- Beetle GBA : 139264 octets (0x22000). Flash et SRAM au début, EEPROM à 0x20000. La fonction qui devine la taille (`adjust_save_ram`) existe mais n'est jamais appelée : la taille ne change pas.

Le Hash doit porter sur la SRAM seule, de la même taille qu'avec mGBA ou la 3DS (ADR 0008 et 0010). Le `.srm` ne dit pas cette taille. Le contenu ne suffit pas : une partie de Pokémon Émeraude qui n'a encore écrit qu'un emplacement laisse la seconde banque de la Flash à 0xFF, et ressemble à une Flash de 64 Kio.

Règle :

- **Envoi** : la taille est celle de la Version courante du serveur. Le Lanceur prend ces octets (à 0x20000 pour une EEPROM chez Beetle GBA). Si un seul octet hors de cette zone n'est pas vierge (tout 0xFF ou tout 0x00), il refuse : rien n'est coupé.
- **Sans Version sur le serveur** : taille inconnue, le Lanceur refuse avec un message (« synchronise d'abord ce jeu depuis mGBA ou la 3DS »). Un `.srm` tout vierge compte comme absent, comme toute Sauvegarde vierge.
- **Récupération** : Copie de sécurité du `.srm`, puis un `.srm` de taille fixe, 0xFF partout et la SRAM reçue à sa place.

## Options écartées

- **Deviner la taille par le contenu** (dernier octet non vierge) : faux pour une Flash de 128 Kio à moitié écrite. Faux Conflit (tailles différentes), ou pire, une Version de 64 Kio envoyée pour un jeu de 128 Kio.
- **Lire le type dans la ROM** (chaînes `FLASH1M_V`, `SRAM_V`, `EEPROM_V` du kit Nintendo) : lecture de toute la ROM à chaque Synchronisation, chaînes parfois fausses (gpSP garde une base de corrections), et une EEPROM de 512 octets ou de 8 Kio porte la même chaîne.
- **Envoyer le `.srm` entier** : un Hash différent de mGBA et de la 3DS pour chaque jeu, donc des Conflits partout.

## Conséquences

- Aucune perte possible : le Lanceur ne coupe que des octets vierges, et fait toujours une Copie de sécurité avant d'écrire.
- Un premier Envoi depuis gpSP ou Beetle GBA est impossible : le jeu doit déjà être sur le serveur.
- Beetle GBA avec l'option `gba_use_mednafen_save_method` (pas de `.srm`) : non géré.
