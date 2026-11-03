# 3dssavesync glossary

3dssavesync keeps the same game save between a 3DS and the emulators of a PC (DS, GB, GBC, GBA and native 3DS games), through a server that never loses anything.

This glossary fixes one word per idea, for the design docs and the ADRs. The code is written in French: each term gives its French name, used in code identifiers and comments.

## Games

**Game** (*Jeu*):
A DS, GB, GBC, GBA or 3DS title, identified by its Key. Two ROMs with the same Key are the same Game.
_Avoid_: title, ROM, cartridge

**System** (*Système*):
The original console of a Game: DS, GB (GB and GBC together), GBA or 3DS. It sets the form of the Key and the place of the Saves.
_Avoid_: platform, console, medium

**Key** (*Clé*):
The identifier of a Game on the Server, read from the ROM header: the Game code for DS, `GBA-` + game code, `GB-` + title + checksum, `3DS-` + title ID (ADR 0007).
_Avoid_: id, identifier, game ID

**ROM**:
The file that holds a Game on a Client: `.nds`, `.gb`, `.gbc`, `.gba`, a 3DS ROM (`.3ds`, `.cci`, `.cxi`), or a `.zip` that contains one of them on the PC. Its file name plays no part in the identity of the Game, except through an Alias.
_Avoid_: image, dump, cartridge

**Alias**:
A Key chosen by the user for a ROM, in place of the Key of its header: a ROM hack often keeps the Key of its original game (ADR 0014).
_Avoid_: nickname, renaming

**Game code** (*Code jeu*):
The 4 ASCII characters of the header of a DS ROM (offset `0x0C`) or GBA ROM (offset `0xAC`), region included (e.g. `IRBF`). Two regions of the same title have two Game codes, so they are two Games.
_Avoid_: game ID, title ID, serial, gamecode

**Duplicate** (*Doublon*):
A Client has several ROMs with the same Key (or, on the 3DS, with the same Save file). A Duplicate Game is not synced.
_Avoid_: copy, conflict

## Saves

**Save** (*Sauvegarde*):
The raw content of the save memory of a Game, byte for byte, with no header or footer.
_Avoid_: save file, `.sav` file, backup

**Local save** (*Sauvegarde locale*):
The Save of a Game as it exists on a Client at a given time.
_Avoid_: local file

**Hash**:
The SHA-256 fingerprint of the content of a Save. Two Saves are identical if and only if their Hashes are equal.
_Avoid_: checksum, digest, fingerprint, ETag

**Backup copy** (*Copie de sécurité*):
A timestamped copy of a Local save, made by the Client just before it overwrites it.
_Avoid_: backup, `.bak`

**SRAM**:
The "game" part of a GB or GBA Save: the file without its Clock trailer. This part is hashed, compared and synced.
_Avoid_: data, content

**Clock trailer** (*Trailer d'horloge*):
The bytes that an emulator adds after the SRAM for the real-time clock (mGBA: 48 bytes for GB, 16 bytes for GBA). It stays on the machine that wrote it.
_Avoid_: RTC, footer, RTC footer

**Blank save** (*Sauvegarde vierge*):
A DS, GB or GBA Save whose SRAM holds only 0x00 or only 0xFF: no game is written in it yet. It counts as absent.
_Avoid_: empty save

**Save archive** (*Archive de sauvegarde*):
The system container of a native 3DS Save; for the 3DS Pokémon games it holds one file, `main`. Azahar stores it as an ordinary folder.
_Avoid_: savedata, container

**Secure value**:
The anti-restore counter of a 3DS game. It is removed after each write of a Save archive on the 3DS, otherwise the game refuses the Save.
_Avoid_: anti-rollback

**TWiLight slot** (*Slot TWiLight*):
An alternative Save of TWiLight Menu++ (`.sav1` to `.sav9`). Out of scope: only slot 0 is synced.
_Avoid_: save number

## Server

**Version**:
A Save received by the Server for a Game, with its date of reception and the Client that sent it. A Version never changes and never disappears.
_Avoid_: revision, snapshot, copy

**Current version** (*Version courante*):
The Version most recently accepted by the Server for a Game.
_Avoid_: last version, head, latest, server version

**History** (*Historique*):
All the Versions of a Game on the Server, from the oldest to the Current version.
_Avoid_: archive, backups, log

**Restore** (*Restauration*):
Making an old Version the content of the Current version, by sending it as a new Version. The History never goes back.
_Avoid_: rollback

## Sync

**Client**:
A machine or an emulator where you play, with its own Local saves and Bases: the 3DS, or an emulator of the PC. Two emulators of the same System on one PC are two Clients.
_Avoid_: device, console, station

**Server** (*Serveur*):
The service that keeps the History of each Game and decides between concurrent Uploads: the local server inside the PC program, or a Docker server.
_Avoid_: cloud, backend, NAS

**Base**:
The Hash of the last Version that a Client synced for a Game (uploaded or downloaded). Each Client has its own Base per Game.
_Avoid_: last sync, ancestor, reference, previous state

**State** (*État*):
The result of the comparison between Local save, Current version and Base for a Game: In sync (*À jour*), To upload (*À envoyer*), To download (*À récupérer*) or Conflict.
_Avoid_: action, status, decision

**Conflict** (*Conflit*):
The State where the Local save and the Current version have both changed since the Base, or do not have the same size. Only the user resolves a Conflict.
_Avoid_: collision, divergence

**Upload** (*Envoi*):
The Client sends its Local save; if the Server accepts it, it becomes a new Current version.
_Avoid_: push

**Download** (*Récupération*):
The Client replaces its Local save with the Current version, after a Backup copy.
_Avoid_: pull

**Launcher** (*Lanceur*):
The PC program (`3dssavesync`; before v3, a PowerShell script) that a frontend or a shortcut calls in place of the emulator: Sync, game, then Sync.
_Avoid_: wrapper, plugin, sync script

**Profile** (*Profil*):
What the Launcher knows about an emulator: how to start it, where and under which name it stores the Saves of each System, and which setting reloads a state at start. Built in (melonDS, mGBA, Azahar, RetroArch, VBA-M, SameBoy, DeSmuME, SkyEmu, NanoBoyAdvance, BGB) or custom in the configuration.
_Avoid_: preset, template
