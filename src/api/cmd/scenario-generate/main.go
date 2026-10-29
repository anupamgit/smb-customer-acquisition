package main

import (
	"flag"
	"fmt"
	"os"

	"advisor/scenario"
)

func main() {
	all := flag.Bool("all", false, "generate all built-in scenarios")
	name := flag.String("scenario", "", "scenario name (see scenario.AllNames)")
	dir := flag.String("dir", "", "data directory (default: find data/scenarios from cwd)")
	flag.Parse()

	var baseDir string
	if *dir != "" {
		baseDir = *dir
	} else if found, ok := scenario.FindDataDir(""); ok {
		baseDir = found
	} else {
		baseDir = scenario.DefaultRelativeDir
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	names := scenario.AllNames()
	if *name != "" {
		names = []string{*name}
	} else if !*all {
		flag.Usage()
		os.Exit(2)
	}

	for _, n := range names {
		sf, err := scenario.Generate(n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		path, err := scenario.Write(baseDir, n, sf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d customers, %d orders)\n", path, len(sf.Customers), len(sf.Orders))
	}
}
