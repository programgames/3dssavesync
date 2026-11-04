# Sync without the Launcher, from the icon near the clock

A save synced only when the game started through the Launcher (frontend, shortcut, **Open** button). A game played by opening mGBA or melonDS directly never uploaded, and a Version from the 3DS waited until the next start through the Launcher.

- The tray process (ADR 0022) checks every 10 s which configured emulators run (`instances`, the same check as the Launcher's refusal). When an emulator's program closes, it syncs the games of that emulator; every 15 minutes (first pass one minute after the session starts), it syncs every closed emulator.
- It is the Sync of **Sync all**: `syncGame` on each game of `gamesOf`, after the pending Uploads. The 3-way state decides: an Upload when only the PC changed; a Download only when the local save is the Base (unchanged since the last Sync), after a Backup copy, through the verified temporary file. A Conflict is never asked and nothing is replaced: a notification names the game, and its next start through the Launcher asks.
- Skipped: an emulator whose program runs, or one with a Launcher lock (`en-cours/`), so that a Launcher's own Sync before or after a game is never doubled. One Sync at a time: the menu items and the watcher share one lock.
- Notifications: only when a save moved, or when the problems differ from the last notified ones (a network failure is not repeated every 15 minutes).
- On by default, also for existing users (`"synchro_auto"` absent); a switch in **Frontends and install** turns it off. It needs the icon: with the icon off, nothing watches.

## Consequences

- A Download can happen while you are not in front of the PC. It only ever replaces a save that did not change since the last Sync, and a Backup copy is made first.
- Race left: you start an emulator during the few seconds of a Download into its save. The emulator then reads the old file or the new one, never a half-written file (temporary file and rename). If it read the old one, your next save is a Conflict at the next Sync, and nothing is lost.
- The full pass reads the ROM headers of every configured folder every 15 minutes; with very large collections on a slow disk this costs some I/O. A watch on file changes (instead of a timer) would need a new dependency or per-OS code.
- macOS has no icon yet (ADR 0022), so no Sync without the Launcher.
