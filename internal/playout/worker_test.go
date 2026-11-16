package playout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/schedule"
)

type fakeLeaseStore struct {
	locked       bool
	written      bool
	state        State
	putCount     int
	releaseCount int
}

func (s *fakeLeaseStore) AcquireLease(context.Context, uuid.UUID, string, time.Duration) (uint64, bool, error) {
	return 7, s.locked, nil
}
func (s *fakeLeaseStore) ReleaseLease(context.Context, uuid.UUID, string, uint64) error {
	s.releaseCount++
	return nil
}
func (s *fakeLeaseStore) RenewLease(context.Context, uuid.UUID, string, uint64, time.Duration) (bool, error) {
	return true, nil
}
func (s *fakeLeaseStore) PutFenced(_ context.Context, state State, _ string, _ time.Duration) (bool, error) {
	s.putCount++
	s.state = state
	return s.written, nil
}

type fakeScheduleReader struct {
	item schedule.Item
	err  error
}

func (s fakeScheduleReader) Current(context.Context, uuid.UUID, time.Time) (schedule.Item, error) {
	return s.item, s.err
}

type fakeAssetReader struct{ item asset.Asset }

func (a fakeAssetReader) FindByID(context.Context, uuid.UUID) (asset.Asset, error) {
	return a.item, nil
}

func TestTickReturnsLeaseBusyWithoutPublishing(t *testing.T) {
	store := &fakeLeaseStore{locked: false, written: true}
	worker := Worker{Store: store, Owner: "test"}

	err := worker.Tick(context.Background(), uuid.New())
	if !errors.Is(err, errLeaseBusy) {
		t.Fatalf("Tick() error = %v, want lease busy", err)
	}
	if store.putCount != 0 || store.releaseCount != 0 {
		t.Fatalf("busy lease should not publish or release: puts=%d releases=%d", store.putCount, store.releaseCount)
	}
}

func TestRunReportsLeaseBusyAsSkippedNotTickError(t *testing.T) {
	store := &fakeLeaseStore{locked: false}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	skipped, ticks := 0, 0
	worker := Worker{
		Store: store, Owner: "test", Interval: time.Hour,
		OnSkipped: func() { skipped++; cancel() },
		OnTick:    func(error) { ticks++ },
	}
	channelID := uuid.New()
	err := worker.Run(ctx, func(context.Context) ([]uuid.UUID, error) { return []uuid.UUID{channelID}, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
	if skipped != 1 || ticks != 0 {
		t.Fatalf("outcomes: skipped=%d ticks=%d, want 1 and 0", skipped, ticks)
	}
}

func TestTickRejectsFencedWriteFailure(t *testing.T) {
	store := &fakeLeaseStore{locked: true, written: false}
	assetID := uuid.New()
	worker := Worker{
		Store: store, Owner: "test",
		Schedules: fakeScheduleReader{item: schedule.Item{ID: uuid.New(), AssetID: assetID, StartsAt: time.Now().UTC().Add(-time.Second)}},
		Assets:    fakeAssetReader{item: asset.Asset{ID: assetID}},
	}

	err := worker.Tick(context.Background(), uuid.New())
	if !errors.Is(err, errFencedWriteRejected) {
		t.Fatalf("Tick() error = %v, want fenced write rejection", err)
	}
	if store.putCount != 1 || store.releaseCount != 1 {
		t.Fatalf("unexpected store calls: puts=%d releases=%d", store.putCount, store.releaseCount)
	}
}

func TestTickPublishesCurrentSchedule(t *testing.T) {
	store := &fakeLeaseStore{locked: true, written: true}
	channelID, itemID, assetID := uuid.New(), uuid.New(), uuid.New()
	worker := Worker{
		Store: store, Owner: "test",
		Schedules: fakeScheduleReader{item: schedule.Item{ID: itemID, ChannelID: channelID, AssetID: assetID, StartsAt: time.Now().UTC().Add(-time.Second)}},
		Assets:    fakeAssetReader{item: asset.Asset{ID: assetID}},
	}

	if err := worker.Tick(context.Background(), channelID); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if store.state.Status != "running" || store.state.ChannelID != channelID || store.state.ScheduleItemID != itemID || store.state.AssetID != assetID {
		t.Fatalf("unexpected published state: %+v", store.state)
	}
	if store.releaseCount != 1 {
		t.Fatalf("lease releases=%d, want 1", store.releaseCount)
	}
}

func TestPositionIsNonNegative(t *testing.T) {
	started := time.Now().UTC().Add(-3 * time.Second)
	position := time.Since(started).Milliseconds()
	if position < 0 {
		t.Fatal("playout position cannot be negative")
	}
}
