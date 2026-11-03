# Home-made tool (server + 3DS homebrew + Launcher) rather than an existing project

Three pieces written for this need: a Python Server, a 3DS homebrew in C, a PowerShell Launcher. No existing project covers 3DS + melonDS + public HTTPS + History + automatic Launcher (research of 2026-10-31, `docs/research.md` part 2 §3, French).

## Rejected options

- **DaeMoon** (mirusu400): good HTTPS 3DS client and Docker server, but no melonDS client. The PC would have had to reproduce its protocol (zip packages, manifest, digest, pairing); its CLI seems to speak plain HTTP only. More costly than our small 3DS client, and a dependency on a v0.1.2 by a single author.
- **GameSync** (pepasjc): 3DS + PC, but local HTTP only (`httpc`), graphical and non-automatic PC client, project covering 15 platforms.
- **ftpd + PC script**: no 3DS code, but the PC must be on and on the same network, ftpd must be started by hand, no History.

We take from DaeMoon the ideas verified on hardware (curl + mbedTLS, embedded ISRG root, Game code read in the ROM), not its code.
