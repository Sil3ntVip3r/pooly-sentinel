package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

const SchedulerStatusMetadataKey = "agent.scheduler.status"

type MetadataStatusStore struct {
	Store interface {
		UpsertMetadata(ctx context.Context, record storage.MetadataRecord) error
		GetMetadata(ctx context.Context, key string) (storage.MetadataRecord, error)
	}
}

func (s MetadataStatusStore) SaveSchedulerStatus(ctx context.Context, status SchedulerStatus) error {
	if s.Store == nil {
		return nil
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return s.Store.UpsertMetadata(ctx, storage.MetadataRecord{
		Key:       SchedulerStatusMetadataKey,
		Value:     redaction.Redact(string(data)),
		UpdatedAt: time.Now().UTC(),
	})
}

func LoadPersistedSchedulerStatus(ctx context.Context, store interface {
	GetMetadata(ctx context.Context, key string) (storage.MetadataRecord, error)
}) (SchedulerStatus, bool, error) {
	if store == nil {
		return SchedulerStatus{}, false, nil
	}
	record, err := store.GetMetadata(ctx, SchedulerStatusMetadataKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return SchedulerStatus{}, false, nil
		}
		return SchedulerStatus{}, false, err
	}
	var status SchedulerStatus
	if err := json.Unmarshal([]byte(record.Value), &status); err != nil {
		return SchedulerStatus{}, false, err
	}
	return status, true, nil
}
