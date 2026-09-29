package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/narratoai/narrato-go/internal/ui"
)

const version = "0.1.0-dev"

func main() {
	cli := flag.Bool("cli", false, "print version and exit (no GUI)")
	flag.Parse()

	if *cli || os.Getenv("NARRATO_CLI") == "1" {
		fmt.Fprintf(os.Stdout, "narrato-go %s\n", version)
		return
	}

	ui.Run()
}
