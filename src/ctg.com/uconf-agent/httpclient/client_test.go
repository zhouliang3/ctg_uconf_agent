package httpclient

import (
	"testing"
	"time"

	"ctg.com/uconf-agent/context"
)

func TestSomething(t *testing.T) {
}

type Retryer interface {
	doRetry(caller UnreliableCaller, ctx *context.RoutineContext, timeoutMsg string) *OutputContext
}
type RoundRobinRetryer struct {
	RetryTimes int
	RetryGap   time.Duration
}

func (retryer RoundRobinRetryer) doRetry(caller UnreliableCaller, ctx *context.RoutineContext, msg string) *OutputContext {
	for i := 0; i < retryer.RetryTimes; i++ {
		if !caller(ctx) {
			retryRemainTimes := retryer.RetryTimes - (i + 1)
			if retryRemainTimes > 0 {
				glog.Errorf("[Rtn%d]%s，将在%d秒后将重试，剩余重试次数:%d", ctx.RoutineId, msg, retryer.RetryGap/time.Second, retryRemainTimes)
				time.Sleep(retryer.RetryGap)
			} else {
				glog.Errorf("[Rtn%d]%s，剩余重试次数:%d\n", ctx.RoutineId, msg, retryRemainTimes)

			}
			continue
		}
		return true
	}
	return false
}

type UnreliableCaller func(ctx *context.RoutineContext) *context.OutputContext
