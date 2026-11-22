// Package winterm tells whether discordo runs in Windows' old console window, which shows neither emoji nor sixel
// images, and opens it in Windows Terminal instead.
package winterm

import (
	"os"
	"os/exec"
)

// relaunchedEnv is set in a discordo started by Relaunch, so that it does not start another.
const relaunchedEnv = "DISCORDO_RELAUNCHED"

// Relaunch starts discordo again, with the same arguments, in Windows Terminal, when it runs in the old console
// window and Windows Terminal is installed, and reports whether it did, in which case this one should exit.
func Relaunch() bool {
	if !OldConsole() || os.Getenv("WT_SESSION") != "" || os.Getenv(relaunchedEnv) != "" {
		return false
	}
	wt, err := exec.LookPath("wt.exe")
	if err != nil {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	args := append([]string{"--", exe}, os.Args[1:]...)
	command := exec.Command(wt, args...)
	command.Env = append(os.Environ(), relaunchedEnv+"=1")
	return command.Start() == nil
}
