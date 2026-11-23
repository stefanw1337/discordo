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

// UnderWSL reports whether a Windows program was started from WSL, which it never is elsewhere.
func UnderWSL() bool {
	return false
}

// WatchPaste watches for nothing outside Windows, where ctrl+v reaches discordo.
func WatchPaste(chan<- struct{}) {}
