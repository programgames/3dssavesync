package io.github.programgames.savesync.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

// The QR code of the PC settings page (client/setup_web_test.go, TestPhonePayload makes the same text).
class PairingTest {
    @Test
    fun valid() {
        assertEquals(Pairing("http://192.0.2.10:8765", "fake-token"), parsePairing("3dssavesync://serveur?adresse=http%3A%2F%2F192.0.2.10%3A8765&jeton=fake-token"))
        assertEquals(Pairing("https://saves.example.com", "t"), parsePairing(" 3dssavesync://serveur?jeton=t&adresse=https%3A%2F%2Fsaves.example.com\n"))
    }

    @Test
    fun urlEncodedCharacters() =
        assertEquals(Pairing("http://192.0.2.10:8765", "faux jeton&=é"), parsePairing("3dssavesync://serveur?adresse=http%3A%2F%2F192.0.2.10%3A8765&jeton=faux+jeton%26%3D%C3%A9"))

    @Test
    fun wrongScheme() {
        assertNull(parsePairing("https://serveur?adresse=http%3A%2F%2F192.0.2.10&jeton=t"))
        assertNull(parsePairing("3dssavesync://autre?adresse=http%3A%2F%2F192.0.2.10&jeton=t"))
        assertNull(parsePairing("fake-token"))
    }

    @Test
    fun missingToken() {
        assertNull(parsePairing("3dssavesync://serveur?adresse=http%3A%2F%2F192.0.2.10"))
        assertNull(parsePairing("3dssavesync://serveur?adresse=http%3A%2F%2F192.0.2.10&jeton="))
    }

    @Test
    fun badAddress() {
        for (a in listOf("", "192.0.2.10%3A8765", "ftp%3A%2F%2F192.0.2.10", "http%3A%2F%2F", "http%3A%2F%2Fa+b", "%zz")) {
            assertNull(a, parsePairing("3dssavesync://serveur?adresse=$a&jeton=t"))
        }
        assertNull(parsePairing("3dssavesync://serveur?jeton=t"))
    }
}
