package inslice

// CompareUints function is used in StringFunc and StringFuncAll to provide a third-party comparison
type CompareUints func(sliceInt uint, item uint) bool

// Uint compares an item to a slice of items and return indexes of the first X matched entries.
func Uint(slice []uint, item uint, count int) (index []int) {
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

// UintVar compares an item to a slice of item pouinters and return indexes of the first X matched entries.
func UintVar(slice []*uint, item uint, count int) (index []int) {
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

// UintAll acts like Uint, looking for all occurrences.
func UintAll(slice []uint, item uint) (index []int) {
	for i, r := range slice {
		if r == item {
			index = append(index, i)
		}
	}
	return
}

// UintVarAll acts like UintVar, looking for all occurrences.
func UintVarAll(slice []*uint, item uint) (index []int) {
	for i, r := range slice {
		if *r == item {
			index = append(index, i)
		}
	}
	return
}

// UintMatch acts like Uint, returning the index of the first found match
// returns -1 if item not found
func UintMatch(slice []uint, item uint) (index int) {
	ret := Uint(slice, item, 1)
	if len(ret) == 0 {
		return -1
	}
	return ret[0]
}

// HasUint returns true if item is found in slice, or false if it hasn't
func HasUint(slice []uint, item uint) bool {
	ret := Uint(slice, item, 1)
	if len(ret) == 0 {
		return false
	}
	return true
}

// UintFunc return up to X results if item matches in slice, using external
// comparison function. For example, pass uints.Contains as func
func UintFunc(slice []uint, item uint, compareFunc CompareUints, count int) (index []int) {
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

// UintFuncAll returns all results if item matches in slice, using external
// comparison function. For example, pass uints.Contains as func
func UintFuncAll(slice []uint, item uint, compareFunc CompareUints) (index []int) {
	for i, r := range slice {
		if compareFunc(r, item) {
			index = append(index, i)
		}
	}
	return
}

// UintVarMatch acts like Uint, returning the index of the first found match
// returns -1 if item not found
func UintVarMatch(slice []*uint, item uint) (index int) {
	ret := UintVar(slice, item, 1)
	if len(ret) == 0 {
		return -1
	}
	return ret[0]
}

// HasUintVar returns true if item is found in slice, or false if it hasn't
func HasUintVar(slice []*uint, item uint) bool {
	ret := UintVar(slice, item, 1)
	if len(ret) == 0 {
		return false
	}
	return true
}

// UintVarFunc return up to X results if item matches in slice, using external
// comparison function. For example, pass uints.Contains as func
func UintVarFunc(slice []*uint, item uint, compareFunc CompareUints, count int) (index []int) {
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

// UintVarFuncAll returns all results if item matches in slice, using external
// comparison function. For example, pass uints.Contains as func
func UintVarFuncAll(slice []*uint, item uint, compareFunc CompareUints) (index []int) {
	for i, r := range slice {
		if compareFunc(*r, item) {
			index = append(index, i)
		}
	}
	return
}
