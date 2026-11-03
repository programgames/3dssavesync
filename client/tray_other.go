//go:build !windows && !linux

package main

import "errors"

// macOS: no tray icon yet; the settings shortcut stays.
var trayAvailable = false

func showIcon(func() []MenuItem, func(string), <-chan struct{}) error {
	return errors.New(tr("L'icône près de l'horloge n'est pas encore disponible sur ce système. Ouvrez les réglages avec « 3dssavesync réglages ».",
		"The icon near the clock isn't available on this system yet. Open the settings with \"3dssavesync settings\"."))
}
