package app

import (
	"context"
	"time"

	flowstatus "github.com/Willxup/flowlens/internal/status"
)

func (a *App) runMaintenance(ctx context.Context) {
	for {
		now := time.Now()
		err := a.maintenance.RunScheduled(ctx, now)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			_ = a.status.SetComponent("maintenance", flowstatus.LevelDegraded, "maintenance_failed", true)
		} else {
			_ = a.status.SetComponent("maintenance", flowstatus.LevelOK, "ready", true)
		}
		next := a.maintenance.NextWake(time.Now())
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}
