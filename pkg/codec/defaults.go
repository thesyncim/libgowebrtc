package codec

import "runtime"

func defaultPreferHWH264() bool {
	return runtime.GOOS == "darwin"
}

func defaultPreferHW() bool {
	return true
}
