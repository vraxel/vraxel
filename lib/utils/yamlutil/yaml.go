package yamlutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"gopkg.in/yaml.v3"
)

// JSONToYAML converts JSON to YAML. Notable implementation details:
//
//   - Duplicate fields, are case-sensitively ignored in an undefined order
//   - The sequence indentation style is compact, which means that the "- " marker for a YAML sequence will be on the same indentation level as the sequence field name
//   - Unlike Unmarshal, all integers, up to 64 bits, are preserved during this round-trip
func JSONToYAML(j []byte) ([]byte, error) {
	// Convert the JSON to an object
	var jsonObj interface{}

	// We are using yaml.Unmarshal here (instead of json.Unmarshal) because the
	// Go JSON library doesn't try to pick the right number type (int, float,
	// etc.) when unmarshalling to interface{}, it just picks float64
	// universally. go-yaml does go through the effort of picking the right
	// number type, so we can preserve number type throughout this process
	err := yaml.Unmarshal(j, &jsonObj)
	if err != nil {
		return nil, err
	}

	// Marshal this object into YAML.
	yamlBytes, err := yaml.Marshal(jsonObj)
	if err != nil {
		return nil, err
	}

	return yamlBytes, nil
}

// YAMLToJSON converts YAML to JSON. Since JSON is a subset of YAML,
// passing JSON through this method should be a no-op.
//
// Some things YAML can do that are not supported by JSON:
//   - In YAML you can have binary and null keys in your maps. These are invalid
//     in JSON, and therefore int, bool and float keys are converted to strings implicitly.
//   - Binary data in YAML with the !!binary tag is not supported. If you want to
//     use binary data with this library, encode the data as base64 as usual but do
//     not use the !!binary tag in your YAML. This will ensure the original base64
//     encoded data makes it all the way through to the JSON.
//   - And more... read the YAML specification for more details.
//
// Notable about the implementation:
//
// - Duplicate fields are case-sensitively ignored in an undefined order. Note that the YAML specification forbids duplicate fields, so this logic is more permissive than it needs to. See YAMLToJSONStrict for an alternative.
// - As per the YAML 1.1 specification, which yaml.v2 used underneath implements, literal 'yes' and 'no' strings without quotation marks will be converted to true/false implicitly.
// - Unlike Unmarshal, all integers, up to 64 bits, are preserved during this round-trip.
// - There are no compatibility guarantees for returned error values.
func YAMLToJSON(y []byte) ([]byte, error) {
	return yamlToJSONTarget(y, nil, yaml.Unmarshal)
}

func yamlToJSONTarget(yamlBytes []byte, jsonTarget *reflect.Value, unmarshalFn func([]byte, interface{}) error) ([]byte, error) {
	// Convert the YAML to an object.
	var yamlObj interface{}
	err := unmarshalFn(yamlBytes, &yamlObj)
	if err != nil {
		return nil, err
	}

	// YAML objects are not completely compatible with JSON objects (e.g. you
	// can have non-string keys in YAML). So, convert the YAML-compatible object
	// to a JSON-compatible object, failing with an error if irrecoverable
	// incompatibilties happen along the way.
	jsonObj, err := convertToJSONableObject(yamlObj, jsonTarget)
	if err != nil {
		return nil, err
	}

	// Convert this object to JSON and return the data.
	jsonBytes, err := json.Marshal(jsonObj)
	if err != nil {
		return nil, err
	}
	return jsonBytes, nil
}

func convertToJSONableObject(yamlObj interface{}, jsonTarget *reflect.Value) (interface{}, error) {
	// Resolve jsonTarget to a concrete value (i.e. not a pointer or an
	// interface). We pass decodingNull as false because we're not actually
	// decoding into the value, we're just checking if the ultimate target is a
	// string.
	if jsonTarget != nil {
		jsonUnmarshaler, textUnmarshaler, pointerValue := indirect(*jsonTarget, false)
		// We have a JSON or Text Umarshaler at this level, so we can't be trying
		// to decode into a string.
		if jsonUnmarshaler != nil || textUnmarshaler != nil {
			jsonTarget = nil
		} else {
			jsonTarget = &pointerValue
		}
	}

	// If yamlObj is a number or a boolean, check if jsonTarget is a string -
	// if so, coerce.  Else return normal.
	// If yamlObj is a map or array, find the field that each key is
	// unmarshalling to, and when you recurse pass the Value for that
	// field back into this function.
	switch typedYAMLObj := yamlObj.(type) {
	case map[interface{}]interface{}:
		return convertToJSONableMap(typedYAMLObj, jsonTarget)
	case []interface{}:
		return convertToJSONableSlice(typedYAMLObj, jsonTarget)
	default:
		return convertToJSONableScalar(typedYAMLObj, yamlObj, jsonTarget)
	}
}

// convertToJSONableMap handles the map[interface{}]interface{} case of
// convertToJSONableObject: it converts each key to a string and recursively
// converts each value, threading the per-key JSON target.
func convertToJSONableMap(typedYAMLObj map[interface{}]interface{}, jsonTarget *reflect.Value) (interface{}, error) {
	// JSON does not support arbitrary keys in a map, so we must convert
	// these keys to strings.
	//
	// From my reading of go-yaml v2 (specifically the resolve function),
	// keys can only have the types string, int, int64, float64, binary
	// (unsupported), or null (unsupported).
	strMap := make(map[string]interface{})
	for k, v := range typedYAMLObj {
		// Resolve the key to a string first.
		keyString, err := convertToJSONableMapKey(k, v)
		if err != nil {
			return nil, err
		}

		strMap[keyString], err = convertToJSONableMapValue(keyString, v, jsonTarget)
		if err != nil {
			return nil, err
		}
	}
	return strMap, nil
}

// convertToJSONableMapKey resolves a YAML map key k (with value v, used only
// for the error message) to its string representation.
func convertToJSONableMapKey(k, v interface{}) (string, error) {
	switch typedKey := k.(type) {
	case string:
		return typedKey, nil
	case int:
		return strconv.Itoa(typedKey), nil
	case int64:
		// go-yaml will only return an int64 as a key if the system
		// architecture is 32-bit and the key's value is between 32-bit
		// and 64-bit. Otherwise, the key type will simply be int.
		return strconv.FormatInt(typedKey, 10), nil
	case float64:
		// Stolen from go-yaml to use the same conversion to string as
		// the go-yaml library uses to convert float to string when
		// Marshaling.
		s := strconv.FormatFloat(typedKey, 'g', -1, 32)
		switch s {
		case "+Inf":
			s = ".inf"
		case "-Inf":
			s = "-.inf"
		case "NaN":
			s = ".nan"
		}
		return s, nil
	case bool:
		if typedKey {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("unsupported map key of type: %s, key: %+#v, value: %+#v",
			reflect.TypeOf(k), k, v)
	}
}

// convertToJSONableMapValue recursively converts a single map value v, selecting
// the JSON target from jsonTarget (a struct field, a map element, or nil) based
// on keyString.
func convertToJSONableMapValue(keyString string, v interface{}, jsonTarget *reflect.Value) (interface{}, error) {
	// jsonTarget should be a struct or a map. If it's a struct, find
	// the field it's going to map to and pass its Value. If
	// it's a map, find the element type of the map and pass the
	// Value created from that type. If it's neither, just pass
	// nil - JSON conversion will error for us if it's a real issue.
	if jsonTarget != nil {
		t := *jsonTarget
		if t.Kind() == reflect.Struct {
			// Find the field that the JSON library would use.
			if f := findJSONableStructField(t, keyString); f != nil {
				return convertToJSONableObject(v, new(t.Field(f.index[0])))
			}
		} else if t.Kind() == reflect.Map {
			// Create a zero value of the map's element type to use as
			// the JSON target.
			return convertToJSONableObject(v, new(reflect.Zero(t.Type().Elem())))
		}
	}
	return convertToJSONableObject(v, nil)
}

// findJSONableStructField returns the struct field of type t that the JSON
// library would map keyString to (exact match preferred, otherwise the first
// case-insensitive match), or nil if none matches.
func findJSONableStructField(t reflect.Value, keyString string) *field {
	keyBytes := []byte(keyString)
	var f *field
	fields := cachedTypeFields(t.Type())
	for i := range fields {
		ff := &fields[i]
		if bytes.Equal(ff.nameBytes, keyBytes) {
			f = ff
			break
		}
		// Do case-insensitive comparison.
		if f == nil && ff.equalFold(ff.nameBytes, keyBytes) {
			f = ff
		}
	}
	return f
}

// convertToJSONableSlice handles the []interface{} case of
// convertToJSONableObject: it recurses into each element, threading the slice
// element JSON target.
func convertToJSONableSlice(typedYAMLObj []interface{}, jsonTarget *reflect.Value) (interface{}, error) {
	// We need to recurse into arrays in case there are any
	// map[interface{}]interface{}'s inside and to convert any
	// numbers to strings.

	// If jsonTarget is a slice (which it really should be), find the
	// thing it's going to map to. If it's not a slice, just pass nil
	// - JSON conversion will error for us if it's a real issue.
	var jsonSliceElemValue *reflect.Value
	if jsonTarget != nil {
		t := *jsonTarget
		if t.Kind() == reflect.Slice {
			jsonSliceElemValue = new(reflect.Indirect(reflect.New(t.Type().Elem())))
		}
	}

	// Make and use a new array.
	arr := make([]interface{}, len(typedYAMLObj))
	for i, v := range typedYAMLObj {
		var err error
		arr[i], err = convertToJSONableObject(v, jsonSliceElemValue)
		if err != nil {
			return nil, err
		}
	}
	return arr, nil
}

// convertToJSONableScalar handles the default (scalar) case of
// convertToJSONableObject: if the JSON target is a string and the YAML value is
// numeric/bool, it coerces the value to its string form.
func convertToJSONableScalar(typedYAMLObj interface{}, yamlObj interface{}, jsonTarget *reflect.Value) (interface{}, error) {
	// If the target type is a string and the YAML type is a number,
	// convert the YAML type to a string.
	if jsonTarget != nil && (*jsonTarget).Kind() == reflect.String {
		// Based on my reading of go-yaml, it may return int, int64,
		// float64, or uint64.
		var s string
		switch typedVal := typedYAMLObj.(type) {
		case int:
			s = strconv.FormatInt(int64(typedVal), 10)
		case int64:
			s = strconv.FormatInt(typedVal, 10)
		case float64:
			s = strconv.FormatFloat(typedVal, 'g', -1, 32)
		case uint64:
			s = strconv.FormatUint(typedVal, 10)
		case bool:
			if typedVal {
				s = "true"
			} else {
				s = "false"
			}
		}
		if len(s) > 0 {
			yamlObj = interface{}(s)
		}
	}
	return yamlObj, nil
}
