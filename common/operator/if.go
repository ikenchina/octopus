package operator

// ternary operator
func IfElse(cond bool, condTrue interface{}, condFalse interface{}) interface{} {
	if cond {
		return condTrue
	}
	return condFalse
}
