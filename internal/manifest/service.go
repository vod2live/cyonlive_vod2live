package manifest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/schedule"
	"github.com/vod2live/cyonlive_vod2live/pkg/hls"
)

type Service struct {
	Assets    asset.PostgresRepository
	Schedules schedule.PostgresRepository
	Client    *http.Client
	Cache     ManifestCache
}

func (s Service) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}
func (s Service) fetch(ctx context.Context, raw string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	resp, err := s.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("source HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}
func (s Service) Master(ctx context.Context, channelID uuid.UUID) ([]byte, error) {
	cacheKey := "vod2live:manifest:" + channelID.String() + ":master"
	if cached, ok := s.Cache.Get(ctx, cacheKey); ok {
		return cached, nil
	}
	items, err := s.Schedules.List(ctx, channelID)
	if err != nil || len(items) == 0 {
		return nil, fmt.Errorf("channel has no schedule")
	}
	a, err := s.Assets.FindByID(ctx, items[0].AssetID)
	if err != nil {
		return nil, err
	}
	text, err := s.fetch(ctx, a.HLSURL)
	if err != nil {
		return nil, err
	}
	p, err := hls.ParseMaster(text)
	if err != nil {
		return nil, err
	}
	body := []byte(hls.RenderMaster(p, "/v1/channels/"+channelID.String()+"/media"))
	s.Cache.Set(ctx, cacheKey, body)
	return body, nil
}
func (s Service) Media(ctx context.Context, channelID uuid.UUID, variant string, now time.Time) ([]byte, error) {
	cacheKey := "vod2live:manifest:" + channelID.String() + ":media:" + variant + ":" + now.UTC().Format("20060102150405")
	if cached, ok := s.Cache.Get(ctx, cacheKey); ok {
		return cached, nil
	}
	items, err := s.Schedules.List(ctx, channelID)
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartsAt.Before(items[j].StartsAt) })
	current := sort.Search(len(items), func(i int) bool { return items[i].StartsAt.After(now) }) - 1
	if current < 0 {
		current = 0
	}
	var out hls.MediaPlaylist
	var sequence uint64
	total := 0
	for i := current; i < len(items) && total < 12; i++ {
		a, e := s.Assets.FindByID(ctx, items[i].AssetID)
		if e != nil {
			continue
		}
		masterText, e := s.fetch(ctx, a.HLSURL)
		if e != nil {
			continue
		}
		master, e := hls.ParseMaster(masterText)
		if e != nil {
			continue
		}
		v, e := selectVariant(master, variant)
		if e != nil {
			continue
		}
		mediaURL := hls.Resolve(a.HLSURL, v.URI)
		mediaText, e := s.fetch(ctx, mediaURL)
		if e != nil {
			continue
		}
		media, e := hls.ParseMedia(mediaText)
		if e != nil {
			continue
		}
		start := 0
		if i == current && now.After(items[i].StartsAt) {
			elapsed := now.Sub(items[i].StartsAt).Seconds()
			for start < len(media.Segments) && elapsed >= media.Segments[start].Duration {
				elapsed -= media.Segments[start].Duration
				start++
			}
		}
		for j := start; j < len(media.Segments) && total < 12; j++ {
			seg := media.Segments[j]
			seg.URI = hls.Resolve(mediaURL, seg.URI)
			if len(out.Segments) > 0 && j == start {
				seg.Discontinuity = true
			}
			out.Segments = append(out.Segments, seg)
			total++
		}
		if out.TargetDuration < media.TargetDuration {
			out.TargetDuration = media.TargetDuration
		}
		sequence = media.MediaSequence + uint64(start)
	}
	if len(out.Segments) == 0 {
		return nil, fmt.Errorf("no playable HLS segments")
	}
	body := []byte(hls.RenderMedia(out, sequence, false))
	s.Cache.Set(ctx, cacheKey, body)
	return body, nil
}

func selectVariant(p hls.MasterPlaylist, wanted string) (hls.Variant, error) {
	for _, v := range p.Variants {
		id := hls.VariantID(v)
		if id == wanted {
			return v, nil
		}
	}
	if n, err := strconv.Atoi(strings.TrimSuffix(wanted, "p")); err == nil {
		for _, v := range p.Variants {
			if v.Height == n {
				return v, nil
			}
		}
	}
	return hls.Variant{}, fmt.Errorf("variant %s not found", wanted)
}
