package versions

import (
	"strconv"
	"strings"
)

// Compare returns -1 if a < b, 0 if a == b, 1 if a > b
//
//	1 == a is newer than b
//	-1 == a is older than b
//	0 == a and b are the same version
func Compare(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	ax := strings.Split(a, "-")
	bx := strings.Split(b, "-")
	av := strings.Split(ax[0], ".")
	bv := strings.Split(bx[0], ".")
	atail := ""
	btail := ""
	if len(ax) > 1 {
		atail = ax[1]
	}
	if len(bx) > 1 {
		btail = bx[1]
	}
	for i := 0; i < len(av) && i < len(bv); i++ {
		ai, _ := strconv.Atoi(av[i])
		bi, _ := strconv.Atoi(bv[i])
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	if len(av) < len(bv) {
		return -1
	}
	if len(av) > len(bv) {
		return 1
	}
	if atail < btail {
		return -1
	}
	if atail > btail {
		return 1
	}
	return 0
}

// Latest returns the latest version of a and b
func Latest(a, b string) string {
	if Compare(a, b) < 0 {
		return a
	}
	return b
}

// Oldest returns the oldest version of a and b
func Oldest(a, b string) string {
	if Compare(a, b) < 0 {
		return b
	}
	return a
}
