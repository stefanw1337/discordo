//go:build !windows && !no_voice

package voice

// keyHeld reports whether the push-to-talk key is held, which only Windows tells a program in a terminal, so that
// elsewhere push to talk is as if the key were always held.
func keyHeld(string) bool { return true }
