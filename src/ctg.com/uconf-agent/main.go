package main

import (
	"flag"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ctg.com/uconf-agent/app"
	"ctg.com/uconf-agent/consts"
	"ctg.com/uconf-agent/context"
	"ctg.com/uconf-agent/fileutils"
	"ctg.com/uconf-agent/host"
	"ctg.com/uconf-agent/httpclient"
	"ctg.com/uconf-agent/zkMgr"

	"github.com/golang/glog"
)

var autoReload bool

//TODO 1 表示连接上了 0 表示没连上
var zkState int32

func main() {
	flag.Parse()
	defer glog.Flush()
	//定时flush日志
	flushLog()
	//解析配置agent文件
	agentConfig := fileutils.Read()
	autoReload = agentConfig.Enabled
	//获取RESTful API的url地址
	zooAction, fileAction := agentConfig.Server.ServerActionAddress()
	//获取机器信息
	machine := host.Info(agentConfig.Server.Ip + ":" + agentConfig.Server.Port)
	//初始化zookeeper
	var prefix string
	if autoReload { //需要连接zk,那么就必须等获取到zk连接信息之后才继续下面的动作
		zooHostsUrl := zooAction + "/" + "hosts"
		zooPrefixUrl := zooAction + "/" + "prefix"
		var hostsResponseMap map[string]interface{}
		var prefixResponseMap map[string]interface{}
		var latch sync.WaitGroup
		latch.Add(2)
		go func() {
			for {
				var err error
				hostsResponseMap, err = httpclient.GetValueFromServer(zooHostsUrl)
				if err != nil {
					glog.Errorf("获取zk服务器地址列表失败，将在%d秒后重试.", consts.ZooServerInfoRetryGap/time.Second)
					time.Sleep(consts.ZooServerInfoRetryGap)
				} else {
					glog.Info("获取zk服务器地址列表成功.")
					break
				}
			}
			latch.Done()
		}()
		go func() {
			for {
				var err error
				prefixResponseMap, err = httpclient.GetValueFromServer(zooPrefixUrl)
				if err != nil {
					glog.Errorf("获取zk根路径失败，将在%d秒后重试.", consts.ZooServerInfoRetryGap/time.Second)
					time.Sleep(consts.ZooServerInfoRetryGap)
				} else {
					glog.Info("获取zk根路径成功.")

					break
				}
			}
			latch.Done()
		}()
		//等待获取zk信息
		latch.Wait()
		hosts := hostsResponseMap["value"].(string)
		glog.Info("zk服务器地址列表:", hosts)
		prefix = prefixResponseMap["value"].(string)
		glog.Info("zk根路径:", prefix)
		servers := strings.Split(hosts, ",")
		zkMgr.InitZk(servers, ZkSateCallBack)
	}
	var stopSignal chan int = make(chan int, 1)
	for _, item := range agentConfig.Apps {
		//获取App指纹
		appIdentity := app.NewIdentity(item.Tenant, item.Name, item.Version, item.Env, machine.Ip, machine.HoseName)
		//调用RESTful API下载配置到临时目录
		for _, conf := range item.Configs {
			//获取下载链接
			url := fileDownloadUrl(fileAction, appIdentity, &conf)
			//获取配置文件下载的保存路径
			path := item.Appdir + string(filepath.Separator) + conf.Dir + string(filepath.Separator) + conf.Name
			//获取配置文件在Zk的节点路径和实例的zk路径
			fileZkPath, instanceZkPath := appInstanceNode(prefix, conf.Name, appIdentity)
			//依次处理配置文件,在新的goroutine中处理
			go func() {
				ctx := context.NewRoutineContext(conf.Name, url, path, fileZkPath, instanceZkPath)
				dealFileItem(ctx)
			}()
		}
	}

	<-stopSignal
}

func ZkSateCallBack(isConnected bool) {
	if isConnected {
		atomic.StoreInt32(&zkState, 1)
	} else {
		atomic.StoreInt32(&zkState, 0)
	}
}
func fileDownloadUrl(serverAddr string, appIdentity *app.Identity, conf *fileutils.AppConfig) string {
	return serverAddr + "?version=" + appIdentity.Version + "&app=" + appIdentity.AppName + "&env=" + appIdentity.Env + "&key=" + conf.Name + "&tenant=" + appIdentity.Tenant + "&type=file"
}

//处理配置文件，包含：下载配置文件，保存配置文件到指定目录，监听配置文件节点，创建Agent实例临时节点
func dealFileItem(ctx *context.RoutineContext) {
	defer glog.Flush()

	for {
		data, success := downloadAndSave(ctx)
		if autoReload { //配置更新是否需要重新下载
			//先判断配置文件zk节点是否存在，或者zk服务器异常连不上
			if atomic.LoadInt32(&zkState) == 1 { //zk连接是否正常，下面代码执行期间也可能出现zk断开的问题
				exists, err := checkFileNode(ctx)
				if err != nil {
					break
				}
				if !exists {
					break
				}
				//TODO 需要设计多次重试后仍然失败的策略
				if success {
					ctx.Data = data
					created := DoRetryCall(createOrUpdateInstNode, ctx, "调用创建实例临时节点方法失败!")
					if created {
						ctx.Data = nil
					}
				}
				DoRetryCall(watchFileNode, ctx, "调用监听配置文件节点方法失败!")
				break
				//				createOrUpdateInstNode(ctx, data)
				//				watchFileNode(ctx, nil)

			} else {
				//zk连接不上，暂时什么都不做
				break
			}

		} else {
			break
		}
	}

}

//创建实例临时节点
func createOrUpdateInstNode(ctx *context.RoutineContext) bool {
	glog.Infof("[Rtn%d]开始创建Agent实例临时节点:%s.", ctx.RoutineId, ctx.InstanceZkPath)
	success := zkMgr.CreateOrUpdateEphemeralNode(ctx.InstanceZkPath, ctx.Data)
	if success {
		glog.Infof("[Rtn%d]Agent实例临时节点创建成功:%s.\n", ctx.RoutineId, ctx.InstanceZkPath)
	} else {
		glog.Errorf("[Rtn%d]Agent实例临时节点创建失败:%s.\n", ctx.RoutineId, ctx.InstanceZkPath)
	}
	return success

}

//监听配置文件zk节点
func watchFileNode(ctx *context.RoutineContext) bool {
	glog.Infof("[Rtn%d]开始监听配置文件节点:%s.\n", ctx.RoutineId, ctx.FileZkPath)
	//监听配置文件的zk节点
	for {
		glog.Flush()
		watcher, success := zkMgr.GetNodeWatcher(ctx.FileZkPath) //需要处理zk异常
		if !success {
			return false
		}
		event := <-watcher
		glog.Infof("[Rtn%d]监听到节点%s发生[%s]事件.", ctx.RoutineId, ctx.FileZkPath, event.Type.String())
		if zkMgr.IsDataChanged(event) {
			glog.Infof("[Rtn%d]配置文件节点发生变化:%s,准备重新下载.", ctx.RoutineId, ctx.FileZkPath)
			break
		}
	}
	return true
}

//下载配置文件并保存
func downloadAndSave(ctx *context.RoutineContext) ([]byte, bool) {
	glog.Infof("[Rtn%d]开始下载配置文件:%s", ctx.RoutineId, ctx.FileName) //下载
	data, success := httpclient.DownloadFromServer(ctx.Url)
	if success {
		glog.Infof("[Rtn%d]下载配置文件%s成功，开始保存文件", ctx.RoutineId, ctx.FileName)
		glog.Infof("[Rtn%d]配置文件%s内容为:\n%s\n", ctx.RoutineId, ctx.FileName, string(data))
		fileutils.WriteFile(ctx.Path, data)
		glog.Infof("[Rtn%d]配置文件已保存到:%s.", ctx.RoutineId, ctx.Path)
	} else {
		glog.Infof("[Rtn%d]下载配置文件%s失败!", ctx.RoutineId)
		return nil, false //下载失败返回一个空的字符串
	}
	return data, success

}

//校验配置文件节点是否存在
func checkFileNode(ctx *context.RoutineContext) (bool, error) {
	glog.Infof("[Rtn%d]开始校验Zk上是否存在配置文件节点:%s.", ctx.RoutineId, ctx.FileZkPath)
	var isExists bool
	var err error
	if isExists, err = zkMgr.ExistsNode(ctx.FileZkPath); !isExists && err == nil {
		glog.Fatalf("[Rtn%d]配置文件节点:%s,不存在.", ctx.RoutineId, ctx.FileZkPath)
		return false, nil
	} else {
		if err != nil {
			glog.Errorf("[Rtn%d]校验Zk上是否存在配置文件节点出现异常.", ctx.RoutineId)
			return false, err
		} else {
			glog.Infof("[Rtn%d]校验成功,Zk上存在配置文件节点:%s.", ctx.RoutineId, ctx.FileZkPath)
			return true, nil
		}
	}

}

// 获得实例保存到zk上的路径
func appInstanceNode(prefix, filename string, app *app.Identity) (string, string) {
	fileZkPath := prefix + "/" + app.AppNodePath() + "/file/" + filename
	instanceZkPath := fileZkPath + "/" + app.InstanceNodePath()
	return fileZkPath, instanceZkPath
}

//定时将日志刷到文件中
func flushLog() {
	ticker := time.NewTicker(time.Second * 5)
	go func() {
		for t := range ticker.C {
			t.Year()
			glog.Flush()
		}
	}()
}

type UnreliableZkCaller func(ctx *context.RoutineContext) bool

//传入适配UnreliableZkCaller类型的方法；调用参数；超时信息，可进行失败重试
func DoRetryCall(caller UnreliableZkCaller, ctx *context.RoutineContext, timeoutMsg string) bool {
	for i := 0; i < consts.UnreliableZkRetryTimes; i++ {
		if !caller(ctx) {
			retryRemainTimes := consts.UnreliableZkRetryTimes - (i + 1)
			if retryRemainTimes > 0 {
				glog.Errorf("[Rtn%d]%s，将在%d秒后将重试，剩余重试次数:%d", ctx.RoutineId, timeoutMsg, consts.UnreliableZkRetryGap/time.Second, retryRemainTimes)
				time.Sleep(consts.UnreliableZkRetryGap)
			} else {
				glog.Errorf("[Rtn%d]%s，剩余重试次数:%d\n", ctx.RoutineId, timeoutMsg, retryRemainTimes)

			}
			continue
		}
		return true
	}
	return false
}
