package playout

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/schedule"
)

type Worker struct {
	Store     RedisStore
	Schedules schedule.PostgresRepository
	Assets    asset.PostgresRepository
	Owner     string
	Interval  time.Duration
}

func (w Worker) Tick(ctx context.Context, channelID uuid.UUID) error {
	locked, err := w.Store.AcquireLock(ctx, channelID, w.Owner, 15*time.Second)
	if err != nil || !locked {
		return err
	}
	defer w.Store.ReleaseLock(ctx, channelID, w.Owner)
	items, err := w.Schedules.List(ctx, channelID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var current *schedule.Item
	for i := range items {
		item := items[i]
		if !item.Enabled || item.StartsAt.After(now) {
			continue
		}
		assetItem, assetErr := w.Assets.FindByID(ctx, item.AssetID)
		if assetErr != nil {
			continue
		}
		end := item.EndsAt
		if end == nil && assetItem.DurationMS > 0 {
			derived := item.StartsAt.Add(time.Duration(assetItem.DurationMS) * time.Millisecond)
			end = &derived
		}
		if end == nil || now.Before(*end) {
			current = &item
			break
		}
	}
	if current == nil {
		return w.Store.Put(ctx, State{ChannelID: channelID, Status: "idle", UpdatedAt: now}, 30*time.Second)
	}
	assetItem, err := w.Assets.FindByID(ctx, current.AssetID)
	if err != nil {
		return err
	}
	position := now.Sub(current.StartsAt).Milliseconds()
	if position < 0 {
		position = 0
	}
	state := State{ChannelID: channelID, ScheduleItemID: current.ID, AssetID: assetItem.ID, Status: "running", PositionMS: position, UpdatedAt: now}
	return w.Store.Put(ctx, state, 30*time.Second)
}

func (w Worker) Run(ctx context.Context, channels func(context.Context) ([]uuid.UUID, error)) error {
	interval := w.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ids, err := channels(ctx)
		if err != nil {
			return fmt.Errorf("list running channels: %w", err)
		}
		for _, id := range ids {
			_ = w.Tick(ctx, id)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
