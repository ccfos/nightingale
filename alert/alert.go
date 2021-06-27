package alert

import "context"

func Start(ctx context.Context) {
	go popEvent()
}
