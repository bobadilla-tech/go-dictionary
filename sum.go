// Package sum provides a minimal example of a well-tested Go library.
package sum

// Sum returns the sum of all provided integers. Sum() with no arguments returns 0.
func Sum(nums ...int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}
