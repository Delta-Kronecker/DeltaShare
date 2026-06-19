package main

import (
	"syscall"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole = kernel32.NewProc("AllocConsole")
)

func init() {
	procAllocConsole.Call()
}
