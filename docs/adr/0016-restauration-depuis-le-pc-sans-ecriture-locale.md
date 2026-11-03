# Restore from the PC, without local write

The Restore (ADR 0004) is done on the PC only: "History" part of the settings page, and `3dssavesync restore`. Nothing on the 3DS.

The Client reads the old Version (`GET /saves/<key>/history/<version>`), checks its Hash, then uploads it as a new Version with `If-Match` = the Hash of the Current version the user saw in the list. It writes no local file and changes no Base.

Reason: the Base of each device stays the Hash of its last sync. The restored Version is therefore, for most devices, a newer Version (`L == B`, `S != B`: To download). Exception: a device whose Base equals the restored Version (it never got the later Versions) sees `L != B`… or `S == B`, i.e. To upload, and its next sync brings back its own game and undoes the Restore. Each device gets it through its normal Download, with its Backup copy. Writing the PC Local save at Restore time would require choosing an emulator, checking that it is not running and making the Backup copy again: this is already the job of the Download.

## Consequences

- An Upload that arrives between the display of the list and the Restore gives `412`: the Restore is refused, nothing is overwritten. The user reloads the list.
- A Version with the same content as the current one: nothing is uploaded.
- Sync the other devices before restoring. Otherwise a device with a game not yet uploaded goes to Conflict, or (see Reason) brings back its unsent game; the page says so before the Restore.
- A Version with a different size from the Local save also gives a Conflict (ADR 0001).
- `X-Client: restauration`: the History shows that a Version comes from a Restore.
- Versions are numbered by their server `<seq>`: a number never changes, even if a Version arrives between the list and the command.
- List limited to the last 30 Versions. For a Pokémon game, each Version of the list is read in full (play time, trainer): up to 30 reads per list.
