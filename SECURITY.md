# Security

3dssavesync moves your game saves between a 3DS, a PC and a server. A security bug here can mean someone reads, replaces or deletes your saves, so please report it privately.

## Report a vulnerability

Use **Report a vulnerability** in the [Security tab](https://github.com/programgames/3dssavesync/security) of this repository. Only the maintainer sees the report.

If you can't find that button, open an issue that says only that you have a security problem to report, with no details. The maintainer will get back to you to continue in private.

Please include the version, what an attacker can do, and the steps to reproduce.

Only the latest release gets security fixes.

## What to keep safe

### The token

The token is the server's password. Anyone who has it can read every save on the server and upload new versions. The server never deletes a version, so an old save can always be restored, but treat the token like a password anyway.

It's stored in:

- `config.json` in the PC settings folder;
- `sdmc:/3ds/3dssavesync/config.ini` on the 3DS SD card;
- the private settings of the Android app (other apps can't read them; uninstalling the app deletes them);
- `.env` next to `docker-compose.yml` on your own server (keep it at `chmod 600`).

Don't post these files in an issue. The settings page never shows a personal server's token. If you think yours leaked on your own server, put a new one in `.env` (`openssl rand -hex 16`), restart the server, and enter it again on the PC, the 3DS and the phone. An empty token is refused: the server doesn't start without one.

### The local server

The local server built into the PC program speaks plain HTTP. It's meant for your home network only:

- **Never forward its port on your router.** The token would cross the Internet in clear text.
- On Windows, allow it on **private networks** only when the firewall asks.

To sync away from home, run your own server behind HTTPS instead: see the [server guide](docs/guide-server.md).

### The settings page

The settings page listens on `127.0.0.1` only, under a random secret path, and stops shortly after you close it. Other machines on your network can't reach it.

### Your own server

Put it behind an HTTPS reverse proxy (the `server/` folder has a Caddy setup that does this for you). Back up its `data/` folder: the server keeps every version, but it can't protect against a lost disk.

### Updates

The PC program, the 3DS app and the Android app download updates only from this repository's GitHub releases, and check each file against the release's `SHA256SUMS` before installing it. They never update without you clicking **Update**. On Android, the system installer also checks the APK signature: an APK signed with another key can't replace the app ([ADR 0026](docs/adr/0026-client-android.md)). Details: [ADR 0021](docs/adr/0021-mise-a-jour-du-homebrew-depuis-github.md) and [ADR 0023](docs/adr/0023-cia-inclus-et-mise-a-jour-du-pc.md).
