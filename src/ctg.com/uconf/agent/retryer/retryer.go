package retryer

import (
	"strconv"
	"sync"
	"time"

	"ctg.com/uconf/agent/consts"
	"ctg.com/uconf/agent/context"
	"github.com/golang/glog"
)

const MinRetryGap = 50 * time.Millisecond

type UnreliableCaller func(ctx *context.RoutineContext) *context.OutputContext

type Retryer interface {
	DoRetry(caller UnreliableCaller, ctx *context.RoutineContext) *context.OutputContext
}

type RoundRobinRetryer struct {
	RetryTimes int
	RetryGap   time.Duration
}

func NewRoundRobinRetryer(retryTimes int, retryGap time.Duration) RoundRobinRetryer {
	if retryGap < MinRetryGap {
		retryGap = MinRetryGap
	}
	return RoundRobinRetryer{retryTimes, retryGap}
}

var zkRetryer Retryer = NewRoundRobinRetryer(consts.UnreliableZkRetryTimes, consts.UnreliableZkRetryGap)

//zk的请求重试机制
func ZkRequestRetryer() Retryer {
	return zkRetryer
}

//http请求重试机制
var httpRetryer Retryer

func InitHttpRequestRetryer(times int, interval time.Duration) {
	httpRetryer = NewRoundRobinRetryer(times, interval)
}
func HttpRequestRetryer() Retryer {
	return httpRetryer
}

//重试RetryTimes次caller方法
func (retryer RoundRobinRetryer) DoRetry(caller UnreliableCaller, ctx *context.RoutineContext) *context.OutputContext {
	var lastOutput *context.OutputContext
	if retryer.RetryTimes <= 0 {
		output := caller(ctx)
		return output
	}
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

var endlessRetryerMap = make(map[time.Duration]EndlessRetryer)
var endlessRetryerLock sync.Mutex

func NewEndlessRetryer(retryGap time.Duration) EndlessRetryer {
	endlessRetryerLock.Lock()
	defer endlessRetryerLock.Unlock()
	if retryGap < MinRetryGap {
		retryGap = MinRetryGap
	}
	if endlessRetryer, isPresent := endlessRetryerMap[retryGap]; isPresent {
		return endlessRetryer
	} else {
		endlessRetryerMap[retryGap] = EndlessRetryer{retryGap}
		return endlessRetryerMap[retryGap]
	}
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
		} else {
			break
		}
	}
	return lastOutput
}

type NoneRetryer struct {
}

func NewNoneRetryer() NoneRetryer {
	return NoneRetryer{}
}

//不重试，快速失败
func (retryer NoneRetryer) DoRetry(caller UnreliableCaller, ctx *context.RoutineContext) *context.OutputContext {
	return caller(ctx)
}

func retryGapConversion(retryGap time.Duration) string {
	if retryGap >= time.Second && retryGap%time.Second == 0 {
		return strconv.Itoa(int(retryGap/time.Second)) + "秒"
	} else {
		return strconv.Itoa(int(retryGap/time.Millisecond)) + "毫秒"
	}
}
