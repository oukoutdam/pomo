package main

import (
	"fmt"
	"os"

	"github.com/oukoutdam/pomo/internal/pomo"
)

func main() {
	if err := pomo.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "pomo:", err)
		os.Exit(1)
	}
}
