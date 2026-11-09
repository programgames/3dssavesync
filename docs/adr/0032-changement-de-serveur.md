# Another server or token: the Bases are forgotten

A Base is the Hash of the last Version a device synced for a Game, on one server. Today the Bases do not say which server they belong to. When the user points a device at another server or another token (a new server, a fresh `data/` folder, the local server instead of the personal one), the old Bases still apply: a Local save equal to its old Base is To download, so the new server's Version replaces it without a question; a Local save that changed is To upload over the new server's Version. A backup copy is made first, but the user is not asked. Cloudpoint met the same bug (its issue #57, fixed in #59: the sync state is cleared when the user key changes).

## Decision

- **A fingerprint of the server with the Bases.** Each device keeps, next to its Bases, the SHA-256 (lowercase hex) of `<address>\n<token>`, where `<address>` is the server address as written in the settings, without spaces around it and without a final `/`. The token itself is never written there.
  - 3DS: one line `SERVEUR=<64 hex>` in `base.ini` (a 64-character value, like a Hash: older versions keep the line, as `INSTALL-`, ADR 0027).
  - PC: the reserved Key `SERVEUR` in each `bases/<emulator>.json` (a map Key → Hash). No Game Key has this form (ADR 0007), so an older version reads it as the Base of a Game that does not exist, and keeps it.
  - Android: the same reserved Key `SERVEUR`, stored with its Bases.
- **At the start of a pass** (scan of the list, Sync all, a launch), the device compares the stored fingerprint with the current one:
  - same: nothing changes;
  - none stored (first pass of this version, or no Bases yet): it is stored, the Bases are kept (an upgrade must not turn every game into a Conflict);
  - another one: every Base is removed (3DS: the `INSTALL-` dates stay), the new fingerprint is stored, and one message says so: "Le serveur a changé : les jeux qui diffèrent du serveur demanderont lequel garder." / "The server changed: games that differ from the server will ask which one to keep." The journal (`journal.log`, the PC log) gets a line, without the address or the token.
- Without a Base, the shared State rule (`docs/design.md`) gives: same Hash as the server → In sync; no Version on the server → To upload; a Version on the server that differs → Conflict. So nothing is replaced without a question, and nothing changes for a game that is already the same on both sides.
- Fingerprint change and sync happen in the same pass, the Bases removed first and written before any transfer.

## Rejected options

- **The address alone**: a new token on the same address can be a new server (reinstalled, fresh `data/`).
- **The token alone, or the token in clear**: the token is a secret (never shown, never logged); a hash is enough to compare.
- **One set of Bases per server, kept side by side**: going back to an old server is rare; a Conflict is the safe answer there too, and the files stay simple.
- **Ask the server for an identifier**: an older server has none, and every client would need a new request.

## Consequences

- A user who only changes the way the address is written (`192.168.1.20:8765` → `http://192.168.1.20:8765`, or a domain name for the same server) sees Conflicts for the games that differ from the server, once. Games that are the same stay In sync.
- Moving the token to another device keeps nothing secret on the SD card or the PC: the fingerprint can't give the token back (the token is random and long).
- Older versions ignore the line or field; a device that goes back to an older version uses its Bases as before.
