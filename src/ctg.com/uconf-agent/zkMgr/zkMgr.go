package zkMgr

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ctg.com/uconf-agent/consts"
	"ctg.com/uconf-agent/context"
	"ctg.com/uconf-agent/retryer"
	"ctg.com/uconf-agent/strutils"
	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

//zk的客户端连接实例，不暴露
var zkConn *zk.Conn

var connectLock sync.Mutex
var servers []string
var connectChn chan int = make(chan int)
var recovering bool = false

type StateCallback func(isConnected bool)

var Callback StateCallback
var ConnWaitCounter int32 = 0
var mainContext *context.RoutineContext

type NodeWatcher <-chan zk.Event

//处理连接状态变更的回调函数
func stateChangeCallback(event zk.Event) {
	if event.Type != zk.EventSession { //不是会话相关事件，此回调方法不处理
		return
	}
	glog.Infof("监听到Zk连接状态变更事件[%v].", event)
	if event.State == zk.StateHasSession {
		glog.Info("与ZK服务器会话建立成功.")
		Callback(true)
		for ; atomic.LoadInt32(&ConnWaitCounter) > 0; atomic.AddInt32(&ConnWaitCounter, -1) {
			connectChn <- 1
		}
	}
	if event.State == zk.StateDisconnected {
		//这是由于网络不通或服务器没启动等原因造成的
		if !recovering {
			glog.Errorln("与zk服务器连接失败.开始重试.")
			Callback(false)
			reconnect()
		}
	}
	if event.State == zk.StateExpired {
		if !recovering {
			glog.Info("会话失效，准备重连.")
			Callback(false)
			reconnect()
		}
	}
	if event.State == zk.StateAuthFailed {
		glog.Errorln("Zk鉴权失败.")
		Callback(false)
	}
}

//断线重连，连接不上就一直重试
func reconnect() {
	connectLock.Lock()
	defer func() {
		recovering = false
		connectLock.Unlock()
	}()
	recovering = true
	endlessRetry := retryer.NewEndlessRetryer(consts.ZkConnectRetryGap)
	endlessRetry.DoRetry(Connect, mainContext)
}

//连接zk的方法，不暴露
func Connect(ctx *context.RoutineContext) *context.OutputContext {
	var err error
	zkConn, _, err = zk.Connect(servers, time.Minute, zk.WithEventCallback(stateChangeCallback))
	checkError("与Zk服务器建立连接时，出现异常.", err)
	atomic.AddInt32(&ConnWaitCounter, 1)
	select {
	case <-connectChn:
		glog.Info("与zk服务器建立连接成功.")
		return context.NewSuccessOutputContext(nil)
	case <-time.After(consts.ZkConnectTimeOut):
		glog.Errorln("与zk服务器建立连接超时.")
		//关闭连接，会触发StateDisconnected事件，然后在callback中进行reconnect
		zkConn.Close()
		return context.NewErrorOutputContext(errors.New("与zk服务器建立连接超时"))
	}
}

//判断节点是否存在
func ExistsNode(path string) (bool, error) {
	//glog.Info("path is ", path)
	var rErr error
	for i := 0; i < consts.ZkCallerRetryTimes; i++ {
		exists, _, err := zkConn.Exists(path)
		if err != nil {
			rErr = err
			retryRemainTimes := consts.ZkCallerRetryTimes - (i + 1)
			if retryRemainTimes > 0 {
				glog.Errorf("调用zk接口判断节点%s是否存在时，出现异常，将在%d秒后将重试，剩余重试次数:%d.", path, consts.ZkCallerRetryGap/time.Second, retryRemainTimes)
				time.Sleep(consts.ZkCallerRetryGap)
			} else {
				glog.Errorf("调用zk接口判断节点%s是否存在时，出现异常，剩余重试次数:%d.", path, retryRemainTimes)
			}

			continue
		} else {
			return exists, nil
		}
	}
	return false, rErr

}

//初始化Zk连接
func InitZk(ctx *context.RoutineContext, servs []string, sc StateCallback) bool {
	glog.Infof("[Rtn%d]开始建立Zk连接.", ctx.RoutineId)
	servers = servs
	Callback = sc
	mainContext = ctx
	output := Connect(ctx)
	if output.Err == nil {
		glog.Infof("[Rtn%d]成功建立Zk连接.", ctx.RoutineId)
		return true
	}
	return false
}

func writeData(path string, data []byte) bool {
	exists, err := ExistsNode(path)
	if err != nil {
		return false
	}
	if !exists {
		return false
	} else {
		_, err := zkConn.Set(path, []byte(data), -1)
		if err != nil {
			glog.Errorf("将数据写入zk节点: s% , 出现异常: v%.", path, err)
			return false
		}
	}
	return true
}

func CreateNode(path string, data []byte) bool {
	return createZkNode(path, data, 0)
}

func createZkNode(path string, data []byte, flags int32) bool {
	exists, err := ExistsNode(path)
	if err != nil {
		return false
	}
	if !exists {
		_, err := zkConn.Create(path, data, flags, zk.WorldACL(zk.PermAll))
		if err != nil {
			glog.Errorf("创建zk节点: s%,出现异常: v%.", path, err)
			return false
		}
	}
	return true
}

//递归创建节点，即：如果父节点不存在，先创建父节点
func CreateNodeRecursion(path string, data []byte) {
	exists, err := ExistsNode(path)
	if err != nil {
		return
	}
	if exists {
		return
	}
	pp := parentPath(path)
	exists, err = ExistsNode(pp)
	if err != nil {
		return
	}
	if !exists {
		CreateNodeRecursion(pp, []byte(""))
	}
	CreateNode(path, data)
}

//创建临时节点
func CreateOrUpdateEphemeralNode(path string, data []byte) bool {
	exists, err := ExistsNode(path)
	if err != nil {
		return false
	}
	if !exists {
		return createZkNode(path, data, zk.FlagEphemeral)
	} else {
		return writeData(path, data)
	}
}
func GetNodeWatcher(path string) (NodeWatcher, bool) {
	_, _, watcher, err := zkConn.GetW(path)
	checkError("调用获取zk节点的watcher出现异常", err)
	if err != nil {
		return watcher, false
	} else {
		return watcher, true
	}
}

//递归删除节点，即：删除节点和其所有子节点，暂时只有测试用
func deleteNodeRecursion(path string) {
	_, stat, _ := zkConn.Get(path)
	if stat.NumChildren > 0 {
		children, _, _ := zkConn.Children(path)
		for _, child := range children {
			childPath := path + "/" + child
			deleteNodeRecursion(childPath)
		}
	}
	//	glog.Info(path)

	zkConn.Delete(path, -1)
}

//判断是否是节点数据变更事件
func IsDataChanged(event zk.Event) bool {
	return event.Type == zk.EventNodeDataChanged
}

//获取父节点zk路径
func parentPath(path string) string {
	i := strings.LastIndex(path, "/")
	parentPath := strutils.Substring(path, 0, i)
	return parentPath
}
func checkError(errMsg string, err error) {
	if err != nil {
		glog.Errorf("%s : %v", errMsg, err)
	}
}
