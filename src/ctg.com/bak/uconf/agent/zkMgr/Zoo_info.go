package zkMgr

import (
	"strings"
	"sync"

	"ctg.com/uconf/agent/consts"
	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/httpclient"
	"ctg.com/uconf/agent/retryer"
	"github.com/golang/glog"
)

type ZooActionMeta struct {
	ZooAction string
}

func NewZooServiceMeta(zooAction string) *ZooActionMeta {
	return &ZooActionMeta{zooAction}
}
func (zoo *ZooActionMeta) HostsActionUrl() string {
	return zoo.ZooAction + "/" + "hosts"
}
func (zoo *ZooActionMeta) PrefixActionUrl() string {
	return zoo.ZooAction + "/" + "prefix"
}
func ZooInfo(zooAction string) ([]string, string) {
	zooActionMeta := NewZooServiceMeta(zooAction)
	var hostsResponseMap map[string]interface{}
	var prefixResponseMap map[string]interface{}
	var latch sync.WaitGroup
	latch.Add(2)
	//无限次数的重试,因FetchZooPrefix的请求的http方法本身具有重试机制，这里做重试是为了缓解服务端请求压力，重试时间间隔设置到分钟级
	endlessRetry := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
	go func() {
		requestContext := context.NewRequestRoutineContext(zooActionMeta.HostsActionUrl(), nil)
		output := endlessRetry.DoRetry(httpclient.RetryableGetJsonData, requestContext)
		if v, ok := output.Result.(map[string]interface{}); ok {
			hostsResponseMap = v
			glog.Infof("[Rtn%d]获取zk服务器地址列表成功.", requestContext.RoutineId)
		}
		latch.Done()
	}()
	go func() {
		requestContext := context.NewRequestRoutineContext(zooActionMeta.PrefixActionUrl(), nil)
		output := endlessRetry.DoRetry(httpclient.RetryableGetJsonData, requestContext)
		if v, ok := output.Result.(map[string]interface{}); ok {
			prefixResponseMap = v
			glog.Infof("[Rtn%d]获取zk根路径成功.", requestContext.RoutineId)
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
