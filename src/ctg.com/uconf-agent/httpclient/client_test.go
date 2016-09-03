package httpclient

import (
	"fmt"
	"testing"
	"time"

	"ctg.com/uconf-agent/consts"
	"ctg.com/uconf-agent/context"
)

type UnreliableCaller func(a, b, c string) bool

func DoRequest(a, b, c string) bool {
	fmt.Printf("a:%s\nb:%s\nc:%s\n", a, b, c)
	return false
}

func Retry(caller UnreliableCaller, a, b, c string) bool {
	for i := 0; i < 3; i++ {
		if !caller(a, b, c) {

			fmt.Println("重试")
			time.Sleep(time.Second * 3)
			continue
		}
		return true
	}
	return false
}

func TestSomething(t *testing.T) {
	DoRetryCall(ACall, &context.RoutineContext{}, nil, "充实各县")
}

type UnreliableZkCaller func(ctx *context.RoutineContext, data []byte) bool

func ACall(ctx *context.RoutineContext, data []byte) bool {
	fmt.Println("Acalled")
	return false
}

//传入适配UnreliableZkCaller类型的方法；调用参数；超时信息，可进行失败重试的调用
func DoRetryCall(caller UnreliableZkCaller, ctx *context.RoutineContext, data []byte, timeoutMsg string) bool {
	for i := 0; i < consts.UnreliableZkRetryTimes; i++ {
		if !caller(ctx, data) {
			retryRemainTimes := consts.UnreliableZkRetryTimes - (i + 1)
			if retryRemainTimes > 0 {
				fmt.Printf("%s，将在%d秒后将重试，剩余重试次数:%d\n", timeoutMsg, consts.UnreliableZkRetryGap/time.Second, retryRemainTimes)
				time.Sleep(consts.UnreliableZkRetryGap)
			} else {
				fmt.Printf("%s，剩余重试次数:%d\n", timeoutMsg, retryRemainTimes)

			}
			continue
		}
		return true
	}
	return false
}
