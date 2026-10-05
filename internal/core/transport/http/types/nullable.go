package http_types

import (
	"encoding/json"
	core_types "github.com/wrzdx/Nero/internal/core/types"
)

type Nullable[T any] struct {
	core_types.Nullable[T]
}

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true

	if string(b) == "null" {
		n.Value = nil
		return nil
	}

	var value T
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	n.Value = &value
	return nil
}

func (n Nullable[T]) ToCore() core_types.Nullable[T] {
	return core_types.Nullable[T]{
		Value: n.Value,
		Set:   n.Set,
	}
}
