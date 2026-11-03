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
CLE = r"([A-Z0-9]{4}|GBA-[A-Z0-9]{4}|GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}|3DS-[0-9A-F]{16})"
CODE = re.compile(rf"^/saves/{CLE}$")
HISTORIQUE = re.compile(rf"^/saves/{CLE}/history(/.*)?$")
NOM = re.compile(rf"^/saves/{CLE}/name$")
CLIENT = re.compile(r"^[a-z0-9]{1,16}$")
TAILLE_MAX = 64 * 1024 * 1024  # plus grande mémoire de sauvegarde DS connue
VERROU = threading.Lock()  # ponytail: verrou global, suffit pour 2 Clients


def creer(adresse, data, jeton):
    def versions(code):
        dossier = os.path.join(data, code)
        if not os.path.isdir(dossier):
            return []
        return sorted(f for f in os.listdir(dossier) if f.endswith(".sav"))

    def courante(code):
        v = versions(code)
        return v[-1] if v else None

    def hash_de(version):
        return version[:-4].rsplit("-", 1)[1]

    def info(code, version):
        _, date, client, h = version[:-4].split("-")
        taille = os.path.getsize(os.path.join(data, code, version))
        # age : calculé avec l'horloge du serveur, car celle de la 3DS n'a pas de fuseau horaire
        age = int(time.time() - calendar.timegm(time.strptime(date, "%Y%m%dT%H%M%SZ")))
        return {"code": code, "version": version, "hash": h, "taille": taille, "date": date, "client": client, "age": age}

    def avec_nom(code, element):
        try:
            with open(os.path.join(data, code, "nom.txt"), encoding="utf-8") as f:
                return {**element, "nom": f.read()}
        except OSError:
            return element

    class Handler(BaseHTTPRequestHandler):
        def autorise(self):
            recu = self.headers.get("Authorization", "")
            if hmac.compare_digest(recu.encode(), f"Bearer {jeton}".encode()):
                return True
            self.repondre(401)
            return False

        def repondre(self, statut, corps=b"", **entetes):
            self.send_response(statut)
            self.send_header("Content-Length", str(len(corps)))
            for nom, valeur in entetes.items():
                self.send_header(nom.replace("_", "-"), valeur)
            self.end_headers()
            self.wfile.write(corps)

        def do_GET(self):
            if not self.autorise():
                return
            if self.path == "/saves":
                codes = sorted(os.listdir(data)) if os.path.isdir(data) else []
                return self.json([avec_nom(c, info(c, v)) for c in codes if (v := courante(c))])
            m = HISTORIQUE.match(self.path)
            if m and not m[2]:
                return self.json([info(m[1], v) for v in versions(m[1])])
            if m:
                v = m[2][1:] if m[2][1:] in versions(m[1]) else None
            else:
                m = CODE.match(self.path)
                v = m and courante(m[1])
            if not v:
                return self.repondre(404)
            with open(os.path.join(data, m[1], v), "rb") as f:
                self.repondre(200, f.read(), ETag=f'"{hash_de(v)}"')

        def json(self, valeur):
            self.repondre(200, json.dumps(valeur).encode(), Content_Type="application/json")

        def do_PUT(self):
            longueur = self.headers.get("Content-Length", "")
            taille = int(longueur) if longueur.isdigit() else 0
            if not 0 < taille <= TAILLE_MAX:
                self.close_connection = True
                return self.repondre(400)
            # Lire le corps avant toute réponse : sinon le client reçoit un reset au lieu du statut.
            corps = self.rfile.read(taille)
            if len(corps) != taille:  # connexion coupée : jamais de Version tronquée
                self.close_connection = True
                return self.repondre(400)
            if not self.autorise():
                return
            m = NOM.match(self.path)
            if m:
                return self.nommer(m[1], corps)
            m = CODE.match(self.path)
            if not m:
                return self.repondre(400)
            si_egal, si_absent = self.headers.get("If-Match"), self.headers.get("If-None-Match")
            if si_egal is None and si_absent != "*":
                return self.repondre(428)
            h = hashlib.sha256(corps).hexdigest()
            client = self.headers.get("X-Client", "")
            client = client if CLIENT.match(client) else "inconnu"
            with VERROU:
                v = courante(m[1])
                attendu = f'"{hash_de(v)}"' if v else None
                if (si_egal or None) != attendu or (si_absent == "*" and v):
                    return self.repondre(412, **({"ETag": attendu} if v else {}))
                if v and hash_de(v) == h:
                    return self.repondre(200, ETag=attendu)
                dossier = os.path.join(data, m[1])
                nom = f"{int(v[:6]) + 1 if v else 1:06d}-{time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())}-{client}-{h}.sav"
                tmp = os.path.join(dossier, nom + ".tmp")
                try:
                    os.makedirs(dossier, exist_ok=True)
                    with open(tmp, "wb") as f:
                        f.write(corps)
                        f.flush()
                        os.fsync(f.fileno())  # une Version acceptée doit survivre à une coupure
                    os.replace(tmp, os.path.join(dossier, nom))
                except OSError:  # disque plein, droits : le Client reçoit une erreur, pas une connexion coupée
                    return self.repondre(500)
            self.repondre(200, ETag=f'"{h}"')

        def nommer(self, code, corps):
            """Nom du jeu (nom de la ROM ou titre 3DS) : seulement pour un jeu qui a déjà une Version."""
            try:
                nom = corps.decode("utf-8").strip()
            except UnicodeDecodeError:
                nom = ""
            if not 0 < len(nom) <= 200 or any(c < " " or c == "\x7f" for c in nom):
                return self.repondre(400)
            if not courante(code):
                return self.repondre(404)
            with VERROU:
                tmp = os.path.join(data, code, "nom.txt.tmp")
                try:
                    with open(tmp, "w", encoding="utf-8") as f:
                        f.write(nom)
                        f.flush()
                        os.fsync(f.fileno())
                    os.replace(tmp, os.path.join(data, code, "nom.txt"))
                except OSError:
                    return self.repondre(500)
            self.repondre(204)

    return ThreadingHTTPServer(adresse, Handler)


if __name__ == "__main__":
    creer(("0.0.0.0", int(os.environ.get("NDSSYNC_PORT", "8080"))),
          os.environ.get("NDSSYNC_DATA", "/data"),
          os.environ["NDSSYNC_TOKEN"]).serve_forever()
