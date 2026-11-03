//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
)

// askSystem shows a dialog outside Windows: osascript (macOS), zenity or kdialog (Linux).
// The last button is the safest: default button, and the choice when the user closes the window.
// "" = no window possible (no tool, no display, tool error): a Conflict replaces nothing.
// ponytail: display under Gamescope (Steam Deck, Game mode) not checked.
func askSystem(text string, buttons []string) string {
	fmt.Fprintln(os.Stderr, text)
	tool := dialogTool()
	if tool == "" {
		return ""
	}
	output, err := exec.Command(tool, dialogArgs(tool, text, buttons)...).Output()
	code := 0
	var end *exec.ExitError
	if errors.As(err, &end) {
		code = end.ExitCode()
	} else if err != nil {
		return ""
	}
	return dialogResponse(tool, buttons, code, string(output))
}

func dialogTool() string {
	if runtime.GOOS == "darwin" {
		return "osascript"
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return ""
	}
	order := []string{"zenity", "kdialog"}
	if strings.Contains(os.Getenv("XDG_CURRENT_DESKTOP"), "KDE") {
		order = []string{"kdialog", "zenity"}
	}
	for _, o := range order {
		if _, err := exec.LookPath(o); err == nil {
			return o
		}
	}
	return ""
}

// dialogArgs returns the tool arguments. At most 3 buttons (limit of AppleScript and kdialog).
func dialogArgs(tool, text string, b []string) []string {
	switch tool {
	case "osascript": // text and buttons as arguments: no AppleScript escaping
		return append([]string{"-e", "on run argv", "-e", "set b to items 2 thru -1 of argv", "-e",
			`button returned of (display dialog (item 1 of argv) with title "3dssavesync" buttons b default button (count of b))`,
			"-e", "end run", text}, b...)
	case "kdialog": // exit codes: 0, 1, 2 = buttons 1, 2, 3
		a := []string{"--title", "3dssavesync"}
		switch len(b) {
		case 1:
			return append(a, "--msgbox", text)
		case 2:
			return append(a, "--yesno", text, "--yes-label", b[0], "--no-label", b[1])
		}
		return append(a, "--yesnocancel", text, "--yes-label", b[0], "--no-label", b[1], "--cancel-label", b[2])
	}
	// zenity: OK = first button, Cancel (and close) = last, middle buttons as --extra-button.
	if len(b) == 1 {
		return []string{"--info", "--no-markup", "--title=3dssavesync", "--text=" + text, "--ok-label=" + b[0]}
	}
	a := []string{"--question", "--no-markup", "--title=3dssavesync", "--text=" + text, "--ok-label=" + b[0], "--cancel-label=" + b[len(b)-1]}
	for _, x := range b[1 : len(b)-1] {
		a = append(a, "--extra-button="+x)
	}
	return a
}

// dialogResponse returns the chosen button from the exit code and the tool output; "" if the tool failed.
func dialogResponse(tool string, b []string, code int, output string) string {
	output = strings.TrimSpace(output)
	switch tool {
	case "osascript":
		if code == 0 && slices.Contains(b, output) {
			return output
		}
	case "kdialog":
		if code >= 0 && code <= 2 {
			return b[min(code, len(b)-1)]
		}
	default: // zenity: 0 = OK; 1 = Cancel, close or middle button (label on the output)
		switch {
		case code == 0:
			return b[0]
		case code == 1 && slices.Contains(b, output):
			return output
		case code == 1:
			return b[len(b)-1]
		}
	}
	return ""
}

func systemNotify(text string) {
	fmt.Fprintln(os.Stderr, text)
	if runtime.GOOS == "darwin" {
		exec.Command("osascript", "-e", "on run argv", "-e", `display notification (item 1 of argv) with title "3dssavesync"`, "-e", "end run", text).Run()
	} else if p, err := exec.LookPath("notify-send"); err == nil {
		exec.Command(p, "3dssavesync", text).Run()
	}
}

func waitNotifications() {} // notify-send and osascript return once the notification shows

// The setup starts in a terminal: nothing to open.
func openConsole() {}

const cleanConsole = false

// noEcho turns off echo for input (token) with stty; with no terminal, nothing to do.
func noEcho() func() {
	c := exec.Command("stty", "-echo")
	c.Stdin = os.Stdin
	if c.Run() != nil {
		return func() {}
	}
	return func() {
		c := exec.Command("stty", "echo")
		c.Stdin = os.Stdin
		c.Run()
	}
}

// systemLanguage reads the locale variables, then the macOS preference (an app started by the Finder has no LANG).
func systemLanguage() string {
	for _, v := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := os.Getenv(v); l != "" && l != "C" && l != "POSIX" {
			return l
		}
	}
	if runtime.GOOS == "darwin" {
		if o, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
			return string(o)
		}
	}
	return ""
}

// countProcesses returns the number of processes with this name. Linux: /proc/<pid>/comm (15 characters at most);
// macOS: ps.
func countProcesses(name string) (n int) {
	name = strings.ToLower(name)
	if len(name) > 15 {
		name = name[:15]
	}
	if comms, err := filepath.Glob("/proc/[0-9]*/comm"); err == nil && len(comms) > 0 {
		for _, c := range comms {
			if o, err := os.ReadFile(c); err == nil && strings.ToLower(strings.TrimSpace(string(o))) == name {
				n++
			}
		}
		return n
	}
	o, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(o), "\n") {
		if strings.ToLower(filepath.Base(strings.TrimSpace(l))) == name {
			n++
		}
	}
	return n
}

// processAlive reports that the process pid still runs (signal 0: checks without sending; EPERM = it exists, owned by
// another user).
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// startDetached starts a process that outlives the setup terminal (new session, no SIGHUP).
func startDetached(exe string, args []string) error {
	c := exec.Command(exe, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// removableRoots returns the mounted volumes (SD card, USB stick): /Volumes (macOS), /media/$USER and
// /run/media/$USER (Linux).
func removableRoots() []string {
	patterns := []string{"/Volumes/*"}
	if runtime.GOOS != "darwin" {
		patterns = []string{filepath.Join("/media", os.Getenv("USER"), "*"), filepath.Join("/run/media", os.Getenv("USER"), "*")}
	}
	var l []string
	for _, m := range patterns {
		f, _ := filepath.Glob(m)
		l = append(l, f...)
	}
	return l
}

// systemFreeSpace returns the free space for the user on the disk of dir.
func systemFreeSpace(dir string) (uint64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(dir, &s); err != nil {
		return 0, err
	}
	return uint64(s.Bavail) * uint64(s.Bsize), nil
}
