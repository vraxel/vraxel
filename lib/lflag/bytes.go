package lflag

import (
	"flag"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// NewBytes returns new `bytes` flag with the given name, defaultValue and description.
func NewBytes(name string, defaultValue int64, description string) *Bytes {
	description += "\nSupports the following optional suffixes for `size` values: KB, MB, GB, TB, KiB, MiB, GiB, TiB"
	b := Bytes{
		N:           defaultValue,
		Name:        name,
		valueString: fmt.Sprintf("%d", defaultValue),
	}
	flag.Var(&b, name, description)
	return &b
}

// Bytes is a flag for holding size in bytes.
//
// It supports the following optional suffixes for values: KB, MB, GB, TB, KiB, MiB, GiB, TiB.
type Bytes struct {
	// N contains parsed value for the given flag.
	N int64

	// Name contains flag name.
	Name string

	valueString string
}

// IntN returns the stored value capped by int type.
func (b *Bytes) IntN() int {
	if b.N > math.MaxInt {
		return math.MaxInt
	}
	if b.N < math.MinInt {
		return math.MinInt
	}
	return int(b.N)
}

// String implements flag.Value interface
func (b *Bytes) String() string {
	return b.valueString
}

// Set implements flag.Value interface
func (b *Bytes) Set(value string) error {
	if value == "" {
		b.N = 0
		b.valueString = ""
		return nil
	}
	value = normalizeBytesString(value)
	n, err := parseBytes(value)
	if err != nil {
		return err
	}
	b.N = n
	b.valueString = value
	return nil
}

// ParseBytes returns int64 in bytes of parsed string with unit suffix
func ParseBytes(value string) (int64, error) {
	value = normalizeBytesString(value)
	return parseBytes(value)
}

func parseBytes(value string) (int64, error) {
	switch {
	case strings.HasSuffix(value, "KB"):
		return parseBytesScaled(value[:len(value)-2], 1000)
	case strings.HasSuffix(value, "MB"):
		return parseBytesScaled(value[:len(value)-2], 1000*1000)
	case strings.HasSuffix(value, "GB"):
		return parseBytesScaled(value[:len(value)-2], 1000*1000*1000)
	case strings.HasSuffix(value, "TB"):
		return parseBytesScaled(value[:len(value)-2], 1000*1000*1000*1000)
	case strings.HasSuffix(value, "KiB"):
		return parseBytesScaled(value[:len(value)-3], 1024)
	case strings.HasSuffix(value, "MiB"):
		return parseBytesScaled(value[:len(value)-3], 1024*1024)
	case strings.HasSuffix(value, "GiB"):
		return parseBytesScaled(value[:len(value)-3], 1024*1024*1024)
	case strings.HasSuffix(value, "TiB"):
		return parseBytesScaled(value[:len(value)-3], 1024*1024*1024*1024)
	default:
		return parseBytesScaled(value, 1)
	}
}

// parseBytesScaled parses num as a float and multiplies it by scale, preserving
// the original per-suffix behaviour (int64(f * scale)). Extracted so parseBytes
// stays a flat suffix switch instead of repeating parse+error-check per case.
func parseBytesScaled(num string, scale float64) (int64, error) {
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, err
	}
	return int64(f * scale), nil
}

func normalizeBytesString(s string) string {
	s = strings.ToUpper(s)
	return strings.ReplaceAll(s, "I", "i")
}
