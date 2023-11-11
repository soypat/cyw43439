/*
package seqs implements sequence number operations as per RFC 793.
*/
package seqs

// Value represents the value of a sequence number.
type Value uint32

// Size represents the size (length) of a sequence number window.
type Size uint32

// LessThan checks if v is before w, i.e., v < w.
func LessThan(v, w Value) bool {
	return int32(v-w) < 0
}

// LessThanEq returns true if v==w or v is before i.e., v < w.
func LessThanEq(v, w Value) bool {
	return v == w || LessThan(v, w)
}

// InRange checks if v is in the range [a,b), i.e., a <= v < b.
func InRange(v, a, b Value) bool {
	return v-a < b-a
}

// InWindow checks if v is in the window that starts at 'first' and spans 'size'
// sequence numbers.
func InWindow(v, first Value, size Size) bool {
	return InRange(v, first, Add(first, size))
}

// Add calculates the sequence number following the [v, v+s) window.
func Add(v Value, s Size) Value {
	return v + Value(s)
}

// Size calculates the size of the window defined by [v, w).
func Sizeof(v, w Value) Size {
	return Size(w - v)
}

// UpdateForward updates v such that it becomes v + s.
func (v *Value) UpdateForward(s Size) {
	*v += Value(s)
}
