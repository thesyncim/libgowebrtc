//go:build (!linux || !cgo) && !windows

package ffi

import "github.com/ebitengine/purego"

// RTLD flags for dlopen - exported from purego for use by lib.go
const (
	RTLD_NOW    = purego.RTLD_NOW
	RTLD_GLOBAL = purego.RTLD_GLOBAL
)

func dlopenLibrary(path string, flags int) (uintptr, error) {
	return purego.Dlopen(path, flags)
}

func dlsymLibrary(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}

func dlcloseLibrary(handle uintptr) error {
	return purego.Dlclose(handle)
}
