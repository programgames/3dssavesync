/* Dessin citro2d (ecran.h). */
#include <citro2d.h>
#include <stdio.h>
#include <string.h>
#include "ecran.h"

static C3D_RenderTarget *haut, *bas;
static C2D_TextBuf tampon, mesure; /* mesure : vidé à chaque mesure */
static C3D_Tex texture; /* icône du jeu : une seule par image */

void ecran_init(void)
{
    fontEnsureMapped(); /* police du système */
    C3D_Init(C3D_DEFAULT_CMDBUF_SIZE);
    C2D_Init(C2D_DEFAULT_MAX_OBJECTS);
    C2D_Prepare();
    haut = C2D_CreateScreenTarget(GFX_TOP, GFX_LEFT);
    bas = C2D_CreateScreenTarget(GFX_BOTTOM, GFX_LEFT);
    tampon = C2D_TextBufNew(4096);
    mesure = C2D_TextBufNew(256);
    C3D_TexInit(&texture, 64, 64, GPU_RGB565);
    C3D_TexSetFilter(&texture, GPU_NEAREST, GPU_NEAREST); /* icône DS agrandie 1,5 fois : pixels nets */
}

void ecran_fin(void)
{
    C3D_TexDelete(&texture);
    C2D_TextBufDelete(mesure);
    C2D_TextBufDelete(tampon);
    C2D_Fini();
    C3D_Fini();
}

void ecran_debut(void)
{
    C2D_TextBufClear(tampon);
    C3D_FrameBegin(C3D_FRAME_SYNCDRAW);
    C2D_TargetClear(haut, RGB(0x0C0E12));
    C2D_TargetClear(bas, RGB(0x14181E));
}

void ecran_haut(void) { C2D_SceneBegin(haut); }
void ecran_bas(void) { C2D_SceneBegin(bas); }
void ecran_finir(void) { C3D_FrameEnd(0); }

void ecran_rect(float x, float y, float l, float h, u32 couleur) { C2D_DrawRectSolid(x, y, 0, l, h, couleur); }

static float largeur_de(float taille, const char *s)
{
    C2D_Text t;
    float l;
    C2D_TextBufClear(mesure);
    C2D_TextParse(&t, mesure, s);
    C2D_TextGetDimensions(&t, taille, taille, &l, NULL);
    return l;
}

float ecran_largeur(float taille, const char *s) { return largeur_de(taille, s); }

float ecran_texte(float x, float y, float taille, u32 couleur, float largeur, const char *s)
{
    C2D_Text t;
    char coupe[256];
    float l = largeur_de(taille, s);
    if (largeur > 0 && l > largeur) { /* raccourci octet par octet, sans couper un caractère UTF-8 */
        size_t n = strlen(s) < sizeof coupe - 4 ? strlen(s) : sizeof coupe - 4;
        do {
            while (n > 0 && (s[--n] & 0xC0) == 0x80);
            memcpy(coupe, s, n);
            strcpy(coupe + n, "\xE2\x80\xA6"); /* … */
            l = largeur_de(taille, coupe);
        } while (n > 0 && l > largeur);
        s = coupe;
    }
    C2D_TextParse(&t, tampon, s);
    C2D_TextOptimize(&t);
    C2D_DrawText(&t, C2D_WithColor, x, y, 0, taille, taille, couleur);
    return l;
}

/* Retour à la ligne fait ici (et pas par C2D_WordWrap) pour connaître la hauteur dessinée. dessiner = 0 : mesure.
 * max > 0 : max lignes au plus, la dernière finit par « … » s'il reste du texte. */
static float paragraphe(float x, float y, float taille, u32 couleur, float largeur, const char *s, int max, int dessiner)
{
    char ligne[256];
    float interligne = fontGetInfo(NULL)->lineFeed * taille, h = 0;
    int lignes = 0;
    while (*s) {
        size_t n = 0, coupe = 0;
        if (max && ++lignes == max && s[strcspn(s, "\n")]) { /* d'autres lignes suivent : la dernière les annonce */
            snprintf(ligne, sizeof ligne, "%.*s\xE2\x80\xA6", (int)(strcspn(s, "\n") < sizeof ligne - 4 ? strcspn(s, "\n") : sizeof ligne - 4), s);
            if (dessiner) ecran_texte(x, y + h, taille, couleur, largeur, ligne);
            return h + interligne;
        }
        if (max && lignes == max) { /* dernière ligne permise : le reste, coupé par ecran_texte */
            if (dessiner) ecran_texte(x, y + h, taille, couleur, largeur, s);
            return h + interligne;
        }
        while (s[n] && s[n] != '\n' && n < sizeof ligne - 1) {
            memcpy(ligne, s, n + 1);
            ligne[n + 1] = '\0';
            if (coupe && largeur_de(taille, ligne) > largeur) break;
            if (s[++n] == ' ' && !strchr(":;!?", s[n + 1])) coupe = n; /* jamais « : » en début de ligne */
        }
        if (s[n] && s[n] != '\n' && coupe) n = coupe; /* ligne trop longue : coupée au dernier espace */
        memcpy(ligne, s, n);
        ligne[n] = '\0';
        if (dessiner) ecran_texte(x, y + h, taille, couleur, largeur, ligne);
        h += interligne;
        s += n;
        if (*s == ' ' || *s == '\n') s++;
    }
    return h;
}

float ecran_paragraphe(float x, float y, float taille, u32 couleur, float largeur, const char *s)
{
    return paragraphe(x, y, taille, couleur, largeur, s, 0, 1);
}

float ecran_lignes(float x, float y, float taille, u32 couleur, float largeur, int max, const char *s)
{
    return paragraphe(x, y, taille, couleur, largeur, s, max, 1);
}

float ecran_hauteur(float taille, float largeur, const char *s) { return paragraphe(0, 0, taille, 0, largeur, s, 0, 0); }

void ecran_icone(const u16 *pixels, int l, int h, float x, float y, float echelle)
{
    /* Tuiles 8×8 en ordre de Morton, ligne de tuiles 0 = haut de l'image (méthode de FBI, screen.c). */
    static const Tex3DS_SubTexture sous = { 64, 64, 0.0f, 1.0f, 1.0f, 0.0f };
    Tex3DS_SubTexture s = sous;
    u16 *d = texture.data;
    int px, py;
    memset(d, 0, 64 * 64 * 2);
    for (py = 0; py < h && py < 64; py++)
        for (px = 0; px < l && px < 64; px++)
            d[(((py >> 3) * 8 + (px >> 3)) << 6) + ((px & 1) | (py & 1) << 1 | (px & 2) << 1 | (py & 2) << 2 | (px & 4) << 2 | (py & 4) << 3)] =
                pixels[py * l + px];
    C3D_TexFlush(&texture);
    s.width = l;
    s.height = h;
    s.right = l / 64.0f;
    s.bottom = 1.0f - h / 64.0f;
    C2D_DrawImageAt((C2D_Image){ &texture, &s }, x, y, 0, NULL, echelle, echelle);
}
