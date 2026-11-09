# Server guide

The server keeps every version of every save and deletes nothing. The 3DS and the PC talk to it over HTTP, with a token. Choose one of two ways.

| | Local server on the PC | Your own server (Docker) |
|---|---|---|
| Setup | nothing more: the PC settings page sets it up | an always-on machine (NAS, Raspberry Pi, server), Docker, a domain name |
| Sync | at home, when the PC is on | anywhere (HTTPS) |
| For | most people | people who already run a server |

Both keep the same files: `data/<game key>/<number>-<date>-<client>-<hash>.sav`. To switch from one to the other, copy the `data` folder.

## Local server on the PC

1. Start the settings page (double-click the program, or `3dssavesync setup`). In the **Server** part, **Local server on this PC** is the default (choice **2** in text mode).
2. Click **Save**. The program picks a port (8765 by default), creates a token, starts the server, and makes it start with your session (Windows: Startup folder; Linux: autostart; macOS: LaunchAgent). With the [icon near the clock](guide-pc.md#the-icon-near-the-clock) (Windows, Linux), the icon program runs the server.
3. On Windows, allow the firewall request for **private networks** ([help](troubleshooting.md#windows-defender-firewall-asks-about-3dssavesync)).
4. The page shows the address and token for the 3DS. The address of the PC on your home network comes first; the others (WSL, Docker, VPN) are under **Other addresses**.
5. Easiest: put the 3DS SD card in the PC, then click **Prepare the 3DS SD card**. `3ds/3dssavesync/config.ini` gets the address and token. Otherwise, enter them on the 3DS: 3dssavesync, **SELECT**, lines **Server** (`http://192.168.1.20:8765` for example) and **Token**.

The data is in `serveur/`, next to `config.json` (field `serveur_local.donnees` for another folder). Command: `3dssavesync serve`.

**Warning**: this server speaks HTTP without encryption. Never open its port on the Internet (router port forwarding): the token would travel in clear text. The 3DS address changes if the PC gets another IP address. A DHCP reservation in your router prevents this.

## Your own server (Docker)

Files in `server/` of this repository. Image: `ghcr.io/programgames/3dssavesync-server`, for x86 (amd64) and ARM (arm64: Raspberry Pi, most NAS).

Common steps:

1. Create a folder, with `data/` inside (owner 1000:1000).
2. Create `.env` (rights 600): `NDSSYNC_TOKEN=<long random token>`, for example from `openssl rand -hex 16`.

**With an existing reverse proxy** (Traefik, nginx, Caddy…): `docker-compose.yml`. The server listens on `127.0.0.1:8080`; the reverse proxy provides HTTPS. The file has a Traefik example.

**Without a reverse proxy**: `docker-compose.caddy.yml` and `Caddyfile`. Add `DOMAINE=saves.example.com` to `.env`, point this name to the machine, open ports 80 and 443, then:

```
docker compose -f docker-compose.caddy.yml up -d
```

Then, in the PC settings page, choose **Personal server**, enter `https://saves.example.com` and the token, and click **Test the connection**.

### HTTPS and the 3DS

The 3DS app knows only the **ISRG Root X1** root certificate (Let's Encrypt), and it needs TLS 1.2 with ECDHE. The `Caddyfile` provided forces Let's Encrypt and an RSA key. With another reverse proxy, use a Let's Encrypt certificate too.

Your own CA, or a self-signed certificate? Put its certificate (PEM, 256 KB at most; several can follow each other) in `sdmc:/config/ssl/cacert.pem` on the SD card, the file other homebrew use, then start the app again. The 3DS then trusts it for your server, on top of Let's Encrypt. `journal.log` says whether the file was added or ignored. Updates from GitHub do not use it ([ADR 0029](adr/0029-certificats-personnalises.md)).

### Backups of the server

Copy `data/` often (restic, Backrest, rsync…). The server deletes nothing, so the copy is simple.

### Check the history

Each version file ends with the SHA-256 hash of its content. `verify` reads every version again and compares:

```
docker compose exec 3dssavesync python /app/server.py verify
```

It prints `DAMAGED <game key>/<version>` for each version whose content changed on the disk, then a count, and exits with code 1 if one is damaged. It changes nothing. Run it after a disk problem, or on a restored backup: `python server/server.py verify path/to/data` (Python 3.11 or newer).

### Test a deployed server

The same test suite as the CI:

```
cd server && NDSSYNC_URL=https://saves.example.com NDSSYNC_TOKEN=... python -m unittest test_server.py
```

## Maintenance and updates

- **Disk use grows with every version.** The server never deletes a version, so `data/` only gets bigger. A 3DS game other than the 8 Pokémon games stores its whole save archive in each version. Check the free space on the disk from time to time.
- **Back up `data/`.** It holds every version of every save. For the local server, it is the `serveur/` folder next to `config.json`. See [Backups of the server](#backups-of-the-server).
- **Update the server first, then the devices**, when the release notes say so. For example, the 3DS games other than the Pokémon games use `3DSA-` keys ([ADR 0028](adr/0028-archives-3ds-completes.md)): an older server refuses them, so these games can't sync until the server is updated.
  - Docker: in the server folder, run `docker compose pull`, then `docker compose up -d` (add `-f docker-compose.caddy.yml` to both if you use Caddy). The `data/` folder and `.env` stay as they are.
  - Local server: it is part of the PC program. Update the PC program, and the server is updated with it.

Something does not work? See [Troubleshooting](troubleshooting.md#server-and-network).
