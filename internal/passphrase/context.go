package passphrase

import (
	"context"
	"time"
)

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = promptTimeout
	}
	return context.WithTimeout(context.Background(), d)
}

func timedOut(ctx context.Context) bool {
	return ctx.Err() == context.DeadlineExceeded
}
