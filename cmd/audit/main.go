package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/furyheimdall/toolfence-deadbugz-guard/audit"
)

func main() {
	path := flag.String("path", "audit.jsonl", "append-only JSONL audit file")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "tail":
		fs := flag.NewFlagSet("tail", flag.ExitOnError)
		n := fs.Int("n", 10, "last N events")
		_ = fs.Parse(args[1:])
		rows, err := audit.Tail(*path, *n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "audit tail: %v\n", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		for _, row := range rows {
			if err := enc.Encode(row); err != nil {
				fmt.Fprintf(os.Stderr, "audit tail: %v\n", err)
				os.Exit(1)
			}
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: audit -path FILE tail -n N\n")
}
