//go:build linux && amd64

package write

import (
	"syscall"
	"unsafe"
)

const (
	linuxRenameat2AMD64 = 316
	linuxRenameExchange = 2
)

func exchangeDirectories(first, second string) bool {
	firstPath, err := syscall.BytePtrFromString(first)
	if err != nil {
		return false
	}
	secondPath, err := syscall.BytePtrFromString(second)
	if err != nil {
		return false
	}
	_, _, errno := syscall.Syscall6(
		linuxRenameat2AMD64,
		^uintptr(99),
		uintptr(unsafe.Pointer(firstPath)),
		^uintptr(99),
		uintptr(unsafe.Pointer(secondPath)),
		linuxRenameExchange,
		0,
	)
	return errno == 0
}
