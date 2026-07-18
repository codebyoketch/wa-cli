// Command wa is the wa-cli entry point.
package main

import (
	"fmt"
	"os"

	"github.com/codebyoketch/wa-cli/internal/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wa: fatal:", err)
		os.Exit(1)
	}

	if err := a.RootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
