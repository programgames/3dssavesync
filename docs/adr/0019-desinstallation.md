# Uninstall puts the frontends back

The setup makes LaunchBox and ES-DE start DS, GB, GBA and 3DS games through 3dssavesync: in LaunchBox, a copy "<emulator> (3dssavesync)" of each emulator becomes the default of its platforms; in ES-DE, a 3dssavesync command comes first. Deleting the program alone would leave these frontends pointing to a missing file: no game would start. So the uninstall undoes them.

- Settings page, part "Frontends and install", button **Uninstall 3dssavesync**, or `3dssavesync uninstall [--oui]`. LaunchBox, BigBox or ES-DE open: nothing is done (an open LaunchBox would write its emulators back when it closes).
- LaunchBox: each emulator whose program is the Launcher is removed with its platforms; the emulator it was copied from (same title without "(3dssavesync)") is the default again. `Emulators.xml` comes back exactly to its state before the setup when nothing else changed (test).
- ES-DE: the 3DSSAVESYNC rule and the 3dssavesync commands are removed; a system left without any command was added by the setup and is removed, so ES-DE uses its own system again.
- Shortcuts that start the Launcher (desktop, menu, startup) and the installed program go to the Recycle Bin on Windows (the program after it closes, through a detached PowerShell); on Linux and macOS, the launchers created by the setup and the program are removed.
- Kept, and named in the report: the settings folder, the backup copies, the local server data. The emulators were never changed (only command-line options), and the saves stay in their folders.

## Consequences

- A frontend changed by hand after the setup keeps the user's changes; only 3dssavesync entries go.
- The 3DS app is removed with FBI; the uninstall only says how.
