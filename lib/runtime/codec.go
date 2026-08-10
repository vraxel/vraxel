package runtime

import (
	"encoding/json"
	"fmt"
	"io"

	"vraxel.io/vraxel/lib/utils/yamlutil"
)

var _ NegotiatedSerializer = &CodecFactory{}

// CodecFactory implements NegotiatedSerializer by managing a set of
// registered serializers
type CodecFactory struct {
	accepts []SerializerInfo
}

// NewCodecFactory creates a CodecFactory with JSON and YAML serializers pre-registered
//
// which creates:
//   - JSON serializer
//   - YAML serializer
func NewCodecFactory() *CodecFactory {
	// Standard JSON serializer (compact)
	jsonSerializer := NewJSONSerializer(SerializerOptions{})
	// Pretty JSON serializer
	jsonPrettySerializer := NewJSONSerializer(SerializerOptions{Pretty: true})
	// YAML serializer (encodes as YAML, decodes YAML->JSON->object)
	yamlSerializer := NewJSONSerializer(SerializerOptions{Yaml: true})

	return &CodecFactory{
		accepts: []SerializerInfo{
			{
				// JSON is the first (default) serializer
				// where JSON is selected when Accept Header is empty or "*/*"
				MediaType:        "application/json",
				MediaTypeType:    "application",
				MediaTypeSubType: "json",
				EncodesAsText:    true,
				Serializer:       jsonSerializer,
				PrettySerializer: jsonPrettySerializer,
			},
			{
				MediaType:        "application/yaml",
				MediaTypeType:    "application",
				MediaTypeSubType: "yaml",
				EncodesAsText:    true,
				Serializer:       yamlSerializer,
				// YAML is inherently readable, so PrettySerializer is the same
				PrettySerializer: yamlSerializer,
			},
		},
	}
}

// SupportedMediaTypes implements NegotiatedSerializer
// Returns all registered serializer info entries, used by the negotiation layer
// to match against the request's Accept header
func (f *CodecFactory) SupportedMediaTypes() []SerializerInfo {
	return f.accepts
}

// SerializerOptions controls the behavior of the Serializer
type SerializerOptions struct {
	// Yaml causes the serializer to encode/decode YAML instead of JSON
	Yaml bool

	// Pretty causes the JSON serializer to emit indented output
	Pretty bool
}

// JSONSerializer implements Serializer for JSON and YAML formats
type JSONSerializer struct {
	options SerializerOptions
	ident   string
}

// NewJSONSerializer creates a new JSON/YAML serializer with the given options
func NewJSONSerializer(opts SerializerOptions) *JSONSerializer {
	id := "json"
	if opts.Yaml {
		id = "yaml"
	} else if opts.Pretty {
		id = "json-pretty"
	}
	return &JSONSerializer{
		options: opts,
		ident:   id,
	}
}

// Encode do encode:
//  1. If Yaml: marshal to JSON first, then convert JSON->YAML
//  2. If Pretty: use json.MarshalIndent
//  3. Otherwise: use json.NewEncoder(w).Encode (streaming, no trailing newline issues)
func (j *JSONSerializer) Encode(obj Object, w io.Writer) error {
	if j.options.Yaml {
		jsonData, err := json.Marshal(obj)
		if err != nil {
			return fmt.Errorf("json marshal for yaml conversion: %w", err)
		}
		yamlData, err := yamlutil.JSONToYAML(jsonData)
		if err != nil {
			return fmt.Errorf("json to yaml conversion: %w", err)
		}
		_, err = w.Write(yamlData)
		return err
	}

	if j.options.Pretty {
		data, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return fmt.Errorf("json marshal indent: %w", err)
		}
		_, err = w.Write(data)
		return err
	}

	// Default: streaming JSON encoder
	return json.NewEncoder(w).Encode(obj)
}

// Decode
//  1. If Yaml: convert YAML->JSON first via yaml.YAMLToJSON
//  2. json.Unmarshal into the target object
func (j *JSONSerializer) Decode(data []byte, into Object) (Object, error) {
	if into == nil {
		return nil, fmt.Errorf("into must not be nil (simplified version requires a target object)")
	}

	effective := data
	if j.options.Yaml {
		jsonData, err := yamlutil.YAMLToJSON(data)
		if err != nil {
			return nil, fmt.Errorf("yaml to json conversion: %w", err)
		}
		effective = jsonData
	}

	if err := json.Unmarshal(effective, into); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return into, nil
}

func (j *JSONSerializer) Identifier() string {
	return j.ident
}
