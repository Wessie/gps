package gps

import "encoding/json"

var (
	DefaultValueCodec  Codec = &JSONCodec{}
	DefaultStreamCodec Codec = &JSONCodec{}
)

type Codec interface {
	Encode(value interface{}) ([]byte, error)
	Decode(v []byte, value interface{}) error
}

type JSONCodec struct{}

func (c *JSONCodec) Encode(value interface{}) ([]byte, error) {
	return json.Marshal(value)
}

func (c *JSONCodec) Decode(v []byte, value interface{}) error {
	return json.Unmarshal(v, value)
}
