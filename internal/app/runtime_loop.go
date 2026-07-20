package app

import (
	"context"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
	flowstatus "github.com/Willxup/flowlens/internal/status"
)

type trafficEvent struct {
	at     time.Time
	sample clashapi.TrafficSample
	err    error
}

// Run drives the minimal collector until cancellation or a fatal state error.
func (r *Runtime) Run(ctx context.Context) error {
	connectionTicker := time.NewTicker(r.interval)
	sealTicker := time.NewTicker(time.Second)
	defer connectionTicker.Stop()
	defer sealTicker.Stop()
	traffic := make(chan trafficEvent, 16)
	go r.readTraffic(ctx, traffic)
	afterGap := r.hasDurable
	for {
		select {
		case <-ctx.Done():
			sealContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := r.Seal(sealContext, time.Now())
			cancel()
			return err
		case event := <-traffic:
			if event.err != nil {
				_ = r.status.Set(flowstatus.LevelDegraded, "clash_unavailable", true)
				continue
			}
			if err := r.ObserveTraffic(event.at, event.sample); err != nil {
				return err
			}
		case <-connectionTicker.C:
			if err := r.observeConnectionsNow(ctx, afterGap); err != nil {
				afterGap = true
				continue
			}
			afterGap = false
		case now := <-sealTicker.C:
			if err := r.Seal(ctx, now); err != nil {
				afterGap = true
			}
		}
	}
}

func (r *Runtime) readTraffic(ctx context.Context, output chan<- trafficEvent) {
	backoff := time.Second
	for {
		stream, err := r.client.Traffic(ctx)
		if err != nil {
			if !sendTrafficEvent(ctx, output, trafficEvent{err: err}) || !waitContext(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = time.Second
		for {
			sample, err := stream.Next()
			if err != nil {
				_ = stream.Close()
				if !sendTrafficEvent(ctx, output, trafficEvent{err: err}) {
					return
				}
				break
			}
			if !sendTrafficEvent(ctx, output, trafficEvent{at: time.Now(), sample: sample}) {
				_ = stream.Close()
				return
			}
		}
		if !waitContext(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

func sendTrafficEvent(ctx context.Context, output chan<- trafficEvent, event trafficEvent) bool {
	select {
	case output <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func nextBackoff(current time.Duration) time.Duration {
	current *= 2
	if current > 30*time.Second {
		return 30 * time.Second
	}
	return current
}
