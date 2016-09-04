package main

import (
	"flag"
	"path/filepath"
	"sync/atomic"
	"time"

	"ctg.com/uconf-agent/app"
	"ctg.com/uconf-agent/consts"
	"ctg.com/uconf-agent/context"
	"ctg.com/uconf-agent/fileutils"
	"ctg.com/uconf-agent/host"
	"ctg.com/uconf-agent/httpclient"
	"ctg.com/uconf-agent/retryer"
	"ctg.com/uconf-agent/zkMgr"
	"github.com/golang/glog"
)

var autoReload bool

//TODO 1 表示连接上了 0 表示没连上
var zkState int32
var MainRoutineContext *context.RoutineContext

func main() {
	flag.Parse()
	defer glog.Flush()
	MainRoutineContext = context.InitMainRoutineContext()
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
	if autoReload { //需要连接zk,那么就必须等获取到zk服务器信息之后才继续下面的动作
		var servers []string
		servers, prefix = zkMgr.ZooInfo(zooAction)
		//TODO 如果建立连接失败暂时不进一步处理
		zkMgr.InitZk(MainRoutineContext, servers, ZkSateCallBack)
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
					ctx.FileContext.Data = data
					roundRobinRetry := retryer.NewRoundRobinRetryer(consts.UnreliableZkRetryTimes, consts.UnreliableZkRetryGap)
					output := roundRobinRetry.DoRetry(CreateOrUpdateInstNode, ctx)
					if output.Err == nil {
						ctx.FileContext.Data = nil
					} else { //TODO 重试多次依然失败暂时无策略

					}
				}
				//当有节点变更事件触发时，此方法返回，否则此方法一直阻塞
				watchFileNode(ctx)

			} else {
				//zk连接不上，先等待连接
				time.Sleep(consts.UnreliableZkRetryGap)
			}

		} else {
			break
		}
	}

}

//创建实例临时节点
func CreateOrUpdateInstNode(ctx *context.RoutineContext) *context.OutputContext {
	glog.Infof("[Rtn%d]开始创建Agent实例临时节点:%s.", ctx.RoutineId, ctx.FileContext.InstanceZkPath)
	success := zkMgr.CreateOrUpdateEphemeralNode(ctx.FileContext.InstanceZkPath, ctx.FileContext.Data)
	if success {
		glog.Infof("[Rtn%d]Agent实例临时节点创建成功:%s.\n", ctx.RoutineId, ctx.FileContext.InstanceZkPath)
		return context.NewSuccessOutputContext(nil)
	} else {
		glog.Errorf("[Rtn%d]Agent实例临时节点创建失败:%s.\n", ctx.RoutineId, ctx.FileContext.InstanceZkPath)
		return context.NewFailOutputContext("Agent实例临时节点创建失败")
	}

}

//监听配置文件zk节点
func watchFileNode(ctx *context.RoutineContext) {
	glog.Infof("[Rtn%d]开始监听配置文件节点:%s.\n", ctx.RoutineId, ctx.FileContext.FileZkPath)
	//监听配置文件的zk节点
	for {
		glog.Flush()
		endlessRetry := retryer.NewEndlessRetryer(consts.UnreliableZkRetryGap)
		output := endlessRetry.DoRetry(Watch, ctx)
		if watcher, ok := output.Result.(zkMgr.NodeWatcher); ok {
			event := <-watcher
			glog.Infof("[Rtn%d]监听到节点%s发生[%s]事件.", ctx.RoutineId, ctx.FileContext.FileZkPath, event.Type.String())
			if zkMgr.IsDataChanged(event) {
				glog.Infof("[Rtn%d]配置文件节点发生变化:%s,准备重新下载.", ctx.RoutineId, ctx.FileContext.FileZkPath)
				break
			}
		} else {
			glog.Errorf("[Rtn%d]监听配置文件接口返回值类型异常", ctx.RoutineId)
			time.Sleep(consts.UnreliableZkRetryGap)
		}
	}
}
func Watch(ctx *context.RoutineContext) *context.OutputContext {
	watcher, success := zkMgr.GetNodeWatcher(ctx.FileContext.FileZkPath) //需要处理zk异常
	if !success {
		errMsg := "监听配置文件节点:" + ctx.FileContext.FileZkPath + "失败"
		return context.NewFailOutputContext(errMsg)
	} else {
		return context.NewSuccessOutputContext(watcher)
	}
}

//下载配置文件并保存
func downloadAndSave(ctx *context.RoutineContext) ([]byte, bool) {
	glog.Infof("[Rtn%d]开始下载配置文件:%s", ctx.RoutineId, ctx.FileContext.FileName) //下载
	data, success := httpclient.DownloadFromServer(ctx.FileContext.Url)
	if success {
		glog.Infof("[Rtn%d]下载配置文件%s成功.", ctx.RoutineId, ctx.FileContext.FileName)
		glog.Infof("[Rtn%d]配置文件%s内容为:\n%s\n", ctx.RoutineId, ctx.FileContext.FileName, string(data))
		glog.Infof("[Rtn%d]开始保存配置文件%s.", ctx.RoutineId, ctx.FileContext.FileName)
		fileutils.WriteFile(ctx.FileContext.Path, data)
		glog.Infof("[Rtn%d]配置文件已保存到:%s.", ctx.RoutineId, ctx.FileContext.Path)
	} else {
		glog.Infof("[Rtn%d]下载配置文件%s失败!", ctx.RoutineId)
		return nil, false //下载失败返回一个空的字符串
	}
	return data, success

}

//校验配置文件节点是否存在
func checkFileNode(ctx *context.RoutineContext) (bool, error) {
	glog.Infof("[Rtn%d]开始校验Zk上是否存在配置文件节点:%s.", ctx.RoutineId, ctx.FileContext.FileZkPath)
	var isExists bool
	var err error
	if isExists, err = zkMgr.ExistsNode(ctx.FileContext.FileZkPath); !isExists && err == nil {
		glog.Fatalf("[Rtn%d]配置文件节点:%s,不存在.", ctx.RoutineId, ctx.FileContext.FileZkPath)
		return false, nil
	} else {
		if err != nil {
			glog.Errorf("[Rtn%d]校验Zk上是否存在配置文件节点出现异常.", ctx.RoutineId)
			return false, err
		} else {
			glog.Infof("[Rtn%d]校验成功,Zk上存在配置文件节点:%s.", ctx.RoutineId, ctx.FileContext.FileZkPath)
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
