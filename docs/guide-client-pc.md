# Lanceur PC : `3dssavesync`

Un seul exécutable, sans dépendance. Sous Windows, il n'ouvre pas de console pendant les parties. Il synchronise les Sauvegardes avant la partie, lance l'émulateur, attend sa fermeture, puis synchronise de nouveau.

## Installation : l'assistant

Double-cliquez sur l'exécutable, ou sur le raccourci « 3dssavesync réglages ». Le programme ouvre une page de réglages dans le navigateur (adresse locale `http://127.0.0.1:<port>/<secret>/`, accessible seulement depuis ce PC). La page présente 4 parties :

1. **Serveur** : serveur personnel (adresse et jeton, bouton « Tester la connexion ») ou serveur local sur ce PC ([guide-serveur.md](guide-serveur.md)) ;
2. **Programme** : copie du programme à un emplacement fixe : `%LOCALAPPDATA%\Programs\3dssavesync` (Windows), `~/.local/bin` (Linux), `~/Applications/3dssavesync` (macOS) ;
3. **Émulateurs** : ceux de la configuration, et ceux trouvés sur le PC (PATH, dossiers usuels, Flatpak, AppImage d'EmuDeck, `/Applications`, émulateurs de LaunchBox), avec leurs dossiers de sauvegardes et de ROM ; on peut aussi en ajouter un à la main ;
4. **Frontends et raccourcis** : LaunchBox (Windows), ES-DE, un raccourci par émulateur (commande `launch`).

Rien n'est écrit avant le bouton **Enregistrer**. Un serveur personnel injoignable bloque l'enregistrement. Chaque fichier modifié (`config.json`, fichiers des frontends) est copié avant (date dans le nom). Le jeton ne s'affiche jamais, sauf l'adresse et le jeton du serveur local, à entrer sur la 3DS. Le programme s'arrête avec le bouton **Terminer**, ou 2 minutes après la fermeture de l'onglet.

Sans navigateur (SSH, Steam Deck en mode Jeu) : `3dssavesync setup --sans-navigateur` affiche l'adresse de la page, et `3dssavesync setup --texte` pose les mêmes questions dans le terminal.

**Réparer** : après une mise à jour ou une réinitialisation d'un frontend (EmuDeck réécrit les fichiers d'ES-DE), ouvrez « 3dssavesync réglages » et cliquez sur « Réparer les frontends ». Ce qui est déjà en place ne change pas. Le frontend doit être fermé. Même chose en ligne de commande : `3dssavesync repair`.

Frontends à configurer à la main (Steam ROM Manager, EmuDeck, RetroDECK, Playnite, RetroBat, Pegasus, Lutris, Attract-Mode) : [guide-frontends.md](guide-frontends.md).

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
| `serveur_local` | Facultatif : `{"port": 8765, "donnees": "…"}` pour `3dssavesync serve` ; `url` vaut alors `http://127.0.0.1:<port>`. Voir [guide-serveur.md](guide-serveur.md). |
| `titres_3ds` | Facultatif : jeux 3DS en plus des 8 Pokémon, `{"<title ID>": "<nom>"}`. Même liste que l'écran « Jeux 3DS » de la 3DS ; seulement des jeux dont la sauvegarde est un seul fichier `main`. |
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
3dssavesync setup [--texte] [--sans-navigateur]   (ou sans argument)
3dssavesync repair
3dssavesync serve             (serveur local, voir guide-serveur.md)
```

- **`run <rom>`** : un jeu. Pour les frontends (LaunchBox, Playnite, ES-DE…). `--emulator` est obligatoire seulement si deux émulateurs gèrent le même Système.
- **`run --rom <rom> -- <commande>`** : synchronise la ROM, puis lance la commande donnée telle quelle. Pour les frontends qui passent par leur propre lanceur (RetroBat) ou un préfixe de commande (Lutris).
- **`launch <nom>`** : sans frontend. Synchronise tous les jeux de l'émulateur (les ROM de ses dossiers `roms`, et les Pokémon 3DS déjà lancés dans Azahar), lance l'émulateur sans jeu, puis synchronise tout à sa fermeture. mGBA ne reçoit son dossier de sauvegardes que si GB et GBA partagent le même dossier.
- **`migrate-v2`** : reprend `sync.config.json` et `base.json` de l'ancien Lanceur PowerShell. Rien n'est remplacé : la commande refuse si la configuration existe déjà. L'installation v2 n'est pas modifiée.

Un frontend qui arrête le programme pendant la partie (signal d'arrêt sous Linux et macOS) n'empêche pas l'envoi : le programme attend la fin de l'émulateur. Un arrêt forcé (`SIGKILL`, « Terminer la tâche ») coupe l'envoi ; il part au lancement suivant.

## LaunchBox

L'assistant ajoute, pour chaque émulateur, un **nouvel** émulateur « <titre> (3dssavesync) » copié de l'émulateur d'origine :

- chemin de l'application = le Lanceur ; paramètres par plateforme = `run --emulator <nom>` (LaunchBox ajoute la ROM à la fin) ;
- « cacher la console » décoché ; extraction des archives décochée pour DS, GB et GBA (le Lanceur lit les `.zip`), gardée pour la 3DS ;
- il devient l'émulateur par défaut de ses plateformes DS, GB, GBA et 3DS.

L'émulateur d'origine reste intact : pour jouer sans synchronisation, choisissez-le dans LaunchBox. Pour revenir en arrière, remettez-le par défaut. LaunchBox et BigBox doivent être fermés (ils réécrivent `Emulators.xml` à la fermeture). Copie avant modification : `Data\Emulators.xml.avant-3dssavesync-<date>`.

Un émulateur déjà modifié à la main pour passer par le Lanceur est reconnu : l'assistant ne crée pas de doublon.

## ES-DE

Dans `ES-DE/custom_systems` (le dossier d'ES-DE ou d'EmuDeck) :

- `es_find_rules.xml` : émulateur `3DSSAVESYNC` = le Lanceur installé ;
- `es_systems.xml` : une commande « 3dssavesync (<nom>) » en tête des systèmes `nds`, `gb`, `gbc`, `gba`, `n3ds` (la première commande est celle par défaut). Un système absent du fichier reçoit un bloc minimal avec seulement cette commande ; pour retrouver les autres émulateurs d'ES-DE, retirez ce bloc.

ES-DE doit être fermé.

## Fenêtres

- **Conflit** : trois boutons nommés avant la partie (« Prendre le serveur », « Garder le PC », « Ne pas jouer »), deux après. Fermer la fenêtre = le dernier bouton, le plus prudent.
- **État rechargé au lancement**, après une Récupération : « Jouer quand même », « Ne pas jouer ».
- **Information** (émulateur déjà lancé, envoi impossible) : une notification, rien à valider.
- **Outils** : Windows : fenêtre et notification natives. macOS : `osascript` (fourni avec le système). Linux : `zenity` ou `kdialog` (celui de KDE en premier sous KDE), notifications par `notify-send`.
- **Sans fenêtre possible** (aucun outil, aucun écran, par exemple en SSH) : un Conflit ne remplace rien, la partie utilise la Sauvegarde du PC, et le message part en notification et dans la sortie d'erreur.
- **Langue** : français si le système est en français, sinon anglais (Windows : langue de l'interface ; Linux et macOS : `LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, `LANG`, puis la langue de macOS). Le journal reste en français.
- **Non vérifié sur appareil** : Linux, macOS, Steam Deck en mode Jeu (Gamescope).
