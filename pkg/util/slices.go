package util

// StringSliceContains checks whether a slice contains a string
func StringSliceContains(strSlice []string, search string) bool {
	for _, v := range strSlice {
		if v == search {
			return true
		}
	}
	return false
}
