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
| `desmume` | DS | Windows : `desmume.ini` à côté de l'exe, `[PathSettings]` `Battery` (défaut `Battery` à côté de l'exe) ; Linux : `~/.config/desmume` | `<ROM>.dsv` ; ROM dans un zip : nom du fichier interne | — |
| `skyemu` | DS, GB, GBA | à côté de la ROM ; sinon « Save File/State Path » (`search_paths.bin`) si le `.sav` y est déjà ou si « Create new files in paths » est coché | `<ROM ou zip>.sav` | — (état écrit automatiquement, restauré seulement à la main) |
| `nanoboyadvance` | GBA | `config.toml` à côté de l'exe (macOS : `~/Library/Application Support/org.github.fleroviux.NanoBoyAdvance/`) : `[general]` `save_folder`, s'il existe | `<ROM ou zip>.sav` | — |
| `bgb` | GB | (à côté de la ROM) | `<ROM>.sav` | — |

**DeSmuME** : le `.dsv` contient la SRAM puis un pied de page de 122 octets. Le Lanceur retire ce pied avant le Hash et l'Envoi : le serveur reçoit la même SRAM brute que celle de la 3DS. À la Récupération, il fait la Copie de sécurité, puis remet le pied du `.dsv` existant (même taille) ou écrit un pied neuf, comme DeSmuME à l'import d'un `.sav`. Un `.dsv` sans pied valide n'est pas synchronisé. macOS : dossier non vérifié, indique `sauvegardes`.

**SkyEmu, NanoBoyAdvance, BGB** : la Sauvegarde est brute. BGB ajoute le Trailer d'horloge de 48 octets des jeux GB à horloge : le Lanceur le garde sur le PC, comme pour mGBA. Avant d'utiliser NanoBoyAdvance, laisse `args_en_plus` vide : la ROM doit être le premier argument. Si tu remplis « Save files » (onglet Misc) dans BGB, indique le même dossier dans `sauvegardes` : le Lanceur ne lit pas ce réglage (clé de `bgb.ini` non documentée). BGB sous Wine : `exe` = `wine`, `args_en_plus` = le chemin de `bgb.exe`, `processus` à préciser (non testé).

**Cœurs RetroArch gérés** : melonDS DS, mGBA, Gambatte, SameBoy, VBA-M, gpSP, Beetle GBA. Le fichier `.rtc` (horloge des jeux GB) reste sur le PC. Exemple :

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

**gpSP et Beetle GBA** : ces cœurs écrivent un `.srm` de taille fixe (128 Kio pour gpSP, 136 Kio pour Beetle GBA), quel que soit le jeu. Le Lanceur envoie seulement la SRAM, à la taille de la Version du serveur : le Hash est le même qu'avec mGBA ou la 3DS. Il refuse la Synchronisation si le `.srm` a des données hors de cette taille, ou si le serveur n'a pas encore ce jeu : synchronise-le d'abord depuis mGBA ou la 3DS. À la Récupération, il fait la Copie de sécurité, puis écrit un `.srm` complet. Beetle GBA : garde l'option `gba_use_mednafen_save_method` à `libretro` (défaut). Détails : [ADR 0011](adr/0011-srm-gpsp-beetle-taille-du-serveur.md).

**État rechargé au lancement** : un émulateur qui recharge un état peut remplacer une Version qui vient du serveur par une partie plus ancienne. Après une Récupération, le Lanceur pose la question « Jouer quand même / Ne pas jouer » et donne le réglage à changer. Sans Récupération, il écrit seulement un avertissement dans le journal.

**Profil personnalisé** : dans la page de réglages, « Ajouter un émulateur à la main », choix « Autre émulateur (personnalisé) » : nom, exécutable, ligne de commande avec `{rom}` et `{sauvegardes}`, Systèmes, dossier des sauvegardes de chaque Système (obligatoire), extension ; la page affiche la commande obtenue avec une ROM d'exemple. Dans `config.json`, c'est une entrée avec `args` (modèle d'arguments) et `sauvegardes` :

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

**Arguments en plus** (profil intégré) : champ « Arguments en plus » de la page, ou `args_en_plus` dans `config.json` (par exemple `["-f"]` pour le plein écran). Ils passent juste avant la ROM ; les arguments propres au profil (`-C savegamePath=…` de mGBA, `-L <cœur>` de RetroArch) restent gérés par le Lanceur.

**Panda3DS** : non géré. Sa seule version publiée (v0.9) lit et écrit les fichiers de sauvegarde à une position fausse ; le Lanceur pourrait envoyer une Sauvegarde 3DS fausse. Détails : `docs/research-v3-emulateurs.md` §5.4.

**Aucune sauvegarde après la partie** : si un jeu n'a pas de sauvegarde avant la partie et n'en a toujours pas après, une notification donne le fichier attendu. Si vous avez sauvegardé en jeu, le dossier ou l'extension de l'émulateur est faux.

## Commandes

```
3dssavesync run [--emulator <nom>] <rom>
3dssavesync run [--emulator <nom>] --rom <rom> -- <commande…>
3dssavesync launch <nom>
3dssavesync migrate-v2 <dossier de sync.ps1>
3dssavesync setup [--texte] [--sans-navigateur]   (ou sans argument)
3dssavesync repair
3dssavesync serve             (serveur local, voir guide-serveur.md)
3dssavesync restore [--oui] [<clé> [<n°>]]
```

- **`run <rom>`** : un jeu. Pour les frontends (LaunchBox, Playnite, ES-DE…). `--emulator` est obligatoire seulement si deux émulateurs gèrent le même Système.
- **`run --rom <rom> -- <commande>`** : synchronise la ROM, puis lance la commande donnée telle quelle. Pour les frontends qui passent par leur propre lanceur (RetroBat) ou un préfixe de commande (Lutris).
- **`launch <nom>`** : sans frontend. Synchronise tous les jeux de l'émulateur (les ROM de ses dossiers `roms`, et les Pokémon 3DS, même jamais lancés dans Azahar), lance l'émulateur sans jeu, puis synchronise tout à sa fermeture. mGBA ne reçoit son dossier de sauvegardes que si GB et GBA partagent le même dossier.
- **`restore`** : remet une ancienne Version de l'Historique du serveur comme Version courante (ADR 0016). Sans argument : les jeux du serveur et leur clé. Avec une clé : les 30 dernières Versions, numérotées (date, appareil, taille ; temps de jeu et dresseur pour un Pokémon). Avec un numéro : la Restauration de cette Version, après une confirmation (`--oui` : sans question). Rien n'est écrit sur le PC : chaque appareil reçoit la Version à sa prochaine synchronisation, après une Copie de sécurité. Un Envoi arrivé entre la liste et la Restauration est gardé : la Restauration est refusée. Même chose dans « 3dssavesync réglages », partie « Historique ». Windows : la commande ouvre sa propre console, comme `repair`.
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
- **Deux jeux en même temps** (un échange entre Pokémon X et Y, par exemple) : possible si les deux sont lancés par 3dssavesync. Chaque Lanceur pose un verrou (`en-cours\` dans le dossier de configuration). Refus seulement si le même jeu tourne déjà, ou si l'émulateur a été ouvert hors de 3dssavesync (son jeu est alors inconnu). Le même jeu deux fois (X et X) demande une seconde copie de l'émulateur, avec son propre dossier de sauvegardes.
- **Outils** : Windows : fenêtre et notification natives. macOS : `osascript` (fourni avec le système). Linux : `zenity` ou `kdialog` (celui de KDE en premier sous KDE), notifications par `notify-send`.
- **Sans fenêtre possible** (aucun outil, aucun écran, par exemple en SSH) : un Conflit ne remplace rien, la partie utilise la Sauvegarde du PC, et le message part en notification et dans la sortie d'erreur.
- **Langue** : français si le système est en français, sinon anglais (Windows : langue de l'interface ; Linux et macOS : `LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, `LANG`, puis la langue de macOS). Le journal reste en français.
- **Non vérifié sur appareil** : Linux, macOS, Steam Deck en mode Jeu (Gamescope).
