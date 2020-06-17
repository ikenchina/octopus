package slice

func Contain(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func InSlice(ele interface{}, container ...interface{}) bool {
	for _, s := range container {
		if s == ele {
			return true
		}
	}
	return false
}
