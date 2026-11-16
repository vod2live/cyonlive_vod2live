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

var (
	errLeaseBusy           = errors.New("playout lease not acquired")
	errFencedWriteRejected = errors.New("playout fenced state write rejected")
)

type leaseStore interface {
	AcquireLease(context.Context, uuid.UUID, string, time.Duration) (uint64, bool, error)
	ReleaseLease(context.Context, uuid.UUID, string, uint64) error
	RenewLease(context.Context, uuid.UUID, string, uint64, time.Duration) (bool, error)
	PutFenced(context.Context, State, string, time.Duration) (bool, error)
}

type scheduleReader interface {
	Current(context.Context, uuid.UUID, time.Time) (schedule.Item, error)
}

type assetReader interface {
	FindByID(context.Context, uuid.UUID) (asset.Asset, error)
}

type Worker struct {
	Store     leaseStore
	Schedules scheduleReader
	Assets    assetReader
	Owner     string
	Interval  time.Duration
	OnTick    func(error)
	OnSkipped func()
}

func (w Worker) Tick(ctx context.Context, channelID uuid.UUID) error {
	leaseTTL := 15 * time.Second
	token, locked, err := w.Store.AcquireLease(ctx, channelID, w.Owner, leaseTTL)
	if err != nil {
		return err
	}
	if !locked {
		return errLeaseBusy
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
		return w.putState(ctx, State{ChannelID: channelID, Status: "idle", FencingToken: token, UpdatedAt: now})
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
	return w.putState(ctx, state)
}

func (w Worker) putState(ctx context.Context, state State) error {
	written, err := w.Store.PutFenced(ctx, state, w.Owner, 30*time.Second)
	if err != nil {
		return err
	}
	if !written {
		return errFencedWriteRejected
	}
	return nil
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
			if errors.Is(err, errLeaseBusy) {
				if w.OnSkipped != nil {
					w.OnSkipped()
				}
				continue
			}
			if w.OnTick != nil {
				w.OnTick(err)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
