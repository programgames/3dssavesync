//go:build !windows && !linux

package main

import "errors"

// macOS : pas encore d'icône près de l'horloge ; le raccourci « 3dssavesync réglages » reste.
var trayDisponible = false

func afficherIcone(func() []ElementMenu, func(string), <-chan struct{}) error {
	return errors.New(tr("L'icône près de l'horloge n'existe pas encore sur ce système. Ouvrez les réglages par « 3dssavesync réglages ».",
		"The icon near the clock is not available on this system yet. Open the settings with \"3dssavesync settings\"."))
}
