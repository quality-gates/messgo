package naming

const my_constant = 5

type Fo struct {
	x int
}

type ThisIsAnExcessivelyLongClassNameThatExceedsForty struct{}

func (f *Fo) a(b int) int {
	q := b + 1
	thisIsAReallyLongLocalVariableNameOver20 := q
	return thisIsAReallyLongLocalVariableNameOver20
}

func getActive() bool { return true }
