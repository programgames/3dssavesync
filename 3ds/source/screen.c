/* Dessin citro2d (screen.h). */
#include <citro2d.h>
#include <stdio.h>
#include <string.h>
#include "screen.h"

static C3D_RenderTarget *top, *bottom;
static C2D_TextBuf text_buf, measure_buf; /* mesure : vidé à chaque mesure */
static C3D_Tex texture; /* icône du jeu : une seule par image */

void screen_init(void)
{
    fontEnsureMapped(); /* police du système */
    C3D_Init(C3D_DEFAULT_CMDBUF_SIZE);
    C2D_Init(C2D_DEFAULT_MAX_OBJECTS);
    C2D_Prepare();
    top = C2D_CreateScreenTarget(GFX_TOP, GFX_LEFT);
    bottom = C2D_CreateScreenTarget(GFX_BOTTOM, GFX_LEFT);
    text_buf = C2D_TextBufNew(4096);
    measure_buf = C2D_TextBufNew(256);
    C3D_TexInit(&texture, 64, 64, GPU_RGB565);
    C3D_TexSetFilter(&texture, GPU_NEAREST, GPU_NEAREST); /* icône DS agrandie 1,5 fois : pixels nets */
}

void screen_exit(void)
{
    C3D_TexDelete(&texture);
    C2D_TextBufDelete(measure_buf);
    C2D_TextBufDelete(text_buf);
    C2D_Fini();
    C3D_Fini();
}

void screen_begin(void)
{
    C2D_TextBufClear(text_buf);
    C3D_FrameBegin(C3D_FRAME_SYNCDRAW);
    C2D_TargetClear(top, RGB(0x0C0E12));
    C2D_TargetClear(bottom, RGB(0x14181E));
}

void screen_top(void) { C2D_SceneBegin(top); }
void screen_bottom(void) { C2D_SceneBegin(bottom); }
void screen_end(void) { C3D_FrameEnd(0); }

void screen_rect(float x, float y, float l, float h, u32 color) { C2D_DrawRectSolid(x, y, 0, l, h, color); }

static float text_width(float size, const char *s)
{
    C2D_Text t;
    float l;
    C2D_TextBufClear(measure_buf);
    C2D_TextParse(&t, measure_buf, s);
    C2D_TextGetDimensions(&t, size, size, &l, NULL);
    return l;
}

float screen_text_width(float size, const char *s) { return text_width(size, s); }

float screen_text(float x, float y, float size, u32 color, float width, const char *s)
{
    C2D_Text t;
    char cut[256];
    float l = text_width(size, s);
    if (width > 0 && l > width) { /* raccourci octet par octet, sans couper un caractère UTF-8 */
        size_t n = strlen(s) < sizeof cut - 4 ? strlen(s) : sizeof cut - 4;
        do {
            while (n > 0 && (s[--n] & 0xC0) == 0x80);
            memcpy(cut, s, n);
            strcpy(cut + n, "\xE2\x80\xA6"); /* … */
            l = text_width(size, cut);
        } while (n > 0 && l > width);
        s = cut;
    }
    C2D_TextParse(&t, text_buf, s);
    C2D_TextOptimize(&t);
    C2D_DrawText(&t, C2D_WithColor, x, y, 0, size, size, color);
    return l;
}

/* Retour à la ligne fait ici (et pas par C2D_WordWrap) pour connaître la hauteur dessinée. dessiner = 0 : mesure.
 * max > 0 : max lignes au plus, la dernière finit par « … » s'il reste du texte. */
static float paragraph(float x, float y, float size, u32 color, float width, const char *s, int max, int draw)
{
    char line[256];
    float line_height = fontGetInfo(NULL)->lineFeed * size, h = 0;
    int lines = 0;
    while (*s) {
        size_t n = 0, cut = 0;
        if (max && ++lines == max && s[strcspn(s, "\n")]) { /* d'autres lignes suivent : la dernière les annonce */
            snprintf(line, sizeof line, "%.*s\xE2\x80\xA6", (int)(strcspn(s, "\n") < sizeof line - 4 ? strcspn(s, "\n") : sizeof line - 4), s);
            if (draw) screen_text(x, y + h, size, color, width, line);
            return h + line_height;
        }
        if (max && lines == max) { /* dernière ligne permise : le reste, coupé par screen_text */
            if (draw) screen_text(x, y + h, size, color, width, s);
            return h + line_height;
        }
        while (s[n] && s[n] != '\n' && n < sizeof line - 1) {
            memcpy(line, s, n + 1);
            line[n + 1] = '\0';
            if (cut && text_width(size, line) > width) break;
            if (s[++n] == ' ' && !strchr(":;!?", s[n + 1])) cut = n; /* jamais « : » en début de ligne */
        }
        if (s[n] && s[n] != '\n' && cut) n = cut; /* ligne trop longue : coupée au dernier espace */
        memcpy(line, s, n);
        line[n] = '\0';
        if (draw) screen_text(x, y + h, size, color, width, line);
        h += line_height;
        s += n;
        if (*s == ' ' || *s == '\n') s++;
    }
    return h;
}

float screen_paragraph(float x, float y, float size, u32 color, float width, const char *s)
{
    return paragraph(x, y, size, color, width, s, 0, 1);
}

float screen_lines(float x, float y, float size, u32 color, float width, int max, const char *s)
{
    return paragraph(x, y, size, color, width, s, max, 1);
}

float screen_paragraph_height(float size, float width, const char *s) { return paragraph(0, 0, size, 0, width, s, 0, 0); }

void screen_icon(const u16 *pixels, int l, int h, float x, float y, float scale)
{
    /* Tuiles 8×8 en ordre de Morton, ligne de tuiles 0 = haut de l'image (méthode de FBI, screen.c). */
    static const Tex3DS_SubTexture full_tex = { 64, 64, 0.0f, 1.0f, 1.0f, 0.0f };
    Tex3DS_SubTexture s = full_tex;
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
    C2D_DrawImageAt((C2D_Image){ &texture, &s }, x, y, 0, NULL, scale, scale);
}
