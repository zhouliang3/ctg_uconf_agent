package retryer

import (
	"fmt"
	"testing"
	"time"

	"ctg.com/uconf/agent/context"
)

var times int = 0

type ServiceResult struct {
	Name string
	Id   int
}

func TestRoundRobinRetryer(t *testing.T) {
	//	ctx := context.NewEmptyRoutineContext()
	//	r := NewRoundRobinRetryer(5, 1*time.Second)
	//	output := r.DoRetry(Service, ctx)
	//	if output.Err == nil {
	//		if v, ok := output.Result.(*ServiceResult); ok {
	//			fmt.Println("name is:", v.Name)
	//		}
	//	}
}
func TestEndlessRetryer(t *testing.T) {
	ctx := context.InitMainRoutineContext()
	endlessRetryer := NewEndlessRetryer(2000 * time.Millisecond)
	output := endlessRetryer.DoRetry(Service, ctx)
	if output.Err == nil {
		if v, ok := output.Result.(*ServiceResult); ok {
			fmt.Println("请求成功")
			fmt.Println("name is:", v.Name)
		}
	}
}
func Service(ctx *context.RoutineContext) *context.OutputContext {
	fmt.Printf("[Rtn%d]\n", ctx.RoutineId)
	times++
	if times > 10 {

		return context.NewSuccessOutputContext(&ServiceResult{"zhouliang", 1})
	}
	return context.NewFailOutputContext("请求失败")
}
