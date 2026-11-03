# One Game = one Game code; file names ignored, slot 0 only, Duplicates ignored

TWiLight and melonDS name the `.sav` after the file name of the ROM, and these names differ between the 3DS and the PC (No-Intro names on the PC). The identity of a Game is therefore the Game code at offset `0x0C` of the ROM header, region included.

- Codes `####`, empty or non-ASCII (homebrews): excluded.
- Only slot 0 (`<ROM name>.sav`) is synced; TWiLight slots `.sav1`..`.sav9` are ignored.
- If a Client has several ROMs with the same Game code, the Game is marked Duplicate and is not synced: the tool never guesses which file is the right one.

## Consequences

- Two regions of the same title (`IRBF`, `IRBO`) are two distinct Games. Save compatibility between languages is not verified, so they are not paired.
- A ROM hack that keeps the Game code of the original game shares its identity: avoid it on one Client (it will show as Duplicate). Completed by ADR 0014 (refusal on the PC, alias).
