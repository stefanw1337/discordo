//go:build !windows

package winterm

// OldConsole reports whether discordo runs in Windows' own console window, which it never does elsewhere.
func OldConsole() bool {
	return false
}
