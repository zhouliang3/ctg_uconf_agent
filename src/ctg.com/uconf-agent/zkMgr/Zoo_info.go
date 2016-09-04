package zkMgr

import (
	"strings"
	"sync"

	"ctg.com/uconf-agent/consts"
	"ctg.com/uconf-agent/context"
	"ctg.com/uconf-agent/httpclient"
	"ctg.com/uconf-agent/retryer"
	"github.com/golang/glog"
)

func ZooInfo(zooAction string) ([]string, string) {
	zooActionMeta := context.NewZooServiceMeta(zooAction)
	var hostsResponseMap map[string]interface{}
	var prefixResponseMap map[string]interface{}
	var latch sync.WaitGroup
	latch.Add(2)
	//无限次数的重试
	endlessRetry := retryer.NewEndlessRetryer(consts.ZooServerInfoRetryGap)
	go func() {
		ctx := context.NewZooActionMetaContext(zooActionMeta)
		output := endlessRetry.DoRetry(FetchZoohosts, ctx)
		if v, ok := output.Result.(map[string]interface{}); ok {
			hostsResponseMap = v
			glog.Infof("[Rtn%d]获取zk服务器地址列表成功.", ctx.RoutineId)
		}
		latch.Done()
	}()
	go func() {
		ctx := context.NewZooActionMetaContext(zooActionMeta)
		output := endlessRetry.DoRetry(FetchZooPrefix, ctx)
		if v, ok := output.Result.(map[string]interface{}); ok {
			prefixResponseMap = v
			glog.Infof("[Rtn%d]获取zk根路径成功.", ctx.RoutineId)
		}
		latch.Done()
	}()
	//等待获取zk信息
	latch.Wait()
	hosts := hostsResponseMap["value"].(string)
	glog.Info("zk服务器地址列表:", hosts)
	prefix := prefixResponseMap["value"].(string)
	glog.Info("zk根路径:", prefix)
	servers := strings.Split(hosts, ",")
	return servers, prefix
}

func FetchZoohosts(ctx *context.RoutineContext) *context.OutputContext {
	hostsResponseMap, err := httpclient.GetValueFromServer(ctx.ZooActionMeta.HostsActionUrl())
	if err != nil {
		//glog.Errorf("[Rtn%d]获取zk服务器地址列表失败.", ctx.RoutineId)
		return context.NewErrorOutputContext(err)
	} else {
		return context.NewSuccessOutputContext(hostsResponseMap)
	}
}

func FetchZooPrefix(ctx *context.RoutineContext) *context.OutputContext {
	hostsResponseMap, err := httpclient.GetValueFromServer(ctx.ZooActionMeta.PrefixActionUrl())
	if err != nil {
		//glog.Errorf("[Rtn%d]获取zk服务器地址列表失败.", ctx.RoutineId)
		return context.NewErrorOutputContext(err)
	} else {
		return context.NewSuccessOutputContext(hostsResponseMap)
	}
}
