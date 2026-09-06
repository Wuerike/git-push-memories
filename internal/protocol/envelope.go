package protocol

const SchemaVersion = 1

type Envelope[T any] struct {
	SchemaVersion int `json:"schema_version"`
	Data          T   `json:"data"`
}

func NewEnvelope[T any](data T) Envelope[T] {
	return Envelope[T]{SchemaVersion: SchemaVersion, Data: data}
}
