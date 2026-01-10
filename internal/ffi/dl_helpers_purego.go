//go:build !linux || !cgo

package ffi

import "github.com/ebitengine/purego"

func dlopenLibrary(path string, flags int) (uintptr, error) {
	return purego.Dlopen(path, flags)
}

func dlsymLibrary(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}

func dlcloseLibrary(handle uintptr) error {
	return purego.Dlclose(handle)
}
