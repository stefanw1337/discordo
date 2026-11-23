//go:build !no_voice

package voice

import "golang.org/x/sys/windows"

var getAsyncKeyState = windows.NewLazySystemDLL("user32.dll").NewProc("GetAsyncKeyState")

// keyHeld reports whether the push-to-talk key named name is held down now, whatever window has the focus.
func keyHeld(name string) bool {
	code, ok := PushToTalkKeys[name]
	if !ok {
		return true
	}
	state, _, _ := getAsyncKeyState.Call(uintptr(code))
	return state&0x8000 != 0
}
