# Serveur : deux façons

Le Serveur garde chaque Version de chaque Sauvegarde et n'efface rien. La 3DS et le PC lui parlent par HTTP avec un jeton. Choisissez une des deux façons.

| | Serveur local sur le PC | Serveur personnel (Docker) |
|---|---|---|
| Installation | rien de plus : l'assistant du PC le configure | une machine toujours allumée (NAS, Raspberry Pi, serveur), Docker, un nom de domaine |
| Synchronisation | à la maison, quand le PC est allumé | partout (HTTPS) |
| Pour qui | la plupart des gens | ceux qui ont déjà un serveur |

Les deux gardent les mêmes fichiers : `data/<Clé>/<n°>-<date>-<client>-<hash>.sav`. Pour changer de façon, copiez le dossier `data`.

## Serveur local sur le PC

1. Lancez l'assistant (`3dssavesync setup`, ou double-clic sur l'exe), étape « Serveur » : le serveur local est proposé au premier lancement (choix **2** en mode texte).
2. L'assistant choisit un port (8765 par défaut), crée un jeton, lance le serveur et le fait démarrer avec la session (Windows : dossier Démarrage ; Linux : autostart ; macOS : LaunchAgent).
3. Sous Windows, acceptez la demande du pare-feu pour les **réseaux privés**.
4. L'assistant affiche l'adresse et le jeton pour la 3DS : l'adresse du PC sur la box d'abord, les autres (WSL, Docker, VPN) sous « Autres adresses ».
5. Le plus simple : mettez la carte SD de la 3DS dans le PC, puis **Préparer la carte SD de la 3DS** : `3ds/3dssavesync/config.ini` reçoit l'adresse et le jeton. Sinon, entrez-les sur la 3DS : 3dssavesync, SELECT, lignes « Serveur » (`http://192.168.1.20:8765` par exemple) et « Jeton ».

Les données sont dans `serveur/`, à côté de `config.json` (champ `serveur_local.donnees` pour un autre dossier). Commande : `3dssavesync serve`.

**Avertissement** : ce serveur parle HTTP sans chiffrement. N'ouvrez jamais ce port sur Internet (box, routeur) : le jeton passerait en clair. L'adresse de la 3DS change si le PC change d'adresse IP ; une réservation DHCP dans la box évite ce cas.

## Serveur personnel (Docker)

Fichiers dans `server/` du dépôt. Image : `ghcr.io/programgames/3dssavesync-server`.

Préparation commune :

1. Créez un dossier, avec `data/` (propriétaire 1000:1000).
2. Créez `.env` (droits 600) : `NDSSYNC_TOKEN=<jeton long et aléatoire>`, par exemple `openssl rand -hex 16`.

**Avec un reverse proxy existant** (Traefik, nginx, Caddy…) : `docker-compose.yml`. Le serveur écoute sur `127.0.0.1:8080` ; le reverse proxy fournit le HTTPS. Un exemple Traefik est dans le fichier.

**Sans reverse proxy** : `docker-compose.caddy.yml` et `Caddyfile`. Ajoutez `DOMAINE=saves.example.com` dans `.env`, faites pointer ce nom vers la machine, ouvrez les ports 80 et 443, puis :

```
docker compose -f docker-compose.caddy.yml up -d
```

**HTTPS et la 3DS** : le homebrew connaît seulement la racine **ISRG Root X1** (Let's Encrypt), et il demande TLS 1.2 avec ECDHE. Le `Caddyfile` fourni force Let's Encrypt et une clé RSA. Avec un autre reverse proxy, utilisez aussi un certificat Let's Encrypt.

**Sauvegarde du serveur** : copiez `data/` régulièrement (restic, Backrest, rsync…). Le serveur n'efface rien : la copie est simple.

Test du serveur déployé (même suite que la CI) :

```
cd server && NDSSYNC_URL=https://saves.example.com NDSSYNC_TOKEN=... python -m unittest test_server.py
```
