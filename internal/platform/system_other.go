//go:build !linux && !windows

package platform

import "context"

func WatchSystem(ctx context.Context, emit func(SystemEvent)) {
	emit(SystemEvent{})
	<-ctx.Done()
}
