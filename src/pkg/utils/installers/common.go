package installers

//go:fix inline
func Bool(b bool) *bool {
	return new(b)
}

//go:fix inline
func String(s string) *string {
	return new(s)
}
