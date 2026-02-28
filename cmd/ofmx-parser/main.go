// Package main provides the CLI entrypoint for OFMX parser execution.
//
// Author: Miroslav Pašek
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/DartenZie/ofmx-parser/internal/app"
)

func main() {
	if err := app.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
