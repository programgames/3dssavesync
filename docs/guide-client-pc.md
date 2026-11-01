# Lanceur PC : `3dssavesync`

Un seul exécutable, sans installation ni dépendance. Sous Windows, il n'ouvre pas de console. Il synchronise les Sauvegardes avant la partie, lance l'émulateur, attend sa fermeture, puis synchronise de nouveau.

## Configuration

Fichier `config.json` dans le dossier de configuration :

- Windows : `%APPDATA%\3dssavesync\config.json`
- Linux : `~/.config/3dssavesync/config.json`
- macOS : `~/Library/Application Support/3dssavesync/config.json`

Un autre emplacement : option `--config <fichier>`. Modèle : [`client/config.example.json`](../client/config.example.json).

| Champ | Rôle |
|---|---|
| `url`, `jeton` | Le Serveur et son jeton. |
| `copies` | Dossier des Copies de sécurité (facultatif ; défaut : `copies` à côté de `config.json`). |
| `emulateurs.<nom>.exe` | L'émulateur. |
| `emulateurs.<nom>.profil` | Facultatif : le Profil (tableau suivant). Défaut : le nom de l'entrée. Sert pour deux installations du même émulateur. |
| `emulateurs.<nom>.sauvegardes` | Facultatif : Système → dossier des Sauvegardes (`DS`, `GB`, `GBA` ; `3DS` : le dossier `sdmc`). Défaut : le réglage de l'émulateur. S'il est présent, l'émulateur ne gère que ces Systèmes. |
| `emulateurs.<nom>.coeurs` | RetroArch seulement : Système → fichier du cœur. |
| `emulateurs.<nom>.roms` | Système → dossier des ROM. Seulement pour la commande `launch`. |
| `emulateurs.<nom>.processus` | Facultatif : nom du processus à surveiller. Défaut : le nom de l'exécutable. Utile pour un Flatpak lancé par un script. |

Chaque entrée de `emulateurs` est un Client distinct : elle a ses propres Bases (`bases/<nom>.json`). Le journal est `3dssavesync.log`, à côté de `config.json`.

## Profils

Sans `sauvegardes`, le Lanceur lit le dossier dans le réglage de l'émulateur. Il cherche le fichier de réglages à côté de l'exécutable (installation portable), puis aux emplacements de Windows, de Linux (dont Flatpak et RetroDECK) et de macOS. Sans réglage, la Sauvegarde est à côté de la ROM.

| Profil | Systèmes | Dossier lu dans | Fichier de Sauvegarde | État rechargé au lancement |
|---|---|---|---|---|
| `melonds` | DS | `melonDS.toml` : `SaveFilePath` | `<ROM>.sav` ; ROM dans un zip : nom du fichier interne | — |
| `mgba` | GB, GBA | `config.ini` : `savegamePath` | `<ROM ou zip>.sav` | `autoload` (actif par défaut) |
| `azahar` | 3DS | `qt-config.ini` : `sdmc_directory`, sinon `user/sdmc` | `sdmc/Nintendo 3DS/…/main` | — |
| `retroarch` | ceux de `coeurs` | `retroarch.cfg` : `savefile_directory` et les options de tri | `<ROM ou zip>.srm` | `savestate_auto_load` |
| `vbam` | GB, GBA | `vbam.ini` : `BatteryDir` | `<ROM ou zip>.sav` | `AutoLoadLastState` |
| `sameboy` | GB | (toujours à côté de la ROM) | `<ROM>.sav` | — |

**Cœurs RetroArch gérés** : melonDS DS, mGBA, Gambatte, SameBoy, VBA-M. Le fichier `.rtc` (horloge des jeux GB) reste sur le PC. Exemple :

```json
"retroarch": {
  "exe": "C:\\RetroArch\\retroarch.exe",
  "coeurs": {
    "DS": "C:\\RetroArch\\cores\\melondsds_libretro.dll",
    "GB": "C:\\RetroArch\\cores\\gambatte_libretro.dll",
    "GBA": "C:\\RetroArch\\cores\\mgba_libretro.dll"
  }
}
```

**État rechargé au lancement** : un émulateur qui recharge un état peut remplacer une Version qui vient du serveur par une partie plus ancienne. Après une Récupération, le Lanceur pose la question « Jouer quand même / Ne pas jouer » et donne le réglage à changer. Sans Récupération, il écrit seulement un avertissement dans le journal.

**Profil personnalisé** : pour un émulateur que le Lanceur ne connaît pas, une entrée avec `args` (modèle d'arguments) et `sauvegardes` :

```json
"monemulateur": {
  "exe": "C:\\Emu\\emu.exe",
  "args": ["--battery-dir={sauvegardes}", "{rom}"],
  "sauvegardes": { "GBA": "C:\\Emu\\saves" },
  "extension": ".sav",
  "nom_interne": false
}
```

`{rom}` est la ROM (retiré par `launch`, qui lance l'émulateur sans jeu) ; `{sauvegardes}` est le dossier du Système. `nom_interne` : `true` si l'émulateur nomme la Sauvegarde d'après le fichier dans le zip, et pas d'après le zip. Le format doit être la Sauvegarde brute (un éventuel Trailer d'horloge GB/GBA est géré).

## Commandes

```
3dssavesync run [--emulator <nom>] <rom>
3dssavesync run [--emulator <nom>] --rom <rom> -- <commande…>
3dssavesync launch <nom>
3dssavesync migrate-v2 <dossier de sync.ps1>
```

- **`run <rom>`** : un jeu. Pour les frontends (LaunchBox, Playnite, ES-DE…). `--emulator` est obligatoire seulement si deux émulateurs gèrent le même Système.
- **`run --rom <rom> -- <commande>`** : synchronise la ROM, puis lance la commande donnée telle quelle. Pour les frontends qui passent par leur propre lanceur (RetroBat) ou un préfixe de commande (Lutris).
- **`launch <nom>`** : sans frontend. Synchronise tous les jeux de l'émulateur (les ROM de ses dossiers `roms`, et les Pokémon 3DS déjà lancés dans Azahar), lance l'émulateur sans jeu, puis synchronise tout à sa fermeture. mGBA ne reçoit son dossier de sauvegardes que si GB et GBA partagent le même dossier.
- **`migrate-v2`** : reprend `sync.config.json` et `base.json` de l'ancien Lanceur PowerShell. Rien n'est remplacé : la commande refuse si la configuration existe déjà. L'installation v2 n'est pas modifiée.

Un frontend qui arrête le programme pendant la partie (signal d'arrêt sous Linux et macOS) n'empêche pas l'envoi : le programme attend la fin de l'émulateur. Un arrêt forcé (`SIGKILL`, « Terminer la tâche ») coupe l'envoi ; il part au lancement suivant.

## LaunchBox

1. **Outils > Gérer les émulateurs** : ajoutez un émulateur `3dssavesync` par émulateur réel.
2. **Chemin de l'application** : `3dssavesync.exe`.
3. **Paramètres de ligne de commande** : `run --emulator melonds` (le nom de l'entrée dans `config.json`).
4. Laissez décochés **Don't use quotes** et **Attempt to hide console window** (l'exécutable n'a pas de console).
5. Décochez **Extract ROM archives before running** pour le DS, le GB et la GBA : le Lanceur lit les `.zip`. Pour Azahar, gardez l'extraction : le Lanceur ne lit pas les `.3ds` dans un `.zip`.
6. **Plateformes associées** : celles de l'émulateur réel.

## Fenêtres

- **Conflit** : trois boutons nommés avant la partie (« Prendre le serveur », « Garder le PC », « Ne pas jouer »), deux après. Fermer la fenêtre = le dernier bouton, le plus prudent.
- **État rechargé au lancement**, après une Récupération : « Jouer quand même », « Ne pas jouer ».
- **Information** (émulateur déjà lancé, envoi impossible) : une notification, rien à valider.
- **Outils** : Windows : fenêtre et notification natives. macOS : `osascript` (fourni avec le système). Linux : `zenity` ou `kdialog` (celui de KDE en premier sous KDE), notifications par `notify-send`.
- **Sans fenêtre possible** (aucun outil, aucun écran, par exemple en SSH) : un Conflit ne remplace rien, la partie utilise la Sauvegarde du PC, et le message part en notification et dans la sortie d'erreur.
- **Langue** : français si le système est en français, sinon anglais (Windows : langue de l'interface ; Linux et macOS : `LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, `LANG`, puis la langue de macOS). Le journal reste en français.
- **Non vérifié sur appareil** : Linux, macOS, Steam Deck en mode Jeu (Gamescope).
