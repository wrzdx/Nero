package chats_transport_http

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	"io"
	"net/http"
	"strings"
)

// Reject obsolete ID fields instead of silently ignoring invited participants.
func decodeUsernameRequest(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("%w: decode json: %w", http_request.ErrInvalidRequest, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: expected a single JSON object", http_request.ErrInvalidRequest)
	}
	if fields := http_request.Validate(dest); len(fields) > 0 {
		return http_request.NewFieldError(fields)
	}
	return nil
}

func requestUsernames(names []string, field string) ([]string, error) {
	normalized, err := chats_service.NormalizeUsernames(names)
	if err == nil {
		return normalized, nil
	}
	detailed, ok := errors.AsType[domain.DetailedError](err)
	if !ok {
		return nil, err
	}
	details := detailed.Details
	fields := make(map[string]string, len(details))
	for key, message := range details {
		fields[strings.Replace(key, "usernames", field, 1)] = message
	}
	return nil, http_request.NewFieldError(fields)
}

func requestUsername(name, field string) (string, error) {
	names, err := requestUsernames([]string{name}, field)
	if err != nil {
		return "", http_request.NewFieldError(map[string]string{field: "must contain 5–32 ASCII letters, digits or underscores"})
	}
	return names[0], nil
}
