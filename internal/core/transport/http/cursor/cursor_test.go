package http_cursor

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"

	"github.com/stretchr/testify/require"
)

func TestEncode(t *testing.T) {
	t.Run("encodes nil payload as nil", func(t *testing.T) {
		encoded, err := Encode[testCursorPayload](nil)

		require.NoError(t, err)
		require.Nil(t, encoded)
	})

	t.Run("encodes payload that can be decoded", func(t *testing.T) {
		payload := validTestCursorPayload()

		encoded, err := Encode(&payload)

		require.NoError(t, err)
		require.NotNil(t, encoded)
		decodedJSON, err := base64.URLEncoding.DecodeString(*encoded)
		require.NoError(t, err)
		require.JSONEq(t, `{
			"id": "00000000-0000-0000-0000-000000000001",
			"created_at": "2026-07-23T12:30:00Z"
		}`, string(decodedJSON))
	})

	t.Run("returns marshal error as internal failure", func(t *testing.T) {
		payload := testCursorPayload{
			ID:        "00000000-0000-0000-0000-000000000001",
			CreatedAt: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
		}

		encoded, err := Encode(&payload)

		require.Error(t, err)
		require.False(t, errors.Is(err, http_request.ErrInvalidRequest))
		require.Nil(t, encoded)
	})
}

func TestDecodeAndValidate(t *testing.T) {
	t.Run("round trips payload", func(t *testing.T) {
		expected := validTestCursorPayload()
		encoded, err := Encode(&expected)
		require.NoError(t, err)
		require.NotNil(t, encoded)

		actual, err := DecodeAndValidate[testCursorPayload](*encoded)

		require.NoError(t, err)
		require.Equal(t, expected, *actual)
	})

	t.Run("decodes empty cursor as nil", func(t *testing.T) {
		payload, err := DecodeAndValidate[testCursorPayload]("")

		require.NoError(t, err)
		require.Nil(t, payload)
	})

	t.Run("rejects malformed base64 and retains cause", func(t *testing.T) {
		payload, err := DecodeAndValidate[testCursorPayload]("%%%")

		require.ErrorIs(t, err, http_request.ErrInvalidRequest)
		require.Contains(t, err.Error(), "illegal base64 data")
		require.Nil(t, payload)
	})

	t.Run("rejects malformed json and retains cause", func(t *testing.T) {
		raw := base64.URLEncoding.EncodeToString([]byte(`{"id":`))

		payload, err := DecodeAndValidate[testCursorPayload](raw)

		require.ErrorIs(t, err, http_request.ErrInvalidRequest)
		require.Contains(t, err.Error(), "unexpected end")
		require.Nil(t, payload)
	})

	testCases := []struct {
		name    string
		payload string
	}{
		{
			name: "missing id",
			payload: `{
				"created_at": "2026-07-23T12:30:00Z"
			}`,
		},
		{
			name: "invalid id",
			payload: `{
				"id": "not-a-uuid",
				"created_at": "2026-07-23T12:30:00Z"
			}`,
		},
		{
			name: "missing non pointer time",
			payload: `{
				"id": "00000000-0000-0000-0000-000000000001"
			}`,
		},
	}
	for _, testCase := range testCases {
		t.Run("rejects "+testCase.name+" as opaque cursor error", func(t *testing.T) {
			raw := base64.URLEncoding.EncodeToString([]byte(testCase.payload))

			payload, err := DecodeAndValidate[testCursorPayload](raw)

			require.ErrorIs(t, err, http_request.ErrInvalidRequest)
			require.Equal(t, map[string]string{
				"cursor": "invalid cursor",
			}, fieldsFromError(t, err))
			require.Nil(t, payload)
		})
	}
}

type testCursorPayload struct {
	ID        string    `json:"id" validate:"required,uuid"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
}

func validTestCursorPayload() testCursorPayload {
	return testCursorPayload{
		ID:        "00000000-0000-0000-0000-000000000001",
		CreatedAt: time.Date(2026, time.July, 23, 12, 30, 0, 0, time.UTC),
	}
}

func fieldsFromError(t *testing.T, err error) map[string]string {
	t.Helper()

	var withFields interface {
		Fields() map[string]string
	}
	require.ErrorAs(t, err, &withFields)
	return withFields.Fields()
}
