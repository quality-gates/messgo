package codesize

func highComplexity(a, b, c, d, e int) int {
	x := 0
	if a > 0 && b > 0 {
		x++
	}
	if a > 1 || b > 1 {
		x++
	}
	for i := 0; i < a; i++ {
		if i%2 == 0 {
			x++
		}
	}
	switch c {
	case 1:
		x++
	case 2:
		x++
	case 3:
		x++
	}
	if d > 0 {
		x++
	}
	if e > 0 {
		x++
	}
	return x
}

type Big struct {
	A, B, C, D, E, F, G, H int
	I, J, K, L, M, N, O, P int
}

func manyParams(a, b, c, d, e, f, g, h, i, j, k int) {}
