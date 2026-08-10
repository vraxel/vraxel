package runtime

import (
	"io"
)

// Object is the base interface that all API resource types must implement
type Object interface {
	// GetTypeMeta returns a pointer to the embedded TypeMeta so the framework
	// can read and set the object's GVK on the wire
	GetTypeMeta() *TypeMeta
}

// Encoder writes an Object in a specific wire format
type Encoder interface {
	// Encode serializes obj into the given writer
	Encode(obj Object, w io.Writer) error

	// Identifier returns a string that uniquely identifies this encoder's
	// output format (used for caching)
	Identifier() string
}

// Decoder reads an Object from raw bytes in a specific wire format
type Decoder interface {
	// Decode deserializes data into an Object
	// If into is non-nil, the decoder should populate it; otherwise it
	// should create a new object of the appropriate type
	Decode(data []byte, into Object) (Object, error)
}

// Serializer combines Encoder and Decoder for a single wire format
type Serializer interface {
	Encoder
	Decoder
}

// NegotiatedSerializer is implemented by a codec factory to provide content-type
// negotiation support. The server uses this to:
//  1. List all supported media types (for Accept header negotiation)
//  2. Retrieve the appropriate encoder/decoder for a given media type
type NegotiatedSerializer interface {
	// SupportedMediaTypes returns the list of all registered serializer formats
	SupportedMediaTypes() []SerializerInfo
}
