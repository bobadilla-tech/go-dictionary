// Command sum is a small showcase CLI around the sum package. It exists for
// demonstration and internal testing; most users should import the sum
// package directly instead of shelling out to this binary.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/bobadilla-tech/go-package"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sum <int> [int...]")
		os.Exit(1)
	}

	nums := make([]int, len(args))
	for i, arg := range args {
		n, err := strconv.Atoi(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid integer %q: %v\n", arg, err)
			os.Exit(1)
		}
		nums[i] = n
	}

	fmt.Println(sum.Sum(nums...))
}
