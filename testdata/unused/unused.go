package unused

type widget struct {
	usedField   int
	unusedField int
}

func (w *widget) show() int {
	return w.usedField
}

func (w *widget) neverCalled() int {
	return 1
}

func compute(a int, unusedParam int) int {
	writeOnly := 0
	writeOnly = 5
	return a
}
