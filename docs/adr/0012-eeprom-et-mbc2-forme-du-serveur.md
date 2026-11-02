# EEPROM GBA et MBC2 GB : une forme unique sur le serveur

Deux types de sauvegarde ont un fichier différent selon l'émulateur, pour les mêmes données (tickets #7 et #8, `docs/research-eeprom-mbc2.md`). Le Hash porte sur la SRAM (ADR 0008) : sans conversion, la 3DS et le PC voient deux versions, donc un Conflit, et un fichier recopié tel quel donne une partie illisible.

- **EEPROM GBA** (512 octets ou 8 Kio) : open_agb_firm écrit les octets dans l'ordre du matériel ; mGBA inverse chaque bloc de 8 octets.
- **MBC2 GB** (512 valeurs de 4 bits) : mGBA écrit 256 octets, deux valeurs par octet (adresse paire dans le quartet bas) ; SameBoy, VBA-M et BGB 512 octets, une valeur par octet ; GameYob (TWiLight Menu++) 8 Kio, les 512 premiers octets utiles.

Règle :

- **EEPROM** : le serveur garde l'ordre de mGBA. La 3DS inverse chaque bloc de 8 octets pour une sauvegarde GBA de 512 octets ou 8 Kio qui n'est pas celle d'un forwarder mGBA (donc open_agb_firm), à l'Envoi comme à la Récupération. Sur GBA, ces deux tailles ne viennent que d'une EEPROM. Le PC ne convertit rien.
- **MBC2** : le serveur garde 512 octets, une valeur par octet, quartet haut à 0.
  - 3DS : jeu MBC2 reconnu par l'octet `0x147` de la ROM (0x05 ou 0x06), car 8 Kio est aussi la taille d'autres jeux GB. À la Récupération : taille du fichier existant ; sinon 256 octets pour un forwarder mGBA et 8 Kio pour TWiLight Menu++.
  - PC : un fichier GB de 256 ou 512 octets ne peut être que MBC2. À la Récupération : taille du fichier existant ; sinon 256 octets chez mGBA (application ou cœur RetroArch) et 512 octets (quartet haut à 0xF, lu par VBA-M) ailleurs.
- **Copies de sécurité** : toujours les octets du fichier, sans conversion.

## Options écartées

- **Garder le fichier de chaque émulateur sur le serveur** : un Hash par émulateur, donc des Conflits à chaque changement de machine.
- **Forme open_agb_firm pour l'EEPROM** : chaque émulateur PC aurait besoin d'une conversion ; mGBA est l'émulateur par défaut du PC et des forwarders.
- **Forme mGBA (256 octets) pour MBC2** : la forme à 512 octets est la plus simple à lire et à écrire pour les trois autres formats.

## Conséquences

- Une EEPROM envoyée par une version antérieure depuis open_agb_firm est sur le serveur dans le mauvais ordre. Avec cette version, la 3DS calcule un autre Hash pour le même fichier : elle propose un Envoi, dans le bon ordre. Ne récupère pas ce jeu sur le PC avant cet Envoi.
- Un fichier MBC2 de 8 Kio envoyé par une version antérieure reste tel quel sur le PC (8 Kio n'y est pas reconnu comme MBC2). La 3DS le convertit à la Récupération.
- Le format EEPROM interne d'autres émulateurs PC (VBA-M, SkyEmu, NanoBoyAdvance) n'est pas vérifié : ordre de mGBA supposé.
