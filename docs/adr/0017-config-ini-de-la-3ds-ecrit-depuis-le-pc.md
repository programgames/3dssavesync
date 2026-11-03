# 3DS config.ini written from the PC, router address first

First start: the PC writes the server address and the token to `3ds/3dssavesync/config.ini` on the 3DS SD card (settings page, "Prepare the 3DS SD card"; `setup --texte`, step "3DS SD card"). The 3DS reads this file at start and connects without typing anything.

- Only `config.ini` is written. A root without a `Nintendo 3DS` folder is refused. The old file is copied first (`config.ini.avant-3dssavesync-<date>`), its other lines (`roms`, `saves`…) are kept; write through a verified temporary file, rename, read back.
- Old folder `3ds/nds-save-sync` alone present: `config.ini` is written there. The homebrew takes it over at start with `base.ini` (`migrate_dir`); a new folder created next to it would prevent this takeover and lose the Bases.
- "Download config.ini" serves the same file, without the other lines, for a copy by FTP.

Local server address for the 3DS: the list of private IPv4 addresses of the PC, sorted. First an address that does not end in `.1` (the PC side of a virtual network: WSL, Docker, VirtualBox, VMware), then an address in `192.168.` (home router range), then the default route address. The default route alone is not enough: an active VPN (NordVPN, WireGuard) takes it, and its address does not lead to the PC from the 3DS. The other addresses stay shown under "Other addresses".

## Consequences

- A router in `10.x` with an active VPN on the PC: the VPN address can come first. The right address is in "Other addresses".
- The PC changes address (DHCP): prepare the card again, or set a DHCP reservation in the router.
