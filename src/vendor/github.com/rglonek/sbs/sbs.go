package sbs

import "unsafe"

// Convert []byte to string
//
// NOTE: For sanity's sake, if modifying the underlying byte slice, it is best to cast again to the string variable after the modification has taken place. This will ensure the len is set correctly.
func ByteSliceToString(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}

// Convert []byte to string using unsafe.SliceData
//
// NOTE: For sanity's sake, if modifying the underlying byte slice, it is best to cast again to the string variable after the modification has taken place. This will ensure the len is set correctly.
func ByteSliceToStringAlt(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Convert string to []byte using unsafe.StringData
//
// NOTE: The resulting byte slice cannot have values modified (panic). Performing an append will result in a copy and new underlying array as expected.
func StringToByteSlice(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
