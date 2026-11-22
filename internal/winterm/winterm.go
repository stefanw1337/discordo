// Package winterm tells whether discordo runs in Windows' old console window, which shows neither emoji nor sixel
// images, and opens it in Windows Terminal instead.
package winterm

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// relaunchedEnv is set in a discordo started by Relaunch, so that it does not start another.
const relaunchedEnv = "DISCORDO_RELAUNCHED"

// Relaunch starts discordo again, with the same arguments, in Windows Terminal, when it runs in the old console
// window and Windows Terminal is installed, and reports whether it did, in which case this one should exit.
func Relaunch() bool {
	console := Console()
	slog.Info("console", "class", console, "wt_session", os.Getenv("WT_SESSION") != "", "relaunched", os.Getenv(relaunchedEnv) != "")
	if !OldConsole() || os.Getenv("WT_SESSION") != "" || os.Getenv(relaunchedEnv) != "" {
		return false
	}
	wt := windowsTerminal()
	if wt == "" {
		slog.Info("windows terminal not found")
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	args := append([]string{"--", exe}, os.Args[1:]...)
	command := exec.Command(wt, args...)
	command.Env = append(os.Environ(), relaunchedEnv+"=1")
	if err := command.Start(); err != nil {
		slog.Error("failed to open windows terminal", "path", wt, "err", err)
		return false
	}
	return true
}

// windowsTerminal returns the path of wt.exe, or "". Installed from the Store, it is an app execution alias in
// WindowsApps, which a search of PATH may not take for a program.
func windowsTerminal() string {
	if path, err := exec.LookPath("wt.exe"); err == nil {
		return path
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		path := filepath.Join(local, "Microsoft", "WindowsApps", "wt.exe")
		if _, err := os.Lstat(path); err == nil {
			return path
		}
	}
	return ""
}
