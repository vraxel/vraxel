package lflag

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// Parse parses command-line flags
// This function must be called instead of lflag.Parse() before using and flags in the program
func Parse() {
	ParseFlagSet(flag.CommandLine, os.Args[1:])
}

func ParseFlagSet(fs *flag.FlagSet, args []string) {
	args = expandArgs(args)
	if err := fs.Parse(args); err != nil {
		log.Fatalf("cannot parse flags %q: %s", args, err)
	}
	if fs.NArg() > 0 {
		log.Fatalf("unprocessed command-line args left: %s; the most likely reason is missing `=` between boolean flag name and value; "+
			"see https://pkg.go.dev/flag#hdr-Command_line_flag_syntax", fs.Args())
	}
}

// expandArgs
func expandArgs(args []string) []string {
	dstArgs := make([]string, 0, len(args))
	for _, arg := range args {
		s := ReplaceString(arg)
		if len(s) > 0 {
			dstArgs = append(dstArgs, s)
		}
	}
	return dstArgs
}

// WriteFlags writes all the explicitly set flags to w.
func WriteFlags(w io.Writer) {
	flag.Visit(func(f *flag.Flag) {
		lname := strings.ToLower(f.Name)
		value := f.Value.String()
		if IsSecretFlag(lname) {
			value = "secret"
		}
		_, _ = fmt.Fprintf(w, "-%s=%q\n", f.Name, value)
	})
}
