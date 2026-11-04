package main

import "strings"

// Language of windows, notifications and errors: French if the OS is in French, otherwise English.
// The log stays in French.
var english = !strings.HasPrefix(strings.ToLower(systemLanguage()), "fr")

func tr(fr, en string) string {
	if english {
		return en
	}
	return fr
}
