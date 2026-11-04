# Uninstall puts the frontends back

The setup makes LaunchBox and ES-DE start DS, GB, GBA and 3DS games through 3dssavesync: in LaunchBox, a copy "<emulator> (3dssavesync)" of each emulator becomes the default of its platforms; in ES-DE, a 3dssavesync command comes first. Deleting the program alone would leave these frontends pointing to a missing file: no game would start. So the uninstall undoes them.

- Settings page, part "Frontends and install", button **Uninstall 3dssavesync**, or `3dssavesync uninstall [--yes]`. LaunchBox, BigBox or ES-DE open: nothing is done (an open LaunchBox would write its emulators back when it closes).
- LaunchBox: each copy "<title> (3dssavesync)" whose program is the Launcher is removed with its platforms; the emulator it was copied from (same title without "(3dssavesync)") is the default again. `Emulators.xml` comes back exactly to its state before the setup when nothing else changed (test).
- LaunchBox, emulators changed in place: the first setup of versions 1 and 2 did not copy the emulator; it set the program of the user's own emulator (title unchanged, for example "MelonDS") to the Launcher, with `run --emulator <name>` on its platforms. Removing it would leave the games without an emulator. So an emulator whose program is the Launcher and whose title has no "(3dssavesync)" stays, with its platforms and their Default value. Its program is the real emulator again: `exe` of the config.json entry named by `--emulator <name>`, written relative to the LaunchBox folder when it is inside it (as LaunchBox writes it). The `run --emulator <name>` command lines become empty (the LaunchBox default). When that exe is unknown or missing, the emulator is left as is and the report says to fix it by hand in LaunchBox (Tools, Manage, Emulators, the emulator, Application Path).
- ES-DE: the 3DSSAVESYNC rule and the 3dssavesync commands are removed; a system left without any command was added by the setup and is removed, so ES-DE uses its own system again.
- Shortcuts that start the Launcher (desktop, menu, startup) and the installed program go to the Recycle Bin on Windows (the program after it closes, through a detached PowerShell); on Linux and macOS, the launchers created by the setup and the program are removed.
- Kept, and named in the report: the settings folder, the backup copies, the local server data. The emulators themselves were never changed (only command-line options), and the saves stay in their folders.

## Consequences

- A frontend changed by hand after the setup keeps the user's changes; only 3dssavesync entries go.
- The options that versions 1 and 2 replaced with `run --emulator <name>` are not known: the user adds them again in LaunchBox if needed.
- The 3DS app is removed with FBI; the uninstall only says how.
