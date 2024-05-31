package inslice

// CompareStrings function is used in StringFunc and StringFuncAll to provide a third-party comparison
type CompareStrings func(sliceString string, item string) bool

// String compares an item to a slice of items and return indexes of the first X matched entries.
func String(slice []string, item string, count int) (index []int) {
	for i, r := range slice {
		if len(index) == count {
			break
		}
		if r == item {
			index = append(index, i)
		}
	}
	return
}

// StringVar compares an item to a slice of item pointers and return indexes of the first X matched entries.
func StringVar(slice []*string, item string, count int) (index []int) {
	for i, r := range slice {
		if len(index) == count {
			break
		}
		if *r == item {
			index = append(index, i)
		}
	}
	return
}

// StringAll acts like String, looking for all occurrences.
func StringAll(slice []string, item string) (index []int) {
	for i, r := range slice {
		if r == item {
			index = append(index, i)
		}
	}
	return
}

// StringVarAll acts like StringVar, looking for all occurrences.
func StringVarAll(slice []*string, item string) (index []int) {
	for i, r := range slice {
		if *r == item {
			index = append(index, i)
		}
	}
	return
}

// StringMatch acts like String, returning the index of the first found match
// returns -1 if item not found
func StringMatch(slice []string, item string) (index int) {
	ret := String(slice, item, 1)
	if len(ret) == 0 {
		return -1
	}
	return ret[0]
}

// HasString returns true if item is found in slice, or false if it hasn't
func HasString(slice []string, item string) bool {
	ret := String(slice, item, 1)
	if len(ret) == 0 {
		return false
	}
	return true
}

// StringFunc return up to X results if item matches in slice, using external
// comparison function. For example, pass strings.Contains as func
func StringFunc(slice []string, item string, compareFunc CompareStrings, count int) (index []int) {
	for i, r := range slice {
		if len(index) == count {
			break
		}
		if compareFunc(r, item) {
			index = append(index, i)
		}
	}
	return
}

// StringFuncAll returns all results if item matches in slice, using external
// comparison function. For example, pass strings.Contains as func
func StringFuncAll(slice []string, item string, compareFunc CompareStrings) (index []int) {
	for i, r := range slice {
		if compareFunc(r, item) {
			index = append(index, i)
		}
	}
	return
}

// StringVarMatch acts like String, returning the index of the first found match
// returns -1 if item not found
func StringVarMatch(slice []*string, item string) (index int) {
	ret := StringVar(slice, item, 1)
	if len(ret) == 0 {
		return -1
	}
	return ret[0]
}

// HasStringVar returns true if item is found in slice, or false if it hasn't
func HasStringVar(slice []*string, item string) bool {
	ret := StringVar(slice, item, 1)
	if len(ret) == 0 {
		return false
	}
	return true
}

// StringVarFunc return up to X results if item matches in slice, using external
// comparison function. For example, pass strings.Contains as func
func StringVarFunc(slice []*string, item string, compareFunc CompareStrings, count int) (index []int) {
	for i, r := range slice {
		if len(index) == count {
			break
		}
		if compareFunc(*r, item) {
			index = append(index, i)
		}
	}
	return
}

// StringVarFuncAll returns all results if item matches in slice, using external
// comparison function. For example, pass strings.Contains as func
func StringVarFuncAll(slice []*string, item string, compareFunc CompareStrings) (index []int) {
	for i, r := range slice {
		if compareFunc(*r, item) {
			index = append(index, i)
		}
	}
	return
}
