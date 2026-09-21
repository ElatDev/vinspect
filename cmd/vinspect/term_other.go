//go:build !windows

package main

import "os"

// enableANSI reports whether f can show ANSI colors. Terminals outside
// Windows always can.
func enableANSI(*os.File) bool { return true }
