package http_cursor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
)

func Encode[T any](payload *T) (*string, error) {
	if payload == nil {
		return nil, nil
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cursor: %w", err)
	}

	encoded := base64.URLEncoding.EncodeToString(jsonData)

	return &encoded, nil
}

func DecodeAndValidate[T any](raw string) (*T, error) {
	if raw == "" {
		return nil, nil
	}

	jsonData, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to decode base64 cursor: %v: %w",
			err,
			http_request.ErrInvalidRequest,
		)
	}

	var dest T

	if err := json.Unmarshal(jsonData, &dest); err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal cursor json: %v: %w",
			err,
			http_request.ErrInvalidRequest,
		)
	}

	if fields := http_request.Validate(dest); len(fields) != 0 {
		return nil, http_request.NewFieldError(map[string]string{"cursor": "invalid cursor"})
	}

	return &dest, nil
}
