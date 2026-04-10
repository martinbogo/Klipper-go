package util

import "fmt"

// RangeInt returns a channel that emits integers from min (inclusive) to max
// (exclusive) in increments of step (default 1).
func RangeInt(min, max int, step ...int) <-chan int {
	if min > max {
		panic("min should less than max")
	}
	ch := make(chan int)
	stepi := 1
	if len(step) > 0 {
		stepi = step[0]
	}
	go func() {
		defer close(ch)
		for i := min; i < max; i = i + stepi {
			ch <- i
		}
	}()
	return ch
}

// FloorDiv returns the integer floor division of a by b.
func FloorDiv(a, b int) int {
	return int(a / b)
}

// JoinIntWithFormat formats each int in elem with format, joined by sep.
func JoinIntWithFormat(elem []int, sep, format string) string {
	elems := make([]interface{}, len(elem))
	for idx := range elem {
		elems[idx] = elem[idx]
	}
	return JoinWithFormat(elems, sep, format)
}

// JoinByteWithFormat formats each byte in elem with format, joined by sep.
func JoinByteWithFormat(elem []byte, sep, format string) string {
	elems := make([]interface{}, len(elem))
	for idx := range elem {
		elems[idx] = elem[idx]
	}
	return JoinWithFormat(elems, sep, format)
}

// JoinWithFormat formats each element in elems with format, joined by sep.
func JoinWithFormat(elems []interface{}, sep, format string) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf(format, elems[0])
	}
	strs := fmt.Sprintf(format, elems[0])
	for _, elem := range elems[1:] {
		strs = sep + fmt.Sprintf(format, elem)
	}
	return strs
}
