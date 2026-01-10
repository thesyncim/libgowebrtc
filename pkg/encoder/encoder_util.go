package encoder

func boolToInt32(value bool) int32 {
	if value {
		return 1
	}
	return 0
}
