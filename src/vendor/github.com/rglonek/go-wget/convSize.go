package wget

import "fmt"

// SizeToString converts size to string value in B/KiB/MiB/GiB
func SizeToString(size int64) string {
	var sizeString string
	if size > 1023 && size < 1024*1024 {
		sizeString = fmt.Sprintf("%.2f KiB", float64(size)/1024)
	} else if size < 1024 {
		sizeString = fmt.Sprintf("%v B", size)
	} else if size >= 1024*1024 && size < 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f MiB", float64(size)/1024/1024)
	} else if size >= 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f GiB", float64(size)/1024/1024/1024)
	}
	return sizeString
}
