# The Launcher replaces melonDS in LaunchBox, without file watching

LaunchBox calls `powershell.exe -File sync.ps1 <rom>` instead of `melonDS.exe`. The Launcher does the Download, starts melonDS, waits for the process to end, then does the Upload.

Reason: melonDS keeps the Save in memory, rewrites the whole file 2 to 3 s after each in-game save (non-atomic write), and does not lock the file (`docs/research.md`, part 1 §2, French). Reading the `.sav` while melonDS runs can give a truncated file; writing it while it runs is lost at the next in-game save. Only the end of the process is a safe point.

## Rejected options

- **Folder watching** (a service that uploads on each change): reads half-written files, and cannot download before the game.
- **Plugin / Lua script**: melonDS has no plugin system.
- **One shortcut per game**: LaunchBox already starts all DS games; a single emulator entry covers the 3710 ROMs.

## Consequences

- The Launcher refuses to start if `melonDS.exe` is already running.
- Loading a melonDS savestate rewrites the `.sav` with an old Save; the Launcher cannot see it. To be noted in the README.
