package core

func Clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func ClampInt32(value, low, high int32) int32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func Abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func AbsInt32(value int32) int32 {
	if value < 0 {
		return -value
	}
	return value
}
