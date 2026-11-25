package winterm

import (
	"log/slog"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	coInitializeEx   = ole32.NewProc("CoInitializeEx")
	coCreateInstance = ole32.NewProc("CoCreateInstance")
	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	createDIBSection = gdi32.NewProc("CreateDIBSection")
	createBitmap     = gdi32.NewProc("CreateBitmap")
	deleteObject     = gdi32.NewProc("DeleteObject")
	createIconInd    = user32.NewProc("CreateIconIndirect")
	destroyIcon      = user32.NewProc("DestroyIcon")

	// The taskbar list and the interface of it that sets overlay icons.
	clsidTaskbarList = windows.GUID{Data1: 0x56FDF344, Data2: 0xFD6D, Data3: 0x11d0, Data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iidTaskbarList3  = windows.GUID{Data1: 0xea1afb91, Data2: 0x9e28, Data3: 0x4b86, Data4: [8]byte{0x90, 0xe9, 0x9e, 0x9f, 0x8a, 0x5e, 0xef, 0xaf}}

	badges     chan Badge
	badgesOnce sync.Once
)

// The methods of ITaskbarList3 used, by their place in its table.
const (
	methodHrInit         = 3
	methodSetOverlayIcon = 18
)

// SetBadge shows badge over the icon of the terminal's button on the taskbar, as Discord does over its own, or
// nothing if there is nothing unread. It does nothing outside a terminal with a window of its own.
func SetBadge(badge Badge) {
	badgesOnce.Do(func() {
		badges = make(chan Badge, 1)
		go showBadges(badges)
	})
	// Only the newest badge matters: an older one waiting is replaced.
	select {
	case <-badges:
	default:
	}
	badges <- badge
}

// showBadges sets the badges it is sent, on a thread of its own, as COM wants.
func showBadges(badges <-chan Badge) {
	runtime.LockOSThread()
	const coinitApartmentThreaded = 2
	_, _, _ = coInitializeEx.Call(0, coinitApartmentThreaded)
	var taskbar *comObject
	const clsctxInprocServer = 1
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidTaskbarList3)), uintptr(unsafe.Pointer(&taskbar)))
	if hr != 0 || taskbar == nil {
		slog.Error("failed to reach the taskbar for its badge", "hresult", hr)
		return
	}
	call(taskbar, methodHrInit)

	var shown uintptr
	for badge := range badges {
		window := terminalWindow()
		if window == 0 {
			continue
		}
		icon := badgeIcon(badge)
		if hr := call(taskbar, methodSetOverlayIcon, window, icon, 0); hr != 0 {
			slog.Error("failed to set the taskbar's badge", "hresult", hr)
		}
		if shown != 0 {
			_, _, _ = destroyIcon.Call(shown)
		}
		shown = icon
	}
}

// comObject is a COM object: its first field is the table of its methods.
type comObject struct {
	methods *[methodSetOverlayIcon + 1]uintptr
}

// call calls method of the COM object object with args.
func call(object *comObject, method int, args ...uintptr) uintptr {
	hr, _, _ := syscall.SyscallN(object.methods[method], append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)...)
	return hr
}

// terminalWindow is the top window of the terminal discordo runs in, whose button is on the taskbar.
func terminalWindow() uintptr {
	console, _, _ := getConsoleWindow.Call()
	if console == 0 {
		return 0
	}
	root, _, _ := getAncestor.Call(console, gaRootOwner)
	if root != 0 {
		return root
	}
	return console
}

// badgeIcon makes the icon of badge, or returns 0 for none.
func badgeIcon(badge Badge) uintptr {
	img := badgeImage(badge)
	if img == nil {
		return 0
	}
	type bitmapInfoHeader struct {
		Size                         uint32
		Width, Height                int32
		Planes, BitCount             uint16
		Compression, SizeImage       uint32
		XPelsPerMeter, YPelsPerMeter int32
		ClrUsed, ClrImportant        uint32
	}
	header := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: badgeSize, Height: -badgeSize, Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	color, _, _ := createDIBSection.Call(0, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0
	}
	defer deleteObject.Call(color) //nolint:errcheck
	// Windows wants blue, green, red and alpha, the colors multiplied by alpha.
	pixels := unsafe.Slice((*byte)(bits), badgeSize*badgeSize*4)
	for i := 0; i < len(img.Pix); i += 4 {
		a := uint32(img.Pix[i+3])
		pixels[i] = byte(uint32(img.Pix[i+2]) * a / 255)
		pixels[i+1] = byte(uint32(img.Pix[i+1]) * a / 255)
		pixels[i+2] = byte(uint32(img.Pix[i]) * a / 255)
		pixels[i+3] = byte(a)
	}
	mask, _, _ := createBitmap.Call(badgeSize, badgeSize, 1, 1, 0)
	if mask == 0 {
		return 0
	}
	defer deleteObject.Call(mask) //nolint:errcheck
	type iconInfo struct {
		Icon               int32
		XHotspot, YHotspot uint32
		Mask, Color        uintptr
	}
	info := iconInfo{Icon: 1, Mask: mask, Color: color}
	icon, _, _ := createIconInd.Call(uintptr(unsafe.Pointer(&info)))
	return icon
}
