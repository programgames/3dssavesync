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
| `emulateurs.<nom>.sauvegardes` | Système → dossier des Sauvegardes (`DS`, `GB`, `GBA` ; `3DS` : le dossier `sdmc` d'Azahar). |
| `emulateurs.<nom>.roms` | Système → dossier des ROM. Seulement pour la commande `launch`. |
| `emulateurs.<nom>.profil` | Facultatif : `melonds`, `mgba` ou `azahar`. Défaut : le nom de l'entrée. Sert pour deux installations du même émulateur. |

Chaque entrée de `emulateurs` est un Client distinct : elle a ses propres Bases (`bases/<nom>.json`). Le journal est `3dssavesync.log`, à côté de `config.json`.

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
- **Information** (émulateur déjà lancé, envoi impossible) : une notification Windows, rien à valider.
- **Linux et macOS** : pas encore de fenêtre (phase 3). Un Conflit ne remplace rien : la partie utilise la Sauvegarde du PC, et le message va dans le journal.
