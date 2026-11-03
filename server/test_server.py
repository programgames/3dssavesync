"""Contrat HTTP du Serveur (docs/design.md §2).

Local (serveur neuf par test) : python -m unittest test_server.py
Contre un serveur déployé (Docker, cloud) : NDSSYNC_URL=https://... NDSSYNC_TOKEN=... python -m unittest test_server.py
Chaque test utilise des Clés aléatoires : un serveur déjà utilisé convient (il garde ces Versions de test pour toujours).
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

DISTANT = os.environ.get("NDSSYNC_URL", "").rstrip("/")
JETON = os.environ.get("NDSSYNC_TOKEN", "jeton-de-test")
SAUVEGARDE = bytes(range(256)) * 2048  # 512 Kio
HASH = hashlib.sha256(SAUVEGARDE).hexdigest()


def code(n=4, alphabet=string.ascii_uppercase + string.digits):
    return "".join(random.choice(alphabet) for _ in range(n))


class ApiTest(unittest.TestCase):
    def setUp(self):
        self.cle, self.autre = code(), code()
        if DISTANT:
            self.url = DISTANT
            return
        self.data = tempfile.TemporaryDirectory()
        self.srv = server.creer(("127.0.0.1", 0), self.data.name, JETON)
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.srv.server_address[1]}"

    def tearDown(self):
        if DISTANT:
            return
        self.srv.shutdown()
        self.srv.server_close()
        self.data.cleanup()

    def requete(self, methode, chemin, corps=None, jeton=JETON, **entetes):
        """Renvoie (statut, en-têtes, corps) sans lever d'exception sur 4xx."""
        req = urllib.request.Request(self.url + chemin, data=corps, method=methode)
        if jeton:
            req.add_header("Authorization", f"Bearer {jeton}")
        for nom, valeur in entetes.items():
            req.add_header(nom.replace("_", "-"), valeur)
        try:
            with urllib.request.urlopen(req) as r:
                return r.status, r.headers, r.read()
        except urllib.error.HTTPError as e:
            return e.code, e.headers, e.read()

    def json(self, chemin):
        statut, _, corps = self.requete("GET", chemin)
        self.assertEqual(statut, 200)
        return json.loads(corps)

    def envoyer(self, corps=SAUVEGARDE, client="pc", **precondition):
        if not precondition:
            precondition = {"If_None_Match": "*"}
        return self.requete("PUT", f"/saves/{self.cle}", corps, X_Client=client, **precondition)

    def test_sans_jeton_401(self):
        self.assertEqual(self.requete("GET", "/saves", jeton=None)[0], 401)

    def test_mauvais_jeton_401(self):
        self.assertEqual(self.requete("GET", "/saves", jeton="faux")[0], 401)

    def test_premier_envoi_puis_recuperation(self):
        statut, entetes, _ = self.envoyer()
        self.assertEqual(statut, 200)
        self.assertEqual(entetes["ETag"], f'"{HASH}"')

        statut, entetes, corps = self.requete("GET", f"/saves/{self.cle}")
        self.assertEqual(statut, 200)
        self.assertEqual(entetes["ETag"], f'"{HASH}"')
        self.assertEqual(corps, SAUVEGARDE)

    def test_if_match_perime_412_avec_etag_courant(self):
        self.envoyer()
        statut, entetes, _ = self.envoyer(b"autre", If_Match='"base-perimee"')
        self.assertEqual(statut, 412)
        self.assertEqual(entetes["ETag"], f'"{HASH}"')
        self.assertEqual(self.requete("GET", f"/saves/{self.cle}")[2], SAUVEGARDE)

    def test_if_match_correct_200(self):
        self.envoyer()
        statut, entetes, _ = self.envoyer(b"nouvelle", If_Match=f'"{HASH}"')
        self.assertEqual(statut, 200)
        self.assertEqual(self.requete("GET", f"/saves/{self.cle}")[2], b"nouvelle")

    def test_if_none_match_alors_que_version_existe_412(self):
        self.envoyer()
        self.assertEqual(self.envoyer(b"autre")[0], 412)

    def test_if_match_sans_version_412(self):
        self.assertEqual(self.envoyer(If_Match=f'"{HASH}"')[0], 412)

    def test_sans_precondition_428(self):
        statut, _, _ = self.requete("PUT", f"/saves/{self.cle}", SAUVEGARDE, X_Client="pc")
        self.assertEqual(statut, 428)

    def test_code_invalide_400(self):
        statut, _, _ = self.requete("PUT", "/saves/irb", SAUVEGARDE, If_None_Match="*")
        self.assertEqual(statut, 400)

    def test_cles_gb_gba_3ds_acceptees(self):  # ADR 0007
        cles = ["GBA-" + code(), f"GB-POKEMON_{code(5)}-91E6", "3DS-" + code(16, "0123456789ABCDEF")]
        for cle in cles:
            statut, _, _ = self.requete("PUT", f"/saves/{cle}", b"x", If_None_Match="*")
            self.assertEqual(statut, 200, cle)
            self.assertEqual(self.requete("GET", f"/saves/{cle}")[2], b"x")
            self.assertEqual(self.requete("GET", f"/saves/{cle}/history")[0], 200)
        presentes = {j["code"] for j in self.json("/saves")}
        self.assertTrue(set(cles) <= presentes)

    def test_cles_invalides_400(self):
        for cle in ("GBA-BPE", "gba-bpef", "GB-POKEMON-91E6X", "GB--91E6", "3DS-0004", "3DS-00040000001b5000",
                    "GB-A..B-91E6", "XYZ-ABCD", "ABCDE"):
            statut, _, _ = self.requete("PUT", f"/saves/{cle}", b"x", If_None_Match="*")
            self.assertEqual(statut, 400, cle)

    def test_corps_vide_400(self):
        self.assertEqual(self.envoyer(b"")[0], 400)

    def test_liste_donne_la_version_courante_de_chaque_jeu(self):
        self.envoyer(client="3ds")
        self.requete("PUT", f"/saves/{self.autre}", b"platine", If_None_Match="*", X_Client="pc")
        liste = {j["code"]: j for j in self.json("/saves")}
        self.assertIn(self.autre, liste)
        jeu = liste[self.cle]
        self.assertEqual(jeu["hash"], HASH)
        self.assertEqual(jeu["taille"], len(SAUVEGARDE))
        self.assertEqual(jeu["client"], "3ds")
        self.assertRegex(jeu["date"], r"^\d{8}T\d{6}Z$")
        self.assertTrue(0 <= jeu["age"] < 60)  # secondes depuis l'Envoi, horloge du serveur

    def test_historique_conserve_chaque_version(self):
        self.envoyer(b"v1", client="3ds")
        h1 = hashlib.sha256(b"v1").hexdigest()
        self.envoyer(b"v2", If_Match=f'"{h1}"')
        histo = self.json(f"/saves/{self.cle}/history")
        self.assertEqual([v["hash"] for v in histo], [h1, hashlib.sha256(b"v2").hexdigest()])
        self.assertEqual([v["client"] for v in histo], ["3ds", "pc"])
        self.assertEqual(self.requete("GET", f"/saves/{self.cle}/history/{histo[0]['version']}")[2], b"v1")

    def test_nom_du_jeu_dans_la_liste(self):
        self.assertEqual(self.requete("PUT", f"/saves/{self.cle}/name", "Pokémon Bleue".encode())[0], 404)  # jeu sans Version
        self.envoyer()
        self.assertNotIn("nom", {j["code"]: j for j in self.json("/saves")}[self.cle])
        self.assertEqual(self.requete("PUT", f"/saves/{self.cle}/name", "Pokémon Bleue".encode())[0], 204)
        self.assertEqual(self.requete("PUT", f"/saves/{self.cle}/name", " Pokémon - Version Bleue ".encode())[0], 204)
        self.assertEqual({j["code"]: j for j in self.json("/saves")}[self.cle]["nom"], "Pokémon - Version Bleue")
        for mauvais in (b"", b"   ", b"a\nb", b"a\x7fb", b"\xff", "x".encode() * 201):
            self.assertEqual(self.requete("PUT", f"/saves/{self.cle}/name", mauvais)[0], 400, mauvais)
        self.assertEqual(self.requete("PUT", f"/saves/{self.cle}/name", b"x", jeton="faux")[0], 401)
        self.assertEqual(len(self.json(f"/saves/{self.cle}/history")), 1)  # le nom ne crée pas de Version

    def test_envoi_identique_ne_cree_pas_de_version(self):
        self.envoyer()
        statut, entetes, _ = self.envoyer(If_Match=f'"{HASH}"')
        self.assertEqual(statut, 200)
        self.assertEqual(entetes["ETag"], f'"{HASH}"')
        self.assertEqual(len(self.json(f"/saves/{self.cle}/history")), 1)

    def test_client_hostile_ne_sort_pas_du_dossier(self):
        self.assertEqual(self.envoyer(client="../../x")[0], 200)
        self.assertEqual(self.json(f"/saves/{self.cle}/history")[0]["client"], "inconnu")

    def test_version_inconnue_404(self):
        self.envoyer()
        self.assertEqual(self.requete("GET", f"/saves/{self.cle}/history/../../server.py")[0], 404)

    @unittest.skipIf(DISTANT, "connexion brute en HTTP local")
    def test_corps_tronque_refuse(self):
        # Connexion coupée en plein envoi : 10 octets reçus sur 100 annoncés.
        import socket
        s = socket.create_connection(("127.0.0.1", self.srv.server_address[1]))
        s.sendall(f"PUT /saves/{self.cle} HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer {JETON}".encode() +
                  b"\r\nIf-None-Match: *\r\nContent-Length: 100\r\n\r\n" + b"x" * 10)
        s.shutdown(socket.SHUT_WR)
        reponse = s.recv(100)
        s.close()
        self.assertIn(b" 400 ", reponse)
        self.assertEqual(self.requete("GET", f"/saves/{self.cle}")[0], 404)

    @unittest.skipIf(DISTANT, "accès au dossier data du serveur")
    def test_numero_suit_la_derniere_version_meme_apres_suppression_manuelle(self):
        self.envoyer(b"v1")
        h1 = hashlib.sha256(b"v1").hexdigest()
        self.envoyer(b"v2", If_Match=f'"{h1}"')
        os.remove(os.path.join(self.data.name, self.cle, self.json(f"/saves/{self.cle}/history")[0]["version"]))
        h2 = hashlib.sha256(b"v2").hexdigest()
        self.assertEqual(self.envoyer(b"v3", If_Match=f'"{h2}"')[0], 200)
        self.assertEqual(self.requete("GET", f"/saves/{self.cle}")[2], b"v3")

    @unittest.skipIf(DISTANT, "accès au dossier data du serveur")
    def test_ecriture_impossible_500(self):
        # Disque plein, droits : une erreur 500, jamais une connexion coupée.
        open(os.path.join(self.data.name, self.cle), "w").close()  # un fichier à la place du dossier du jeu
        self.assertEqual(self.envoyer()[0], 500)
        self.requete("PUT", f"/saves/{self.autre}", b"x", If_None_Match="*")
        os.mkdir(os.path.join(self.data.name, self.autre, "nom.txt.tmp"))  # nom.txt.tmp impossible à écrire
        self.assertEqual(self.requete("PUT", f"/saves/{self.autre}/name", b"Pokemon")[0], 500)

    @unittest.skipIf(DISTANT, "accès au dossier data du serveur")
    def test_liste_comme_tests_saves_json(self):
        """tests/saves.json : la liste de référence, lue par le test C de la 3DS (test_liste.c) et comparée à celle du
        serveur Go (serveur_test.go). NDSSYNC_FIXTURE=1 python -m unittest test_server.py la réécrit."""
        jeux = {  # 3DS : la plus récente seulement ; ADAF sans nom, entre deux jeux nommés ; nom avec } et "
            "3DS-0004000000055D00": [("000003-20260101T120000Z-3ds", b"x" * 1024, "Pokémon X")],
            "ADAF": [("000001-20260102T080910Z-pc", b"v1", None), ("000002-20260102T081011Z-pc", b"\0" * 524288, None)],
            "GBA-BPEF": [("000001-20260103T235959Z-3ds", b"\xff" * 131072, 'Pokémon Émeraude {hack} "FR"')],
        }
        for cle, versions in jeux.items():
            os.mkdir(os.path.join(self.data.name, cle))
            for debut, corps, nom in versions:
                with open(os.path.join(self.data.name, cle, f"{debut}-{hashlib.sha256(corps).hexdigest()}.sav"), "wb") as f:
                    f.write(corps)
                if nom:
                    with open(os.path.join(self.data.name, cle, "nom.txt"), "w", encoding="utf-8") as f:
                        f.write(nom)
        statut, _, corps = self.requete("GET", "/saves")
        self.assertEqual(statut, 200)
        fixture = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "tests", "saves.json")
        if os.environ.get("NDSSYNC_FIXTURE"):
            with open(fixture, "wb") as f:
                f.write(corps)
        with open(fixture, "rb") as f:
            attendu = json.load(f)
        recu = json.loads(corps)
        for j in attendu + recu:  # age : dépend de l'heure du test
            self.assertGreater(j.pop("age"), 0)
        self.assertEqual(recu, attendu)

    def test_jeu_inconnu_404(self):
        self.assertEqual(self.requete("GET", f"/saves/{self.cle}")[0], 404)


if __name__ == "__main__":
    unittest.main()
