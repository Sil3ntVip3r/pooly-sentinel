package journal

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type cursorState struct {
	Cursor string `json:"cursor"`
}

func loadCursor(ctx context.Context, store resources.StateStore, stream string) (string, bool, error) {
	if store == nil {
		return "", false, nil
	}
	raw, err := store.Get(ctx, "journal", stream)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	var state cursorState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return "", true, err
	}
	return state.Cursor, state.Cursor != "", nil
}

func saveCursor(ctx context.Context, store resources.StateStore, persist bool, stream string, cursor string) error {
	if !persist || store == nil || cursor == "" {
		return nil
	}
	data, err := json.Marshal(cursorState{Cursor: cursor})
	if err != nil {
		return err
	}
	return store.Upsert(ctx, "journal", stream, "ok", string(data))
}
