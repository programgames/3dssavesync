# Recherche v3 : intégration de `3dssavesync` dans les frontends

> Working notes (French), kept for the record of design decisions.

Date : 2026-11-01. Ce document regarde comment chaque frontend lance un émulateur. Le but : remplacer l'émulateur par `3dssavesync run <rom> [--emulator <profil>]`. L'outil synchronise la sauvegarde, lance le vrai émulateur, attend sa fin, puis envoie la sauvegarde.

Règle de citation : chaque fait renvoie à une source primaire (documentation officielle, code source à un commit fixe, wiki officiel). La mention **NON VÉRIFIÉ** marque un fait sans source primaire. La mention **Constat local** marque un fait observé sur la machine de développement (fichiers de ce dépôt ou installation LaunchBox locale). Un constat local n'est pas une source primaire.

## Commits de référence

| Projet | Dépôt | Commit | Date du commit |
|---|---|---|---|
| ES-DE | gitlab.com/es-de/emulationstation-de | `ef267ae693caeee89ca8ca888a38e9eab9bce90a` | 2026-10-29 |
| EmuDeck | github.com/dragoonDorise/EmuDeck | `dddb30229adbbd54d5ff7283cfd70e11c4322c26` | 2026-10-05 |
| Steam ROM Manager (SRM) | github.com/SteamGridDB/steam-rom-manager | `12e76c49d172c8529eab269b02b31d0d1837f488` | 2026-11-01 |
| RetroDECK | github.com/RetroDECK/RetroDECK | `7786a849179eb2ca94e8ca413259e28adbcb1e39` | 2026-10-03 |
| RetroBat | github.com/RetroBat-Official/retrobat | `54e5e31d459adab8a10f29224af0069d20baa891` | 2026-10-30 |
| batocera-emulationstation | github.com/batocera-linux/batocera-emulationstation | `703116c6aefb61c68f1a5ea1e725ea8bb4d6ba52` | (dernier commit au 2026-11-01) |
| Pegasus Frontend | github.com/mmatyas/pegasus-frontend | `2a13b4f49c5c4a18d308b0cb5e7c99a2e6b8ac9b` | 2026-10-29 |
| Lutris | github.com/lutris/lutris | `89cc4398a6064cd1bbf9bf4bed6820592b566c67` | 2026-10-31 |
| Attract-Mode | github.com/mickelson/attract | `126b4894f7b96778090e90bd83f746d706897bf3` | 2026-09-03 |

Les liens de ce document utilisent ces commits. Les pages de documentation web (LaunchBox, Playnite, Pegasus, Batocera, Go, freedesktop, Apple, Microsoft) n'ont pas de version fixe. Je les ai lues le 2026-11-01.

---

## 1. Faits transverses qui changent la conception de l'outil

1. **Console Windows.** Le lieur Go produit un « GUI binary » avec `-H windowsgui`, et un « console binary » sinon ([cmd/link](https://pkg.go.dev/cmd/link)). Un binaire GUI n'ouvre pas de fenêtre console. Avec ce binaire, aucun frontend n'a besoin d'une option « cacher la console ». Conséquence : la sortie texte de la ligne de commande n'est pas visible dans un terminal. Comment garder les deux usages (un second exécutable console, ou une autre technique) : **NON VÉRIFIÉ**, à décider.
2. **Ordre des arguments.** Le paquet `flag` de Go arrête l'analyse « just before the first non-flag argument » ([flag](https://pkg.go.dev/flag)). Avec `flag` seul, `run <rom> --emulator x` ignore `--emulator`. Plusieurs frontends ajoutent la ROM **à la fin** de la commande (LaunchBox, voir §2). Recommandation : écrire les modèles sous la forme `run --emulator <profil> <rom>`, ou analyser les drapeaux après la ROM.
3. **Mode « passthrough ».** RetroBat et Batocera lancent un lanceur intermédiaire avec des variables dynamiques (`%CONTROLLERSCONFIG%`, `%CORE%`, voir §6). Lutris ajoute un « Command prefix » devant la commande complète de l'émulateur (§8). Pour ces cas, l'outil a besoin d'une forme qui reçoit la commande complète, par exemple `3dssavesync run --rom <rom> -- <commande de l'émulateur…>`. Ce mode n'existe pas dans le contrat actuel. C'est une proposition.
4. **Arrêt forcé.** Lutris tue les processus du jeu quand l'utilisateur arrête le jeu, et tue les processus orphelins quand le processus principal se termine (§8). Steam peut aussi arrêter un raccourci (**NON VÉRIFIÉ** : quel signal, quel délai). L'outil doit traiter SIGTERM (Linux) et la fermeture de console (Windows) : attendre la fin de l'émulateur, puis faire l'envoi. Une perte d'envoi reste possible si le frontend envoie SIGKILL.
5. **Archives.** ES-DE, Playnite, Pegasus, SRM et Attract-Mode passent le chemin du fichier tel quel (détails dans chaque section). LaunchBox peut extraire l'archive et passer le chemin d'un fichier temporaire (§2). L'outil doit donc accepter un `.zip` et un fichier extrait dans un dossier temporaire. Le Lanceur v2 lit déjà les `.zip` lui-même (Constat local : `pc/sync.ps1`, ligne 89).
6. **Fichiers réécrits par des tiers.** LaunchBox réécrit `Emulators.xml` à sa fermeture (constat sur une installation réelle). EmuDeck écrase ses fichiers ES-DE et SRM à chaque réinitialisation (§4). Recommandation : une commande idempotente (par exemple `3dssavesync frontends repair`) qui réapplique la configuration et fait une copie de sauvegarde avant chaque écriture.

---

## 2. LaunchBox / BigBox (Windows)

**Définition de l'émulateur.**
- Interface : Tools > Manage > Emulators, onglet Details ([LaunchBox, problème connu « Hide Console »](https://feedback.launchbox-app.com/p/known-issue-attempt-to-hide-console-window-on-startupshutdown-causing)).
- Champs de l'interface : « Application Path », « Default Command-Line Parameters », puis une ligne par plateforme dans « Associated Platforms » avec ses propres paramètres et la case « Extract ROMs » ([LaunchBox, Setting Up an Emulator Manually](https://feedback.launchbox-app.com/help/articles/3038328-setting-up-an-emulator-manually)).
- Priorité des paramètres : paramètres propres au jeu, puis ligne de la plateforme associée, puis paramètres par défaut. « The first non-empty level wins » (même source).
- Fichier : `<LaunchBox>\Data\Emulators.xml`. BigBox lit les mêmes données. Constat sur une installation réelle :
  - élément `<Emulator>` : `ID`, `Title`, `ApplicationPath`, `CommandLine`, `NoQuotes`, `NoSpace`, `HideConsole`, `FileNameWithoutExtensionAndPath`, `AutoExtract`, `MonitorStartupShutdownWithProcess`, `UsePauseScreen`, `SuspendProcessOnPause`, `AutoHotkeyScript`, `ExitAutoHotkeyScript`, etc. ;
  - élément `<EmulatorPlatform>` : `Emulator` (ID de l'émulateur), `Platform`, `CommandLine`, `Default`, `M3uDiscLoadEnabled`, `AutoExtract`.
- La correspondance exacte entre `NoQuotes` et une case de l'interface : **NON VÉRIFIÉ**.

**Variable de la ROM.** `%romfile%` place le chemin à une position précise. Sans `%romfile%`, « LaunchBox normally places the ROM path at the end ». `%noromfile%` supprime le chemin ([Setting Up an Emulator Manually](https://feedback.launchbox-app.com/help/articles/3038328-setting-up-an-emulator-manually)). LaunchBox met le chemin entre guillemets si `NoQuotes` vaut `false` : **NON VÉRIFIÉ** (le Lanceur v2 fonctionne avec cette valeur, Constat local).

**Attente de la fin.** Aucune page officielle ne dit que LaunchBox attend la fin du processus. Le champ `MonitorStartupShutdownWithProcess` suggère un suivi du processus : **NON VÉRIFIÉ**. Le Lanceur v2 attend lui-même la fin de l'émulateur. Il fonctionne aujourd'hui (Constat local, `README.md`). Comportement si le processus lancé crée un enfant et se termine : **NON VÉRIFIÉ**.

**Risque de l'écran de pause.** Avec `UsePauseScreen` et `SuspendProcessOnPause` à `true`, LaunchBox suspend le processus suivi. Ce processus devient `3dssavesync`, pas l'émulateur. La commande « quitter » de l'écran de pause peut tuer `3dssavesync` avant l'envoi : **NON VÉRIFIÉ**. Testez ce cas avant la sortie de v3.

**Console.** Option « Attempt to hide console window » (`HideConsole`). Le forum officiel des retours signale que cette option casse certains émulateurs (Dolphin, Mesen, Vita3K) ([problème connu](https://feedback.launchbox-app.com/p/known-issue-attempt-to-hide-console-window-on-startupshutdown-causing)). Avec un binaire `-H windowsgui` (§1), mettez `HideConsole` à `false`.

**Archives.** « Extract ROMs » est une option de compatibilité. Les fichiers extraits vont dans « LaunchBox's temporary 7-Zip area ». LaunchBox choisit d'abord `.cue`, `.m3u`, `.gdi`, `.mds`, `eboot.bin`, `default.xbe`, sinon le premier fichier extrait ([Setting Up an Emulator Manually](https://feedback.launchbox-app.com/help/articles/3038328-setting-up-an-emulator-manually)). Un rapport sur le suivi de bogues du développeur cite le dossier `LaunchBox\7-Zip\Temp` ([bitbucket #2257](https://bitbucket.org/jasondavidcarr/launchbox/issues/2257/setting-extract-rom-archives-before)) : chemin exact **NON VÉRIFIÉ** pour la version actuelle. Constat local : `AutoExtract` vaut `true` pour la ligne « Nintendo DS » de MelonDS. Recommandation : mettez `AutoExtract` à `false` sur les lignes enveloppées. L'outil reçoit alors le vrai chemin de la ROM.

**Configuration automatique.** Le script v2 `setup/launchbox.ps1` montre la méthode : copie horodatée de `Emulators.xml`, puis modification de `ApplicationPath`, `HideConsole` et `EmulatorPlatform/CommandLine`. LaunchBox et BigBox doivent être fermés. Pour v3 :
- `ApplicationPath` = chemin de `3dssavesync.exe` ;
- `Emulator/CommandLine` = vide ;
- `EmulatorPlatform/CommandLine` = `run --emulator melonds` (LaunchBox ajoute la ROM à la fin, d'où l'ordre du §1) ;
- `HideConsole` = `false` ; `AutoExtract` = `false`.

Méthode moins invasive : créez un **nouvel** élément `<Emulator>` (nouvel `ID`) au lieu de modifier l'émulateur existant. L'utilisateur garde l'émulateur d'origine. Il suffit de déplacer `Default` vers la nouvelle ligne de plateforme. La validité d'un émulateur ajouté à la main dans le XML (champs obligatoires) : **NON VÉRIFIÉ**. Copiez un élément existant complet pour limiter le risque.

---

## 3. Playnite (Windows)

**Définition.** Un émulateur a des profils. Un profil personnalisé a les champs Executable, Arguments, Working Directory, Supported File Types et un script de démarrage optionnel ([Playnite, Adding new emulators](https://api.playnite.link/docs/manual/features/emulationSupport/addingNewEmulators.html)). Le modèle SDK `CustomEmulatorProfile` a les propriétés `Executable`, `Arguments`, `WorkingDirectory`, `ImageExtensions`, `Platforms`, `StartupScript`, `TrackingMode`, `TrackingPath` ([SDK CustomEmulatorProfile](https://api.playnite.link/docs/api/Playnite.SDK.Models.CustomEmulatorProfile.html)).

**Variables.** Syntaxe `{Nom}`. `{ImagePath}` = chemin complet de la ROM ; aussi `{ImageName}`, `{ImageNameNoExt}`, `{ImageDir}`, `{EmulatorDir}`, `{PlayniteDir}` ([Game variables](https://api.playnite.link/docs/manual/advanced/gameVariables.html)). L'exemple officiel met la variable entre guillemets : `-f "{ImagePath}"` ([Adding new emulators](https://api.playnite.link/docs/manual/features/emulationSupport/addingNewEmulators.html)).

**Attente et processus enfants.** Mode de suivi (même source) :
- Default : Playnite choisit la méthode ;
- Process : le jeu tourne tant que le processus d'origine **ou un de ses enfants** tourne ;
- Original process : seul le processus d'origine compte ;
- Folder : tout processus lancé depuis `Tracking path`.

Avec `3dssavesync` qui reste vivant, « Original process » suffit. « Process » fonctionne aussi.

**Console.** Aucune option documentée. Utilisez le binaire `-H windowsgui` (§1).

**Archives.** La documentation ne décrit pas d'extraction. Elle liste `zip` et `7z` comme types reconnus ([Adding new emulators](https://api.playnite.link/docs/manual/features/emulationSupport/addingNewEmulators.html)). Playnite passe donc le chemin de l'archive : déduction, extraction absente **NON VÉRIFIÉ** dans le code.

**Configuration automatique.** Les données sont dans le dossier `library` sous `%AppData%\Playnite` (version installée) ou dans le dossier du programme (version portable) ([FAQ Playnite](https://api.playnite.link/docs/manual/gettingStarted/helpAndTroubleshooting/faq.html)). Le fichier `library\emulators.db` est cité par un projet tiers, pas par la documentation officielle : **NON VÉRIFIÉ**. Le format de la base (LiteDB) : **NON VÉRIFIÉ**. Ne modifiez pas la base directement. Deux options sûres :
1. Étapes manuelles dans l'interface (recommandé pour v3).
2. Une extension Playnite (script) qui utilise l'API SDK. Effort plus grand. Hors v3.

---

## 4. ES-DE (Windows, Linux, macOS, Steam Deck)

**Fichiers.**
- Dossier des données : `~/ES-DE` sur Linux et macOS, `C:\Users\<user>\ES-DE` sur Windows (installeur), `ES-DE\ES-DE` en version portable ([USERGUIDE.md](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/USERGUIDE.md)). Sur Steam Deck, ES-DE est une application Linux : même chemin `~/ES-DE` (déduction ; **NON VÉRIFIÉ** par une ligne explicite).
- Fichiers personnalisés : `ES-DE/custom_systems/es_systems.xml` et `ES-DE/custom_systems/es_find_rules.xml`. Ils complètent les fichiers fournis. Ne modifiez pas les fichiers fournis ([INSTALL.md](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/INSTALL.md)).
- Pour changer un système, le fichier personnalisé contient seulement les `<system>` à changer ; l'exemple officiel fournit un bloc `<system>` complet (même source). Un bloc personnalisé remplace-t-il tout le bloc fourni ? La doc le laisse entendre sans le dire : **NON VÉRIFIÉ** à la lettre. Copiez le bloc fourni complet.
- La première balise `<command>` est la commande par défaut : « The first row will be the default emulator » (même source). Les autres `<command label="…">` sont des émulateurs alternatifs, choisis par système ou par jeu.

**Variables** (citations de [INSTALL.md](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/INSTALL.md)) :
- `%ROM%` : chemin absolu « with most special characters escaped with a backslash » ;
- `%ROMRAW%` : chemin sans échappement, à mettre entre guillemets ; `%ROMRAWWIN%` : idem avec `\` (pas sur Windows) ;
- `%ESCAPESPECIALS%` : Windows seulement, échappe `&()^=;,` pour `%ROM%` ;
- `%BASENAME%`, `%FILENAME%`, `%GAMEDIR%`, `%STARTDIR%`, `%ESPATH%`, `%EMUDIR%`, `%INJECT%` ;
- `%EMULATOR_<NOM>%` : chemin trouvé par les règles de `es_find_rules.xml` ; c'est « the recommended way » ;
- `%HIDEWINDOW%` : Windows seulement, « hiding console windows when launching scripts » ;
- `%RUNINBACKGROUND%` : ES-DE continue à tourner pendant le jeu (ne l'utilisez pas ici).

Les commandes fournies utilisent `%ROM%` sans guillemets sur les deux systèmes, par exemple `%EMULATOR_MELONDS% -f %ROM%` pour `nds` ([es_systems.xml Linux](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/resources/systems/linux/es_systems.xml), [es_systems.xml Windows](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/resources/systems/windows/es_systems.xml)). Utilisez donc `%ROM%` sans guillemets.

**Règles de recherche.** Types : `winregistrypath`, `winregistryvalue`, `systempath` (variable PATH), `staticpath` (chemin absolu du binaire). `~` et `%ESPATH%` sont acceptés dans `staticpath` ([INSTALL.md](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/INSTALL.md)). Exemple fourni pour MELONDS sous Linux : `systempath` `melonds`, `melonDS`, `net.kuribo64.melonDS`, puis des `staticpath` AppImage et Flatpak (`/var/lib/flatpak/exports/bin/net.kuribo64.melonDS`) ([es_find_rules.xml Linux](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/resources/systems/linux/es_find_rules.xml)).

**Attente de la fin** ([PlatformUtil.cpp](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/es-core/src/utils/PlatformUtil.cpp)) :
- Windows : `CreateProcessW`, puis une boucle `WaitForSingleObject(pi.hProcess, 10)`. ES-DE attend **le processus direct** seulement. Si ce processus crée un enfant et se termine, ES-DE revient. `3dssavesync` reste vivant : pas de problème.
- Unix : ES-DE ajoute ` 2>&1 &` à la commande, l'ouvre avec `popen()`, lit la sortie jusqu'à la fin du tube, puis appelle `pclose()`. ES-DE attend donc que **tous** les processus qui gardent la sortie du tube se terminent. Un enfant de `3dssavesync` qui hérite de la sortie prolonge l'attente.
- Avec `%HIDEWINDOW%`, ES-DE met `SW_HIDE` dans `STARTUPINFO` (même fichier). Avec un binaire `windowsgui`, n'utilisez pas `%HIDEWINDOW%` : `SW_HIDE` peut aussi s'appliquer à la première fenêtre d'un programme GUI (**NON VÉRIFIÉ** pour `3dssavesync`, qui n'a pas de fenêtre).

**Archives.** `FileData::launchGame` remplace `%ROM%` par le chemin échappé du fichier, sans extraction ([FileData.cpp](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/es-app/src/FileData.cpp)). Les systèmes `nds` et `n3ds` acceptent `.zip` et `.7z`. L'outil reçoit le chemin de l'archive.

**Modèle de configuration.**

`custom_systems/es_find_rules.xml` :

```xml
<ruleList>
  <emulator name="3DSSAVESYNC">
    <rule type="systempath">
      <entry>3dssavesync</entry>
    </rule>
    <rule type="staticpath">
      <entry>~/.local/bin/3dssavesync</entry>
      <entry>~/Applications/3dssavesync</entry>
    </rule>
  </emulator>
</ruleList>
```

Sur Windows, mettez le chemin absolu de `3dssavesync.exe` dans `staticpath`.

`custom_systems/es_systems.xml` : copiez le bloc `<system>` `nds` du fichier fourni **de la version installée**, puis mettez cette ligne en premier :

```xml
<command label="melonDS + 3dssavesync">%EMULATOR_3DSSAVESYNC% run --emulator melonds %ROM%</command>
```

Faites de même pour `n3ds` (profil `azahar`) et `gba`/`gb`/`gbc` (profil `mgba`).

**Configuration automatique.** Faisable. Étapes de l'installeur :
1. Arrêtez si ES-DE tourne.
2. Copiez `custom_systems/*.xml` existants avec un suffixe horodaté.
3. Si le fichier personnalisé contient déjà un bloc `nds` (cas EmuDeck, §5), ajoutez la ligne `<command>` en tête de ce bloc. Sinon, copiez le bloc fourni et ajoutez la ligne.
4. Ajoutez l'émulateur `3DSSAVESYNC` dans `es_find_rules.xml` (créez le fichier si absent).

Avertissement : n'utilisez pas `<loadExclusive/>`. Cette balise bloque le fichier fourni ([INSTALL.md](https://gitlab.com/es-de/emulationstation-de/-/blob/ef267ae693caeee89ca8ca888a38e9eab9bce90a/INSTALL.md)). Ne prenez pas le bloc `nds` d'une autre version d'ES-DE : la liste des commandes change d'une version à l'autre.

---

## 5. EmuDeck (Steam Deck : ES-DE + Steam ROM Manager)

**Ce qu'EmuDeck installe.**
- Fichiers ES-DE personnalisés : `configs/emulationstation/custom_systems/es_systems.xml` et `es_find_rules.xml`. Le bloc `nds` d'EmuDeck contient `melonDS (Standalone)` = `%EMULATOR_MELONDS% -f %ROM%` ([es_systems.xml EmuDeck](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/configs/emulationstation/custom_systems/es_systems.xml)). EmuDeck ne redéfinit pas MELONDS dans ses règles : la règle fournie par ES-DE s'applique (Flatpak). EmuDeck redéfinit AZAHAR avec `~/Applications/azahar*.AppImage` ([es_find_rules.xml EmuDeck](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/configs/emulationstation/custom_systems/es_find_rules.xml)).
- Analyseur SRM « Nintendo DS - melonDS (Standalone) » : `executable.path` = `…/Emulation/tools/launchers/melonds.sh`, `executableArgs` = `"${filePath}" -f` ([nintendo_nds_melonds.json](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/configs/steam-rom-manager/userData/parsers/optional/nintendo_nds_melonds.json)).
- Lanceur `melonds.sh` : `flatpak run net.kuribo64.melonDS "${@}"`, puis `cloud_sync_uploadForced` ([melonds.sh](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/tools/launchers/melonds.sh)). EmuDeck a donc **son propre envoi de sauvegardes** (CloudSync) après la fin de l'émulateur. Ce script suppose que `flatpak run` bloque jusqu'à la fin de l'émulateur.

**Écrasements.** Ce point décide de la méthode.
- `update_launchers` copie chaque lanceur de `tools/launchers` depuis le dépôt EmuDeck, à chaque lancement via `launcherInit` ([helperFunctions.sh L1545-1557](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/functions/helperFunctions.sh#L1545-L1557), [launcherInit.sh](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/functions/launcherInit.sh)). **Ne modifiez pas `melonds.sh`** : EmuDeck remet l'original au lancement suivant.
- La réinitialisation d'ES-DE copie `es_systems.xml` et `es_find_rules.xml` d'EmuDeck par `rsync --backup --suffix=.bak` ([emuDeckESDE.sh L127-131, L435-436](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/functions/ToolScripts/emuDeckESDE.sh#L127-L131)). La mise à jour simple utilise `--ignore-existing` (L181-183) et garde nos changements.
- L'initialisation de SRM copie `userConfigurations.json` par `rsync --backup --suffix=.bak` ([emuDeckSRM.sh L70](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/functions/ToolScripts/emuDeckSRM.sh#L70)).
- Le dossier `customParsers` d'EmuDeck ne sert pas pour nous : `addParser` ne lit que les analyseurs du dépôt EmuDeck, par nom de fichier ([helperFunctions.sh L1474](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/functions/helperFunctions.sh#L1474)).

**Conflit CloudSync.** Si CloudSync d'EmuDeck est actif pour le DS, deux outils envoient la même sauvegarde. Dites à l'utilisateur de désactiver CloudSync d'EmuDeck pour ces systèmes. Comment le désactiver par système : **NON VÉRIFIÉ**.

**Méthode recommandée.**
- Côté ES-DE : ajoutez la ligne `<command>` de §4 en tête du bloc `nds` du fichier `custom_systems/es_systems.xml` d'EmuDeck, et l'émulateur `3DSSAVESYNC` dans `es_find_rules.xml`. Après une réinitialisation EmuDeck, `3dssavesync frontends repair` réapplique le changement.
- Côté SRM : ajoutez un **nouvel** analyseur (§12), ne modifiez pas celui d'EmuDeck. Après une réinitialisation EmuDeck, vérifiez que l'analyseur existe encore (l'ancien fichier reste en `.bak`).
- Profil côté outil : `melonds-flatpak` lance `flatpak run net.kuribo64.melonDS -f <rom>`.

---

## 6. RetroBat (Windows, EmulationStation + emulatorLauncher)

**Définition.** Le fichier de base est `emulationstation\.emulationstation\es_systems.cfg` ([wiki RetroBat, structure des dossiers](https://wiki.retrobat.org/get-started/retrobat-folder-structure)). Le dossier utilisateur d'EmulationStation est `home=.\.emulationstation` ([emulatorLauncher.cfg](https://github.com/RetroBat-Official/retrobat/blob/54e5e31d459adab8a10f29224af0069d20baa891/system/templates/emulationstation/emulatorLauncher.cfg)). Le système `nds` a cette commande ([es_systems.cfg L3014-3021](https://github.com/RetroBat-Official/retrobat/blob/54e5e31d459adab8a10f29224af0069d20baa891/system/templates/emulationstation/es_systems.cfg#L3014-L3021)) :

```
"%HOME%\emulatorLauncher.exe" -gameinfo %GAMEINFOXML% %CONTROLLERSCONFIG% -system %SYSTEM% -emulator %EMULATOR% -core %CORE% -rom %ROM%
```

Extensions : `.nds .bin .zip .7z .decomp`.

**Version d'EmulationStation.** RetroBat utilise un portage Windows de batocera-emulationstation. Indice : le modèle `batocera-store.cfg` dans le même dossier ([templates](https://github.com/RetroBat-Official/retrobat/tree/54e5e31d459adab8a10f29224af0069d20baa891/system/templates/emulationstation)). La branche exacte compilée pour RetroBat : **NON VÉRIFIÉ**. Les faits suivants viennent du code de batocera-emulationstation.

**Surcouche.** EmulationStation lit chaque fichier `es_systems_*.cfg` du dossier utilisateur ([SystemData.cpp L807-870](https://github.com/batocera-linux/batocera-emulationstation/blob/703116c6aefb61c68f1a5ea1e725ea8bb4d6ba52/es-app/src/SystemData.cpp#L807-L870)). Pour un `<name>` existant, il remplace **seulement les balises présentes** dans la surcouche. Un fichier avec `<name>nds</name>` et `<command>` change donc seulement la commande.

**Attente de la fin.** `ProcessStartInfo::run()` utilise `ShellExecuteEx`, puis `WaitForSingleObject(lpExecInfo.hProcess, INFINITE)` ([Platform.cpp L100-129](https://github.com/batocera-linux/batocera-emulationstation/blob/703116c6aefb61c68f1a5ea1e725ea8bb4d6ba52/es-core/src/utils/Platform.cpp#L100-L129)). Attente du **processus direct**. `3dssavesync` reste vivant : pas de problème.

**Archives.** emulatorLauncher contient du code d'extraction (`EmulatorLauncher.Common/Compression`) ([emulatorlauncher](https://github.com/RetroBat-Official/emulatorlauncher/tree/ad62ac2f0577c7a797f9e621e4a78cb598dc464d/EmulatorLauncher.Common/Compression)). Si `3dssavesync` s'intercale avant emulatorLauncher, il reçoit le chemin de l'archive.

**Console.** EmulationStation passe `SW_SHOW` ou `SW_HIDE` selon un réglage (`nShow`, même fichier Platform.cpp). Utilisez le binaire `windowsgui`.

**Modèle** (exige le mode passthrough du §1) : fichier `emulationstation\.emulationstation\es_systems_3dssavesync.cfg` :

```xml
<?xml version="1.0"?>
<systemList>
  <system>
    <name>nds</name>
    <command>"C:\Program Files\3dssavesync\3dssavesync.exe" run --rom %ROM% -- "%HOME%\emulatorLauncher.exe" -gameinfo %GAMEINFOXML% %CONTROLLERSCONFIG% -system %SYSTEM% -emulator %EMULATOR% -core %CORE% -rom %ROM%</command>
  </system>
</systemList>
```

**Autre méthode : événements.** EmulationStation déclenche `game-start` avec le chemin de la ROM avant le lancement, et `game-end` **sans** argument après ([FileData.cpp L726, L765](https://github.com/batocera-linux/batocera-emulationstation/blob/703116c6aefb61c68f1a5ea1e725ea8bb4d6ba52/es-app/src/FileData.cpp#L726)). Un dossier `.emulationstation/scripts/` existe dans emulatorlauncher ([scripts](https://github.com/RetroBat-Official/emulatorlauncher/tree/ad62ac2f0577c7a797f9e621e4a78cb598dc464d/.emulationstation/scripts)). L'outil peut faire « télécharger » sur `game-start` et « envoyer » sur `game-end` sans envelopper. L'outil doit alors retenir le dernier jeu. Le caractère bloquant des scripts sur Windows : **NON VÉRIFIÉ**.

**Configuration automatique.** Faisable avec un **nouveau** fichier `es_systems_3dssavesync.cfg`. Le fichier de base reste intact. Retour arrière : supprimez le fichier. La mise à jour de RetroBat garde-t-elle ce fichier ? **NON VÉRIFIÉ**. Recommandation : v3 en étapes manuelles documentées, auto-configuration plus tard après un test réel.

---

## 7. Pegasus Frontend

**Définition.** Fichier `metadata.pegasus.txt` (ou `metadata.txt`, ou `*.metadata.pegasus.txt`) dans un dossier de jeux déclaré, ou dans `<config dir>/metafiles` ([Pegasus, meta files](https://pegasus-frontend.org/docs/user-guide/meta-files/)). Le champ `launch` d'une collection donne la commande commune. Le `launch` (ou `command`) d'un jeu le remplace. `workdir` (alias `cwd`) donne le dossier de travail (même source). Dossier de configuration : `C:\Users\<user>\AppData\Local\pegasus-frontend\`, `~/.config/pegasus-frontend/`, `~/Library/Preferences/pegasus-frontend/` ([config dirs](https://pegasus-frontend.org/docs/user-guide/config-dirs/)).

**Variables.** `{file.path}`, `{file.uri}`, `{file.name}`, `{file.basename}`, `{file.dir}`, `{env.NOM}`. Pegasus remplace les variables « as-is, without additional formatting ». Mettez-les entre guillemets ([meta files](https://pegasus-frontend.org/docs/user-guide/meta-files/)).

**Attente.** Pegasus découpe la commande lui-même (`utils::tokenize_command`), puis lance `QProcess::start` sans shell, et attend le signal `finished` ([ProcessLauncher.cpp L177, L234-246](https://github.com/mmatyas/pegasus-frontend/blob/2a13b4f49c5c4a18d308b0cb5e7c99a2e6b8ac9b/src/backend/ProcessLauncher.cpp#L177)). Attente du **processus direct**.

**Console.** Aucune option documentée. Utilisez le binaire `windowsgui`.

**Archives.** La documentation ne décrit pas d'extraction ; `{file.path}` est le chemin du fichier.

**Modèle :**

```
launch: "C:/Program Files/3dssavesync/3dssavesync.exe" run --emulator melonds "{file.path}"
```

**Configuration automatique.** Peu sûre. Le champ `launch` est dans les fichiers de l'utilisateur, souvent un par collection. Un nouveau fichier avec une collection séparée crée des doublons (déduction ; **NON VÉRIFIÉ**). Recommandation : étapes manuelles.

---

## 8. Lutris (Linux)

**Définition.** Le runner `linux` a les options `exe`, `args`, `working_dir` ([linux.py](https://github.com/lutris/lutris/blob/89cc4398a6064cd1bbf9bf4bed6820592b566c67/lutris/runners/linux.py)). Lutris a aussi des runners d'émulateurs, par exemple `share/lutris/json/melonds.json`, `mgba.json`, et `lutris/runners/azahar.py` ([arbre Lutris](https://github.com/lutris/lutris/tree/89cc4398a6064cd1bbf9bf4bed6820592b566c67/share/lutris/json)). L'option système `prefix_command` (« Command prefix ») ajoute une commande devant la commande du jeu ([sysoptions.py L475-480](https://github.com/lutris/lutris/blob/89cc4398a6064cd1bbf9bf4bed6820592b566c67/lutris/sysoptions.py#L475-L480)).

**Fichiers.** `CONFIG_DIR` = `~/.config/lutris` s'il existe, sinon `~/.local/share/lutris`. Jeux : `CONFIG_DIR/games` ; runners : `CONFIG_DIR/runners` ([settings.py L22-35](https://github.com/lutris/lutris/blob/89cc4398a6064cd1bbf9bf4bed6820592b566c67/lutris/settings.py#L22-L35)). La liste des jeux vit aussi dans une base `pga.db` : **NON VÉRIFIÉ** dans ce commit. Lutris installe un jeu depuis un fichier YAML avec `-i` / `--install` ([application.py L151-157](https://github.com/lutris/lutris/blob/89cc4398a6064cd1bbf9bf4bed6820592b566c67/lutris/gui/application.py#L151-L157)).

**Variable de la ROM.** Le runner `linux` n'a pas de variable : le chemin de la ROM va en dur dans `args`. Les runners d'émulateurs prennent un fichier de jeu (détail du champ : **NON VÉRIFIÉ**).

**Attente et enfants.** Lutris surveille l'arbre des processus du jeu toutes les 2 s (`beat`). Si le processus principal se termine et que des processus restent, Lutris les tue : « orphan processes remain […] cleaning up » ([game.py L984-1030](https://github.com/lutris/lutris/blob/89cc4398a6064cd1bbf9bf4bed6820592b566c67/lutris/game.py#L984-L1030)). `3dssavesync` doit donc rester vivant jusqu'à la fin de l'envoi. Le bouton « Stop » appelle `force_stop` : risque de perte d'envoi (§1, point 4).

**Modèles.**
- Jeu `linux` : `exe` = `~/.local/bin/3dssavesync`, `args` = `run --emulator melonds "/chemin/jeu.nds"`.
- Runner d'émulateur + préfixe : `prefix_command` = `3dssavesync run --rom "/chemin/jeu.nds" --`. Ce cas exige le mode passthrough du §1, et la ROM se répète par jeu. Peu pratique.

**Configuration automatique.** Possible par jeu avec `lutris -i` et un YAML généré. Lourd. Recommandation : étapes manuelles pour v3.

---

## 9. Steam ROM Manager (raccourcis non-Steam)

**Définition.** Un analyseur (parser) a « Executable », « Command line arguments », « Executable modifier » (défaut `"${exePath}"`), « Start in directory » ([executable-location.md](https://github.com/SteamGridDB/steam-rom-manager/blob/12e76c49d172c8529eab269b02b31d0d1837f488/src/lang/en-US/markdown/executable-location.md), [executable-arguments.md](https://github.com/SteamGridDB/steam-rom-manager/blob/12e76c49d172c8529eab269b02b31d0d1837f488/src/lang/en-US/markdown/executable-arguments.md), [executable-modifier.md](https://github.com/SteamGridDB/steam-rom-manager/blob/12e76c49d172c8529eab269b02b31d0d1837f488/src/lang/en-US/markdown/executable-modifier.md)). L'Executable modifier change l'APP ID Steam (même source).

**Variables.** `${filePath}` (chemin complet du fichier trouvé), `${fileDir}`, `${fileName}`, `${fileExt}`, `${exePath}`, `${exeDir}`, `${title}`, `${fuzzyTitle}`, etc. ([parser-variables.md](https://github.com/SteamGridDB/steam-rom-manager/blob/12e76c49d172c8529eab269b02b31d0d1837f488/src/lang/en-US/markdown/parser-variables.md)). Les exemples officiels mettent `"${filePath}"` entre guillemets.

**Fichiers.** `userData` dans le dossier de données de l'application : `%APPDATA%\steam-rom-manager\userData`, `~/Library/Application Support/steam-rom-manager/userData`, `$XDG_CONFIG_HOME/steam-rom-manager/userData` (défaut `~/.config/…`). Version portable : `userData` à côté de l'exécutable. `SRM_USER_DATA_DIR` remplace les deux ([paths.ts](https://github.com/SteamGridDB/steam-rom-manager/blob/12e76c49d172c8529eab269b02b31d0d1837f488/src/shared/paths.ts)). Les analyseurs sont dans `userData/userConfigurations.json` ([emuDeckSRM.sh L8](https://github.com/dragoonDorise/EmuDeck/blob/dddb30229adbbd54d5ff7283cfd70e11c4322c26/functions/ToolScripts/emuDeckSRM.sh#L8)).

**Ligne de commande.** SRM a une interface en ligne de commande : `create`, `edit`, `view`, `delete` pour les analyseurs ; `srm create --ptype <type> --help` liste les réglages. Un exécutable autonome `srm-cli-<version>-<os>-<arch>` existe ([README.md L87-93](https://github.com/SteamGridDB/steam-rom-manager/blob/12e76c49d172c8529eab269b02b31d0d1837f488/README.md#L87-L93)).

**Attente.** C'est Steam qui lance le raccourci, pas SRM. Le comportement de Steam (suivi du processus, enfant qui se détache, signal à l'arrêt) : **NON VÉRIFIÉ**. Indice : le lanceur EmuDeck enchaîne `flatpak run` puis un envoi, il suppose donc un appel bloquant (§5).

**Archives.** SRM met dans `${filePath}` le fichier trouvé par le motif glob ; il n'extrait rien (déduction de la doc ; extraction absente **NON VÉRIFIÉ** dans le code).

**Modèle** (analyseur Glob) :
- Executable : `/home/deck/.local/bin/3dssavesync` (Windows : `C:\Program Files\3dssavesync\3dssavesync.exe`) ;
- Command line arguments : `run --emulator melonds "${filePath}"` ;
- ROMs directory : dossier des ROM DS ; motif : `**/${title}@(.nds|.NDS|.zip|.ZIP)`.

**Configuration automatique.** Faisable avec la commande `srm create` (pas d'écriture directe du JSON). Fermez SRM avant. Copiez `userConfigurations.json` avant. Steam doit-il être fermé pendant l'écriture des raccourcis : **NON VÉRIFIÉ**. Gardez l'action finale « ajouter à Steam » manuelle dans l'interface SRM.

---

## 10. RetroDECK (Steam Deck, Flatpak)

**Bac à sable.** RetroDECK est un Flatpak. Ses permissions incluent `--filesystem=host`, `--share=network`, `--socket=wayland`, `--socket=x11`, mais **pas** `--talk-name=org.freedesktop.Flatpak` ([net.retrodeck.retrodeck.yml](https://github.com/RetroDECK/RetroDECK/blob/7786a849179eb2ca94e8ca413259e28adbcb1e39/net.retrodeck.retrodeck.yml)). Conséquence : depuis le bac à sable, RetroDECK ne peut pas lancer un programme de l'hôte par `flatpak-spawn --host` (déduction des permissions). Un binaire statique dans `~` est visible dans le bac à sable grâce à `--filesystem=host`. Il s'exécuterait **dans** le bac à sable (**NON VÉRIFIÉ** en test).

**ES-DE interne.** La configuration ES-DE est sous `$XDG_CONFIG_HOME/ES-DE` dans le bac à sable ([all_vars.sh L13](https://github.com/RetroDECK/RetroDECK/blob/7786a849179eb2ca94e8ca413259e28adbcb1e39/functions/all_vars.sh#L13)). Le modèle de RetroDECK dit : « Custom systems typically do not work by default in RetroDECK » ([es_find_rules_template.xml](https://github.com/RetroDECK/RetroDECK/blob/7786a849179eb2ca94e8ca413259e28adbcb1e39/config/retrodeck/helper_files/es_find_rules_template.xml)). Ne visez pas cette voie en v3.

**Lancement depuis l'extérieur.** RetroDECK accepte `flatpak run net.retrodeck.retrodeck [-e emulator] [-s system] [-m] game` ([run_game.sh L9](https://github.com/RetroDECK/RetroDECK/blob/7786a849179eb2ca94e8ca413259e28adbcb1e39/functions/run_game.sh#L9)). La fonction lit la commande dans `es_systems.xml` et l'exécute au premier plan par `eval` (L164). Le profil `retrodeck` de l'outil peut donc lancer `flatpak run net.retrodeck.retrodeck -s nds <rom>`, depuis SRM (§9) ou un raccourci `.desktop` (§13). `flatpak run` reste-t-il bloqué jusqu'à la fin du jeu ? Probable (même hypothèse qu'EmuDeck, §5) mais **NON VÉRIFIÉ**.

**Sauvegardes.** RetroDECK a aussi un module `functions/cloud_sync.sh` ([arbre RetroDECK](https://github.com/RetroDECK/RetroDECK/tree/7786a849179eb2ca94e8ca413259e28adbcb1e39/functions)). Son contenu et un éventuel conflit : **NON VÉRIFIÉ**. Les emplacements de sauvegarde dans `~/retrodeck/saves` : **NON VÉRIFIÉ**.

**Configuration automatique.** Non pour l'ES-DE interne. Oui par SRM ou `.desktop` avec le profil `retrodeck`.

---

## 11. Batocera (distribution Linux, pour mémoire)

- Surcouche : `/userdata/system/configs/emulationstation/es_systems_<nom>.cfg` ; seules les valeurs présentes changent ; la doc conseille de garder la commande par défaut `emulatorlauncher.py %CONTROLLERSCONFIG% -system %SYSTEM% -rom %ROM%` ([wiki Batocera, customize systems](https://wiki.batocera.org/emulationstation:customize_systems)).
- Scripts d'événements : `/userdata/system/scripts/`, arguments `gameStart`/`gameStop`, système, émulateur, cœur, chemin de la ROM ; le script doit être exécutable (`chmod +x`) ([wiki Batocera, launch a script](https://wiki.batocera.org/launch_a_script)). Le caractère bloquant : **NON VÉRIFIÉ**.
- Pertinence : la voie « scripts » permet une synchro sans envelopper. Batocera n'est pas une cible de v3.

---

## 12. Attract-Mode

**Définition.** Un fichier par émulateur, `emulators/<nom>.cfg`, dans `$HOME/.attract` (Linux, macOS) ou le dossier courant (Windows). `--config` change ce dossier ([Readme.md L22-28](https://github.com/mickelson/attract/blob/126b4894f7b96778090e90bd83f746d706897bf3/Readme.md#L22-L28)). Clés : `name`, `executable`, `args`, `workdir`, `rompath`, `romext`, `system`, `info_source`, `import_extras`, `nb_mode_wait`, `exit_hotkey`, `pause_hotkey` ([fe_info.cpp L856-870](https://github.com/mickelson/attract/blob/126b4894f7b96778090e90bd83f746d706897bf3/src/fe_info.cpp#L856-L870)).

**Variables.** `[romfilename]` (chemin complet), `[name]`, `[rompath]`, `[romext]`, `[emulator]`, `[workdir]`, `[system]`, `[systemn]`, `[nothing]` ([fe_settings.cpp L2303-2316](https://github.com/mickelson/attract/blob/126b4894f7b96778090e90bd83f746d706897bf3/src/fe_settings.cpp#L2303-L2316)). Aucun guillemet automatique : mettez `"[romfilename]"` entre guillemets (déduction du code de substitution).

**Attente.** `run_program` bloque si `nb_mode_wait <= 0` ([fe_window.cpp L519-526](https://github.com/mickelson/attract/blob/126b4894f7b96778090e90bd83f746d706897bf3/src/fe_window.cpp#L519-L526)). Sur Unix, attente par `waitpid` sur le pid direct ([fe_util.cpp L962-1011](https://github.com/mickelson/attract/blob/126b4894f7b96778090e90bd83f746d706897bf3/src/fe_util.cpp#L962-L1011)). Laissez `nb_mode_wait` à 0.

**Console.** Sur Windows, `CreateProcessW` reçoit `CREATE_NO_WINDOW` ([fe_util.cpp L1116-1121](https://github.com/mickelson/attract/blob/126b4894f7b96778090e90bd83f746d706897bf3/src/fe_util.cpp#L1116-L1121)).

**Modèle** (`emulators/melonDS-3dssavesync.cfg`) :

```
executable           /home/user/.local/bin/3dssavesync
args                 run --emulator melonds "[romfilename]"
rompath              /chemin/roms/nds
romext               .nds;.zip
```

**Configuration automatique.** Un **nouveau** fichier `.cfg` ne touche rien d'existant. L'utilisateur doit ensuite relier une liste de ROM et un affichage à cet émulateur. Recommandation : fichier généré + étapes manuelles.

Note : le projet actif s'appelle aussi « Attract-Mode Plus » (fork). Je n'ai pas vérifié ses différences : **NON VÉRIFIÉ**.

---

## 13. Raccourcis simples (Windows, Linux, macOS)

**Windows `.lnk`.** L'objet `WScript.Shell` crée un raccourci avec `TargetPath`, `Arguments`, `WorkingDirectory`, `WindowStyle`, `IconLocation` ([Microsoft Learn, Create a desktop shortcut with WSH](https://learn.microsoft.com/en-us/troubleshoot/windows-client/admin-development/create-desktop-shortcut-with-wsh)). Modèle : `TargetPath` = `C:\Program Files\3dssavesync\3dssavesync.exe`, `Arguments` = `run --emulator melonds "D:\ROMs\jeu.nds"`. Explorer n'attend pas la fin : sans objet, l'outil bloque lui-même. Console : binaire `windowsgui`.

**Linux `.desktop`.** Clé `Exec`. Un argument avec un caractère réservé (espace, tabulation, guillemet, apostrophe…) va entre guillemets doubles. Certains caractères dans les guillemets prennent une barre oblique inverse en plus (liste exacte : voir la spécification ; je ne l'ai pas recopiée). `%f` = un fichier, `%%` = un `%` littéral ([freedesktop, Exec variables](https://specifications.freedesktop.org/desktop-entry/latest/exec-variables.html)). Modèle :

```
[Desktop Entry]
Type=Application
Name=Jeu (3dssavesync)
Exec=/home/deck/.local/bin/3dssavesync run --emulator melonds "/run/media/deck/SD/roms/nds/Jeu.nds"
```

Une entrée générique avec `%f` permet « Ouvrir avec 3dssavesync ». Un fichier `.desktop` peut aussi entrer dans Steam comme jeu non-Steam (**NON VÉRIFIÉ**).

**macOS.** Un paquet `.app` contient `Contents/Info.plist`, `Contents/MacOS/` (exécutable) et `Contents/Resources/` ; la clé `CFBundleExecutable` nomme l'exécutable ([Apple, Bundle Programming Guide](https://developer.apple.com/library/archive/documentation/CoreFoundation/Conceptual/CFBundles/BundleTypes/BundleTypes.html)). Un `.app` minimal avec un script `exec 3dssavesync run …` dans `Contents/MacOS` donne un lanceur sans terminal. Un fichier `.command` s'ouvre dans Terminal : **NON VÉRIFIÉ**. Automator « Run Shell Script » enregistré en application : **NON VÉRIFIÉ** (la page Apple lue ne le décrit pas). Gatekeeper et signature d'un `.app` généré : **NON VÉRIFIÉ**.

**Configuration automatique.** Oui, et sans risque : l'outil crée de **nouveaux** fichiers (par exemple `3dssavesync shortcut <rom>`). Aucun fichier existant ne change.

---

## 14. Steam Deck, mode Jeu (gamescope) : dialogue et notification

Question : un processus enfant peut-il afficher `zenity`, `kdialog` ou `notify-send` au-dessus du jeu ?

- Le README de gamescope ne décrit pas l'affichage des fenêtres d'un processus enfant, ni un service de notifications ([gamescope README](https://github.com/ValveSoftware/gamescope/blob/309eee74f967d458cf71836619132010f88ee796/README.md)). Il cite seulement des notifications (« when notifications are up ») dans le contexte du rendu direct par DRM/KMS.
- Indice, pas preuve : RetroDECK, qui vise le Steam Deck, affiche des fenêtres `zenity` dans son chemin de lancement de jeu ([run_game.sh L73, L200](https://github.com/RetroDECK/RetroDECK/blob/7786a849179eb2ca94e8ca413259e28adbcb1e39/functions/run_game.sh#L73)). Son enveloppe `rd_zenity` filtre des erreurs GTK « on Steam Deck » ([zenity_processing.sh L3-9](https://github.com/RetroDECK/RetroDECK/blob/7786a849179eb2ca94e8ca413259e28adbcb1e39/functions/zenity_processing.sh#L3-L9)).
- Ordre d'affichage d'une fenêtre `zenity` par rapport à l'émulateur dans gamescope : **NON VÉRIFIÉ**.
- `notify-send` en mode Jeu (présence d'un service `org.freedesktop.Notifications`) : **NON VÉRIFIÉ**.
- `kdialog` en mode Jeu : **NON VÉRIFIÉ**.

Recommandation : en v3, ne dépendez d'aucune fenêtre en mode Jeu. Montrez un dialogue `zenity` **avant** le lancement de l'émulateur seulement (cas de conflit), avec un délai et un choix par défaut sûr. Écrivez toujours le résultat dans le journal. Testez sur un vrai Steam Deck avant d'ajouter plus.

---

## 15. Tableau récapitulatif

| Frontend | Emplacement de la configuration | Modèle de commande pour `3dssavesync` | Attend la fin ? | Auto-config faisable ? Comment |
|---|---|---|---|---|
| LaunchBox / BigBox | `<LaunchBox>\Data\Emulators.xml` (`Emulator`, `EmulatorPlatform`) | `ApplicationPath`=`3dssavesync.exe` ; `EmulatorPlatform/CommandLine`=`run --emulator melonds` (ROM ajoutée à la fin) | NON VÉRIFIÉ (v2 fonctionne : constat local) | **Oui.** LaunchBox fermé, copie horodatée, nouvel `<Emulator>` ou modification comme v2 ; `HideConsole`=false, `AutoExtract`=false |
| Playnite | `%AppData%\Playnite\library` (base) ; interface Emulators | Executable=`3dssavesync.exe` ; Arguments=`run --emulator melonds "{ImagePath}"` ; suivi « Original process » | Oui (modes de suivi documentés) | **Non** (base non documentée). Étapes manuelles ; extension SDK plus tard |
| ES-DE | `~/ES-DE/custom_systems/es_systems.xml` + `es_find_rules.xml` | `<command label="melonDS + 3dssavesync">%EMULATOR_3DSSAVESYNC% run --emulator melonds %ROM%</command>` en premier | Oui (Windows : processus direct ; Unix : fin du tube) | **Oui.** ES-DE fermé, copie, ajout du bloc système complet de la version installée + règle `3DSSAVESYNC` |
| EmuDeck (ES-DE) | `~/ES-DE/custom_systems/*.xml` (fichiers EmuDeck) | Comme ES-DE, ligne ajoutée au bloc `nds` d'EmuDeck | Oui | **Oui, fragile.** Réinitialisation EmuDeck = écrasement (`.bak`) → `frontends repair` ; ne pas toucher `tools/launchers/*.sh` ; couper CloudSync EmuDeck |
| EmuDeck (SRM) | `~/.config/steam-rom-manager/userData/userConfigurations.json` | Nouvel analyseur : exe=`~/.local/bin/3dssavesync`, args=`run --emulator melonds-flatpak "${filePath}"` | NON VÉRIFIÉ (Steam) | **Oui** par `srm create` ; vérifier après réinitialisation EmuDeck |
| RetroDECK | Bac à sable Flatpak, `$XDG_CONFIG_HOME/ES-DE` | Depuis l'hôte : profil `retrodeck` → `flatpak run net.retrodeck.retrodeck -s nds <rom>` | Probable, NON VÉRIFIÉ | **Non** pour l'ES-DE interne. Oui par SRM ou `.desktop` |
| RetroBat | `RetroBat\emulationstation\.emulationstation\es_systems_3dssavesync.cfg` (surcouche) | `"…\3dssavesync.exe" run --rom %ROM% -- "%HOME%\emulatorLauncher.exe" … -rom %ROM%` (mode passthrough) | Oui (processus direct, code Batocera-ES) | **Possible** (nouveau fichier). v3 : manuel, car le fork exact et la tenue aux mises à jour sont NON VÉRIFIÉ |
| Pegasus | `metadata.pegasus.txt` (dossier des jeux ou `<config>/metafiles`) | `launch: "…/3dssavesync" run --emulator melonds "{file.path}"` | Oui (processus direct, `QProcess`) | **Non.** Étapes manuelles |
| Lutris | `~/.local/share/lutris` (ou `~/.config/lutris`) `games/*.yml`, `runners/*.yml` | Runner `linux` : exe=`3dssavesync`, args=`run --emulator melonds "/chemin/jeu.nds"` | Oui (arbre de processus ; tue les orphelins) | **Possible** par `lutris -i <yaml>` par jeu ; v3 : manuel |
| Steam ROM Manager | `<appdata>/steam-rom-manager/userData/userConfigurations.json` | exe=`3dssavesync`, args=`run --emulator <profil> "${filePath}"` | NON VÉRIFIÉ (Steam) | **Oui** par `srm create` ; ajout à Steam manuel |
| Batocera | `/userdata/system/configs/emulationstation/es_systems_*.cfg` ; `/userdata/system/scripts/` | Scripts `gameStart`/`gameStop` | NON VÉRIFIÉ | Hors v3 |
| Attract-Mode | `$HOME/.attract/emulators/<nom>.cfg` (Windows : dossier courant) | `executable 3dssavesync` ; `args run --emulator melonds "[romfilename]"` | Oui si `nb_mode_wait`=0 | **Partiel** : nouveau `.cfg`, liaison manuelle |
| Raccourci `.lnk` | Fichier créé | `TargetPath`=`3dssavesync.exe`, `Arguments`=`run --emulator melonds "<rom>"` | Sans objet | **Oui** (nouveau fichier) |
| Raccourci `.desktop` | `~/.local/share/applications/` ou bureau | `Exec=… run --emulator melonds "<rom>"` | Sans objet | **Oui** (nouveau fichier) |
| macOS `.app` | Paquet créé | `Contents/MacOS/<script>` → `exec 3dssavesync run …` | Sans objet | **Oui** (nouveau paquet ; signature NON VÉRIFIÉ) |

---

## 16. Liste de support recommandée pour v3

**Niveau 1 : configuration automatique par l'installeur** (copie de sauvegarde, frontend fermé, commande `repair` idempotente) :
1. LaunchBox / BigBox (Windows). Déjà fait en v2 ; adapter au binaire Go.
2. ES-DE autonome (Windows, Linux, macOS, Steam Deck).
3. Steam ROM Manager, par `srm create` (Steam Deck et bureau).
4. Raccourcis générés : `.lnk`, `.desktop`, `.app`.

**Niveau 2 : étapes manuelles documentées** (modèle prêt à copier) :
- EmuDeck (ES-DE + SRM), avec l'avertissement sur les réinitialisations et sur CloudSync.
- Playnite.
- RetroBat (après ajout du mode passthrough).
- Pegasus, Lutris, Attract-Mode.
- RetroDECK, par SRM ou `.desktop` avec le profil `retrodeck` seulement.

**Hors v3 :**
- ES-DE interne de RetroDECK (bac à sable).
- Batocera (distribution ; voie « scripts » à étudier).
- Daijishō (Android).

**Travaux de conception induits :**
1. Binaire Windows `-H windowsgui`, et une solution pour la sortie console.
2. `--emulator` avant la ROM dans tous les modèles, ou analyse des drapeaux après la ROM.
3. Mode passthrough `run --rom <rom> -- <commande…>` (RetroBat, préfixe Lutris).
4. Traitement de SIGTERM et de la fermeture : finir l'envoi.
5. Identification du jeu même si la ROM vient d'un dossier temporaire d'extraction.
6. Tests réels avant sortie : écran de pause LaunchBox, « Quitter le jeu » de Steam en mode Jeu, `flatpak run` bloquant, `zenity` dans gamescope.
