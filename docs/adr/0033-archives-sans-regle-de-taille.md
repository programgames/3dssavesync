# 3DS archives: no size rule in the State

The shared State rule (`docs/design.md`, ADR 0001) gives a Conflict when the Local save and the Current version have different sizes, even when one of them is the Base. For a raw save this protects the game: melonDS pads or cuts a `.sav` to its own size, and a save of another size can corrupt it. ADR 0028 and ADR 0031 kept this check for 3DS archives (`3DSA-`, `3DSX-`) to ask instead of a silent Download when a game adds a file.

Real use showed the cost. Pokémon X and Y keep their battle videos in their extdata (`user/btvideo/000`, `001`, …): each session that records a battle adds a file, so the archive grows. After one Upload, every next session on the same PC made a Conflict, although the server had not changed since the Base. The automatic sync can't ask, so the user only saw "Conflict on Pokemon X (data): start this game", and nothing was synced.

## Decision

- For a `3DSA-` or `3DSX-` Key, the State skips the size rule (rule 4). Rules 5 to 7 decide: the side that equals the Base is the old one. A raw save (DS, GB, GBA, the 8 Pokémon `/main`, Virtual Console) keeps the size rule.
- The State function of the three clients takes this as a parameter (`archive`), from the Key. `tests/cas_etat.csv` has an `archive` column and four archive cases.

## Rejected options

- **Drop the size rule for every save**: a DS or GB save of another size is a real danger (ADR 0001).
- **Keep it and ask the user**: the automatic sync and Sync all can't ask; the question waits for the next start of a game, and the extdata of a twin game had no game that asked (fixed in the same change: the PC reads the extdata ID in the ROM).

## Consequences

- An archive of another size is uploaded or downloaded without a question when only one side changed. The safety nets stay: the Backup copy before each write, the read back, the cut-write mark (ADR 0028, 0031).
- A device of an older version still asks for these archives; nothing breaks.
