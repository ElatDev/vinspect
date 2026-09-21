package main

import (
	"os"
	"syscall"
)

var procSetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")

const enableVirtualTerminalProcessing = 0x0004

// enableANSI turns on escape-sequence processing for a Windows console, and
// arranges for restoreConsole to put the original mode back on exit. It
// reports false if f is not a console that accepts the change.
//
// Text needs no such help: Go writes to consoles with WriteConsoleW, so the
// console code page does not affect the decode tree's box-drawing characters.
func enableANSI(f *os.File) bool {
	h := syscall.Handle(f.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	if ok, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing)); ok == 0 {
		return false
	}
	restoreConsole = func() { procSetConsoleMode.Call(uintptr(h), uintptr(mode)) }
	return true
}
