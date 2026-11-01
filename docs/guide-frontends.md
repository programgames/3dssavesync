# Frontends : configuration à la main

L'assistant (`3dssavesync setup`) configure seul **LaunchBox**, **ES-DE** et les **raccourcis**. Pour les autres frontends, suivez cette page. Sources et détails : [research-v3-frontends.md](research-v3-frontends.md).

Règles communes :

- `<exe>` = le chemin du Lanceur installé : `%LOCALAPPDATA%\Programs\3dssavesync\3dssavesync.exe` (Windows), `~/.local/bin/3dssavesync` (Linux), `~/Applications/3dssavesync/3dssavesync` (macOS).
- `<nom>` = le nom de l'émulateur dans `config.json` : `melonds`, `mgba`, `azahar`, `retroarch`, `vbam`, `sameboy`.
- Mettez toujours `--emulator <nom>` **avant** la ROM.
- Désactivez la synchronisation propre au frontend pour DS, GB, GBA et 3DS (CloudSync d'EmuDeck, par exemple) : deux outils enverraient les mêmes sauvegardes.
- Après une mise à jour ou une réinitialisation d'un frontend, ouvrez « 3dssavesync réglages » et choisissez **Réparer**.

## Steam ROM Manager (Steam Deck et bureau)

Ajoutez un **nouvel** analyseur (ne modifiez pas ceux d'EmuDeck) :

| Champ | Valeur |
|---|---|
| Parser type | Glob |
| ROMs directory | dossier des ROM DS (par exemple `~/Emulation/roms/nds`) |
| User's glob | `${title}@(.nds\|.NDS\|.zip\|.ZIP)` |
| Executable | `<exe>` |
| Command line arguments | `run --emulator <nom> "${filePath}"` |

Puis « Add games » et « Save to Steam » dans l'interface de SRM. Pour un Flatpak, le profil de `config.json` doit avoir `"processus"` (l'assistant le remplit).

## EmuDeck (Steam Deck)

1. Lancez l'assistant dans le mode Bureau : il configure ES-DE (`~/ES-DE/custom_systems`), qu'EmuDeck utilise.
2. Pour le mode Jeu, ajoutez l'analyseur Steam ROM Manager ci-dessus.
3. Désactivez CloudSync d'EmuDeck pour DS, GB, GBA et 3DS. L'assistant avertit s'il semble actif.
4. Ne modifiez pas les scripts de `~/Emulation/tools/launchers` : EmuDeck les remet à chaque lancement.
5. Après une réinitialisation d'EmuDeck : **Réparer**.

## RetroDECK

L'ES-DE interne de RetroDECK est dans un bac à sable : l'assistant ne le configure pas. Utilisez Steam ROM Manager ou un raccourci, avec un profil personnalisé dans `config.json` :

```json
"retrodeck-nds": {
  "exe": "/usr/bin/flatpak",
  "args": ["run", "net.retrodeck.retrodeck", "-s", "nds", "{rom}"],
  "sauvegardes": { "DS": "/home/deck/retrodeck/saves/nds/melonds" },
  "processus": "melonDS"
}
```

Non vérifié : `flatpak run` attend-il la fin du jeu ?

## Playnite (Windows)

Bibliothèque > Configurer les émulateurs > Ajouter un profil personnalisé :

| Champ | Valeur |
|---|---|
| Executable | `<exe>` |
| Arguments | `run --emulator <nom> "{ImagePath}"` |
| Supported file types | `nds, zip` (DS) ; `gb, gbc, gba, zip` (mGBA) ; `3ds, cci, cxi` (Azahar) |
| Tracking mode | Original process |

Puis choisissez ce profil pour les jeux concernés.

## RetroBat (Windows)

Créez `RetroBat\emulationstation\.emulationstation\es_systems_3dssavesync.cfg`. RetroBat garde son fichier d'origine ; pour revenir en arrière, supprimez ce fichier.

```xml
<?xml version="1.0"?>
<systemList>
  <system>
    <name>nds</name>
    <command>"<exe>" run --rom %ROM% -- "%HOME%\emulatorLauncher.exe" -gameinfo %GAMEINFOXML% %CONTROLLERSCONFIG% -system %SYSTEM% -emulator %EMULATOR% -core %CORE% -rom %ROM%</command>
  </system>
</systemList>
```

`run --rom … --` synchronise la ROM, puis lance la commande de RetroBat telle quelle. L'émulateur de `config.json` qui gère ce Système donne le dossier des sauvegardes.

## Pegasus

Dans `metadata.pegasus.txt` de la collection :

```
launch: "<exe>" run --emulator <nom> "{file.path}"
```

## Lutris (Linux)

Un jeu avec le runner « Linux » :

| Champ | Valeur |
|---|---|
| Executable | `<exe>` |
| Arguments | `run --emulator <nom> "/chemin/du/jeu.nds"` |

N'utilisez pas « Stop » de Lutris pendant la partie : il tue le Lanceur avant l'envoi (l'envoi part au lancement suivant).

## Attract-Mode

Nouveau fichier `emulators/<nom>-3dssavesync.cfg` (dans `~/.attract`, ou le dossier d'Attract-Mode sous Windows) :

```
executable           <exe>
args                 run --emulator <nom> "[romfilename]"
rompath              /chemin/roms/nds
romext               .nds;.zip
nb_mode_wait         0
```

Puis reliez une liste de ROM à cet émulateur dans les réglages d'Attract-Mode.
