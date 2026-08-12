package main

import (
	"flag"
	"fmt"
	"os"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/config"
	"vraxel.io/vraxel/lib/openapi"
	"vraxel.io/vraxel/pkg/apis"
)

// routes registers every module on a bare apiserver -- no database, no
// authorizer -- purely to read back the route table. That table is what
// the spec describes: deriving endpoints from source-code shape is how
// the spec came to advertise operations answering 405 and to miss real
// paths such as /api/audit/v1/logs.
func routes() []openapi.Route {
	// Registration may read the global config (e.g. a module stamps the
	// deployment's server.name into a query helper at construction). It
	// must never touch the database, but config is a legitimate global,
	// always set in a running process -- so provide a defaulted one here
	// rather than force every constructor to be config-free.
	cfg := &config.Config{}
	config.SetDefaults(cfg)
	config.Set(cfg)

	srv := apiserver.New(apiserver.Config{})
	for _, register := range apis.Registrars() {
		register(srv)
	}
	infos := srv.Routes()
	out := make([]openapi.Route, 0, len(infos))
	for _, r := range infos {
		out = append(out, openapi.Route{
			Method: r.Method, Path: r.Path, Kind: r.Kind,
			Group: r.Group, Resource: r.Resource,
			TypeName: r.TypeName, Name: r.Name,
		})
	}
	return out
}

func main() {
	var (
		apisDir string
		output  string
		format  string
		title   string
		version string
	)

	// A private FlagSet, not the global flag.CommandLine: importing
	// pkg/apis pulls in the VictoriaMetrics-lineage libraries, which
	// register their own -version flag in init(). Sharing the global set
	// would panic with "flag redefined: version" before main runs.
	fs := flag.NewFlagSet("openapi-gen", flag.ExitOnError)
	fs.StringVar(&apisDir, "apis-dir", "pkg/apis", "Directory containing API type definitions")
	fs.StringVar(&output, "output", "", "Output file path (default: stdout)")
	fs.StringVar(&format, "format", "json", "Output format: json or yaml")
	fs.StringVar(&title, "title", "Vraxel API", "API title")
	fs.StringVar(&version, "version", "v1", "API version")
	_ = fs.Parse(os.Args[1:])

	parser := openapi.NewParser(apisDir)
	groups, err := parser.Parse()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error parsing API types: %v\n", err)
		os.Exit(1)
	}

	generator := openapi.NewGenerator(title, "Vraxel Platform API", version)
	generator.SetRoutes(routes())
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
