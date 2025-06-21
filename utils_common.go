package main

func Convert(num uint64) (float64, string) {
	units := []struct {
		Factor uint64
		Suffix string
	}{
		{1 << 30, "GB"},
		{1 << 20, "MB"},
		{1 << 10, "KB"},
		{1, "bytes"},
	}

	for _, unit := range units {
		if num >= unit.Factor {
			return float64(num) / float64(unit.Factor), unit.Suffix
		}
	}

	return float64(num), "bytes"
}
