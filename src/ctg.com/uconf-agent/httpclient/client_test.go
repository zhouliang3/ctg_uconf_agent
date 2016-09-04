package httpclient

import (
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	//	"ctg.com/uconf-agent/consts"
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
	//DoRetryCall(ACall, &context.RoutineContext{}, nil, "充实各县")
}

type UnreliableZkCaller func(ctx *context.RoutineContext, data []byte) bool

func ACall(ctx *context.RoutineContext, data []byte) bool {
	fmt.Println("Acalled")
	return false
}

func TestGet(t *testing.T) {
}
