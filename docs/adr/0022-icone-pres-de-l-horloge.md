# Icon near the clock, one resident program

The settings were only reachable through a shortcut, and the local server ran as an invisible `serve` process: nothing showed that it was running, or that uploads were pending. Users asked for an icon near the clock.

- New command `3dssavesync tray`. Its menu: **Open settings** (starts `setup`, detached), **Send now (N pending)** (only when `en-attente.json` has entries; same pass as at a game launch, then a notification), a grey line with the local server state, **Quit**. Windows: double-click opens the settings; Linux: left click.
- The tray process runs the local server itself (same code as `serve`). One autostart entry, **3dssavesync**, starts `tray`; the old **3dssavesync serveur** entry is removed, so the server never starts twice. Icon turned off (`"icone_horloge": false`) and local server on: the entry starts `serve`, as before. Default: icon on, for new and existing users (the field is absent).
- Windows: raw Win32 through `syscall` (`Shell_NotifyIconW`, a hidden window, `TrackPopupMenu`), icon from the embedded `icone.ico`. A hidden top-level window, not a message-only one: only the first gets `TaskbarCreated`, to show the icon again after Explorer restarts.
- Linux: StatusNotifierItem and `com.canonical.dbusmenu` on the session D-Bus, with `github.com/godbus/dbus/v5` (pure Go, no dependency of its own; still builds with `CGO_ENABLED=0`). The program's only dependency. KDE Plasma shows the icon; GNOME needs the AppIndicator extension. No watcher: a message says so, and the local server keeps running.
- macOS: no icon yet; `tray` says so, autostart keeps `serve`.
- One icon per settings folder: a named mutex (Windows) or `flock` on `tray.verrou` (Linux). A second `tray` exits without a message.
- Stop request: the file `arret-tray` in the settings folder, read every 2 s. **Save** in the settings stops the icon and starts it again with the new settings (port, server, icon). The uninstall stops it, so that the exe can go to the Recycle Bin.

## Consequences

- Existing users get the icon at their next **Save** in the settings (the update goes through the settings page): the autostart entry changes only there.
- After the update, a `serve` started by the old entry keeps running until the next restart; the new tray sees the port in use and an answering server, and shows it as running.
- Each **Save** restarts the icon (up to about 2 s).
- Quit stops the local server too, until the next session.
- Not done: a notification balloon from the icon itself (the usual notifications are used), macOS (a Cocoa status item needs cgo or a lot of raw Objective-C calls).
