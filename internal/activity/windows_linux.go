package activity

import (
	"os"
	"path/filepath"
	"strconv"
)

// Windows returns the programs the user runs, read from /proc, without titles, which only the display server knows.
func Windows() []Window {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var windows []Window
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		// Only the user's own processes can be read; the others are skipped.
		exe, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if err != nil {
			continue
		}
		windows = append(windows, Window{Exe: filepath.Base(exe)})
	}
	return windows
}
