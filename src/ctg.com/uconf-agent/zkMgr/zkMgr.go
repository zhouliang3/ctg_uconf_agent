package zkMgr

import (
	"log"
	"strings"
	"sync"
	"time"

	"ctg.com/uconf-agent/strutils"
	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

//zk的客户端连接实例，不暴露
var zkConn *zk.Conn

const RetryTimes int = 3
const RetryGap = time.Second * 5
const ConnectTimeOut = time.Second * 5

var connectLock sync.Mutex
var servers []string
var connectChn chan int = make(chan int)
var recovering bool = false

//处理连接状态变更的回调函数
func stateChangeCallback(event zk.Event) {
	if event.Type != zk.EventSession { //不是会话相关事件，此回调方法不处理
		return
	}
	glog.Infof("监听到Zk连接状态变更事件[%v].", event)
	if event.State == zk.StateHasSession {
		glog.Info("与ZK服务器会话建立成功.")
		connectChn <- 1
	}
	if event.State == zk.StateDisconnected {
		//这是由于网络不通或服务器没启动等原因造成的
		if !recovering {
			glog.Errorln("与zk服务器连接失败.开始重试.")
			reconnect()
		}
	}
	if event.State == zk.StateExpired {
		if !recovering {
			glog.Info("会话失效，准备重连.")
			reconnect()
		}
	}
	if event.State == zk.StateAuthFailed {
		glog.Errorln("Zk鉴权失败.")
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

	for i := 0; ; i++ {
		glog.Infof("尝试第%d次连接zk服务器.", i+1)
		if connected := connect(); !connected {
			glog.Errorf("尝试第%d次连接zk服务器失败.", i+1)
			time.Sleep(RetryGap)
		} else {
			break
		}
	}
}

//连接zk的方法，不暴露
func connect() bool {
	var err error
	zkConn, _, err = zk.Connect(servers, time.Minute, zk.WithEventCallback(stateChangeCallback))
	checkError(err)
	select {
	case <-connectChn:
		glog.Info("与zk服务器建立连接成功.")
		return true
	case <-time.After(ConnectTimeOut):
		glog.Errorln("与zk服务器连接连接超时.")
		//关闭连接，会触发StateDisconnected事件，然后在callback中进行reconnect
		zkConn.Close()
		return false
	}
}

//判断节点是否存在
func ExistsNode(path string) bool {
	//glog.Info("path is ", path)
	exists, _, err := zkConn.Exists(path)
	if err != nil {
		log.Fatalf("check zknode s% exists err v% ", path, err)
	}
	return exists
}

//初始化Zk连接
func InitZk(servs []string) bool {
	glog.Info("开始建立Zk连接.")
	servers = servs
	isConnected := connect()

	glog.Info("成功建立Zk连接.")
	return isConnected
}

func writeData(path string, data []byte) bool {
	if !ExistsNode(path) {
		return false
	} else {
		_, err := zkConn.Set(path, []byte(data), -1)
		if err != nil {
			log.Fatalf("set value of node s% err v% ", path, err)
			return false
		}
	}
	return true
}

func CreateNode(path string, data []byte) bool {
	return createZkNode(path, data, 0)
}

func createZkNode(path string, data []byte, flags int32) bool {
	if !ExistsNode(path) {
		_, err := zkConn.Create(path, data, flags, zk.WorldACL(zk.PermAll))
		if err != nil {
			log.Fatalf("create node s% err v% ", path, err)
			if !ExistsNode(path) {
				return false
			}
		}
	}
	return true
}

//递归创建节点，即：如果父节点不存在，先创建父节点
func CreateNodeRecursion(path string, data []byte) {
	if ExistsNode(path) {
		return
	}
	pp := parentPath(path)
	if !ExistsNode(pp) {
		CreateNodeRecursion(pp, []byte(""))
	}
	CreateNode(path, data)
}

//创建临时节点
func CreateOrUpdateEphemeralNode(path string, data []byte) bool {
	if !ExistsNode(path) {
		return createZkNode(path, data, zk.FlagEphemeral)
	} else {
		return writeData(path, data)
	}
}
func GetNodeWatcher(path string) <-chan zk.Event {
	_, _, watcher, err := zkConn.GetW(path)
	checkError(err)
	return watcher
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
func checkError(err error) {
	if err != nil {
		log.Fatalf("Get : %v", err)
	}
}
