/* Update of the app from the GitHub releases (ADR 0021): check, download, SHA256SUMS check, install of the .cia or
 * the .3dsx. VERSION comes from the Makefile: "3.7.0" for a tag, "dev" otherwise (never updated). */
#ifndef APP_UPDATE_H
#define APP_UPDATE_H

#include "settings.h"

extern update_info_t update_info; /* maj.ini */
extern char update_detail[400];   /* result of the last check or install for the settings screen; last line: technical */
extern int update_failed;         /* update_detail describes a failure */

/* At start: reads maj.ini (written again if another version ran last). argv0: the running .3dsx, NULL if unknown. */
void update_start(const char *argv0);

/* Check of the day, if the Wi-Fi is on and none was done today. Failure: in update_detail only. */
void update_check_if_due(void);

/* Settings, Version line: check, then install if the user chooses it. 1 if installed: the app must close. */
int update_app(void);

#endif
