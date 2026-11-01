# 3dssavesync

Jouer à la même partie sur une 3DS et sur un PC, sans copier de fichier à la main. Couvre la DS, le Game Boy / Game Boy Color, la GBA et des jeux 3DS natifs (par défaut : les 8 Pokémon, de X à Ultra-Lune).

> **État** : version 3 en cours. Le code repris de la v2 fonctionne pour une installation précise (Windows + LaunchBox + melonDS / mGBA / Azahar). Le plan de la v3 (installation simple, émulateurs autonomes, Linux / macOS / Steam Deck, anglais) est dans [docs/v3-plan.md](docs/v3-plan.md).

## Architecture

```
        3DS (Luma3DS)                              Serveur personnel                         PC
 ┌──────────────────────────────┐          ┌───────────────────────────────┐         ┌──────────────────────────────┐
 │ homebrew (.cia / .3dsx)      │  HTTPS   │ server/server.py (Python)     │  HTTPS  │ client/ : 3dssavesync (Go)   │
 │  lit les sauvegardes :       │ ───────► │  dans Docker, derrière un     │ ◄────── │  remplace l'émulateur        │
 │  · DS  (TWiLight, forwarders)│ ◄─────── │  reverse proxy HTTPS          │ ──────► │  · melonDS (DS)              │
 │  · GB  (TWiLight, mGBA)      │          │  data/ : chaque Version       │         │  · mGBA (GB, GBC, GBA)       │
 │  · GBA (open_agb_firm, mGBA) │          │  gardée pour toujours         │         │  · Azahar (3DS)              │
 │  · 3DS (archives système)    │          │                               │         │                              │
 └──────────────────────────────┘          └───────────────────────────────┘         └──────────────────────────────┘
```

**Le serveur** (`server/`) : un fichier Python sans dépendance, dans un conteneur Docker. Il garde chaque Version reçue (`data/<Clé>/<n°>-<date>-<client>-<hash>.sav`) et n'efface jamais rien. Il refuse un envoi si quelqu'un a envoyé entre-temps (`412`) : aucun écrasement silencieux. Accès par jeton (`Authorization: Bearer`).

**Le homebrew 3DS** (`3ds/`, C + libctru, HTTPS avec curl + mbedTLS) : il liste les jeux avec leur État (à jour, à envoyer, à récupérer, Conflit) ; START synchronise tout. Il demande confirmation avant d'écraser une sauvegarde et en garde toujours une copie.

**Le Lanceur PC** (`client/`, Go, un seul exécutable sans dépendance) : le frontend ou un raccourci l'appelle à la place de l'émulateur. Il récupère la sauvegarde, lance le jeu, attend la fermeture de l'émulateur, puis envoie la nouvelle sauvegarde. Profils intégrés : melonDS, mGBA, Azahar, RetroArch (cœurs melonDS DS, mGBA, Gambatte, SameBoy, VBA-M), VBA-M, SameBoy, plus un profil personnalisé. Installation : un double-clic lance un assistant (serveur, émulateurs, LaunchBox, ES-DE, raccourcis) ; autres frontends : [guide-frontends.md](docs/guide-frontends.md). Guide : [docs/guide-client-pc.md](docs/guide-client-pc.md). L'ancien Lanceur PowerShell (`pc/sync.ps1`) reste disponible.

## Comment la synchronisation décide

Pas de dates (l'horloge de la 3DS n'est pas fiable) : chaque machine compare trois empreintes SHA-256, la sienne, celle du serveur et celle de la dernière synchronisation (la **Base**). Une seule machine a changé : on copie. Les deux : **Conflit**, on demande, jamais de choix automatique. Détails : [docs/design.md](docs/design.md), [ADR 0001](docs/adr/0001-sync-trois-points-par-hash.md).

Chaque jeu a une **Clé** lue dans la ROM, identique sur la 3DS et le PC ([ADR 0007](docs/adr/0007-cle-de-jeu-par-systeme.md)) :

| Système | Clé |
|---|---|
| DS | `ADAF` (code jeu) |
| GB / GBC | `GB-PM_CRYSTAL-F2E2` (titre + somme de contrôle) |
| GBA | `GBA-BPEF` (code jeu) |
| 3DS | `3DS-00040000001B5000` (title ID) |

## Pièges connus

- **Savestates** : charger un état remplace la sauvegarde par une ancienne. mGBA recharge un état au lancement par défaut (`autoload`) : à désactiver. Le Lanceur pose la question après une Récupération si ce réglage (ou celui de RetroArch, VBA-M) est actif.
- **Sauvegarde vierge** (que des `0x00` ou `0xFF`) : comptée comme absente, jamais envoyée.
- **Jeux 3DS dans Azahar** : un jeu jamais lancé dans Azahar n'a pas de dossier de sauvegarde ; le lancer une fois et sauvegarder en jeu.
- **Horloge d'Or / Argent / Cristal** : chaque machine garde sa propre horloge ([ADR 0008](docs/adr/0008-gb-gba-par-fichiers-sram-seule.md)).
- **Forwarders mGBA** : leur sauvegarde (`sdmc:/mGBA/forwarders/`) prime ; touche **X** du homebrew pour revenir à TWiLight / open_agb_firm ([guide](docs/guide-forwarders-mgba.md)).

## Développement

```bash
cd server && python -m unittest test_server.py
NDSSYNC_URL=https://... NDSSYNC_TOKEN=... python -m unittest test_server.py   # même contrat, contre un serveur déployé
cd client && go test ./...                         # Lanceur Go, contre le vrai serveur (Python requis)
powershell -NoProfile -File pc/test_etat.ps1
powershell -NoProfile -File pc/test_lanceur.ps1
```

Tests C (gcc sur le PC) et homebrew (devkitPro) :

```bash
cd 3ds && gcc -std=c99 -I source source/etat.c test/test_etat.c -o /tmp/t && /tmp/t ../tests/cas_etat.csv
cd 3ds && gcc -std=c99 -I source source/cle.c test/test_cle.c -o /tmp/c && /tmp/c
cd 3ds && gcc -std=c99 -I source source/fichiers.c test/test_fichiers.c -o /tmp/f && /tmp/f
cd 3ds && make cia      # makerom et bannertool dans le PATH
```

GitHub Actions lance ces tests (Lanceur Go sur Windows, Linux et macOS) et compile le `.cia` / `.3dsx` ; un tag `v*` publie une release avec les exécutables PC.

Documentation : [GLOSSARY](docs/GLOSSARY.md), [ADR](docs/adr/), recherches [v1](docs/research.md), [v2](docs/research-v2.md), [v3](docs/).

Licence : MIT.
