package main

import (
	"flag"
	"fmt"
	"os"

	"vraxel.io/vraxel/lib/openapi"
)

func main() {
	var (
		apisDir string
		output  string
		format  string
		title   string
		version string
	)

	flag.StringVar(&apisDir, "apis-dir", "pkg/apis", "Directory containing API type definitions")
	flag.StringVar(&output, "output", "", "Output file path (default: stdout)")
	flag.StringVar(&format, "format", "json", "Output format: json or yaml")
	flag.StringVar(&title, "title", "Vraxel API", "API title")
	flag.StringVar(&version, "version", "v1", "API version")
	flag.Parse()

	parser := openapi.NewParser(apisDir)
	groups, err := parser.Parse()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error parsing API types: %v\n", err)
		os.Exit(1)
	}

	generator := openapi.NewGenerator(title, "Vraxel Platform API", version)
	doc := generator.Generate(groups)

	var w *os.File
	if output != "" {
		w, err = os.Create(output)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer func(w *os.File) {
			_ = w.Close()
		}(w)
	} else {
		w = os.Stdout
	}

	switch format {
	case "yaml":
		if err := openapi.WriteYAML(w, doc); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error writing YAML: %v\n", err)
			os.Exit(1)
		}
	default:
		if err := openapi.WriteJSON(w, doc); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(1)
		}
	}
}
