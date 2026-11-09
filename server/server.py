"""Self-hosted 3dssavesync server: History of the DS, GB, GBA and 3DS Saves (docs/design.md §2, ADR 0004).

Environment variables (NDSSYNC_ prefix from before v3, kept for existing installs): NDSSYNC_TOKEN (required), NDSSYNC_DATA (default /data), NDSSYNC_PORT (default 8080).
A Version = data/<code>/<seq>-<date>-<client>-<hash>.sav; the Current version has the highest <seq>.
Nothing is ever deleted or rewritten, except data/<code>/nom.txt: the game name sent by a Client, for the History.
python server.py verify [data]: reads every Version again and lists those whose content no longer matches their Hash.
python server.py report [data]: disk use per Game, largest first (reads only).
"""
import calendar
import hashlib
import hmac
import json
import os
import re
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Key of a Game (ADR 0007): DS, GBA, GB/GBC, 3DS (3DS- raw /main, 3DSA- whole archive, ADR 0028; 3DSX- extdata, ADR 0031). Never "/" or ".": safe as a folder name.
KEY = r"([A-Z0-9]{4}|GBA-[A-Z0-9]{4}|GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}|3DSA?-[0-9A-F]{16}|3DSX-[0-9A-F]{8})"
SAVE = re.compile(rf"^/saves/{KEY}$")
HISTORY = re.compile(rf"^/saves/{KEY}/history(/.*)?$")
NAME = re.compile(rf"^/saves/{KEY}/name$")
CLIENT = re.compile(r"^[a-z0-9]{1,16}$")
MAX_SIZE = 64 * 1024 * 1024  # largest known DS save memory
LOCK = threading.Lock()  # ponytail: global lock, enough for 2 Clients


def hash_of(version):
    return version[:-4].rsplit("-", 1)[1]


def fields(version):
    """(seq, date, client, hash) of a Version file name."""
    return version[:-4].split("-")


def name_of(folder):
    try:
        with open(os.path.join(folder, "nom.txt"), encoding="utf-8") as f:
            return f.read()
    except OSError:
        return None


def human(size):
    for unit in ("B", "KiB", "MiB"):
        if size < 1024:
            break
        size /= 1024
    else:
        unit = "GiB"
    return f"{size} B" if unit == "B" else f"{size:.1f} {unit}"


def verify(data):
    """Recomputes the Hash of every Version. Returns (number of Versions, damaged ones as "<code>/<version>")."""
    count, damaged = 0, []
    for code in sorted(os.listdir(data)):
        folder = os.path.join(data, code)
        if not os.path.isdir(folder):
            continue
        for version in sorted(f for f in os.listdir(folder) if f.endswith(".sav")):
            count += 1
            try:
                with open(os.path.join(folder, version), "rb") as f:
                    ok = hashlib.file_digest(f, "sha256").hexdigest() == hash_of(version)
            except OSError:
                ok = False
            if not ok:
                damaged.append(f"{code}/{version}")
    return count, damaged


def report(data):
    """Lines of text: one per Game, largest total first, then a total line. Writes nothing."""
    games, leftovers = [], 0
    for code in sorted(os.listdir(data)):
        folder = os.path.join(data, code)
        if not os.path.isdir(folder):
            continue
        files = os.listdir(folder)
        leftovers += sum(f.endswith(".tmp") for f in files)  # a write cut before its os.replace
        versions = sorted(f for f in files if f.endswith(".sav"))
        if not versions:
            continue
        sizes = [os.path.getsize(os.path.join(folder, v)) for v in versions]
        day = [f"{d[:4]}-{d[4:6]}-{d[6:8]}" for d in (fields(versions[0])[1], fields(versions[-1])[1])]
        games.append((sum(sizes), code, len(versions), sizes[-1], *day, name_of(folder) or ""))
    games.sort(key=lambda g: (-g[0], g[1]))
    table = [("KEY", "VERSIONS", "TOTAL", "CURRENT", "OLDEST", "NEWEST", "NAME")]
    table += [(code, str(n), human(total), human(cur), old, new, name) for total, code, n, cur, old, new, name in games]
    widths = [max(len(row[i]) for row in table) for i in range(6)]
    lines = ["  ".join([row[0].ljust(widths[0]), *(c.rjust(w) for c, w in zip(row[1:6], widths[1:])), row[6]]).rstrip()
             for row in table]
    total = f"{len(games)} games, {sum(g[2] for g in games)} versions, {human(sum(g[0] for g in games))}"
    return lines + [total + (f", {leftovers} .tmp leftovers (cut uploads)" if leftovers else "")]


def creer(address, data, token):
    def versions(code):
        folder = os.path.join(data, code)
        if not os.path.isdir(folder):
            return []
        return sorted(f for f in os.listdir(folder) if f.endswith(".sav"))

    def current(code):
        v = versions(code)
        return v[-1] if v else None

    def info(code, version):
        _, date, client, h = fields(version)
        size = os.path.getsize(os.path.join(data, code, version))
        # age: computed with the server clock, because the 3DS clock has no time zone
        age = int(time.time() - calendar.timegm(time.strptime(date, "%Y%m%dT%H%M%SZ")))
        return {"code": code, "version": version, "hash": h, "taille": size, "date": date, "client": client, "age": age}

    def with_name(code, element):
        name = name_of(os.path.join(data, code))
        return element if name is None else {**element, "nom": name}

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
            # Read the body before any response: otherwise the client gets a reset instead of the status.
            body = self.rfile.read(size)
            if len(body) != size:  # connection cut: never a truncated Version
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
                        os.fsync(f.fileno())  # an accepted Version must survive a power cut
                    os.replace(tmp, os.path.join(folder, name))
                except OSError:  # disk full, permissions: the Client gets an error, not a cut connection
                    return self.respond(500)
            self.respond(200, ETag=f'"{h}"')

        def set_name(self, code, body):
            """Name of the game (ROM name or 3DS title): only for a game that already has a Version."""
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


if __name__ == "__main__" and sys.argv[1:2] == ["verify"]:
    count, damaged = verify(sys.argv[2] if len(sys.argv) > 2 else os.environ.get("NDSSYNC_DATA", "/data"))
    for version in damaged:
        print(f"DAMAGED {version}")
    print(f"{count} versions read, {len(damaged)} damaged")
    sys.exit(1 if damaged else 0)
elif __name__ == "__main__" and sys.argv[1:2] == ["report"]:
    for line in report(sys.argv[2] if len(sys.argv) > 2 else os.environ.get("NDSSYNC_DATA", "/data")):
        print(line)
elif __name__ == "__main__":
    creer(("0.0.0.0", int(os.environ.get("NDSSYNC_PORT", "8080"))),
          os.environ.get("NDSSYNC_DATA", "/data"),
          os.environ["NDSSYNC_TOKEN"]).serve_forever()
