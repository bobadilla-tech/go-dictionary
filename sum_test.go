package sum

import "testing"

func TestSum(t *testing.T) {
	tests := []struct {
		name string
		nums []int
		want int
	}{
		{"no args", nil, 0},
		{"single value", []int{5}, 5},
		{"multiple positive", []int{1, 2, 3, 4}, 10},
		{"negative numbers", []int{-1, -2, -3}, -6},
		{"mixed", []int{-5, 10, -2, 7}, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sum(tt.nums...); got != tt.want {
				t.Errorf("Sum(%v) = %d, want %d", tt.nums, got, tt.want)
			}
		})
	}
}
