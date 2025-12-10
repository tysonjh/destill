// Package main provides the view command for querying and displaying findings from Postgres.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: destill view <request-id>")
		os.Exit(1)
	}

	requestID := os.Args[1]
	fmt.Printf("Viewing findings for request: %s\n", requestID)
	fmt.Println("(View command implementation coming in Phase 6)")
}

