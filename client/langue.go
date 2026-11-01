package main

import "strings"

// Langue des fenêtres, des notifications et des erreurs : français si le système est en français, sinon anglais.
// Le journal reste en français.
var anglais = !strings.HasPrefix(strings.ToLower(langueSysteme()), "fr")

func tr(fr, en string) string {
	if anglais {
		return en
	}
	return fr
}
