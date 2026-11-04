# 3dssavesync

**Jouez à la même partie sur votre 3DS et sur votre PC. La sauvegarde vous suit.**

*[Documentation complète en anglais](README.md)*

3dssavesync synchronise les sauvegardes des jeux **DS**, **Game Boy / Game Boy Color**, **GBA** et **3DS** (par défaut les 8 Pokémon, de X à Ultra-Lune) entre une 3DS et les émulateurs d'un PC. Le serveur garde chaque version de chaque sauvegarde et n'efface rien. Une copie de sécurité est faite avant chaque remplacement. Si les deux côtés ont changé, vous choisissez.

Une 3DS modifiée (Luma3DS) est nécessaire.

| Application 3DS : liste des jeux | Conflit : choisir la version à garder |
|---|---|
| ![Écran principal de l'application 3DS](docs/images/homebrew-fr-1.png) | ![Écran de conflit de l'application 3DS](docs/images/homebrew-fr-2.png) |

## Installation

1. **PC** : téléchargez le programme de votre système dans la [dernière version](https://github.com/programgames/3dssavesync/releases/latest), lancez-le, puis suivez les 4 étapes de la page qui s'ouvre (serveur local par défaut) : **Enregistrer et continuer** écrit les réglages.
2. **Carte SD** : mettez la carte SD de la 3DS dans le PC, puis cliquez sur **Préparer la carte SD de la 3DS**. La carte reçoit l'adresse, le jeton et `3dssavesync.cia`.
3. **3DS** : dans FBI, ouvrez **SD**, **cias**, **3dssavesync.cia**, **Install and delete CIA**, puis lancez 3dssavesync : il se connecte sans rien taper.


Un problème ? Lisez le [dépannage](docs/troubleshooting.md) (en anglais). Les programmes affichent leurs messages en français si le système est en français.

## Licence

MIT.
