//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbOK          = 0x00000000
	mbIconError   = 0x00000010
	mbSystemModal = 0x00001000
)

// fatalDialog shows a native Windows message box for a fatal startup
// failure. The shipped app has zero console (-H=windowsgui strips it, per
// the "zero CLI surface for end users" requirement) — without this, a
// startup failure (e.g. can't create characters.db) is completely invisible
// to a player, the app just silently does nothing. Zero-dependency syscall,
// not a GUI toolkit import, to keep the pure-Go/no-cgo build intact.
func fatalDialog(title, message string) {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	msgPtr, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(msgPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|mbIconError|mbSystemModal),
	)
}
