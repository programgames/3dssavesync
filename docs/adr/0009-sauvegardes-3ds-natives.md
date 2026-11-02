# Sauvegardes 3DS natives : le fichier `main` de l'archive, code maison inspiré de Checkpoint

Pour les 8 Pokémon 3DS (X, Y, Rubis Oméga, Saphir Alpha, Soleil, Lune, Ultra-Soleil, Ultra-Lune), la sauvegarde est une archive (`ARCHIVE_USER_SAVEDATA`) qui contient un seul fichier, `main`. Azahar range la même archive en fichiers ordinaires (`…/title/00040000/<low>/data/00000001/main`). On synchronise ce fichier seul.

- 3DS, lecture : ouvrir l'archive (SD, sinon cartouche), lire `/main`.
- 3DS, écriture : Copie de sécurité, écrire `/main`, `ControlArchive(COMMIT)`, puis supprimer la secure value (comme Checkpoint, JKSM, Cloudpoint), sinon le jeu refuse une sauvegarde restaurée.
- PC : lire le title ID dans l'en-tête NCSD/NCCH de la ROM extraite par LaunchBox, puis le fichier `main` d'Azahar. Un `00000001.metadata` existant n'est jamais touché.
- Table fixe des 8 title ID (un seul par jeu, toutes régions).

## Options écartées

- **Copier le code de Checkpoint** : C++ sous GPL-3 ; il imposerait la GPL-3 au homebrew et un second langage. On reprend sa méthode (même ordre d'opérations), sur le modèle du code C sous MIT de DaeMoon.
- **Toutes les sauvegardes 3DS, multi-fichiers** : demande un format de paquet (zip ou manifeste) côté Serveur. Inutile pour les 8 jeux visés.

## Conséquences

- Un jeu jamais lancé n'a pas d'archive sur la 3DS : on ne crée rien, on demande de lancer le jeu une fois et de sauvegarder.
- Azahar (révisé le 2026-11-02, issue #9) : pour un jeu jamais lancé, le Lanceur crée le dossier, `main` et un `00000001.metadata` de valeur connue (relevée sur Lune et Ultra-Soleil, même valeur supposée pour les 8 Pokémon). Test sur une copie d'Azahar : Pokémon X charge la partie sans reformater. Sans valeur connue, ou si un `.metadata` existe déjà sans dossier, le Lanceur refuse.
- Le `.cia` reprend la liste `FileSystemAccess` de Checkpoint : un `.3dsx` peut ne pas avoir le droit d'ouvrir la sauvegarde d'un autre titre (sources contradictoires).
