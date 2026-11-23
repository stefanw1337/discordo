package activity

import (
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	getWindowText  = user32.NewProc("GetWindowTextW")
	getWindowTextN = user32.NewProc("GetWindowTextLengthW")

	// enumMu guards found, which the one callback enumWindows is given adds to, as callbacks are never freed.
	enumMu      sync.Mutex
	found       []Window
	exes        map[uint32]string
	enumWindows = windows.NewCallback(func(window windows.HWND, _ uintptr) uintptr {
		if !windows.IsWindowVisible(window) {
			return 1
		}
		title := windowTitle(window)
		if title == "" {
			return 1
		}
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(window, &pid); err != nil || pid == 0 {
			return 1
		}
		exe, ok := exes[pid]
		if !ok {
			exe = processExe(pid)
			exes[pid] = exe
		}
		if exe != "" {
			found = append(found, Window{Exe: exe, Title: title})
		}
		return 1
	})
)

// Windows returns the windows that are shown, with the names of their programs' files.
func Windows() []Window {
	enumMu.Lock()
	defer enumMu.Unlock()
	found, exes = nil, map[uint32]string{}
	_ = windows.EnumWindows(enumWindows, nil)
	return found
}

func windowTitle(window windows.HWND) string {
	n, _, _ := getWindowTextN.Call(uintptr(window))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	n, _, _ = getWindowText.Call(uintptr(window), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// processExe returns the name of the file of the process pid, or "" if it may not be looked at.
func processExe(pid uint32) string {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(process, 0, &buf[0], &size); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:size]))
}
