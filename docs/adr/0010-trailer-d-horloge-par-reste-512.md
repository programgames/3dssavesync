# Trailer d'horloge GB/GBA : reconnu par le reste modulo 512

L'ADR 0008 calcule la SRAM par `taille - (taille mod 1024)`. Pour une SRAM de 512 octets (GBA EEPROM 4 Kbit, GB MBC2 chez SameBoy, VBA-M, BGB), la règle donne 0 : le code garde alors le fichier entier, Trailer d'horloge compris. Une EEPROM 4 Kbit avec le Trailer mGBA (528 octets) n'a donc pas le même Hash que la même SRAM sans Trailer (512 octets) sur l'autre machine. Inversement, un fichier de 1536 octets sans Trailer serait coupé à 1024. Ticket #6, `docs/research-v3-emulateurs.md` §8.1.

Toutes les SRAM GB/GBA connues sont des multiples de 512 octets. Les Trailers connus font 16 octets (mGBA GBA), 44 octets (ancien format VBA) et 48 octets (mGBA, VBA-M, BGB, GB MBC3+TIMER). Règle, la même sur la 3DS (`sram_taille`, `3ds/source/cle.c`) et sur le PC (`lireLocal`, `client/sync.go`) :

- `r = taille mod 512` ;
- si `r` vaut 16, 44 ou 48 et que `taille > r`, la SRAM est `taille - r` ;
- sinon la SRAM est le fichier entier.

Le reste de l'ADR 0008 ne change pas : Hash, taille et Envoi portent sur la SRAM seule, et chaque machine garde son propre Trailer.

## Options écartées

- **Garder `mod 1024`, avec un cas à part pour 512** : deux règles au lieu d'une, et le cas 1536 reste faux.
- **Lire la taille de SRAM dans l'en-tête de la ROM** : exact pour le GB, impossible pour le GBA (pas de champ de taille) ; demande la ROM à chaque lecture.

## Conséquences

- Aucune Sauvegarde existante ne change de Hash pour les tailles Pokémon : GB 32 Kio (+ 0, 44 ou 48 octets), GBA 128 Kio (+ 0 ou 16 octets) donnent la même SRAM qu'avant.
- Changent de SRAM : les fichiers de 512 octets + Trailer (avant : fichier entier) et les fichiers ≥ 1 Kio dont le reste n'est pas un Trailer (avant : coupés au Kio). Les jeux Pokémon ne sont pas dans ce cas.
- GB MBC2 et GBA EEPROM restent hors périmètre pour d'autres raisons (tailles différentes selon l'émulateur, ordre des octets d'open_agb_firm) : §8.2 et synthèse de `docs/research-v3-emulateurs.md`.
- `pc/sync.ps1` (Lanceur v2, remplacé par `3dssavesync`) garde l'ancienne règle.
