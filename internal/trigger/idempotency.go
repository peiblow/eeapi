package trigger

import (
	"context"
	"time"

	"github.com/peiblow/eeapi/internal/database/redis"
)

// watermark is source-observation dedup: it answers "have I already emitted for
// this exact observation?". Distinct from the AVM consumer group's delivery
// dedup (XAck) — this stops the same file state from firing the agent twice.
type watermark struct {
	rdb *redis.Client
}

func (w *watermark) firstSeen(ctx context.Context, key string, ttl time.Duration) bool {
	if w == nil || w.rdb == nil {
		return true
	}
	ok, err := w.rdb.Client.SetNX(ctx, "synx:seen:"+key, 1, ttl).Result()
	if err != nil {
		return true
	}
	return ok
}
