package main

import (
	"os"
	"syscall"
)

func init() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")

	kernel32.NewProc("FreeConsole").Call()
	kernel32.NewProc("AllocConsole").Call()

	conout, _ := syscall.UTF16PtrFromString("CONOUT$")
	conin, _ := syscall.UTF16PtrFromString("CONIN$")

	hOut, _ := syscall.CreateFile(conout, syscall.GENERIC_WRITE, syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	hErr, _ := syscall.CreateFile(conout, syscall.GENERIC_WRITE, syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	hIn, _ := syscall.CreateFile(conin, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, 0, 0)

	syscall.Stdout = hOut
	syscall.Stderr = hErr
	syscall.Stdin = hIn

	os.Stdout = os.NewFile(uintptr(hOut), "stdout")
	os.Stderr = os.NewFile(uintptr(hErr), "stderr")
	os.Stdin = os.NewFile(uintptr(hIn), "stdin")
}
