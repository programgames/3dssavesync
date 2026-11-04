/* View layer of the 3DS app: layout of both screens, palette, button bar, message band, progress, questions and
 * the frame of a modal screen. Built on screen.h (citro2d). Not testable on the PC: compare Azahar captures. */
#ifndef VIEWS_H
#define VIEWS_H

#include <3ds.h>
#include "screen.h"

/* Console language: French, otherwise English (main.c). */
extern int english;
#define T(fr, en) (english ? (en) : (fr))

/* Palette: one meaning per color (yellow only says "To upload"). */
#define WHITE RGB(0xE8EAED)
#define GREY_LIGHT RGB(0x8A929C)  /* label */
#define GREY_DARK RGB(0x5A626C)   /* disabled */
#define C_GREEN RGB(0x4CC060)     /* In sync, success */
#define C_YELLOW RGB(0xF0C040)    /* To upload */
#define C_CYAN RGB(0x40C8E8)      /* To download, version available */
#define C_RED RGB(0xF05050)       /* Conflict, failure */
#define DARK_GREEN RGB(0x2F6B3A)  /* dot of In sync, which does not catch the eye */
#define BAR RGB(0x1E242C)         /* button bar, footer of the top screen */
#define CARD RGB(0x2A313B)        /* Conflict cards, message band */
#define SEL RGB(0x263445)         /* selected line */
#define ACCENT RGB(0x4A8FE0)      /* mark of the selected settings line */
#define BOX RGB(0x0C0E12)         /* inside of a check box */
#define GLYPH_A RGB(0xE04848)     /* the A, B, X, Y buttons in the New 3DS colors */
#define GLYPH_B RGB(0xF0C030)
#define GLYPH_X RGB(0x4080F0)
#define GLYPH_Y RGB(0x40B060)

/* Layout. Top screen 400 × 240, bottom screen 320 × 240. */
#define TOP_WIDTH 400
#define BOTTOM_WIDTH 320
#define SCREEN_HEIGHT 240
#define TEXT_WIDTH 304       /* text of the bottom screen, margins of 8 */
#define ROW_WIDTH 308        /* button row, margins of 6 */
#define BAND_WIDTH 302       /* text of the message band, from x = 10 */
#define PROGRESS_WIDTH 288   /* progress screen, margins of 16 */
#define ICON_SIZE 48
#define ICON_X (BOTTOM_WIDTH - 6 - ICON_SIZE) /* game icon, top right of the bottom screen */
#define UNDER_ICON 58        /* first y below the game icon */
#define TOP_LINES 13         /* lines of a list on the top screen (footer below) */
#define LIST_Y 27            /* first line of a list on the top screen, 15 pixels each */
#define FOOTER_Y 224         /* footer of the top screen */
#define BUTTON_HEIGHT 22
#define BAR_ROW_1 (SCREEN_HEIGHT - BUTTON_HEIGHT - 4)     /* buttons of a one-line bar */
#define BAR_ROW_2 (SCREEN_HEIGHT - 2 * BUTTON_HEIGHT - 4) /* first line of a two-line bar */
#define BAR_TOP_1 (SCREEN_HEIGHT - BUTTON_HEIGHT - 7)     /* top of the background of a one-line bar */
#define BAR_TOP_2 (SCREEN_HEIGHT - 2 * BUTTON_HEIGHT - 7)

/* A button of the bar: it can also be tapped with the stylus. */
typedef struct {
    u32 button;
    const char *text;
    int active; /* 0: greyed out (A with nothing to do) */
} button_t;

/* Glyph of the system font for a button (A, B, X, Y), or START / SELECT. */
const char *glyph(u32 button);

/* One row of buttons from y; gap = 0: buttons spread over the whole width. */
void draw_button_row(const button_t *t, int n, float y, float gap);

/* Touch area of a button outside the bar (Conflict cards). */
void touch_area(float x, float y, float l, float h, u32 button);

/* Background of the button bar: 1 or 2 lines, at the bottom of the bottom screen. */
void draw_bar_background(int lines);

/* Message band right above bottom: color mark, text, then the technical line in grey (NULL: none). */
void draw_band(float bottom, const char *text, const char *technical, u32 color);

/* One "label  value" line of the details. Returns the y of the next line. */
float detail(float y, const char *label, const char *value);

/* Buttons pressed (with repeat for the arrows), or bar button under the stylus. B if the app closes. */
u32 read_keys(void);

/* Top screen of the questions and progress: the context of what is going on (list or settings). */
extern void (*draw_current_top)(void);
extern const char *operation; /* title of the long operation in progress (Sync all), otherwise NULL */
extern u32 progress_color;    /* progress bar: yellow Upload, cyan Download, green the rest */

/* Progress of long loops: on the bottom screen, the operation, the text ("step\ngame") and a bar if total > 0. */
void show_progress(int done, int total, const char *format, ...);

/* Frame of a modal screen: modal_begin draws the top screen (top, or draw_current_top if NULL) and opens the bottom
 * screen with no touch area; the caller draws its content; modal_end draws a one-line bar of n buttons, ends the frame
 * and returns the keys pressed. */
void modal_begin(void (*top)(void));
u32 modal_end(const button_t *b, int n, float gap);

/* Question on the bottom screen: the title is the question, the subtitle the game; in the text, a "label\tvalue" line
 * goes into the grid of the details; the warning goes into a red band. 1 if A, the named action; 0 if B, Cancel. */
int confirm(const char *title, const char *subtitle, const char *text, const char *warning, const char *action);

#endif
