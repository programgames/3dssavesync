# Design

Vocabulary: [GLOSSARY.md](GLOSSARY.md). Decisions: [adr/](adr/). Technical facts: [`notes/research-v3-*.md`](notes/) (French working notes).

## 1. The deep module: `etat`

All the sync logic fits in one pure function. It reads no file and opens no connection: the Client gives it three observations, it returns a State.

### Interface

```
etat(local, serveur, base) -> A_JOUR | A_ENVOYER | A_RECUPERER | CONFLIT

local   = (hash, taille) or ABSENT   -- Local save
serveur = (hash, taille) or ABSENT   -- Current version
base    = hash or ABSENT             -- Base of this Client for this Game
```

What the caller must know, and nothing else:

- Do not call it when `local` and `serveur` are both ABSENT (nothing to sync).
- After an `A_JOUR` result, or after a successful Upload or Download, the caller writes `base := synced hash`. This is the only write of the Base.
- After a `412`, the caller calls `etat` again with the Hash returned in the `ETag` of the `412` response. The result is `CONFLIT` (the other Client played) or `A_JOUR` (the other Client uploaded the same content). No special rule for the `412`.

### Implementation (rule order = ADR 0001)

1. `local == serveur` (same Hash) → `A_JOUR`
2. `serveur` ABSENT → `A_ENVOYER`
3. `local` ABSENT → `A_RECUPERER`
4. different sizes → `CONFLIT`
5. `local.hash == base` → `A_RECUPERER`
6. `serveur.hash == base` → `A_ENVOYER`
7. otherwise → `CONFLIT`

### Two implementations, one case table

The module exists in C (3DS homebrew) and in Go (PC program). Both read `tests/cas_etat.csv` in their tests. A case added to the file is checked on both sides.

| case | local | server | base | expected |
|---|---|---|---|---|
| identical | a/512 | a/512 | - | A_JOUR |
| identical, old base | a/512 | a/512 | z | A_JOUR |
| first Upload | a/512 | - | - | A_ENVOYER |
| first Download | - | a/512 | - | A_RECUPERER |
| server newer | a/512 | b/512 | a | A_RECUPERER |
| local newer | b/512 | a/512 | a | A_ENVOYER |
| both changed | b/512 | c/512 | a | CONFLIT |
| never synced, different | a/512 | b/512 | - | CONFLIT |
| different size, server newer | a/512 | b/256 | a | CONFLIT |
| different size, local newer | b/8 | a/512 | a | CONFLIT |
| after 412, other content | b/512 | c/512 | a | CONFLIT |
| after 412, same content | b/512 | b/512 | a | A_JOUR |

(`a/512` = Hash `a`, size 512 KiB. Real Hashes are lowercase hexadecimal SHA-256.)

## 2. Server: `server/server.py`

Python, standard library only (`http.server.ThreadingHTTPServer`, `hashlib`, `hmac`, `json`). FastAPI brings nothing for five routes without complex validation, and would add uvicorn and pydantic. The server is never exposed directly: Traefik terminates TLS in front of it.

### Routes

All require `Authorization: Bearer <token>` (compared with `hmac.compare_digest`), otherwise `401`. `<code>` must match `^[A-Z0-9]{4}$`, otherwise `400` (PUT) or `404` (GET).

| Route | Response |
|---|---|
| `GET /saves` | `200` JSON `[{code, hash, taille, date, client, nom?}]`, one entry per Game (Current version); `nom` if a Client named the game ([ADR 0018](adr/0018-nom-du-jeu-sur-le-serveur.md)) |
| `GET /saves/<code>` | `200` raw bytes + `ETag: "<hash>"`, or `404` |
| `PUT /saves/<code>` | see below |
| `GET /saves/<code>/history` | `200` JSON `[{version, hash, taille, date, client}]`, oldest first |
| `GET /saves/<code>/history/<version>` | `200` raw bytes of one Version, or `404` |
| `PUT /saves/<code>/name` | body = game name (UTF-8, 1-200 characters): `204`, `404` without Version, `400` invalid name |

### `PUT /saves/<code>`

Headers: `If-Match: "<hash>"` (the client believes the Current version is this one) or `If-None-Match: *` (the client believes there is none). A `PUT` without one of them → `428`. `X-Client: 3ds` or `pc`. Body: the raw Save (1 byte to 64 MiB).

Under a global lock:

1. If the precondition is false → `412` + `ETag` of the Current version (absent if there is none).
2. The server computes the Hash of the body. If it equals the Current version → `200`, no new Version.
3. Otherwise it writes (full body checked: otherwise `400`) `data/<code>/<seq>-<date>-<client>-<hash>.sav` through a temporary file then `os.replace` → `200` + `ETag`.

`<seq>` is a 6-digit counter (`<seq>` of the last Version + 1). The Current version is the file with the highest `<seq>`: no pointer to maintain, no dependency on the clock. Nothing is ever deleted or rewritten (ADR 0004).

### Deployment

`server/docker-compose.yml` + `Dockerfile` (`python:3-slim`, user 1000) + `.env` (`NDSSYNC_TOKEN`, chmod 600) + `./data`, behind an HTTPS reverse proxy (example Traefik labels in the file).

## 3. PC program: `client/`

One Go program, `3dssavesync`, for Windows, Linux and macOS. It has one dependency (`godbus/dbus`, for the icon near the clock on Linux) and nothing to install next to it. Configuration: `config.json` in the settings folder (see the [PC guide](guide-pc.md#configuration)). Base: `bases/<emulator>.json` (`{key: hash}`), one file per emulator.

A frontend calls `3dssavesync run [--emulator <name>] "<rom>"` instead of the emulator (ADR 0006). The `launch <name>` command and the shortcuts do the same for every game of an emulator.

1. If the same game already runs, or the emulator was opened outside 3dssavesync → message, stop.
2. Read the Key of the ROM (ADR 0007), or its Alias (ADR 0014). Unknown ROM → a notification, and the emulator starts without sync.
3. Local save = the file the emulator profile names for this ROM (`client/profiles.go`).
4. **Before the game**: `GET /saves`, then `etat`.
   - `A_RECUPERER` → Backup copy (if the file exists), download into a temporary file, Hash check, rename.
   - `A_ENVOYER` → Upload.
   - `CONFLIT` → **Use server save** / **Keep PC save** (immediate Upload with `If-Match` = server Hash) / **Don't play**.
   - Server unreachable (30 s timeout) → "Play with the PC save?". The save goes to the server at the next launch.
5. Start the emulator and wait for its process to end.
6. **After the game**: read the Local save again, `etat` with the Base. `A_ENVOYER` → Upload with `If-Match` = Base. `412` → `etat` again with the new Current version; `CONFLIT` → **Keep PC save** / **Do nothing**. Network error → the save is written to `en-attente.json` and uploaded first at the next `run` or `launch`.

Backup copies: `<copies>/<emulator>/<name>-<date>`. The Go tests (`client/launcher_test.go`) run this flow against the local server (`client/server.go`) in a temporary folder; `client/server_test.go` checks that it keeps the contract of `server/server.py` by running `server/test_server.py` against it.

## 4. 3DS homebrew: `3ds/`

C + libctru, text console, `3ds-curl` + `3ds-mbedtls`.

| File | Role |
|---|---|
| `source/main.c` | configuration (`swkbd`), scan, list, actions, Backup copies |
| `source/state.c` / `state.h` | the `sync_state` module (no libctru dependency) |
| `source/games.c` | status of a Game in the list: State, Duplicate, Unknown, hidden, Blank save rule (no libctru dependency) |
| `source/settings.c` | `config.ini`, `base.ini`, `alias.ini`, `titres.ini`, `maj.ini`: one INI reader, writes by `replace_file` (no libctru dependency) |
| `source/result.c` | result of an action and its message text (no libctru dependency) |
| `source/views.c` / `views.h` | layout, palette, button bar, message band, progress, questions |
| `source/app_update.c` | update of the app from GitHub (ADR 0021) |
| `source/net.c` | the 4 HTTP calls with curl, ISRG Root X1 embedded |
| `test/*.c` | host tests of the modules without libctru (`test_state.c` reads `tests/cas_etat.csv`), compiled with `gcc` on the PC |

Files on the SD card, in `sd:/3ds/3dssavesync/` (before v3: `sd:/3ds/nds-save-sync/`, taken over at first start): `config.ini` (URL, token, `roms`, `saves`, `roms_gb`, `saves_gb`, `roms_gba`, `saves_gba`; empty saves = next to the ROM), `titres.ini` (synced 3DS games, `<title ID>=<name>`; absent = the 8 Pokémon), `base.ini` (`KEY=hash`), `alias.ini` (optional, `<ROM name>=KEY`, ADR 0014), `copies/`.

1. Read `config.ini`; absent → configuration screen.
2. Scan `roms/**/*.nds` (subfolders up to 4 levels, ADR 0015): Game code of each ROM, or its Alias. Several ROMs for one Game code → Duplicate.
3. For each Game: Local save = `saves/<ROM name>.sav` (slot 0; ROM in a subfolder: ADR 0015), SHA-256 Hash through mbedTLS.
4. `GET /saves`. A Game present on the server, with a local ROM but no `.sav`, is `A_RECUPERER`. A Game without a local ROM is not shown.
5. List: Game, State. Up/Down to choose; A on a Game = its action (or the Conflict choice: A keep the 3DS, X take the server, B nothing); START = "Sync all" (skips Conflicts and Duplicates, one single confirmation listing the Downloads that overwrite).
6. Download: Backup copy → temporary file → Hash check → `rename`.

## 5. Phase 2 order

1. `tests/cas_etat.csv` + `etat` with TDD (C and PowerShell).
2. Server + `server/test_server.py` (`unittest`): Upload, Download, `412`, `428`, `401`, History kept, identical Upload without a new Version.
3. Launcher against a local server, with copies of Saves and a fake `melonDS.exe` that changes the file.
4. 3DS prototype (`GET /saves` over HTTPS on the real server), then full version.
5. End to end with a copy of a real Pokémon Save: 3DS → server → PC → melonDS → server → 3DS.

## 6. v2: GB, GBC, GBA and 3DS Pokémon

The core does not change (`etat`, conditional Upload, Backup copy). What is added:

- **Key per System**, read in the ROM: DS `ADAF`, GBA `GBA-BPEF`, GB `GB-<title>-<checksum>`, 3DS `3DS-<title ID>` (ADR 0007). The server accepts only these forms.
- **GB/GBA**: `.sav` files next to the ROMs on the 3DS (TWiLight, open_agb_firm), `<save folder>\GB` / `GBA` folders on the PC (mGBA, `-C savegamePath=…` per launch). Hash, size and Upload cover the **SRAM only**; at Download each machine puts back its own Clock trailer (ADR 0008). Trailer = remainder of `size mod 512` if it is 16, 44 or 48 bytes (ADR 0010).
- **3DS**: the `main` file of the save archive (8 Pokémon, fixed table), read and written by `titles3ds.c` (commit, secure value); on the PC, `main` in the Azahar folder, never anything else in that folder; a game never started is created in Azahar with a known `.metadata`, never on the 3DS (ADR 0009).
- **Launcher**: one profile per emulator (executable, process, save folder, extensions); everything else is shared.
- **Homebrew**: sources `sd:/roms/nds`, `sd:/roms/gb`, `sd:/roms/gba` (`config.ini`: `roms_gb`, `roms_gba`) and the 3DS archives; the System is shown in the list.
