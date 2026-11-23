package winterm

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	getClassName     = user32.NewProc("GetClassNameW")
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

// UnderWSL reports whether discordo was started from a WSL shell, as a Windows program run from Linux, whose output
// WSL passes on without the sequences that draw sixels, which then show as text.
func UnderWSL() bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot) //nolint:errcheck

	type process struct {
		parent uint32
		name   string
	}
	processes := map[uint32]process{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		processes[entry.ProcessID] = process{entry.ParentProcessID, strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))}
	}

	id := uint32(windows.GetCurrentProcessId())
	for range 8 {
		p, ok := processes[id]
		if !ok || p.parent == 0 || p.parent == id {
			return false
		}
		id = p.parent
		switch processes[id].name {
		case "wsl.exe", "wslhost.exe", "wslservice.exe":
			return true
		}
	}
	return false
}
