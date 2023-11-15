package subscription

import (
	"context"
	"time"

	"github.com/jensneuse/abstractlogger"
)

// TimeOutParams is a struct to configure a TimeOutChecker.
type TimeOutParams struct {
	Name           string
	Logger         abstractlogger.Logger
	TimeOutContext context.Context
	TimeOutAction  func()
	Timer          *time.Timer
}

// TimeOutChecker is a function that can be used in a go routine to perform a time-out action
// after a specific duration or prevent the time-out action by canceling the time-out context before.
// Use TimeOutParams for configuration.
func TimeOutChecker(params TimeOutParams) {
	if params.Timer == nil {
		params.Logger.Error("timer is nil",
			abstractlogger.String("name", params.Name),
		)
		return
	}
	defer params.Timer.Stop()

	for {
		select {
		case <-params.TimeOutContext.Done():
			return
		case <-params.Timer.C:
			params.Logger.Error("time out happened",
				abstractlogger.String("name", params.Name),
			)
			params.TimeOutAction()
			return
		}
	}
}
