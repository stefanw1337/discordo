package voice

// PushToTalkKeys are the keys push to talk can be held with, by the names the settings give them, as Windows'
// virtual-key codes. They are read wherever the focus is, as Discord's are, which only Windows lets a program do.
var PushToTalkKeys = map[string]int{
	"mouse4":     0x05,
	"mouse5":     0x06,
	"capslock":   0x14,
	"rightctrl":  0xA3,
	"rightalt":   0xA5,
	"backtick":   0xC0,
	"f13":        0x7C,
	"scrolllock": 0x91,
}
