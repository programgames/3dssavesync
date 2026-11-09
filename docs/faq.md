# FAQ

Short answers, with a link to the page that has the details. Something else? See [Troubleshooting](troubleshooting.md), or ask in [Discussions](https://github.com/programgames/3dssavesync/discussions).

### My game is not in the list

- **DS, GB, GBA**: check the ROM and save folders. A game with no save on either side has nothing to sync yet: play it and save once. See [A game is missing from the list](troubleshooting.md#a-game-is-missing-from-the-list).
- **Native 3DS games**: every game with a save is synced. A game you never started has no save: start it once and save. A game you turned off is not in the list: turn it on again in **SELECT** > **3DS games**. See [Native 3DS games](guide-3ds.md#native-3ds-games).
- **Not synced** under a 3DS game: the reason is under the game, for example a save larger than 16 MB. Super Mario Maker is never synced.
- **Azahar** (PC or Android): start the game once in Azahar and save. The 8 Pokémon games are the exception: 3dssavesync can create their save folder.

### I downloaded an old save over a newer one, or uploaded the wrong one

The server keeps every version. Restore the one you want from the history:

- **PC**: **3dssavesync settings** > **Games** > the game > **Restore this version**. See [Restore an old version](guide-pc.md#restore-an-old-version).
- **Android**: open the game > **Show the history** > the version > **Restore**. See [History and Restore](guide-android.md#history-and-restore).

The restored version becomes the current one, and each device gets it at its next sync, after a backup copy. A save that was never uploaded is not on the server. Before a download replaced it, the device made a backup copy: see [Where are the backup copies?](troubleshooting.md#where-are-the-backup-copies).

### Can I use two 3DS consoles?

Yes. Install the 3DS app on each one, and prepare each SD card from the PC (or enter **Server** and **Token** in **SELECT**). Both consoles sync the same saves through the server. Sync on one console before you play on the other: if you play on both without a sync, you get a conflict and choose which save to keep. Super Mario Maker is never synced, because its save works only on the console that made it.

### I lost the SD card or the token

Your saves stay on the server.

- **SD card**: format a new card, prepare it from the PC, then install the 3DS app again ([Quick start](../README.md#quick-start)). The new card has no record of the last sync (`base.ini`), so a game whose save differs from the server's shows a conflict: press **X** to use the server save. A 3DS game you install again must be started once before it has a save.
- **Token**: the PC keeps it in `config.json`. For the local server, the settings page shows the address and token in the box "Enter these on the 3DS". Your own server keeps it in `.env`. If you think it leaked, see [The token](../SECURITY.md#the-token).

### I use Azahar on the PC and on Android

Both sync the same 3DS saves as the 3DS, through the same server: the key of a game is its title ID. Every game with a save in Azahar syncs, except the games you turn off. On Android, close Azahar before you sync. See [3DS games (Azahar)](guide-pc.md#3ds-games-azahar) and [3DS games on Android](guide-android.md#3ds-games).

### Is my token safe?

The token is the password of the server. Treat it like a password: don't post `config.json`, `config.ini` or `.env`, and keep the QR code for the phone private.

> [!WARNING]
> The local server on the PC speaks HTTP without encryption. On your home network, the token travels in clear text. Never open its port on the Internet (router port forwarding). To sync away from home, run [your own server](guide-server.md#your-own-server-docker) behind HTTPS.

The 3DS file `journal.log` never holds your server address or token. Read `3dssavesync.log` before you post it, and remove anything private. More in [SECURITY.md](../SECURITY.md).
