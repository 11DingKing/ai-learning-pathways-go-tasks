package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

type ID string

func NewID(prefix string) (ID, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", FieldError{Field: "prefix", Message: "is required"}
	}
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return ID(prefix + "_" + hex.EncodeToString(buffer)), nil
}

func ParseID(field, value string) (ID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", FieldError{Field: field, Message: "is required"}
	}
	if len(value) > 100 {
		return "", FieldError{Field: field, Message: "is too long"}
	}
	for _, part := range strings.Split(value, "_") {
		if part == "" {
			return "", FieldError{Field: field, Message: "has invalid format"}
		}
	}
	return ID(value), nil
}

func (id ID) String() string { return string(id) }

func (id ID) Valid() bool {
	return strings.TrimSpace(string(id)) != ""
}
