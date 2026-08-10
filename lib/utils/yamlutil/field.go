package yamlutil

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// indirect walks down 'value' allocating pointers as needed,
// until it gets to a non-pointer.
// if it encounters an Unmarshaler, indirect stops and returns that.
// if decodingNull is true, indirect stops at the last pointer so it can be set to nil.
func indirect(value reflect.Value, decodingNull bool) (json.Unmarshaler, encoding.TextUnmarshaler, reflect.Value) {
	// If 'value' is a named type and is addressable,
	// start with its address, so that if the type has pointer methods,
	// we find them.
	if value.Kind() != reflect.Ptr && value.Type().Name() != "" && value.CanAddr() {
		value = value.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if next, ok := indirectInterface(value, decodingNull); ok {
			value = next
			continue
		}

		if value.Kind() != reflect.Ptr {
			break
		}

		if value.Elem().Kind() != reflect.Ptr && decodingNull && value.CanSet() {
			break
		}
		indirectAllocPtr(&value)
		if u, tu, ok := indirectUnmarshaler(value); ok {
			return u, tu, reflect.Value{}
		}
		value = value.Elem()
	}
	return nil, nil, value
}

// indirectInterface unwraps a non-nil interface to a usefully-addressable
// pointer element, reporting whether the loop in indirect should continue
// with the returned value.
func indirectInterface(value reflect.Value, decodingNull bool) (reflect.Value, bool) {
	if value.Kind() == reflect.Interface && !value.IsNil() {
		element := value.Elem()
		if element.Kind() == reflect.Ptr && !element.IsNil() && (!decodingNull || element.Elem().Kind() == reflect.Ptr) {
			return element, true
		}
	}
	return value, false
}

// indirectAllocPtr allocates a new value for a nil pointer, setting it in
// place when addressable.
func indirectAllocPtr(value *reflect.Value) {
	if value.IsNil() {
		if value.CanSet() {
			value.Set(reflect.New(value.Type().Elem()))
		} else {
			*value = reflect.New(value.Type().Elem())
		}
	}
}

// indirectUnmarshaler reports whether value implements one of the supported
// unmarshaler interfaces, returning the matched implementation.
func indirectUnmarshaler(value reflect.Value) (json.Unmarshaler, encoding.TextUnmarshaler, bool) {
	if value.Type().NumMethod() > 0 {
		if u, ok := value.Interface().(json.Unmarshaler); ok {
			return u, nil, true
		}
		if u, ok := value.Interface().(encoding.TextUnmarshaler); ok {
			return nil, u, true
		}
	}
	return nil, nil, false
}

// A field represents a single field found in a struct.
type field struct {
	name      string
	nameBytes []byte                 // []byte(name)
	equalFold func(s, t []byte) bool // bytes.EqualFold or equivalent

	tag       bool
	index     []int
	typ       reflect.Type
	omitEmpty bool
	quoted    bool
}

func fillField(f field) field {
	f.nameBytes = []byte(f.name)
	f.equalFold = foldFunc(f.nameBytes)
	return f
}

// byName sorts field by name, breaking ties with depth,
// then breaking ties with "name came from json tag", then
// breaking ties with index sequence.
type byName []field

func (x byName) Len() int { return len(x) }

func (x byName) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (x byName) Less(i, j int) bool {
	if x[i].name != x[j].name {
		return x[i].name < x[j].name
	}
	if len(x[i].index) != len(x[j].index) {
		return len(x[i].index) < len(x[j].index)
	}
	if x[i].tag != x[j].tag {
		return x[i].tag
	}
	return byIndex(x).Less(i, j)
}

// byIndex sorts field by index sequence.
type byIndex []field

func (x byIndex) Len() int { return len(x) }

func (x byIndex) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (x byIndex) Less(i, j int) bool {
	for k, xik := range x[i].index {
		if k >= len(x[j].index) {
			return false
		}
		if xik != x[j].index[k] {
			return xik < x[j].index[k]
		}
	}
	return len(x[i].index) < len(x[j].index)
}

// typeFields returns a list of fields that JSON should recognize for the given type.
// The algorithm is breadth-first search over the set of structs to include - the top struct
// and then any reachable anonymous structs.
func typeFields(t reflect.Type) []field {
	// Anonymous fields to explore at the current level and the next.
	var current []field
	next := []field{{typ: t}}

	// Count of queued names for current level and the next.
	var count map[reflect.Type]int
	var nextCount map[reflect.Type]int

	// Types already visited at an earlier level.
	visited := map[reflect.Type]bool{}

	// Fields found.
	var fields []field

	for len(next) > 0 {
		current, next = next, current[:0]
		count, nextCount = nextCount, map[reflect.Type]int{}

		for _, f := range current {
			if visited[f.typ] {
				continue
			}
			visited[f.typ] = true

			// Scan f.typ for fields to include.
			fields, next = scanStructFields(f, count, nextCount, fields, next)
		}
	}

	sort.Sort(byName(fields))

	fields = deleteHiddenFields(fields)
	sort.Sort(byIndex(fields))

	return fields
}

// scanStructFields scans f.typ for fields to include, appending found fields
// to fields and any anonymous structs to explore to next, returning the
// updated slices.
func scanStructFields(f field, count, nextCount map[reflect.Type]int, fields, next []field) ([]field, []field) {
	for i := 0; i < f.typ.NumField(); i++ {
		sf := f.typ.Field(i)
		if sf.PkgPath != "" { // unexported
			continue
		}
		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts := parseTag(tag)
		if !isValidTag(name) {
			name = ""
		}
		index := make([]int, len(f.index)+1)
		copy(index, f.index)
		index[len(f.index)] = i

		ft := sf.Type
		if ft.Name() == "" && ft.Kind() == reflect.Ptr {
			// Follow pointer.
			ft = ft.Elem()
		}

		// Record found field and index sequence.
		if name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
			fields = appendFoundField(fields, sf, ft, name, opts, index, count[f.typ])
			continue
		}

		// Record new anonymous struct to explore in next round.
		nextCount[ft]++
		if nextCount[ft] == 1 {
			next = append(next, fillField(field{name: ft.Name(), index: index, typ: ft}))
		}
	}
	return fields, next
}

// appendFoundField records a found field and its index sequence, duplicating
// it when the struct was reached via multiple instances so the annihilation
// code sees a duplicate. typeCount is count[f.typ] for the enclosing struct.
func appendFoundField(fields []field, sf reflect.StructField, ft reflect.Type, name string, opts tagOptions, index []int, typeCount int) []field {
	tagged := name != ""
	if name == "" {
		name = sf.Name
	}
	fields = append(fields, fillField(field{
		name:      name,
		tag:       tagged,
		index:     index,
		typ:       ft,
		omitEmpty: opts.Contains("omitempty"),
		quoted:    opts.Contains("string"),
	}))
	if typeCount > 1 {
		// If there were multiple instances, add a second,
		// so that the annihilation code will see a duplicate.
		// It only cares about the distinction between 1 or 2,
		// so don't bother generating any more copies.
		fields = append(fields, fields[len(fields)-1])
	}
	return fields
}

// deleteHiddenFields deletes all fields that are hidden by the Go rules for
// embedded fields, except that fields with JSON tags are promoted. The input
// must already be sorted in primary order of name, secondary order of field
// index length.
func deleteHiddenFields(fields []field) []field {
	// The fields are sorted in primary order of name, secondary order
	// of field index length. Loop over names; for each name, delete
	// hidden fields by choosing the one dominant field that survives.
	out := fields[:0]
	for advance, i := 0, 0; i < len(fields); i += advance {
		// One iteration per name.
		// Find the sequence of fields with the name of this first field.
		fi := fields[i]
		name := fi.name
		for advance = 1; i+advance < len(fields); advance++ {
			fj := fields[i+advance]
			if fj.name != name {
				break
			}
		}
		if advance == 1 { // Only one field with this name
			out = append(out, fi)
			continue
		}
		dominant, ok := dominantField(fields[i : i+advance])
		if ok {
			out = append(out, dominant)
		}
	}
	return out
}

// dominantField looks through the fields, all of which are known to
// have the same name, to find the single field that dominates the
// others using Go's embedding rules, modified by the presence of
// JSON tags. If there are multiple top-level fields, the boolean
// will be false: This condition is an error in Go, and we skip all
// the fields.
func dominantField(fields []field) (field, bool) {
	// The fields are sorted in increasing index-length order. The winner
	// must therefore be one with the shortest index length. Drop all
	// longer entries, which is easy: just truncate the slice.
	length := len(fields[0].index)
	tagged := -1 // Index of first tagged field.
	for i, f := range fields {
		if len(f.index) > length {
			fields = fields[:i]
			break
		}
		if f.tag {
			if tagged >= 0 {
				// Multiple tagged fields at the same level: conflict.
				// Return no field.
				return field{}, false
			}
			tagged = i
		}
	}
	if tagged >= 0 {
		return fields[tagged], true
	}
	// All remaining fields have the same length. If there's more than one,
	// we have a conflict (two fields named "X" at the same level) and we
	// return no field.
	if len(fields) > 1 {
		return field{}, false
	}
	return fields[0], true
}

var fieldCache struct {
	sync.RWMutex
	m map[reflect.Type][]field
}

// cachedTypeFields is like typeFields but uses a cache to avoid repeated work.
func cachedTypeFields(t reflect.Type) []field {
	fieldCache.RLock()
	f := fieldCache.m[t]
	fieldCache.RUnlock()
	if f != nil {
		return f
	}

	// Compute fields without lock.
	// Might duplicate effort but won't hold other computations back.
	f = typeFields(t)
	if f == nil {
		f = []field{}
	}

	fieldCache.Lock()
	if fieldCache.m == nil {
		fieldCache.m = map[reflect.Type][]field{}
	}
	fieldCache.m[t] = f
	fieldCache.Unlock()
	return f
}

func isValidTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:<=>?@[]^_{|}~ ", c):
			// Backslash and quote chars are reserved, but
			// otherwise any punctuation chars are allowed
			// in a tag name.
		default:
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
				return false
			}
		}
	}
	return true
}

const (
	caseMask     = ^byte(0x20) // Mask to ignore case in ASCII.
	kelvin       = '\u212a'
	smallLongEss = '\u017f'
)

// foldFunc returns one of four different case folding equivalence
// functions, from most general (and slow) to fastest:
//
// 1) bytes.EqualFold, if the key s contains any non-ASCII UTF-8
// 2) equalFoldRight, if s contains special folding ASCII ('k', 'K', 's', 'S')
// 3) asciiEqualFold, no special, but includes non-letters (including _)
// 4) simpleLetterEqualFold, no specials, no non-letters.
//
// The letters S and K are special because they map to 3 runes, not just 2:
//   - S maps to s and to U+017F 'ſ' Latin small letter long s
//   - k maps to K and to U+212A 'K' Kelvin sign
//
// See http://play.golang.org/p/tTxjOc0OGo
//
// The returned function is specialized for matching against s and
// should only be given s. It's not curried for performance reasons.
func foldFunc(s []byte) func(s, t []byte) bool {
	nonLetter := false
	special := false // special letter
	for _, b := range s {
		if b >= utf8.RuneSelf {
			return bytes.EqualFold
		}
		upper := b & caseMask
		if upper < 'A' || upper > 'Z' {
			nonLetter = true
		} else if upper == 'K' || upper == 'S' {
			// See above for why these letters are special.
			special = true
		}
	}
	if special {
		return equalFoldRight
	}
	if nonLetter {
		return asciiEqualFold
	}
	return simpleLetterEqualFold
}

// equalFoldRight is a specialization of bytes.EqualFold when s is
// known to be all ASCII (including punctuation), but contains an 's',
// 'S', 'k', or 'K', requiring a Unicode fold on the bytes in t.
// See comments on foldFunc.
func equalFoldRight(s, t []byte) bool {
	for _, sb := range s {
		if len(t) == 0 {
			return false
		}
		tb := t[0]
		if tb < utf8.RuneSelf {
			if !equalFoldRightASCII(sb, tb) {
				return false
			}
			t = t[1:]
			continue
		}
		// sb is ASCII and t is not. t must be either kelvin
		// sign or long s; sb must be s, S, k, or K.
		size, ok := equalFoldRightNonASCII(sb, t)
		if !ok {
			return false
		}
		t = t[size:]

	}

	return len(t) <= 0
}

// equalFoldRightASCII reports whether ASCII bytes sb and tb fold-match.
func equalFoldRightASCII(sb, tb byte) bool {
	if sb != tb {
		sbUpper := sb & caseMask
		if 'A' <= sbUpper && sbUpper <= 'Z' {
			if sbUpper != tb&caseMask {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

// equalFoldRightNonASCII matches an ASCII sb against the leading non-ASCII
// rune of t (kelvin sign or long s), returning the rune size on success.
func equalFoldRightNonASCII(sb byte, t []byte) (int, bool) {
	tr, size := utf8.DecodeRune(t)
	switch sb {
	case 's', 'S':
		if tr != smallLongEss {
			return 0, false
		}
	case 'k', 'K':
		if tr != kelvin {
			return 0, false
		}
	default:
		return 0, false
	}
	return size, true
}

// asciiEqualFold is a specialization of bytes.EqualFold for use when
// s is all ASCII (but may contain non-letters) and contains no
// special-folding letters.
// See comments on foldFunc.
func asciiEqualFold(s, t []byte) bool {
	if len(s) != len(t) {
		return false
	}
	for i, sb := range s {
		tb := t[i]
		if sb == tb {
			continue
		}
		if ('a' <= sb && sb <= 'z') || ('A' <= sb && sb <= 'Z') {
			if sb&caseMask != tb&caseMask {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

// simpleLetterEqualFold is a specialization of bytes.EqualFold for
// use when s is all ASCII letters (no underscores, etc.) and also
// doesn't contain 'k', 'K', 's', or 'S'.
// See comments on foldFunc.
func simpleLetterEqualFold(s, t []byte) bool {
	if len(s) != len(t) {
		return false
	}
	for i, b := range s {
		if b&caseMask != t[i]&caseMask {
			return false
		}
	}
	return true
}

// tagOptions is the string following a comma in a struct field's "json"
// tag, or the empty string. It does not include the leading comma.
type tagOptions string

// parseTag splits a struct field's json tag into its name and
// comma-separated options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tagOptions(tag[idx+1:])
	}
	return tag, ""
}

// Contains reports whether a comma-separated list of options
// contains a particular substr flag. substr must be surrounded by a
// string boundary or commas.
func (o tagOptions) Contains(optionName string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var next string
		i := strings.Index(s, ",")
		if i >= 0 {
			s, next = s[:i], s[i+1:]
		}
		if s == optionName {
			return true
		}
		s = next
	}
	return false
}
