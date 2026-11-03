"""Serveur personnel 3dssavesync : Historique des Sauvegardes DS, GB, GBA et 3DS (docs/design.md §2, ADR 0004).

Variables d'environnement (préfixe NDSSYNC_ d'avant la v3, gardé pour les installations existantes) : NDSSYNC_TOKEN (obligatoire), NDSSYNC_DATA (défaut /data), NDSSYNC_PORT (défaut 8080).
Une Version = data/<code>/<seq>-<date>-<client>-<hash>.sav ; la Version courante a le plus grand <seq>.
Rien n'est jamais supprimé ni réécrit, sauf data/<code>/nom.txt : le nom du jeu donné par un Client, pour l'Historique.
"""
import calendar
import hashlib
import hmac
import json
import os
import re
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Clé d'un Jeu (ADR 0007) : DS, GBA, GB/GBC, 3DS. Jamais de "/" ni de "." : sûre comme nom de dossier.
KEY = r"([A-Z0-9]{4}|GBA-[A-Z0-9]{4}|GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}|3DS-[0-9A-F]{16})"
SAVE = re.compile(rf"^/saves/{KEY}$")
HISTORY = re.compile(rf"^/saves/{KEY}/history(/.*)?$")
NAME = re.compile(rf"^/saves/{KEY}/name$")
CLIENT = re.compile(r"^[a-z0-9]{1,16}$")
MAX_SIZE = 64 * 1024 * 1024  # plus grande mémoire de sauvegarde DS connue
LOCK = threading.Lock()  # ponytail: verrou global, suffit pour 2 Clients


def creer(address, data, token):
    def versions(code):
        folder = os.path.join(data, code)
        if not os.path.isdir(folder):
            return []
        return sorted(f for f in os.listdir(folder) if f.endswith(".sav"))

    def current(code):
        v = versions(code)
        return v[-1] if v else None

    def hash_of(version):
        return version[:-4].rsplit("-", 1)[1]

    def info(code, version):
        _, date, client, h = version[:-4].split("-")
        size = os.path.getsize(os.path.join(data, code, version))
        # age : calculé avec l'horloge du serveur, car celle de la 3DS n'a pas de fuseau horaire
        age = int(time.time() - calendar.timegm(time.strptime(date, "%Y%m%dT%H%M%SZ")))
        return {"code": code, "version": version, "hash": h, "taille": size, "date": date, "client": client, "age": age}

    def with_name(code, element):
        try:
            with open(os.path.join(data, code, "nom.txt"), encoding="utf-8") as f:
                return {**element, "nom": f.read()}
        except OSError:
            return element

    class Handler(BaseHTTPRequestHandler):
        def authorized(self):
            received = self.headers.get("Authorization", "")
            if hmac.compare_digest(received.encode(), f"Bearer {token}".encode()):
                return True
            self.respond(401)
            return False

        def respond(self, status, body=b"", **headers):
            self.send_response(status)
            self.send_header("Content-Length", str(len(body)))
            for name, value in headers.items():
                self.send_header(name.replace("_", "-"), value)
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            if not self.authorized():
                return
            if self.path == "/saves":
                codes = sorted(os.listdir(data)) if os.path.isdir(data) else []
                return self.json([with_name(c, info(c, v)) for c in codes if (v := current(c))])
            m = HISTORY.match(self.path)
            if m and not m[2]:
                return self.json([info(m[1], v) for v in versions(m[1])])
            if m:
                v = m[2][1:] if m[2][1:] in versions(m[1]) else None
            else:
                m = SAVE.match(self.path)
                v = m and current(m[1])
            if not v:
                return self.respond(404)
            with open(os.path.join(data, m[1], v), "rb") as f:
                self.respond(200, f.read(), ETag=f'"{hash_of(v)}"')

        def json(self, value):
            self.respond(200, json.dumps(value).encode(), Content_Type="application/json")

        def do_PUT(self):
            length = self.headers.get("Content-Length", "")
            size = int(length) if length.isdigit() else 0
            if not 0 < size <= MAX_SIZE:
                self.close_connection = True
                return self.respond(400)
            # Lire le corps avant toute réponse : sinon le client reçoit un reset au lieu du statut.
            body = self.rfile.read(size)
            if len(body) != size:  # connexion coupée : jamais de Version tronquée
                self.close_connection = True
                return self.respond(400)
            if not self.authorized():
                return
            m = NAME.match(self.path)
            if m:
                return self.set_name(m[1], body)
            m = SAVE.match(self.path)
            if not m:
                return self.respond(400)
            if_match, if_none_match = self.headers.get("If-Match"), self.headers.get("If-None-Match")
            if if_match is None and if_none_match != "*":
                return self.respond(428)
            h = hashlib.sha256(body).hexdigest()
            client = self.headers.get("X-Client", "")
            client = client if CLIENT.match(client) else "inconnu"
            with LOCK:
                v = current(m[1])
                expected = f'"{hash_of(v)}"' if v else None
                if (if_match or None) != expected or (if_none_match == "*" and v):
                    return self.respond(412, **({"ETag": expected} if v else {}))
                if v and hash_of(v) == h:
                    return self.respond(200, ETag=expected)
                folder = os.path.join(data, m[1])
                name = f"{int(v[:6]) + 1 if v else 1:06d}-{time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())}-{client}-{h}.sav"
                tmp = os.path.join(folder, name + ".tmp")
                try:
                    os.makedirs(folder, exist_ok=True)
                    with open(tmp, "wb") as f:
                        f.write(body)
                        f.flush()
                        os.fsync(f.fileno())  # une Version acceptée doit survivre à une coupure
                    os.replace(tmp, os.path.join(folder, name))
                except OSError:  # disque plein, droits : le Client reçoit une erreur, pas une connexion coupée
                    return self.respond(500)
            self.respond(200, ETag=f'"{h}"')

        def set_name(self, code, body):
            """Nom du jeu (nom de la ROM ou titre 3DS) : seulement pour un jeu qui a déjà une Version."""
            try:
                name = body.decode("utf-8").strip()
            except UnicodeDecodeError:
                name = ""
            if not 0 < len(name) <= 200 or any(c < " " or c == "\x7f" for c in name):
                return self.respond(400)
            if not current(code):
                return self.respond(404)
            with LOCK:
                tmp = os.path.join(data, code, "nom.txt.tmp")
                try:
                    with open(tmp, "w", encoding="utf-8") as f:
                        f.write(name)
                        f.flush()
                        os.fsync(f.fileno())
                    os.replace(tmp, os.path.join(data, code, "nom.txt"))
                except OSError:
                    return self.respond(500)
            self.respond(204)

    return ThreadingHTTPServer(address, Handler)


if __name__ == "__main__":
    creer(("0.0.0.0", int(os.environ.get("NDSSYNC_PORT", "8080"))),
          os.environ.get("NDSSYNC_DATA", "/data"),
          os.environ["NDSSYNC_TOKEN"]).serve_forever()
