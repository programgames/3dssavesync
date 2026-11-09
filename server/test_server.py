"""HTTP contract of the Server (docs/design.md §2).

Local (a new server per test): python -m unittest test_server.py
Against a deployed server (Docker, cloud): NDSSYNC_URL=https://... NDSSYNC_TOKEN=... python -m unittest test_server.py
Each test uses random Keys: a server already in use is fine (it keeps these test Versions forever).
"""
import hashlib
import json
import os
import random
import string
import tempfile
import threading
import unittest
import urllib.error
import urllib.request

import server

REMOTE = os.environ.get("NDSSYNC_URL", "").rstrip("/")
TOKEN = os.environ.get("NDSSYNC_TOKEN", "jeton-de-test")
SAVE_DATA = bytes(range(256)) * 2048  # 512 KiB
HASH = hashlib.sha256(SAVE_DATA).hexdigest()


def code(n=4, alphabet=string.ascii_uppercase + string.digits):
    return "".join(random.choice(alphabet) for _ in range(n))


class ApiTest(unittest.TestCase):
    def setUp(self):
        self.key, self.other = code(), code()
        if REMOTE:
            self.url = REMOTE
            return
        self.data = tempfile.TemporaryDirectory()
        self.srv = server.creer(("127.0.0.1", 0), self.data.name, TOKEN)
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.srv.server_address[1]}"

    def tearDown(self):
        if REMOTE:
            return
        self.srv.shutdown()
        self.srv.server_close()
        self.data.cleanup()

    def request(self, method, path, body=None, token=TOKEN, **headers):
        """Returns (status, headers, body) without raising on 4xx."""
        req = urllib.request.Request(self.url + path, data=body, method=method)
        if token:
            req.add_header("Authorization", f"Bearer {token}")
        for name, value in headers.items():
            req.add_header(name.replace("_", "-"), value)
        try:
            with urllib.request.urlopen(req) as r:
                return r.status, r.headers, r.read()
        except urllib.error.HTTPError as e:
            return e.code, e.headers, e.read()

    def json(self, path):
        status, _, body = self.request("GET", path)
        self.assertEqual(status, 200)
        return json.loads(body)

    def upload(self, body=SAVE_DATA, client="pc", **precondition):
        if not precondition:
            precondition = {"If_None_Match": "*"}
        return self.request("PUT", f"/saves/{self.key}", body, X_Client=client, **precondition)

    def test_no_token_401(self):
        self.assertEqual(self.request("GET", "/saves", token=None)[0], 401)

    def test_wrong_token_401(self):
        self.assertEqual(self.request("GET", "/saves", token="faux")[0], 401)

    def test_first_upload_then_download(self):
        status, headers, _ = self.upload()
        self.assertEqual(status, 200)
        self.assertEqual(headers["ETag"], f'"{HASH}"')

        status, headers, body = self.request("GET", f"/saves/{self.key}")
        self.assertEqual(status, 200)
        self.assertEqual(headers["ETag"], f'"{HASH}"')
        self.assertEqual(body, SAVE_DATA)

    def test_stale_if_match_412_with_current_etag(self):
        self.upload()
        status, headers, _ = self.upload(b"autre", If_Match='"base-perimee"')
        self.assertEqual(status, 412)
        self.assertEqual(headers["ETag"], f'"{HASH}"')
        self.assertEqual(self.request("GET", f"/saves/{self.key}")[2], SAVE_DATA)

    def test_if_match_correct_200(self):
        self.upload()
        status, _, _ = self.upload(b"nouvelle", If_Match=f'"{HASH}"')
        self.assertEqual(status, 200)
        self.assertEqual(self.request("GET", f"/saves/{self.key}")[2], b"nouvelle")

    def test_if_none_match_when_version_exists_412(self):
        self.upload()
        self.assertEqual(self.upload(b"autre")[0], 412)

    def test_if_match_without_version_412(self):
        self.assertEqual(self.upload(If_Match=f'"{HASH}"')[0], 412)

    def test_no_precondition_428(self):
        status, _, _ = self.request("PUT", f"/saves/{self.key}", SAVE_DATA, X_Client="pc")
        self.assertEqual(status, 428)

    def test_invalid_code_400(self):
        status, _, _ = self.request("PUT", "/saves/irb", SAVE_DATA, If_None_Match="*")
        self.assertEqual(status, 400)

    def test_gb_gba_3ds_keys_accepted(self):  # ADR 0007
        keys = ["GBA-" + code(), f"GB-POKEMON_{code(5)}-91E6", "3DS-" + code(16, "0123456789ABCDEF"),
                "3DSA-" + code(16, "0123456789ABCDEF"),  # ADR 0028: whole archive
                "3DSX-" + code(8, "0123456789ABCDEF")]  # ADR 0031: extdata
        for key in keys:
            status, _, _ = self.request("PUT", f"/saves/{key}", b"x", If_None_Match="*")
            self.assertEqual(status, 200, key)
            self.assertEqual(self.request("GET", f"/saves/{key}")[2], b"x")
            self.assertEqual(self.request("GET", f"/saves/{key}/history")[0], 200)
        present = {j["code"] for j in self.json("/saves")}
        self.assertTrue(set(keys) <= present)

    def test_invalid_keys_400(self):
        for key in ("GBA-BPE", "gba-bpef", "GB-POKEMON-91E6X", "GB--91E6", "3DS-0004", "3DS-00040000001b5000", "3DSB-0004000000055D00", "3DSA-0004",
                    "3DSX-0000055d", "3DSX-0004000000055D00",
                    "GB-A..B-91E6", "XYZ-ABCD", "ABCDE"):
            status, _, _ = self.request("PUT", f"/saves/{key}", b"x", If_None_Match="*")
            self.assertEqual(status, 400, key)

    def test_empty_body_400(self):
        self.assertEqual(self.upload(b"")[0], 400)

    def test_list_gives_current_version_of_each_game(self):
        self.upload(client="3ds")
        self.request("PUT", f"/saves/{self.other}", b"platine", If_None_Match="*", X_Client="pc")
        listing = {j["code"]: j for j in self.json("/saves")}
        self.assertIn(self.other, listing)
        game = listing[self.key]
        self.assertEqual(game["hash"], HASH)
        self.assertEqual(game["taille"], len(SAVE_DATA))
        self.assertEqual(game["client"], "3ds")
        self.assertRegex(game["date"], r"^\d{8}T\d{6}Z$")
        self.assertTrue(0 <= game["age"] < 60)  # seconds since the Upload, server clock

    def test_history_keeps_every_version(self):
        self.upload(b"v1", client="3ds")
        h1 = hashlib.sha256(b"v1").hexdigest()
        self.upload(b"v2", If_Match=f'"{h1}"')
        history = self.json(f"/saves/{self.key}/history")
        self.assertEqual([v["hash"] for v in history], [h1, hashlib.sha256(b"v2").hexdigest()])
        self.assertEqual([v["client"] for v in history], ["3ds", "pc"])
        self.assertEqual(self.request("GET", f"/saves/{self.key}/history/{history[0]['version']}")[2], b"v1")

    def test_game_name_in_list(self):
        self.assertEqual(self.request("PUT", f"/saves/{self.key}/name", "Pokémon Bleue".encode())[0], 404)  # game without a Version
        self.upload()
        self.assertNotIn("nom", {j["code"]: j for j in self.json("/saves")}[self.key])
        self.assertEqual(self.request("PUT", f"/saves/{self.key}/name", "Pokémon Bleue".encode())[0], 204)
        self.assertEqual(self.request("PUT", f"/saves/{self.key}/name", " Pokémon - Version Bleue ".encode())[0], 204)
        self.assertEqual({j["code"]: j for j in self.json("/saves")}[self.key]["nom"], "Pokémon - Version Bleue")
        for bad in (b"", b"   ", b"a\nb", b"a\x7fb", b"\xff", b"x" * 201):
            self.assertEqual(self.request("PUT", f"/saves/{self.key}/name", bad)[0], 400, bad)
        self.assertEqual(self.request("PUT", f"/saves/{self.key}/name", b"x", token="faux")[0], 401)
        self.assertEqual(len(self.json(f"/saves/{self.key}/history")), 1)  # the name creates no Version

    def test_identical_upload_creates_no_version(self):
        self.upload()
        status, headers, _ = self.upload(If_Match=f'"{HASH}"')
        self.assertEqual(status, 200)
        self.assertEqual(headers["ETag"], f'"{HASH}"')
        self.assertEqual(len(self.json(f"/saves/{self.key}/history")), 1)

    def test_hostile_client_stays_in_its_folder(self):
        self.assertEqual(self.upload(client="../../x")[0], 200)
        self.assertEqual(self.json(f"/saves/{self.key}/history")[0]["client"], "inconnu")

    def test_unknown_version_404(self):
        self.upload()
        self.assertEqual(self.request("GET", f"/saves/{self.key}/history/../../server.py")[0], 404)

    @unittest.skipIf(REMOTE, "connexion brute en HTTP local")
    def test_truncated_body_refused(self):
        # Connection cut during the upload: 10 bytes received out of 100 announced.
        import socket
        s = socket.create_connection(("127.0.0.1", self.srv.server_address[1]))
        s.sendall(f"PUT /saves/{self.key} HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer {TOKEN}".encode() +
                  b"\r\nIf-None-Match: *\r\nContent-Length: 100\r\n\r\n" + b"x" * 10)
        s.shutdown(socket.SHUT_WR)
        response = s.recv(100)
        s.close()
        self.assertIn(b" 400 ", response)
        self.assertEqual(self.request("GET", f"/saves/{self.key}")[0], 404)

    @unittest.skipIf(REMOTE, "accès au dossier data du serveur")
    def test_number_follows_last_version_after_manual_removal(self):
        self.upload(b"v1")
        h1 = hashlib.sha256(b"v1").hexdigest()
        self.upload(b"v2", If_Match=f'"{h1}"')
        os.remove(os.path.join(self.data.name, self.key, self.json(f"/saves/{self.key}/history")[0]["version"]))
        h2 = hashlib.sha256(b"v2").hexdigest()
        self.assertEqual(self.upload(b"v3", If_Match=f'"{h2}"')[0], 200)
        self.assertEqual(self.request("GET", f"/saves/{self.key}")[2], b"v3")

    @unittest.skipIf(REMOTE, "accès au dossier data du serveur")
    def test_write_failure_500(self):
        # Disk full, permissions: a 500 error, never a cut connection.
        open(os.path.join(self.data.name, self.key), "w").close()  # a file in place of the game's folder
        self.assertEqual(self.upload()[0], 500)
        self.request("PUT", f"/saves/{self.other}", b"x", If_None_Match="*")
        os.mkdir(os.path.join(self.data.name, self.other, "nom.txt.tmp"))  # nom.txt.tmp cannot be written
        self.assertEqual(self.request("PUT", f"/saves/{self.other}/name", b"Pokemon")[0], 500)

    @unittest.skipIf(REMOTE, "accès au dossier data du serveur")
    def test_list_matches_tests_saves_json(self):
        """tests/saves.json: the reference list, read by the 3DS C test (test_list.c) and compared with the list of the
        Go server (server_test.go). NDSSYNC_FIXTURE=1 python -m unittest test_server.py rewrites it."""
        games = {  # 3DS: latest only; ADAF without a name, between two named games; name with } and "
            "3DS-0004000000055D00": [("000003-20260101T120000Z-3ds", b"x" * 1024, "Pokémon X")],
            "ADAF": [("000001-20260102T080910Z-pc", b"v1", None), ("000002-20260102T081011Z-pc", b"\0" * 524288, None)],
            "GBA-BPEF": [("000001-20260103T235959Z-3ds", b"\xff" * 131072, 'Pokémon Émeraude {hack} "FR"')],
        }
        for key, versions in games.items():
            os.mkdir(os.path.join(self.data.name, key))
            for prefix, body, name in versions:
                with open(os.path.join(self.data.name, key, f"{prefix}-{hashlib.sha256(body).hexdigest()}.sav"), "wb") as f:
                    f.write(body)
                if name:
                    with open(os.path.join(self.data.name, key, "nom.txt"), "w", encoding="utf-8") as f:
                        f.write(name)
        status, _, body = self.request("GET", "/saves")
        self.assertEqual(status, 200)
        fixture = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "tests", "saves.json")
        if os.environ.get("NDSSYNC_FIXTURE"):
            with open(fixture, "wb") as f:
                f.write(body)
        with open(fixture, "rb") as f:
            expected = json.load(f)
        received = json.loads(body)
        for j in expected + received:  # age: depends on the time of the test
            self.assertGreater(j.pop("age"), 0)
        self.assertEqual(received, expected)

    def test_unknown_game_404(self):
        self.assertEqual(self.request("GET", f"/saves/{self.key}")[0], 404)


class VerifyTest(unittest.TestCase):
    def test_lists_only_the_versions_whose_content_changed(self):
        with tempfile.TemporaryDirectory() as data:
            os.makedirs(os.path.join(data, "ABCD"))
            good = f"000001-20260101T000000Z-pc-{HASH}.sav"
            bad = f"000002-20260102T000000Z-3ds-{HASH}.sav"
            for name, body in ((good, SAVE_DATA), (bad, SAVE_DATA[:-1] + b"\x00")):
                with open(os.path.join(data, "ABCD", name), "wb") as f:
                    f.write(body)
            with open(os.path.join(data, "ABCD", "nom.txt"), "w") as f:
                f.write("not a Version")
            self.assertEqual(server.verify(data), (2, [f"ABCD/{bad}"]))


class ReportTest(unittest.TestCase):
    def test_games_largest_first_with_counts_and_totals(self):
        with tempfile.TemporaryDirectory() as data:
            files = {"ABCD": [("000001-20260101T000000Z-pc-aa.sav", 10), ("000002-20260305T120000Z-3ds-bb.sav", 20)],
                     "GBA-WXYZ": [("000001-20260210T000000Z-pc-cc.sav", 2048),
                                  ("000002-20260211T000000Z-pc-dd.sav.tmp", 5)]}
            for key, versions in files.items():
                os.makedirs(os.path.join(data, key))
                for name, size in versions:
                    with open(os.path.join(data, key, name), "wb") as f:
                        f.write(b"x" * size)
            with open(os.path.join(data, "ABCD", "nom.txt"), "w", encoding="utf-8") as f:
                f.write("Pokémon Platine")
            before = sorted(os.walk(data))
            self.assertEqual(server.report(data), [
                "KEY       VERSIONS    TOTAL  CURRENT      OLDEST      NEWEST  NAME",
                "GBA-WXYZ         1  2.0 KiB  2.0 KiB  2026-02-10  2026-02-10",
                "ABCD             2     30 B     20 B  2026-01-01  2026-03-05  Pokémon Platine",
                "2 games, 3 versions, 2.0 KiB, 1 .tmp leftovers (cut uploads)",
            ])
            self.assertEqual(sorted(os.walk(data)), before)  # reads only


if __name__ == "__main__":
    unittest.main()
