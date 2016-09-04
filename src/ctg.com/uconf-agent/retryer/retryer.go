package retryer

import (
	"strconv"
	"time"

	"ctg.com/uconf-agent/context"
	"github.com/golang/glog"
)

const MinRetryGap = 100 * time.Millisecond

type UnreliableCaller func(ctx *context.RoutineContext) *context.OutputContext

type Retryer interface {
	DoRetry(caller UnreliableCaller, ctx *context.RoutineContext) *context.OutputContext
}

type RoundRobinRetryer struct {
	RetryTimes int
	RetryGap   time.Duration
}

func NewRoundRobinRetryer(retryTimes int, retryGap time.Duration) *RoundRobinRetryer {
	if retryGap < MinRetryGap {
		retryGap = MinRetryGap
	}
	return &RoundRobinRetryer{retryTimes, retryGap}
}

//重试RetryTimes次caller方法
func (retryer RoundRobinRetryer) DoRetry(caller UnreliableCaller, ctx *context.RoutineContext) *context.OutputContext {
	var lastOutput *context.OutputContext
	for i := 0; i < retryer.RetryTimes; i++ {
		output := caller(ctx)
		lastOutput = output
		if output.Err != nil {
			retryRemainTimes := retryer.RetryTimes - (i + 1)
			if retryRemainTimes > 0 {
				glog.Errorf("[Rtn%d]%v , 将在%s后将重试，剩余重试次数:%d", ctx.RoutineId, output.Err, retryGapConversion(retryer.RetryGap), retryRemainTimes)
				time.Sleep(retryer.RetryGap)
			} else {
				glog.Errorf("[Rtn%d]%v , 剩余重试次数:%d\n", ctx.RoutineId, output.Err, retryRemainTimes)

			}
			continue
		}
		return output
	}
	return lastOutput
}

type EndlessRetryer struct {
	RetryGap time.Duration
}

func NewEndlessRetryer(retryGap time.Duration) *EndlessRetryer {
	if retryGap < MinRetryGap {
		retryGap = MinRetryGap
	}
	return &EndlessRetryer{retryGap}
}

//无限次数的重试caller对应的方法
func (retryer EndlessRetryer) DoRetry(caller UnreliableCaller, ctx *context.RoutineContext) *context.OutputContext {
	var lastOutput *context.OutputContext
	for i := 0; ; i++ {
		output := caller(ctx)
		lastOutput = output
		if output.Err != nil {
			glog.Errorf("[Rtn%d]%v , 将在%s后将重试.", ctx.RoutineId, output.Err, retryGapConversion(retryer.RetryGap))
			time.Sleep(retryer.RetryGap)
			continue
		}
		return output
	}
	return lastOutput
}

func retryGapConversion(retryGap time.Duration) string {
	if retryGap >= time.Second && retryGap%time.Second == 0 {
		return strconv.Itoa(int(retryGap/time.Second)) + "秒"
	} else {
		return strconv.Itoa(int(retryGap/time.Millisecond)) + "毫秒"
	}
}
