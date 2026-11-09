# 3dssavesync

**Jouez à la même partie sur votre 3DS et sur votre PC. La sauvegarde vous suit.**

*[Documentation complète en anglais](README.md)*

3dssavesync synchronise les sauvegardes des jeux **DS**, **Game Boy / Game Boy Color**, **GBA** et **3DS** (chaque jeu 3DS qui a une sauvegarde) entre une 3DS, les émulateurs d'un PC et un téléphone Android. Les 8 Pokémon gardent leur fichier `main` brut, Super Mario Maker n'est jamais synchronisé, et chaque appareil peut désactiver un jeu. Le serveur garde chaque version de chaque sauvegarde et n'efface rien. Chaque appareil fait une copie de sécurité avant de remplacer une sauvegarde. Gardez aussi vos propres sauvegardes (Checkpoint ou JKSM sur la 3DS). Si les deux côtés ont changé, vous choisissez.

Une 3DS modifiée (Luma3DS) est nécessaire.

| Application 3DS : liste des jeux | Conflit : choisir la version à garder |
|---|---|
| ![Écran principal de l'application 3DS](docs/images/homebrew-fr-1.png) | ![Écran de conflit de l'application 3DS](docs/images/homebrew-fr-2.png) |

## Installation

1. **PC** : téléchargez le programme de votre système dans la [dernière version](https://github.com/programgames/3dssavesync/releases/latest), lancez-le, puis suivez les 4 étapes de la page qui s'ouvre (serveur local par défaut) : **Enregistrer et continuer** écrit les réglages.
2. **Carte SD** : mettez la carte SD de la 3DS dans le PC, puis cliquez sur **Préparer la carte SD de la 3DS**. La carte reçoit l'adresse, le jeton et `3dssavesync.cia`.
3. **3DS** : dans FBI, ouvrez **SD**, **cias**, **3dssavesync.cia**, **Install and delete CIA**, puis lancez 3dssavesync : il se connecte sans rien taper.


Une question ? Lisez la [FAQ](docs/faq.md) ou demandez dans les [Discussions](https://github.com/programgames/3dssavesync/discussions). Un problème ? Lisez le [dépannage](docs/troubleshooting.md) (en anglais). Les programmes affichent leurs messages en français si le système est en français.

## Licence

MIT.
