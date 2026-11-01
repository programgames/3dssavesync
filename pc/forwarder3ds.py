"""Crée un forwarder mGBA pour la 3DS (un .cia par jeu), en ligne de commande.

Reprend les étapes de mGBA 0.11-dev (src/platform/qt/ForwarderGenerator3DS.cpp, MPL 2.0), sans l'interface :
- chaque outil externe est vérifié (mGBA ignore les échecs, d'où l'icône mGBA restée sur nos forwarders) ;
- titre et icône écrits par cet outil dans le SMDH (pas d'accents perdus par bannertool sous Windows).

Outils (forwarder-kit de mGBA) : ctrtool, 3dstool, bannertool, makerom dans --outils.
Base : le .cia du build 3DS « dev » de mGBA (https://mgba.io/latest.ini, platform.3ds, dev.name).

  python forwarder3ds.py --rom "X.gba" --titre "Pokémon Rubis" --icone i.png --banniere b.png --sortie Rubis.cia
"""
import argparse, hashlib, os, shutil, struct, subprocess, sys, tempfile

MGBA_DEV = os.environ.get("MGBA_DEV", "")  # dossier de mGBA 0.11-dev (outils du forwarder-kit)


def lancer(outils, *args):
    r = subprocess.run([os.path.join(outils, args[0] + ".exe"), *args[1:]], capture_output=True, text=True, errors="replace")
    if r.returncode:
        sys.exit(f"échec : {' '.join(args)}\n{r.stdout}{r.stderr}")
    return r.stdout


def base36(octets, n):  # identique à ForwarderGenerator::base36
    alpha, res, courant, j = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ", "", 0, 0
    for _ in range(n):
        if courant < 36:
            courant = (courant << 8 | octets[j]) & 0xFFFFFFFF
            j += 1
        res += alpha[courant % 36]
        courant //= 36
    return res


def ecrire_titre(smdh, titre):
    """Titre court et long, toutes langues, en UTF-16 (SMDH : 16 × 0x200 octets à partir de 0x8)."""
    court = titre.encode("utf-16-le")[:0x7E].ljust(0x80, b"\0")
    long_ = titre.encode("utf-16-le")[:0xFE].ljust(0x100, b"\0")
    with open(smdh, "r+b") as f:
        assert f.read(4) == b"SMDH"
        for langue in range(16):
            f.seek(0x8 + langue * 0x200)
            f.write(court + long_)


def creer(rom, titre, icone, banniere, sortie, outils, base):
    w = tempfile.mkdtemp(prefix="forwarder3ds-")
    j = lambda chemin: os.path.normpath(os.path.join(w, chemin))  # ctrtool refuse les « / » mêlés aux « \ »
    try:
        sortie_ctr = lancer(outils, "ctrtool", f"--contents={j('cxi')}", base)
        index = [l.strip()[-8:] for l in sortie_ctr.splitlines() if "|- ContentId:" in l][-1]
        cxi = j(f"cxi.0000.{index}")
        lancer(outils, "3dstool", "-xf", cxi, "--exh", j("exheader.bin"), "--header", j("header.bin"), "--exefs", j("exefs.bin"))
        lancer(outils, "3dstool", "-xf", j("exefs.bin"), "--header", j("exeheader.bin"), "--exefs-dir", j("exefs"))

        # Title ID et code produit dérivés du hash de la ROM (même jeu = même titre installé, mis à jour par FBI).
        h = hashlib.sha256(open(rom, "rb").read()).digest()
        tid = struct.pack("<I", (struct.unpack("<I", h[:4])[0] & 0x7FFFFFF) + 0x0300000)
        with open(j("header.bin"), "r+b") as f:
            for o in (0x108, 0x118):
                f.seek(o); f.write(tid)
            f.seek(0x150); f.write(("MGBA-" + base36(h, 11)).encode())
            f.seek(0x18D); t = bytearray(f.read(3)); t[0] |= 1; t[2] &= ~2 & 0xFF  # romfs présent et montable
            f.seek(0x18D); f.write(t)
        with open(j("exheader.bin"), "r+b") as f:
            for o in (0x1C8, 0x200, 0x600):
                f.seek(o); f.write(tid)

        # RomFS : la ROM et son nom (le .sav du forwarder porte ce nom : sdmc:/mGBA/forwarders/<nom>.sav).
        os.makedirs(j("romfs"))
        open(j("romfs/filename"), "wb").write(os.path.basename(rom).encode("utf-8"))
        shutil.copy(rom, j("romfs"))
        lancer(outils, "3dstool", "-cf", j("romfs.bin"), "-t", "romfs", "--romfs-dir", j("romfs"))

        icn = j("exefs/icon.icn")
        os.remove(icn)  # jamais l'icône de la base par erreur
        lancer(outils, "bannertool", "makesmdh", "-s", "forwarder", "-l", "forwarder", "-p", "mGBA Forwarder", "-i", icone, "-o", icn)
        ecrire_titre(icn, titre)

        bnr = j("exefs/banner.bnr")
        b = open(bnr, "rb").read()
        open(j("banner.bcwav"), "wb").write(b[struct.unpack("<I", b[0x84:0x88])[0]:])  # le son de la base
        os.remove(bnr)
        lancer(outils, "bannertool", "makebanner", "-i", banniere, "-ca", j("banner.bcwav"), "-o", bnr)

        lancer(outils, "3dstool", "-cf", j("exefs.bin"), "-t", "exefs", "--header", j("exeheader.bin"), "--exefs-dir", j("exefs"))
        os.remove(cxi)
        lancer(outils, "3dstool", "-cf", cxi, "-t", "cxi", "--exh", j("exheader.bin"), "--header", j("header.bin"),
               "--exefs", j("exefs.bin"), "--romfs", j("romfs.bin"))
        lancer(outils, "makerom", "-f", "cia", "-o", sortie, "-content", f"{cxi}:0:0")
        print(f"{sortie} : title ID 00040000{struct.unpack('<I', tid)[0]:08X}, {os.path.getsize(sortie)} o")
    finally:
        shutil.rmtree(w, ignore_errors=True)


if __name__ == "__main__":
    p = argparse.ArgumentParser(description="Forwarder mGBA 3DS (.cia) pour une ROM GB/GBC/GBA.")
    p.add_argument("--rom", required=True)
    p.add_argument("--titre", required=True)
    p.add_argument("--icone", required=True, help="PNG 48×48")
    p.add_argument("--banniere", required=True, help="PNG 256×128")
    p.add_argument("--sortie", required=True)
    p.add_argument("--outils", default=MGBA_DEV, required=not MGBA_DEV)
    p.add_argument("--base", required=True, help=".cia du build 3DS dev de mGBA")
    a = p.parse_args()
    creer(a.rom, a.titre, a.icone, a.banniere, a.sortie, a.outils, a.base)
