# The Server never deletes or changes a Version

Each accepted Upload creates a new file `data/<Game code>/<seq>-<date>-<client>-<Hash>.sav`. The Current version is the one with the highest `<seq>`: no pointer to maintain, no dependency on the clock. No route deletes, overwrites or moves back the Current version. A Restore is done by sending an old Version again as a new Upload from a Client.

Reason: a Pokémon game is hundreds of hours. The worst case (stolen token, Client bug, wrong choice in a Conflict) must stay repairable by copying a file.

## Consequences

- `<seq>` is the `<seq>` of the last Version + 1: removing a file by hand does not break the order.
- The Server refuses a body shorter than its `Content-Length` (cut connection): never a truncated Version.
- The disk grows without limit: about 512 KiB per Upload, a few hundred MiB per year at worst. No purge in v1.
- The History lives on the same disk as the Server; the `data/` folder must be part of an external backup (restic, Backrest, etc.).
- Test data is moved out of `data/` (`quarantaine-tests/`), never deleted by a script.
