package main

import (
	"fmt"
	"os"

	"github.com/florian/vinculum/apps/vnclm/internal/commands"
)

func main() {
	if err := commands.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
