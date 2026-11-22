package winterm

import "golang.org/x/sys/windows"

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	isWindowVisible  = windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible")
)

// OldConsole reports whether discordo runs in Windows' own console window (conhost). A terminal such as Windows
// Terminal, WezTerm or VS Code's runs it in a pseudoconsole, whose console window is hidden.
func OldConsole() bool {
	window, _, _ := getConsoleWindow.Call()
	if window == 0 {
		return false
	}
	visible, _, _ := isWindowVisible.Call(window)
	return visible != 0
}
