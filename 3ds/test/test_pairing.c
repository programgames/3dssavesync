/* Checks pairing.c: the QR code text is the PC's, byte for byte (client/setup_web_test.go, TestPhonePayload).
 * Built on the PC: gcc -std=c99 -Wall -I source source/pairing.c test/test_pairing.c -o /tmp/q && /tmp/q */
#include <stdio.h>
#include <string.h>
#include "pairing.h"

static int failures, total;

static void check(int ok, const char *what)
{
    total++;
    if (!ok) {
        printf("ECHEC : %s\n", what);
        failures++;
    }
}

int main(void)
{
    const char *pc = "3dssavesync://serveur?adresse=http%3A%2F%2F192.0.2.10%3A8765&jeton=faux+jeton%26%3D%C3%A9";
    char t[200];
    check(pairing_payload("http://192.0.2.10:8765", "faux jeton&=\xC3\xA9", t, sizeof t) && !strcmp(t, pc), t);
    check(pairing_payload("https://saves.example.com", "aZ09-._~", t, sizeof t)
              && !strcmp(t, "3dssavesync://serveur?adresse=https%3A%2F%2Fsaves.example.com&jeton=aZ09-._~"), t);
    check(pairing_payload("http://192.0.2.10:8765", "faux jeton&=\xC3\xA9", t, strlen(pc) + 1), "exact size fits");
    check(!pairing_payload("http://192.0.2.10:8765", "faux jeton&=\xC3\xA9", t, strlen(pc)) && strlen(t) == strlen(pc) - 1, "one byte short: cut, 0");
    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
