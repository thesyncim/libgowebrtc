//go:build windows

package ffi

import (
	"fmt"
	"syscall"
	"unsafe"
)

// RTLD flags - not used on Windows but defined for compatibility
const (
	RTLD_NOW    = 0
	RTLD_GLOBAL = 0
)

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	loadLibraryW    = kernel32.NewProc("LoadLibraryW")
	getProcAddress  = kernel32.NewProc("GetProcAddress")
	freeLibrary     = kernel32.NewProc("FreeLibrary")
)

func dlopenLibrary(path string, flags int) (uintptr, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	handle, _, err := loadLibraryW.Call(uintptr(unsafe.Pointer(pathPtr)))
	if handle == 0 {
		return 0, fmt.Errorf("LoadLibrary failed: %w", err)
	}
	return handle, nil
}

func dlsymLibrary(handle uintptr, name string) (uintptr, error) {
	namePtr, err := syscall.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	addr, _, err := getProcAddress.Call(handle, uintptr(unsafe.Pointer(namePtr)))
	if addr == 0 {
		return 0, fmt.Errorf("GetProcAddress(%s) failed: %w", name, err)
	}
	return addr, nil
}

func dlcloseLibrary(handle uintptr) error {
	ret, _, err := freeLibrary.Call(handle)
	if ret == 0 {
		return fmt.Errorf("FreeLibrary failed: %w", err)
	}
	return nil
}
