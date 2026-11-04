package main

// Sync without the Launcher (ADR 0024): the tray icon syncs the games of an emulator when its program closes, and
// every closed emulator every 15 minutes. It is the Sync of Sync all: Uploads, and Downloads only over a save
// unchanged since the last Sync (the 3-way state), each after a Backup copy; a Conflict gives a notification only.

import (
	"strings"
	"sync"
	"time"
)

// autoSync reports the Sync without the Launcher on, unless "synchro_auto": false.
func (c *Config) autoSync() bool { return c.AutoSync == nil || *c.AutoSync }

const (
	watchTick   = 10 * time.Second // how often the emulators are checked for a close
	watchPeriod = 15 * time.Minute // how often every closed emulator is synced (Versions from the 3DS)
)

// watchSaves runs until end. syncing is shared with the menu items: one Sync at a time.
func watchSaves(config string, syncing *sync.Mutex, end <-chan struct{}) {
	running := map[string]bool{}        // emulator -> its program ran at the last check
	next := time.Now().Add(time.Minute) // first full pass soon after the session starts
	notified := map[string]bool{}       // problem lines already notified: not repeated every 15 minutes nor at each close
	for {
		select {
		case <-end:
			return
		case <-time.After(watchTick):
		}
		var cfg Config
		if readJSON(config, &cfg) != nil || !cfg.autoSync() {
			continue
		}
		closed := closedEmulators(&cfg, running)
		full := time.Now().After(next)
		if len(closed) == 0 && !full || !syncing.TryLock() { // a Sync all from the menu covers this pass
			continue
		}
		if full {
			closed, next = nil, time.Now().Add(watchPeriod) // nil: every emulator
		}
		r := syncEmulators(config, closed)
		syncing.Unlock()
		fresh := newProblem(notified, r.problems, full)
		if text := autoSyncText(r); text != "" && (r.uploaded+r.downloaded > 0 || fresh) {
			notify(text)
		}
	}
}

// newProblem reports a problem line not notified yet, and records the lines of this pass. A full pass forgets the
// lines that are gone (a problem that comes back is notified again); the pass of a closed emulator only adds: its
// problems are a part of those of the last full pass (an unknown games folder).
func newProblem(notified map[string]bool, problems []string, full bool) bool {
	fresh := false
	for _, p := range problems {
		fresh = fresh || !notified[p]
	}
	if full {
		clear(notified)
	}
	for _, p := range problems {
		notified[p] = true
	}
	return fresh
}

// closedEmulators returns the emulators whose program ran at the last check and no longer runs; running is updated.
func closedEmulators(cfg *Config, running map[string]bool) (closed []string) {
	for name, e := range cfg.Emulators {
		open := instances(e.process()) > 0
		if running[name] && !open {
			closed = append(closed, name)
		}
		running[name] = open
	}
	return closed
}

// autoSyncText is the notification of an automatic pass: the problems, then the counts if a save moved; "" if
// nothing moved and nothing went wrong.
func autoSyncText(r syncReport) string {
	text := r.problems
	if r.uploaded+r.downloaded > 0 {
		text = append(text, r.counts())
	}
	if len(text) == 0 {
		return ""
	}
	return tr("Synchronisation automatique\n", "Automatic sync\n") + strings.Join(text, "\n")
}
