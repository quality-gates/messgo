package p

type T struct{ n int }

func Run(t *T) { f := Run; _ = f; a0(t) }
func a0(t *T)  { a1(t); b1(t) }
func b0(t *T)  { a1(t); b1(t) }
func a1(t *T)  { a2(t); b2(t) }
func b1(t *T)  { a2(t); b2(t) }
func a2(t *T)  { a3(t); b3(t) }
func b2(t *T)  { a3(t); b3(t) }
func a3(t *T)  { a4(t); b4(t) }
func b3(t *T)  { a4(t); b4(t) }
func a4(t *T)  { a5(t); b5(t) }
func b4(t *T)  { a5(t); b5(t) }
func a5(t *T)  { a6(t); b6(t) }
func b5(t *T)  { a6(t); b6(t) }
func a6(t *T)  { a7(t); b7(t) }
func b6(t *T)  { a7(t); b7(t) }
func a7(t *T)  { a8(t); b8(t) }
func b7(t *T)  { a8(t); b8(t) }
func a8(t *T)  { a9(t); b9(t) }
func b8(t *T)  { a9(t); b9(t) }
func a9(t *T)  { a10(t); b10(t) }
func b9(t *T)  { a10(t); b10(t) }
func a10(t *T) { a11(t); b11(t) }
func b10(t *T) { a11(t); b11(t) }
func a11(t *T) { a12(t); b12(t) }
func b11(t *T) { a12(t); b12(t) }
func a12(t *T) { t.n++ }
func b12(t *T) { t.n++ }
