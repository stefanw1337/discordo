package winterm

import (
	"time"

	"golang.org/x/sys/windows"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	getAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	getForegroundWindow = user32.NewProc("GetForegroundWindow")
	getAncestor         = user32.NewProc("GetAncestor")
	getWindow           = user32.NewProc("GetWindow")
)

const (
	vkControl = 0x11
	vkShift   = 0x10
	vkAlt     = 0x12
	vkV       = 0x56
	// gwOwner and gaRootOwner find the terminal's window, which owns the pseudoconsole's.
	gwOwner     = 4
	gaRootOwner = 3
)

// WatchPaste sends to pressed whenever ctrl+v is pressed while the terminal discordo runs in has the focus, in a
// terminal that keeps ctrl+v for itself, as Windows Terminal does, and so passes on nothing when there is only an
// image to paste. It watches only in such a terminal, a pseudoconsole's, where the key never reaches discordo.
func WatchPaste(pressed chan<- struct{}) {
	if Console() != "PseudoConsoleWindow" {
		return
	}
	go func() {
		held := false
		for range time.Tick(30 * time.Millisecond) {
			down := keyDown(vkControl) && keyDown(vkV) && !keyDown(vkShift) && !keyDown(vkAlt)
			if down && !held && terminalFocused() {
				select {
				case pressed <- struct{}{}:
				default:
				}
			}
			held = down
		}
	}()
}

func keyDown(code uintptr) bool {
	state, _, _ := getAsyncKeyState.Call(code)
	return state&0x8000 != 0
}

// terminalFocused reports whether the window in the foreground is the terminal discordo runs in.
func terminalFocused() bool {
	console, _, _ := getConsoleWindow.Call()
	foreground, _, _ := getForegroundWindow.Call()
	if console == 0 || foreground == 0 {
		return false
	}
	owner, _, _ := getWindow.Call(console, gwOwner)
	root, _, _ := getAncestor.Call(console, gaRootOwner)
	return foreground == console || foreground == owner || foreground == root
}
