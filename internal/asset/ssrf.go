package asset

import (
	"context"
	"net/url"

	"github.com/vod2live/cyonlive_vod2live/internal/safefetch"
)

func validateOrigin(ctx context.Context, rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	return safefetch.ValidateURL(ctx, u, allowPrivate)
}
