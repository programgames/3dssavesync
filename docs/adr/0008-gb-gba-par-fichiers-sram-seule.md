# GB, GBC and GBA: sync the SRAM only, keep the launchers already in use

Survey of 2026-11-01: on the 3DS, GBA already runs under open_agb_firm (`useSavesFolder=false`, so a raw `.sav` next to the ROM in `sd:/roms/gba/`) and GB/GBC under TWiLight (`.sav` next to the ROM in `sd:/roms/gb/`). On the PC, mGBA. These are ordinary files, like the DS `.sav`: v2 syncs them the same way, without changing emulator.

The only format difference is the real-time clock: mGBA adds a Clock trailer at the end of the `.sav` (48 bytes GB MBC3+TIMER, 16 bytes GBA), open_agb_firm takes the clock of the 3DS, TWiLight writes none. Rule:

- the SRAM is the "game" part: `size - (size mod 1024)` (trailers are smaller than 1 KiB); replaced by ADR 0010 (remainder modulo 512);
- the Hash, the compared size and the uploaded Version cover **the SRAM only**;
- at Download, the Client writes the received SRAM **then puts back its own Clock trailer** if it had one: each machine keeps its clock.

## Rejected options

- **mGBA 3DS everywhere** (forwarders with a HOME icon): files identical to the PC, but changes the 3DS emulator, hides the ROM in each CIA (no readable Key) and requires mGBA 0.11-dev. Kept as an optional step.
- **Convert the trailers between emulators**: different formats (GameYob does not update the fields mGBA reads), small gain, real risk. Each machine keeps its clock instead.
- **Virtual Console injects**: GBA save in the NAND (AGBSAVE), extractable only with GodMode9.

## Consequences

- Gold, Silver, Crystal (clock): the in-game time can differ from one machine to the other; the game progress itself is the same.
- open_agb_firm writes the `.sav` only at power-off with POWER: sync at the next start.
