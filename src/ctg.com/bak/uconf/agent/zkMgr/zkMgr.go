package zkMgr

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/strutils"
	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

//zk的客户端连接实例，不暴露
var zkConn *zk.Conn

var connectLock sync.Mutex
var callbackLock sync.Mutex
var servers []string
var connectChn chan int = make(chan int)
var recovering bool = false

type StateCallback func(isConnected bool)

var Callback StateCallback
var ConnWaitCounter int32 = 0
var mainContext *context.RoutineContext

type NodeWatcher <-chan zk.Event

var ReconnectWatcher = make(chan zk.Event, 6)
var toReconnected bool = false

//处理连接状态变更的回调函数
func stateChangeCallback(event zk.Event) {
	if event.Type != zk.EventSession { //不是会话相关事件，此回调方法不处理
		return
	}
	glog.Infof("监听到Zk连接状态变更事件[%v].", event)
	if event.State == zk.StateHasSession {
		glog.Info("与ZK服务器会话建立成功.")
		if toReconnected {
			ReconnectWatcher <- event
			toReconnected = false
		}
		connectChn <- 1
	}
	if event.State == zk.StateDisconnected {
		toReconnected = true
		glog.Errorln("与zk服务器连接断开.")
	}
	if event.State == zk.StateExpired {
		toReconnected = true
		glog.Errorln("与zk服务器会话超时.")
	}
	if event.State == zk.StateAuthFailed {
		glog.Fatal("Zk鉴权失败.")
	}
}

func invokeCallback(connected bool) {
	callbackLock.Lock()
	defer callbackLock.Unlock()
	Callback(connected)
}

//连接zk的方法，不暴露
func Connect() {
	var err error
	zkConn, _, err = zk.Connect(servers, time.Minute, zk.WithEventCallback(stateChangeCallback))
	if err != nil {
		glog.Fatal("与Zk服务器建立连接时，出现异常.", err)
	}
}

//判断节点是否存在,zk层的判断存在方法暂时不采用重试机制，有业务层自己封装进行重试
func ExistsNode(path string) (bool, error) {
	fmt.Println("进入校验节点是否存在的zkmgr方法,", *zkConn)
	exists, _, err := zkConn.Exists(path)
	fmt.Println("退出校验节点是否存在的zkmgr方法", path)
	return exists, err
}

//初始化Zk连接
func InitZk(ctx *context.RoutineContext, servs []string) {
	glog.Infof("[Rtn%d]开始建立Zk连接.", ctx.RoutineId)
	servers = servs
	Connect()
	<-connectChn
	//连接成功
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
