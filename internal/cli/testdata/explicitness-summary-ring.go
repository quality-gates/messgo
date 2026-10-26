package p

type T struct{ n int }

func f0(t *T) {
	t.n++
	if t.n < 100 {
		f1(t)
		f2(t)
	}
}
func f1(t *T) {
	t.n++
	if t.n < 100 {
		f2(t)
		f3(t)
	}
}
func f2(t *T) {
	t.n++
	if t.n < 100 {
		f3(t)
		f4(t)
	}
}
func f3(t *T) {
	t.n++
	if t.n < 100 {
		f4(t)
		f5(t)
	}
}
func f4(t *T) {
	t.n++
	if t.n < 100 {
		f5(t)
		f6(t)
	}
}
func f5(t *T) {
	t.n++
	if t.n < 100 {
		f6(t)
		f7(t)
	}
}
func f6(t *T) {
	t.n++
	if t.n < 100 {
		f7(t)
		f8(t)
	}
}
func f7(t *T) {
	t.n++
	if t.n < 100 {
		f8(t)
		f9(t)
	}
}
func f8(t *T) {
	t.n++
	if t.n < 100 {
		f9(t)
		f10(t)
	}
}
func f9(t *T) {
	t.n++
	if t.n < 100 {
		f10(t)
		f11(t)
	}
}
func f10(t *T) {
	t.n++
	if t.n < 100 {
		f11(t)
		f12(t)
	}
}
func f11(t *T) {
	t.n++
	if t.n < 100 {
		f12(t)
		f13(t)
	}
}
func f12(t *T) {
	t.n++
	if t.n < 100 {
		f13(t)
		f14(t)
	}
}
func f13(t *T) {
	t.n++
	if t.n < 100 {
		f14(t)
		f15(t)
	}
}
func f14(t *T) {
	t.n++
	if t.n < 100 {
		f15(t)
		f0(t)
	}
}
func f15(t *T) {
	t.n++
	if t.n < 100 {
		f0(t)
		f1(t)
	}
}
