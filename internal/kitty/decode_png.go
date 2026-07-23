package kitty

import (
	"context"
	"io"

	"cervterm/internal/termimage"
)

func decodePNG(ctx context.Context, store *termimage.Store, imageID termimage.ImageID, payload *termimage.SealedEncodedPayload) (*termimage.DecodedCandidate, error) {
	return termimage.DecodePNG(ctx, store, imageID, func() (io.Reader, error) {
		return base64Reader(ctx, payload)
	})
}
