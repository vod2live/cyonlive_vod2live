package playout

import (
	"context"
	"errors"
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
	OnTick    func(error)
}

func (w Worker) Tick(ctx context.Context, channelID uuid.UUID) error {
	leaseTTL := 15 * time.Second
	token, locked, err := w.Store.AcquireLease(ctx, channelID, w.Owner, leaseTTL)
	if err != nil || !locked {
		return err
	}
	defer func() { _ = w.Store.ReleaseLease(ctx, channelID, w.Owner, token) }()
	now := time.Now().UTC()
	current, err := w.Schedules.Current(ctx, channelID, now)
	if errors.Is(err, schedule.ErrNotFound) {
		if renewed, renewErr := w.Store.RenewLease(ctx, channelID, w.Owner, token, leaseTTL); renewErr != nil {
			return renewErr
		} else if !renewed {
			return fmt.Errorf("playout lease lost")
		}
		_, putErr := w.Store.PutFenced(ctx, State{ChannelID: channelID, Status: "idle", FencingToken: token, UpdatedAt: now}, w.Owner, 30*time.Second)
		return putErr
	}
	if err != nil {
		return err
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
			err := w.Tick(ctx, id)
			if err != nil {
				if w.OnTick != nil {
					w.OnTick(err)
				}
				continue
			}
			if w.OnTick != nil {
				w.OnTick(nil)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
