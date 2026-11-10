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
	leaseTTL := 15 * time.Second
	token, locked, err := w.Store.AcquireLease(ctx, channelID, w.Owner, leaseTTL)
	if err != nil || !locked {
		return err
	}
	defer w.Store.ReleaseLease(ctx, channelID, w.Owner, token)
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
		if renewed, renewErr := w.Store.RenewLease(ctx, channelID, w.Owner, token, leaseTTL); renewErr != nil {
			return renewErr
		} else if !renewed {
			return fmt.Errorf("playout lease lost")
		}
		_, putErr := w.Store.PutFenced(ctx, State{ChannelID: channelID, Status: "idle", FencingToken: token, UpdatedAt: now}, w.Owner, 30*time.Second)
		return putErr
	}
	assetItem, err := w.Assets.FindByID(ctx, current.AssetID)
	if err != nil {
		return err
	}
	position := now.Sub(current.StartsAt).Milliseconds()
	if position < 0 {
		position = 0
	}
	state := State{ChannelID: channelID, ScheduleItemID: current.ID, AssetID: assetItem.ID, Status: "running", PositionMS: position, FencingToken: token, UpdatedAt: now}
	if renewed, renewErr := w.Store.RenewLease(ctx, channelID, w.Owner, token, leaseTTL); renewErr != nil {
		return renewErr
	} else if !renewed {
		return fmt.Errorf("playout lease lost")
	}
	_, err = w.Store.PutFenced(ctx, state, w.Owner, 30*time.Second)
	return err
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
