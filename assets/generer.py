"""Génère les PNG et le .ico depuis icone.svg et banniere.svg.

Usage (Windows, Microsoft Edge et Pillow) : python assets/generer.py
Edge dessine chaque taille depuis le SVG (net à 16 px), Pillow découpe et assemble le .ico.
Ensuite, icône de l'exe Windows : cd client && go generate
"""
import os
import pathlib
import subprocess
import tempfile

from PIL import Image

ICI = pathlib.Path(__file__).resolve().parent
RACINE = ICI.parent
TAILLES = [16, 24, 32, 48, 64, 128, 256]
EDGE = [p for p in (r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe",
                    r"C:\Program Files\Microsoft\Edge\Application\msedge.exe") if os.path.exists(p)][0]

icone = (ICI / "icone.svg").read_text(encoding="utf-8")
banniere = (ICI / "banniere.svg").read_text(encoding="utf-8")

# Une seule page : chaque taille à une position connue, puis découpage.
places, x = [], 0
for t in TAILLES:
    places.append((x, t, icone, t, t))
    x += t
places.append((x, 0, banniere, 256, 128))
html = "".join(f'<div style="position:absolute;left:{x}px;top:0;width:{w}px;height:{h}px">'
               f'{svg.replace("<svg ", f"<svg width={w!r} height={h!r} ", 1)}</div>'
               for x, _, svg, w, h in places)
html = f'<!doctype html><meta charset="utf-8"><body style="margin:0;background:transparent">{html}</body>'

with tempfile.TemporaryDirectory() as tmp:
    page, capture = pathlib.Path(tmp, "page.html"), pathlib.Path(tmp, "capture.png")
    page.write_text(html, encoding="utf-8")
    subprocess.run([EDGE, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--force-device-scale-factor=1",
                    "--default-background-color=00000000", f"--user-data-dir={tmp}\\profil",
                    f"--window-size={x + 256},256", f"--screenshot={capture}", page.as_uri()],
                   check=True, capture_output=True)
    planche = Image.open(capture).convert("RGBA")
    planche.load()

images = {t: planche.crop((x, 0, x + t, t)) for x, t, _, _, _ in places[:-1]}
bx = places[-1][0]
(RACINE / "client" / "icone").mkdir(exist_ok=True)
images[256].save(RACINE / "client" / "icone" / "icone-256.png", optimize=True)
images[256].save(RACINE / "client" / "icone" / "icone.ico", sizes=[(t, t) for t in TAILLES],
                 append_images=[images[t] for t in TAILLES if t != 256])
# L'icône SMDH n'a pas de transparence : coins carrés, remplis du fond.
Image.alpha_composite(Image.new("RGBA", (48, 48), "#182848"), images[48]).convert("RGB").save(
    RACINE / "3ds" / "meta" / "icon.png", optimize=True)
planche.crop((bx, 0, bx + 256, 128)).convert("RGB").save(RACINE / "3ds" / "meta" / "banner.png", optimize=True)
print("ok")
