package inslice

// CompareInts function is used in StringFunc and StringFuncAll to provide a third-party comparison
type CompareInts func(sliceInt int, item int) bool

// Int compares an item to a slice of items and return indexes of the first X matched entries.
func Int(slice []int, item int, count int) (index []int) {
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

// IntVar compares an item to a slice of item pointers and return indexes of the first X matched entries.
func IntVar(slice []*int, item int, count int) (index []int) {
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

// IntAll acts like Int, looking for all occurrences.
func IntAll(slice []int, item int) (index []int) {
	for i, r := range slice {
		if r == item {
			index = append(index, i)
		}
	}
	return
}

// IntVarAll acts like IntVar, looking for all occurrences.
func IntVarAll(slice []*int, item int) (index []int) {
	for i, r := range slice {
		if *r == item {
			index = append(index, i)
		}
	}
	return
}

// IntMatch acts like Int, returning the index of the first found match
// returns -1 if item not found
func IntMatch(slice []int, item int) (index int) {
	ret := Int(slice, item, 1)
	if len(ret) == 0 {
		return -1
	}
	return ret[0]
}

// HasInt returns true if item is found in slice, or false if it hasn't
func HasInt(slice []int, item int) bool {
	ret := Int(slice, item, 1)
	if len(ret) == 0 {
		return false
	}
	return true
}

// IntFunc return up to X results if item matches in slice, using external
// comparison function. For example, pass ints.Contains as func
func IntFunc(slice []int, item int, compareFunc CompareInts, count int) (index []int) {
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

// IntFuncAll returns all results if item matches in slice, using external
// comparison function. For example, pass ints.Contains as func
func IntFuncAll(slice []int, item int, compareFunc CompareInts) (index []int) {
	for i, r := range slice {
		if compareFunc(r, item) {
			index = append(index, i)
		}
	}
	return
}

// IntVarMatch acts like Int, returning the index of the first found match
// returns -1 if item not found
func IntVarMatch(slice []*int, item int) (index int) {
	ret := IntVar(slice, item, 1)
	if len(ret) == 0 {
		return -1
	}
	return ret[0]
}

// HasIntVar returns true if item is found in slice, or false if it hasn't
func HasIntVar(slice []*int, item int) bool {
	ret := IntVar(slice, item, 1)
	if len(ret) == 0 {
		return false
	}
	return true
}

// IntVarFunc return up to X results if item matches in slice, using external
// comparison function. For example, pass ints.Contains as func
func IntVarFunc(slice []*int, item int, compareFunc CompareInts, count int) (index []int) {
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

// IntVarFuncAll returns all results if item matches in slice, using external
// comparison function. For example, pass ints.Contains as func
func IntVarFuncAll(slice []*int, item int, compareFunc CompareInts) (index []int) {
	for i, r := range slice {
		if compareFunc(*r, item) {
			index = append(index, i)
		}
	}
	return
}
