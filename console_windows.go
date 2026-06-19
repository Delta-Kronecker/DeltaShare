package main

import "syscall"

func init() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	kernel32.NewProc("FreeConsole").Call()
	kernel32.NewProc("AllocConsole").Call()
}
