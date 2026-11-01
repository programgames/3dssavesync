# Le Lanceur remplace melonDS dans LaunchBox, sans surveillance de fichiers

LaunchBox appelle `powershell.exe -File sync.ps1 <rom>` à la place de `melonDS.exe`. Le Lanceur fait la Récupération, lance melonDS, attend la fin du processus, puis fait l'Envoi.

Raison : melonDS garde la Sauvegarde en mémoire, réécrit le fichier entier 2 à 3 s après chaque sauvegarde en jeu (écriture non atomique), et ne verrouille pas le fichier (`docs/research.md`, partie 1 §2). Lire le `.sav` pendant que melonDS tourne peut donner un fichier tronqué ; l'écrire pendant qu'il tourne est perdu à la prochaine sauvegarde en jeu. Seule la fin du processus est un point sûr.

## Options écartées

- **Surveillance du dossier** (service qui envoie à chaque modification) : lit des fichiers à moitié écrits, et ne peut pas récupérer avant la partie.
- **Plugin / script Lua** : melonDS n'a pas de système de plugins.
- **Un raccourci par jeu** : LaunchBox lance déjà tous les jeux DS ; une seule entrée d'émulateur couvre les 3710 ROM.

## Conséquences

- Le Lanceur refuse de démarrer si `melonDS.exe` tourne déjà.
- Charger un savestate melonDS réécrit le `.sav` avec une ancienne Sauvegarde ; le Lanceur ne peut pas le voir. À signaler dans le README.
