//go:build !windows

package winterm

// Console returns the window class of Windows' console, which there is none of elsewhere.
func Console() string {
	return ""
}

// OldConsole reports whether discordo runs in Windows' own console window, which it never does elsewhere.
func OldConsole() bool {
	return false
}
