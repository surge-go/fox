package openapi

import (
	"encoding/json"
	"os"
)

func Marshal(doc *Document) ([]byte, error) {
	return json.Marshal(doc)
}

func MarshalIndent(doc *Document, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(doc, prefix, indent)
}

func Unmarshal(data []byte) (*Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func ReadFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Unmarshal(data)
}
