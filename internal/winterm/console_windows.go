package winterm

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	getClassName     = windows.NewLazySystemDLL("user32.dll").NewProc("GetClassNameW")
)

// Console returns the window class of the console discordo runs in: "ConsoleWindowClass" for Windows' own console
// window (conhost), "PseudoConsoleWindow" for a terminal such as Windows Terminal, WezTerm or VS Code's, which runs
// it in a pseudoconsole, or "" if it has none.
func Console() string {
	window, _, _ := getConsoleWindow.Call()
	if window == 0 {
		return ""
	}
	name := make([]uint16, 256)
	n, _, _ := getClassName.Call(window, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	return windows.UTF16ToString(name[:n])
}

// OldConsole reports whether discordo runs in Windows' own console window (conhost), which shows neither emoji nor
// sixel images.
func OldConsole() bool {
	return Console() == "ConsoleWindowClass"
}
