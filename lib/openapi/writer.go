package openapi

import (
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// WriteJSON writes the OpenAPI document as JSON to the given writer.
func WriteJSON(w io.Writer, doc *Document) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("encode openapi json: %w", err)
	}
	return nil
}

// WriteYAML writes the OpenAPI document as YAML to the given writer.
func WriteYAML(w io.Writer, doc *Document) error {
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("encode openapi yaml: %w", err)
	}
	return nil
}
