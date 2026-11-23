package activity

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Windows returns the programs the user runs, as ps lists them, without titles.
func Windows() []Window {
	out, err := exec.Command("ps", "-xo", "comm=").Output()
	if err != nil {
		return nil
	}
	var windows []Window
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			windows = append(windows, Window{Exe: filepath.Base(line)})
		}
	}
	return windows
}
