package runtime

import "strings"

// SerializerInfo describes a single registered wire format
type SerializerInfo struct {
	// MediaType is the MIME type, e.g. "application/json", "application/yaml"
	MediaType string

	// MediaTypeType is the primary type, e.g. "application"
	MediaTypeType string

	// MediaTypeSubType is the subtype, e.g. "json", "yaml"
	MediaTypeSubType string

	// EncodesAsText indicates whether the format is human-readable text
	EncodesAsText bool

	// Serializer is the serializer instance for this format
	Serializer Serializer

	// PrettySerializer is an optional serializer that produces human-readable output
	PrettySerializer Serializer
}

// SerializerInfoForMediaType finds the SerializerInfo that matches the given
// media type string. Returns the matched info and true, or a zero value and false
func SerializerInfoForMediaType(types []SerializerInfo, mediaType string) (SerializerInfo, bool) {
	for _, info := range types {
		if strings.EqualFold(info.MediaType, mediaType) {
			return info, true
		}
	}
	return SerializerInfo{}, false
}
