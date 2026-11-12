package media

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vod2live/cyonlive_vod2live/internal/library"
	"github.com/vod2live/cyonlive_vod2live/internal/storage"
)

type Worker struct {
	Jobs       library.Repository
	Storage    *storage.Client
	FFmpegPath string
	TempDir    string
	Poll       time.Duration
	KeepSource bool
}

func (w Worker) Run(ctx context.Context) error {
	if err := w.Storage.EnsureBucket(ctx); err != nil {
		return err
	}
	if w.Poll <= 0 {
		w.Poll = 5 * time.Second
	}
	for {
		job, err := w.Jobs.ClaimJob(ctx)
		if err == nil {
			if err = w.process(ctx, job); err != nil {
				log.Printf("media job %s failed: %v", job.ID, err)
				_ = w.Jobs.FinishJob(ctx, job, "", 0, err.Error())
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.Poll):
		}
	}
}

func (w Worker) process(ctx context.Context, job library.Job) error {
	dir, err := os.MkdirTemp(w.TempDir, "job-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(dir, "source")
	if err := w.Storage.Download(ctx, job.ObjectKey, source); err != nil {
		return fmt.Errorf("download source: %w", err)
	}
	out := filepath.Join(dir, "hls", "720p")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	ffmpeg := w.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	playlist := filepath.Join(out, "playlist.m3u8")
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", source, "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "libx264", "-preset", "veryfast", "-profile:v", "main", "-g", "48", "-keyint_min", "48", "-sc_threshold", "0", "-c:a", "aac", "-b:a", "128k", "-f", "hls", "-hls_time", "6", "-hls_playlist_type", "vod", "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4", "-hls_segment_filename", filepath.Join(out, "segment-%05d.m4s"), playlist}
	if err := exec.CommandContext(ctx, ffmpeg, args...).Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	master := filepath.Join(dir, "hls", "master.m3u8")
	if err := os.WriteFile(master, []byte("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1280x720,CODECS=\"avc1.4d401f,mp4a.40.2\"\n720p/playlist.m3u8\n"), 0o644); err != nil {
		return err
	}
	root := filepath.Join(dir, "hls")
	var total int64
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		key := fmt.Sprintf("hls/organization/%s/asset/%s/%s", job.OrganizationID, job.AssetID, filepath.ToSlash(rel))
		ct := contentType(path)
		if err := w.Storage.UploadFile(ctx, key, path, ct); err != nil {
			return err
		}
		info, e := os.Stat(path)
		if e == nil {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return err
	}
	masterKey := fmt.Sprintf("hls/organization/%s/asset/%s/master.m3u8", job.OrganizationID, job.AssetID)
	if err := w.Jobs.FinishJob(ctx, job, w.Storage.ObjectURI(masterKey), total, ""); err != nil {
		return err
	}
	delta := total
	if !w.KeepSource {
		delta -= job.SourceBytes
	}
	if err := w.Jobs.AdjustUsage(ctx, job.OrganizationID, job.TenantID, delta); err != nil {
		return err
	}
	if !w.KeepSource {
		_ = w.Storage.Delete(ctx, job.ObjectKey)
	}
	return nil
}
func contentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	}
	return "application/octet-stream"
}
