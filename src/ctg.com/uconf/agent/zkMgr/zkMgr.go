package zkMgr

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ctg.com/uconf/agent/consts"
	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/retryer"
	"ctg.com/uconf/agent/strutils"
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
			//TODO 网络原因断开重连的时候，成功之后应该在应用层重新创建临时节点和监听配置文件节点的
			//TODO 断开和重连期间发生了配置文件的变更则会导致配置不一致的问题，所以应该从已发布的配置文件表中重新加载配置数据
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

//判断节点是否存在,zk层的判断存在方法暂时不采用重试机制，有业务层自己封装进行重试
func ExistsNode(path string) (bool, error) {
	exists, _, err := zkConn.Exists(path)
	return exists, err
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
