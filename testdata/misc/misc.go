package misc

import "os"

type bad_name struct {
	first_field int
}

func process(enable bool, items []int) {
	if len(items) > 0 {
	}
	for i := 0; i < len(items); i++ {
		println("debug", i)
	}
	x := 0
	if x = compute(); x > 0 {
		doThing()
	} else {
		doOther()
	}
	m := map[string]int{"a": 1, "a": 2}
	_ = m
	os.Exit(1)
goto_label:
	goto goto_label
}

func compute() int { return 1 }
func doThing()     {}
func doOther()     {}
