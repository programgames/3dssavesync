package main

import "strings"

// Langue des fenêtres, des notifications et des erreurs : français si le système est en français, sinon anglais.
// Le journal reste en français.
var english = !strings.HasPrefix(strings.ToLower(systemLanguage()), "fr")

func tr(fr, en string) string {
	if english {
		return en
	}
	return fr
}
