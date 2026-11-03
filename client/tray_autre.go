//go:build !windows && !linux

package main

import "errors"

// macOS : pas encore d'icône près de l'horloge ; le raccourci « 3dssavesync réglages » reste.
var trayDisponible = false

func afficherIcone(func() []ElementMenu, func(string), <-chan struct{}) error {
	return errors.New(tr("L'icône près de l'horloge n'est pas encore disponible sur ce système. Ouvrez les réglages avec « 3dssavesync réglages ».",
		"The icon near the clock isn't available on this system yet. Open the settings with \"3dssavesync settings\"."))
}
