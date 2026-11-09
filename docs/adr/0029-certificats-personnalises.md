# Custom CA certificates for the Server on the 3DS

The 3DS app trusts only ISRG Root X1 for the Server (ADR 0003). A user whose server has a certificate from a private CA, or a self-signed one, can't connect, and must switch to Let's Encrypt or to plain HTTP. Other 3DS homebrew (cloudpoint, for example) read extra roots from `sdmc:/config/ssl/cacert.pem`.

## Decision

- **One file, the shared path.** At start, after the network, the app reads `sdmc:/config/ssl/cacert.pem` once. It holds PEM certificates (a bundle, or one self-signed server certificate).
- **On top of ISRG, never instead.** Server requests trust ISRG Root X1 followed by the file, as one `CURLOPT_CAINFO_BLOB`. A Let's Encrypt server keeps working with the file present.
- **GitHub stays as it is.** Update checks and downloads keep ISRG + the GitHub roots (ADR 0021): a file on the SD card can't redirect an app update.
- **Limits.** 256 KiB at most: a larger file is ignored. A missing, empty or unreadable file changes nothing. A file that is ignored, and a file that is added, each give one line in `journal.log`.
- **No check of the content.** mbedTLS parses the blob: certificates it can't read are skipped, ISRG always parses, so a damaged file gives the same result as no file.

## Rejected options

- **A setting to turn certificate checks off**: the token would go to whoever answers, on any Wi-Fi.
- **A file in the app folder (`sdmc:/3ds/3dssavesync/`)**: a user with several homebrew would keep one copy per app; the shared path is already a habit.
- **Replace ISRG with the file**: a wrong file would cut a working Let's Encrypt server.
- **The same roots for GitHub**: a private CA could then sign a fake release; SHA256SUMS comes from the same host, so it would not catch it.

## Consequences

- The file is read only at start: after a change, start the app again.
- A private CA can sign a certificate for any name: the user trusts it for the Server, as for the other homebrew that read this file.
- The PC and Android clients use their system's certificate store; this file is for the 3DS only.
