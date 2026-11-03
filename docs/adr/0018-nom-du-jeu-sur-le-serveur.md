# Game name stored on the server

The History page showed server Keys (`ADAF`, `GB-POKEMON_BLUE-56A4`): the server knows only Keys, and the PC finds names only in ROM folders, which LaunchBox users do not set. The devices that play a game know its name (ROM file name, or 3DS title), so they give it to the server.

- `PUT /saves/<code>/name`, body = the name in UTF-8 (1 to 200 characters, no control character). `204`; `404` if the game has no Version; `400` otherwise. No precondition: the name is not a save and creates no Version.
- The server keeps it in `data/<code>/nom.txt` (the only file it rewrites) and adds `"nom"` to each entry of `GET /saves` when it knows one.
- The 3DS sends the name of each listed game the server does not name yet, once, after `GET /saves`. It stops at the first refusal: an older server answers `400`, and the 3DS works as before.
- The PC sends it during the sync of a launched game, when the server has no name. A failure is only logged.
- The History page uses, in order: 3DS title table, ROM folders, server name, Key.

## Consequences

- The last device that names a game wins: two ROM files with the same Key (renamed copy) can swap the name. The name is only for display.
- Games played only on a device with an old version keep their Key until another device names them.
