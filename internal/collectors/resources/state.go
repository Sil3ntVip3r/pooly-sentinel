package resources

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type StorageStateStore struct {
	Store *storage.Store
}

func (s StorageStateStore) Get(ctx context.Context, collector string, target string) (string, error) {
	if s.Store == nil {
		return "", storage.ErrNotFound
	}
	record, err := s.Store.GetCollectorState(ctx, collector, target)
	if err != nil {
		return "", err
	}
	return record.StateJSON, nil
}

func (s StorageStateStore) Upsert(ctx context.Context, collector string, target string, status string, stateJSON string) error {
	if s.Store == nil {
		return nil
	}
	return s.Store.UpsertCollectorState(ctx, storage.CollectorStateRecord{
		Collector:     collector,
		Target:        target,
		Status:        status,
		StateJSON:     stateJSON,
		LastAttemptAt: ptrTime(time.Now().UTC()),
		LastSuccessAt: ptrTime(time.Now().UTC()),
	})
}

type MemoryStateStore struct {
	mu   sync.Mutex
	data map[string]string
	fail bool
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{data: map[string]string{}}
}

func (s *MemoryStateStore) SetFail(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = fail
}

func (s *MemoryStateStore) Get(ctx context.Context, collector string, target string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return "", errors.New("state failure")
	}
	value, ok := s.data[stateKey(collector, target)]
	if !ok {
		return "", storage.ErrNotFound
	}
	return value, nil
}

func (s *MemoryStateStore) Upsert(ctx context.Context, collector string, target string, status string, stateJSON string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("state failure")
	}
	s.data[stateKey(collector, target)] = stateJSON
	return nil
}

func stateKey(collector string, target string) string {
	return collector + "\x00" + target
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func loadState[T any](ctx context.Context, store StateStore, collector string, target string) (T, bool, error) {
	var zero T
	if store == nil {
		return zero, false, nil
	}
	raw, err := store.Get(ctx, collector, target)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return zero, false, nil
		}
		return zero, false, err
	}
	var state T
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return zero, false, err
	}
	return state, true, nil
}

func saveState(ctx context.Context, store StateStore, persist bool, collector string, target string, state any) error {
	if !persist || store == nil {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return store.Upsert(ctx, collector, target, "ok", string(data))
}

type CounterDelta struct {
	Delta uint64
	Reset bool
	Valid bool
}

func CalculateCounterDelta(previous uint64, current uint64, hasPrevious bool) CounterDelta {
	if !hasPrevious {
		return CounterDelta{Valid: false}
	}
	if current < previous {
		return CounterDelta{Reset: true, Valid: false}
	}
	return CounterDelta{Delta: current - previous, Valid: true}
}

type DailyCounter struct {
	Day   string `json:"day"`
	Total uint64 `json:"total"`
}

func UpdateDailyCounter(previous DailyCounter, now time.Time, delta CounterDelta) DailyCounter {
	day := now.UTC().Format("2006-01-02")
	if previous.Day != day {
		previous = DailyCounter{Day: day}
	}
	if delta.Valid && !delta.Reset {
		previous.Total += delta.Delta
	}
	return previous
}
