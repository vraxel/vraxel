package buildinfo

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var version = flag.Bool("version", false, "Show Vraxel Server version")

// Version must be set via -ldflags '-X'
var Version string

var shortVersionRe = regexp.MustCompile(`v\d+\.\d+\.\d+(?:-enterprise)?(?:-cluster)?`)

// devVersionRe matches the Makefile-built dev/CI Version string and extracts a
// concise `YYYYMMDD-HHMMSS-gSHA` identifier from it. The Makefile produces
// Versions shaped like `vraxel-server-20260513-061329-heads-main-0-g470c9a01`
// when no semver tag is present; without this fallback ShortVersion() returns
// "" and the system-info card shows blank.
var devVersionRe = regexp.MustCompile(`\d{8}-\d{6}-.*?(g[0-9a-f]+)$`)

// ShortVersion returns a human-readable build identifier. It never returns "":
//   - "" Version (ldflags unset, e.g. `go run`) -> "dev"
//   - semver tag present                         -> "vX.Y.Z[-suffix]"
//   - Makefile-built dev/CI string               -> "YYYYMMDD-gSHA"
//   - anything else                              -> the full Version string
func ShortVersion() string { return ShortVersionOf(Version) }

// ShortVersionOf is ShortVersion applied to an arbitrary version string.
// The agent's self-upgrade needs it to read the version out of a staged
// binary's -version output, which prints the full build string while
// every version the server knows is a short one.
func ShortVersionOf(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	if m := shortVersionRe.FindString(version); m != "" {
		return m
	}
	if m := devVersionRe.FindStringSubmatch(version); m != nil {
		return m[0][:8] + "-" + m[1]
	}
	return version
}

// Init must be called after flag.Parse call.
func Init() {
	if *version {
		printVersion()
		os.Exit(0)
	}
}

func init() {
	oldUsage := flag.Usage
	flag.Usage = func() {
		printVersion()
		oldUsage()
	}
}

func printVersion() {
	_, _ = fmt.Fprintf(flag.CommandLine.Output(), "%s\n", Version)
}
