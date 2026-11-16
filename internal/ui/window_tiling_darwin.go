//go:build darwin

package ui

import (
	"runtime"
	"time"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// AppKit owns the tiling geometry, including the user's margin preference.
// These selectors are private, so check for them and retain the normal Zoom
// behavior on macOS versions that do not implement them.
var (
	fillSelector   = objc.RegisterName("_zoomFill:")
	untileSelector = objc.RegisterName("_zoomUntile:")
)

type windowRect struct {
	X, Y, Width, Height float64
}

type windowFillFrame struct {
	window  uintptr
	frame   windowRect
	pending bool
}

func nsString(s string) objc.ID {
	value := s + "\x00"
	result := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), unsafe.StringData(value))
	runtime.KeepAlive(value)
	return result
}

func responds(window objc.ID, selector objc.SEL) bool {
	return objc.Send[bool](window, objc.RegisterName("respondsToSelector:"), selector)
}

func windowFrame(window objc.ID) windowRect {
	return objc.Send[windowRect](window, objc.RegisterName("frame"))
}

// titleBarDoubleClicked handles only Rove's custom drag regions. Dispatching
// after the frame avoids resizing the window in the middle of a UI render.
func (a *App) titleBarDoubleClicked() {
	win := a.Win
	mygo.App.WhenReady(func() {
		if win == nil || win.IsDestroyed() {
			return
		}
		window := objc.ID(win.NativeHandle())
		if window == 0 {
			return
		}
		defaults := objc.ID(objc.GetClass("NSUserDefaults")).Send(objc.RegisterName("standardUserDefaults"))
		action := defaults.Send(objc.RegisterName("stringForKey:"), nsString("AppleActionOnDoubleClick"))
		switch {
		case objc.Send[bool](action, objc.RegisterName("isEqualToString:"), nsString("None")):
			return
		case objc.Send[bool](action, objc.RegisterName("isEqualToString:"), nsString("Minimize")):
			window.Send(objc.RegisterName("performMiniaturize:"), uintptr(0))
		case objc.Send[bool](action, objc.RegisterName("isEqualToString:"), nsString("Fill")) && responds(window, fillSelector):
			a.fillWindow(win, window)
		default:
			window.Send(objc.RegisterName("performZoom:"), uintptr(0))
		}
	})
}

func (a *App) toggleWindowMaximize() {
	win := a.Win
	mygo.App.WhenReady(func() {
		if win == nil || win.IsDestroyed() {
			return
		}
		window := objc.ID(win.NativeHandle())
		if window == 0 {
			return
		}
		if !responds(window, fillSelector) || !responds(window, untileSelector) {
			win.ToggleMaximize()
			return
		}
		frame := windowFrame(window)
		if a.fillFrame != nil && a.fillFrame.window == uintptr(window) &&
			(a.fillFrame.pending || a.fillFrame.frame == frame) {
			window.Send(untileSelector, uintptr(0))
			a.fillFrame = nil
			return
		}
		a.fillWindow(win, window)
	})
}

func (a *App) fillWindow(win *mygo.Window, window objc.ID) {
	previous := windowFrame(window)
	fill := &windowFillFrame{window: uintptr(window), pending: true}
	a.fillFrame = fill
	window.Send(fillSelector, uintptr(0))
	// The window server applies Fill asynchronously. Record the resulting
	// frame only after its animation, and only while this is still the
	// most recent Fill command. A later resize then stops restore from
	// mistaking the changed window for our filled frame.
	time.AfterFunc(700*time.Millisecond, func() {
		mygo.App.WhenReady(func() {
			if a.fillFrame != fill || win.IsDestroyed() {
				return
			}
			current := objc.ID(win.NativeHandle())
			if current == 0 || uintptr(current) != fill.window {
				a.fillFrame = nil
				return
			}
			fill.frame = windowFrame(current)
			fill.pending = false
			if fill.frame == previous {
				a.fillFrame = nil
			}
		})
	})
}
