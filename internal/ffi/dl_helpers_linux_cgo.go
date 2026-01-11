//go:build linux && cgo

package ffi

/*
#cgo LDFLAGS: -ldl

#include <dlfcn.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// RTLD flags for dlopen - using C constants from dlfcn.h
const (
	RTLD_NOW    = C.RTLD_NOW
	RTLD_GLOBAL = C.RTLD_GLOBAL
)

func dlopenLibrary(path string, flags int) (uintptr, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	handle := C.dlopen(cpath, C.int(flags))
	if handle == nil {
		return 0, fmt.Errorf("dlopen: %s", C.GoString(C.dlerror()))
	}
	return uintptr(handle), nil
}

func dlsymLibrary(handle uintptr, name string) (uintptr, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	symbol := C.dlsym(unsafe.Pointer(handle), cname)
	if symbol == nil {
		return 0, fmt.Errorf("dlsym: %s", C.GoString(C.dlerror()))
	}
	return uintptr(symbol), nil
}

func dlcloseLibrary(handle uintptr) error {
	if handle == 0 {
		return nil
	}
	if rc := C.dlclose(unsafe.Pointer(handle)); rc != 0 {
		return fmt.Errorf("dlclose: %s", C.GoString(C.dlerror()))
	}
	return nil
}
