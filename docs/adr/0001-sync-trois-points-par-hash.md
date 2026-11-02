# Three-way sync by Hash, never by date

Each Client keeps a Base per Game and compares three Hashes: Local save (L), Current version (S), Base (B). File modification dates cannot be used: nds-bootstrap writes the sectors of the `.sav` without updating the FAT entry, so the `mtime` does not move when you play on the 3DS (see `docs/research.md`, part 1 §1, French), and the 3DS clock can be set by hand.

| Case | State |
|---|---|
| L == S | In sync (and B := L) |
| S absent | To upload |
| L absent | To download |
| size(L) != size(S) | Conflict |
| L == B and S != B | To download |
| S == B and L != B | To upload |
| otherwise | Conflict |

## Consequences

- A different size always gives a Conflict: melonDS truncates or pads a `.sav` to the size of its database, and nds-bootstrap writes modulo the file size. Syncing a wrong size can corrupt the game.
- The Upload carries `If-Match: <S as seen by the Client>`. If another Client uploaded in the meantime, the Server answers `412` and the Client goes back to Conflict. No Conflict is ever resolved automatically.
