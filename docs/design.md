# Conception

Vocabulaire : `docs/GLOSSARY.md`. Décisions : `docs/adr/`. Faits techniques : `docs/research-v3-*.md`.

## 1. Le module profond : `etat`

Toute la logique de Synchronisation tient dans une fonction pure. Elle ne lit aucun fichier et n'ouvre aucune connexion : le Client lui donne trois observations, elle rend un État.

### Interface

```
etat(local, serveur, base) -> A_JOUR | A_ENVOYER | A_RECUPERER | CONFLIT

local   = (hash, taille) ou ABSENT   -- Sauvegarde locale
serveur = (hash, taille) ou ABSENT   -- Version courante
base    = hash ou ABSENT             -- Base de ce Client pour ce Jeu
```

Ce que l'appelant doit savoir, et rien d'autre :

- Appel interdit si `local` et `serveur` sont tous deux ABSENT (rien à synchroniser).
- Après un résultat `A_JOUR`, ou après un Envoi ou une Récupération réussis, l'appelant écrit `base := hash synchronisé`. C'est la seule écriture de Base.
- Après un `412`, l'appelant rappelle `etat` avec le Hash renvoyé dans l'`ETag` de la réponse `412`. Le résultat est `CONFLIT` (l'autre Client a joué) ou `A_JOUR` (l'autre Client a envoyé le même contenu). Aucune règle spéciale pour le `412`.

### Implémentation (ordre des règles = ADR 0001)

1. `local == serveur` (même Hash) → `A_JOUR`
2. `serveur` ABSENT → `A_ENVOYER`
3. `local` ABSENT → `A_RECUPERER`
4. tailles différentes → `CONFLIT`
5. `local.hash == base` → `A_RECUPERER`
6. `serveur.hash == base` → `A_ENVOYER`
7. sinon → `CONFLIT`

### Deux implémentations, une seule table de cas

Le module existe en C (homebrew 3DS) et en PowerShell (Lanceur). Les deux lisent `tests/cas_etat.csv` dans leurs tests. Un cas ajouté au fichier est vérifié des deux côtés.

| cas | local | serveur | base | attendu |
|---|---|---|---|---|
| identiques | a/512 | a/512 | - | A_JOUR |
| identiques, base ancienne | a/512 | a/512 | z | A_JOUR |
| premier Envoi | a/512 | - | - | A_ENVOYER |
| première Récupération | - | a/512 | - | A_RECUPERER |
| serveur plus récent | a/512 | b/512 | a | A_RECUPERER |
| local plus récent | b/512 | a/512 | a | A_ENVOYER |
| les deux ont changé | b/512 | c/512 | a | CONFLIT |
| jamais synchronisé, différents | a/512 | b/512 | - | CONFLIT |
| taille différente, serveur plus récent | a/512 | b/256 | a | CONFLIT |
| taille différente, local plus récent | b/8 | a/512 | a | CONFLIT |
| après 412, autre contenu | b/512 | c/512 | a | CONFLIT |
| après 412, même contenu | b/512 | b/512 | a | A_JOUR |

(`a/512` = Hash `a`, taille 512 Kio. Les vrais Hash sont des SHA-256 hexadécimaux en minuscules.)

## 2. Serveur : `server/server.py`

Python, bibliothèque standard seulement (`http.server.ThreadingHTTPServer`, `hashlib`, `hmac`, `json`). FastAPI n'apporte rien pour cinq routes sans validation complexe, et ajouterait uvicorn et pydantic. Le serveur n'est jamais exposé directement : Traefik termine le TLS devant lui.

### Routes

Toutes exigent `Authorization: Bearer <jeton>` (comparaison `hmac.compare_digest`), sinon `401`. `<code>` doit correspondre à `^[A-Z0-9]{4}$`, sinon `400` (PUT) ou `404` (GET).

| Route | Réponse |
|---|---|
| `GET /saves` | `200` JSON `[{code, hash, taille, date, client}]`, une entrée par Jeu (Version courante) |
| `GET /saves/<code>` | `200` octets bruts + `ETag: "<hash>"`, ou `404` |
| `PUT /saves/<code>` | voir ci-dessous |
| `GET /saves/<code>/history` | `200` JSON `[{version, hash, taille, date, client}]`, de la plus ancienne à la plus récente |
| `GET /saves/<code>/history/<version>` | `200` octets bruts d'une Version, ou `404` |

### `PUT /saves/<code>`

En-têtes : `If-Match: "<hash>"` (le client croit que la Version courante est celle-ci) ou `If-None-Match: *` (le client croit qu'il n'y en a pas). Un `PUT` sans l'un des deux → `428`. `X-Client: 3ds` ou `pc`. Corps : la Sauvegarde brute (1 octet à 64 Mio).

Sous un verrou global :

1. Si la précondition est fausse → `412` + `ETag` de la Version courante (absent s'il n'y en a pas).
2. Le serveur calcule le Hash du corps. S'il est égal à la Version courante → `200`, aucune nouvelle Version.
3. Sinon il écrit (corps complet vérifié : sinon `400`) `data/<code>/<seq>-<date>-<client>-<hash>.sav` via un fichier temporaire puis `os.replace` → `200` + `ETag`.

`<seq>` est un compteur sur 6 chiffres (`<seq>` de la dernière Version + 1). La Version courante est le fichier de plus grand `<seq>` : aucun pointeur à maintenir, aucune dépendance à l'horloge. Rien n'est jamais supprimé ni réécrit (ADR 0004).

### Déploiement

`server/docker-compose.yml` + `Dockerfile` (`python:3-slim`, utilisateur 1000) + `.env` (`NDSSYNC_TOKEN`, chmod 600) + `./data`, derrière un reverse proxy HTTPS (exemple de labels Traefik dans le fichier).

## 3. Lanceur PC : `pc/sync.ps1`

PowerShell 5.1 (intégré à Windows 11) : `Invoke-WebRequest`, `Get-FileHash`, `System.IO.Compression` (lire l'en-tête dans un `.zip`) et `System.Windows.Forms.MessageBox` sont tous natifs. Rien à installer. Configuration : `pc/sync.config.json` (URL, jeton, chemin de `melonDS.exe`, dossier des sauvegardes). Base : `pc/base.json` (`{code: hash}`).

LaunchBox appelle : `powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "<chemin>\sync.ps1" "<rom>"`.

1. Si `melonDS.exe` tourne → message, arrêt.
2. Lire le Code jeu de la ROM (`.nds`, ou première entrée `.nds` d'un `.zip`). Code invalide → lancer melonDS sans Synchronisation.
3. Sauvegarde locale = `D:\Sauvegardes\DS\<nom de la ROM sans extension>.sav`.
4. **Avant la partie** : `GET /saves`, puis `etat`.
   - `A_RECUPERER` → Copie de sécurité (si le fichier existe), téléchargement dans un fichier temporaire, vérification du Hash, remplacement.
   - `A_ENVOYER` → Envoi.
   - `CONFLIT` → boîte Oui (prendre le serveur) / Non (garder le PC : Envoi immédiat avec `If-Match` = Hash serveur) / Annuler (ne pas jouer).
   - Serveur injoignable (délai 10 s, assez pour envoyer 512 Kio) → « Jouer avec la Sauvegarde locale ? ».
5. Lancer `melonDS.exe "<rom>"` et attendre la fin du processus.
6. **Après la partie** : relire la Sauvegarde locale, `etat` avec la Base. `A_ENVOYER` → Envoi avec `If-Match` = Base. `412` → `etat` avec l'`ETag` reçu ; `CONFLIT` → boîte « Garder le PC » / « Ne rien faire ». Erreur réseau → message, la Sauvegarde reste locale.

Copies de sécurité : `D:\Sauvegardes\DS\ndssync-copies\<nom>-<date>.sav`.

## 4. Homebrew 3DS : `3ds/`

C + libctru, console texte, `3ds-curl` + `3ds-mbedtls`.

| Fichier | Rôle |
|---|---|
| `source/main.c` | configuration (`swkbd`), scan, liste, actions, Copies de sécurité |
| `source/etat.c` / `etat.h` | le module `etat` (aucune dépendance libctru) |
| `source/net.c` | les 4 appels HTTP avec curl, ISRG Root X1 embarqué |
| `test/test_etat.c` | lit `tests/cas_etat.csv`, compilé avec `gcc` sur PC |

Fichiers sur la SD, dans `sd:/3ds/3dssavesync/` (avant la v3 : `sd:/3ds/nds-save-sync/`, repris au premier lancement) : `config.ini` (URL, jeton, `roms`, `saves`, `roms_gb`, `saves_gb`, `roms_gba`, `saves_gba` ; sauvegardes vides = à côté de la ROM), `titres.ini` (jeux 3DS synchronisés, `<title ID>=<nom>` ; absent = les 8 Pokémon), `base.ini` (`CLÉ=hash`), `copies/`.

1. Lire `config.ini` ; absent → écran de configuration.
2. Scanner `roms/*.nds` (racine seulement) : Code jeu de chaque ROM. Plusieurs ROM pour un Code jeu → Doublon.
3. Pour chaque Jeu : Sauvegarde locale = `saves/<nom ROM>.sav` (slot 0), Hash SHA-256 via mbedTLS.
4. `GET /saves`. Un Jeu présent sur le serveur, avec une ROM locale mais sans `.sav`, est `A_RECUPERER`. Un Jeu sans ROM locale n'est pas affiché.
5. Liste : Jeu, État. Haut/Bas pour choisir ; A sur un Jeu = son action (ou le choix de Conflit : A garder la 3DS, X prendre le serveur, B rien) ; START = « Tout synchroniser » (ignore les Conflits et les Doublons, une seule confirmation listant les Récupérations qui écrasent).
6. Récupération : Copie de sécurité → fichier temporaire → vérification du Hash → `rename`.

## 5. Ordre de la phase 2

1. `tests/cas_etat.csv` + `etat` en TDD (C et PowerShell).
2. Serveur + `server/test_server.py` (`unittest`) : Envoi, Récupération, `412`, `428`, `401`, Historique conservé, Envoi identique sans nouvelle Version.
3. Lanceur contre un serveur local, avec des copies de Sauvegardes et un faux `melonDS.exe` qui modifie le fichier.
4. Prototype 3DS (`GET /saves` en HTTPS sur le serveur réel), puis version complète.
5. Bout en bout avec une copie d'une vraie Sauvegarde Pokémon : 3DS → serveur → PC → melonDS → serveur → 3DS.

## 6. v2 : GB, GBC, GBA et Pokémon 3DS

Le cœur ne change pas (`etat`, Envoi conditionnel, Copie de sécurité). Ce qui s'ajoute :

- **Clé par Système**, lue dans la ROM : DS `ADAF`, GBA `GBA-BPEF`, GB `GB-<titre>-<somme>`, 3DS `3DS-<title ID>` (ADR 0007). Le serveur n'accepte que ces formes.
- **GB/GBA** : fichiers `.sav` à côté des ROM sur la 3DS (TWiLight, open_agb_firm), dossiers `D:\Sauvegardes\GB` / `GBA` sur le PC (mGBA, `-C savegamePath=…` par lancement). Hash, taille et Envoi portent sur la **SRAM seule** ; à la Récupération chaque machine remet son propre Trailer d'horloge (ADR 0008).
- **3DS** : le fichier `main` de l'archive de sauvegarde (8 Pokémon, table fixe), lu et écrit par `titres3ds.c` (commit, secure value) ; côté PC, `main` dans le dossier Azahar, jamais rien d'autre dans ce dossier ; un jeu jamais lancé n'est pas créé (ADR 0009).
- **Lanceur** : une table `$SYSTEMES` (exécutable, processus, dossier, extensions) ; tout le reste est commun.
- **Homebrew** : sources `sd:/roms/nds`, `sd:/roms/gb`, `sd:/roms/gba` (`config.ini` : `roms_gb`, `roms_gba`) et les archives 3DS ; le Système est affiché dans la liste.
