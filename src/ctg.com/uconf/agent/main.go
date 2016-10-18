package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	//	"sync"
	"sync/atomic"
	"time"

	"ctg.com/uconf/agent/app"
	"ctg.com/uconf/agent/consts"
	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/fileutils"
	"ctg.com/uconf/agent/host"
	"ctg.com/uconf/agent/httpclient"
	"ctg.com/uconf/agent/retryer"
	"ctg.com/uconf/agent/zkMgr"
	"github.com/golang/glog"
)

var autoReload bool

// 1 表示连接上了 0 表示没连上
var zkState int32
var MainRoutineContext *context.RoutineContext
var zooAction, fileAction, appAction, cfglistAction string
var prefix string

//任务队列
var jobs chan task = make(chan task, 20)

func main() {
	//agent -path=ab -appId=cd -address=ee
	var routpath = flag.String("path", "", "托管应用的绝对路径")
	var iappId = flag.String("appId", "", "托管应用编码")
	var iaddress = flag.String("address", "", "配置中心上下文地址，形如:10.142.90.23:8082/uconf-web")
	flag.Parse()
	defer glog.Flush()
	glog.Info("命令行参数为:", os.Args)
	MainRoutineContext = context.InitMainRoutineContext()
	//定时flush日志
	flushLog()
	//解析配置agent文件
	agentConfig := fileutils.Read()
	autoReload = agentConfig.Enabled
	//获取RESTful API的url地址
	zooAction, fileAction, appAction, cfglistAction = agentConfig.Server.ServerActionAddress()
	//获取机器信息
	machine := host.Info(agentConfig.Server.Ip + ":" + agentConfig.Server.Port)
	//初始化zookeeper
	if autoReload { //需要连接zk,那么就必须等获取到zk服务器信息之后才继续下面的动作
		var servers []string
		servers, prefix = zkMgr.ZooInfo(zooAction)
		//TODO 如果建立连接失败暂时不进一步处理
		zkMgr.InitZk(MainRoutineContext, servers, ZkSateCallBack)
	}
	var stopSignal chan int = make(chan int, 1)
	//开启工作线程
	startupWorkers()
	//根据app.Id获取app详细信息,先构造一个重试器，无限重试
	//	appRetryer := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
	//var latch sync.WaitGroup
	for _, appCfg := range agentConfig.Apps {
		//latch.Add(1)
		taski := task{machine, appCfg}
		jobs <- taski
		//go worker(machine, appCfg, latch)
	}
	//等待获取应用信息完成
	//latch.Wait()
	<-stopSignal
}
func startupWorkers() {
	for i := 0; i < consts.MaxRoutineNums; i++ {
		go worker()
	}

}
func worker() {
	taski := <-jobs
	//构造app实例
	appInstance := app.NewInstance(taski.machine.Ip, taski.machine.HoseName, taski.appCfg.Dir)
	loadAppInfoByKey(&taski.appCfg, appInstance)

	//需要根据app的信息从配置中心获取此app所有的配置文件列表，然后，依次处理这些文件，并监听配置文件列表的变化
	appFilelistLoad(appInstance)
	//latch.Done()
}

type task struct {
	machine *host.Machine
	appCfg  fileutils.App
}

//根据Agent配置文件uconf.yml中配置的app key获取app的[name,tenant,version,env]信息
func loadAppInfoByKey(aInfo *fileutils.App, appInstance *app.Instance) {
	appRetryer := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
	//根据app.Id获取[name,tenant,version,env]
	glog.Info("开始根据app key,获取app的[name,tenant,version,env]")
	url := appAction + "?appKey=" + aInfo.Key
	requestContext := context.NewRequestRoutineContext(url, nil)
	output := appRetryer.DoRetry(httpclient.RetryableGetJsonData, requestContext)
	if data, ok := output.Result.(map[string]interface{}); ok {
		if success, ok := data["success"].(string); ok {
			if success == "true" {
				if result, ok := data["result"].(map[string]interface{}); ok {
					appInstance.AppName = result["appCode"].(string)
					appInstance.Tenant = result["tenantCode"].(string)
					appInstance.Env = result["envCode"].(string)
					appInstance.Version = result["appVersion"].(string)
					glog.Infof("获取成功,App信息为[name=%s,tenant=%s,version=%s,env=%s].", appInstance.AppName, appInstance.Tenant, appInstance.Env, appInstance.Version)
					return
				} else {
					glog.Error("解析返回的result为map类型失败")
				}
			} else {
				glog.Errorf("根据appKey:%s获取[name,tenant,version,env]失败.", aInfo.Key)
			}
		} else {
			glog.Error("解析返回的success字符为String类型失败")
		}
	} else {
		glog.Error("解析返回的字符为map类型失败")
	}

}

//调用配置中心接口，下载应用下的所有已发布的配置文件
func appFilelistLoad(appInstance *app.Instance) {
	listUrl := filelistLoadUrl(cfglistAction, appInstance)
	glog.Infof("准备发送Http请求获取应用[%s]的所有配置文件,请求的Http接口:%s", appInstance.AppName, listUrl)
	//下载所有的配置
	cfglistRetryer := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
	conflistContext := context.NewRequestRoutineContext(listUrl, nil)
	listOutut := cfglistRetryer.DoRetry(httpclient.RetryableGetFileList, conflistContext)

	if cfglist, ok := listOutut.Result.(httpclient.CfgListRespose); ok {
		if "true" == cfglist.Success {
			if len(cfglist.Result) > 0 {
				for _, cfg := range cfglist.Result {
					if "file" != cfg.ConfigType {
						continue
					}
					fileName := cfg.ConfigName
					fileValue := cfg.ConfigValue
					filepath := appInstance.Dir + string(filepath.Separator) + fileName
					//需要开始监听每个配置文件的变化
					//获取监听上下文
					fileZkPath, instanceZkPath := appInstanceNode(prefix, fileName, appInstance)
					url := fileDownloadUrl(fileAction, appInstance, fileName)
					ctx := context.NewRoutineContext(fileName, url, filepath, fileZkPath, instanceZkPath)
					ctx.FileContext.Data = []byte(fileValue)
					go dealFileItem(ctx)

				}
			} else {
				glog.Warning("应用[%s]未查询到配置文件信息!", appInstance.AppName)
			}
		} else {
			glog.Error("查询配置信息返回失败!")
			glog.Errorf("返回的详细内容为:%v", cfglist)

		}
	} else {
		glog.Error("查询配置信息返回的数据类型错误!")
	}
}

func ZkSateCallBack(isConnected bool) {
	if isConnected {
		atomic.StoreInt32(&zkState, 1)
	} else {
		atomic.StoreInt32(&zkState, 0)
	}
}
func fileDownloadUrl(serverAddr string, appInstance *app.Instance, filename string) string {
	return serverAddr + "?version=" + appInstance.Version + "&app=" + appInstance.AppName + "&env=" + appInstance.Env + "&key=" + filename + "&tenant=" + appInstance.Tenant + "&type=file"
}

func filelistLoadUrl(serverAddr string, appInstance *app.Instance) string {
	return serverAddr + "?configType=file&version=" + appInstance.Version + "&app=" + appInstance.AppName + "&env=" + appInstance.Env + "&tenant=" + appInstance.Tenant
}

//处理配置文件，包含：下载配置文件，保存配置文件到指定目录，监听配置文件节点，创建Agent实例临时节点
func dealFileItem(ctx *context.RoutineContext) {
	defer glog.Flush()
	for {
		var data []byte
		var success bool = false
		if ctx.FileContext.Data == nil || len(ctx.FileContext.Data) < 1 { //判断是否需要下载
			data, success = download(ctx)
		} else {
			success = true
			data = ctx.FileContext.Data
		}
		save(ctx, data)
		if !autoReload {
			return
		}
		//先判断配置文件zk节点是否存在，或者zk服务器异常连不上
		if atomic.LoadInt32(&zkState) == 1 { //zk连接是否正常，下面代码执行期间也可能出现zk断开的问题
			exists, err := checkFileNode(ctx)
			if err != nil {
				break
			}
			if !exists {
				break
			}
			if success {
				zkRetryer := retryer.ZkRequestRetryer()
				output := zkRetryer.DoRetry(CreateOrUpdateInstNode, ctx)
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

	}

}

//创建实例临时节点
func CreateOrUpdateInstNode(ctx *context.RoutineContext) *context.OutputContext {
	defer func() {
		ctx.FileContext.Data = nil
	}()
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
func download(ctx *context.RoutineContext) ([]byte, bool) {
	glog.Infof("[Rtn%d]开始下载配置文件:%s", ctx.RoutineId, ctx.FileContext.FileName) //下载
	data, success := httpclient.DownloadFromServer(ctx.FileContext.Url)
	if success {
		glog.Infof("[Rtn%d]下载配置文件%s成功.", ctx.RoutineId, ctx.FileContext.FileName)
	} else {
		glog.Infof("[Rtn%d]下载配置文件%s失败!", ctx.RoutineId)
		return nil, false //下载失败返回一个空的字符串
	}
	return data, success

}
func save(ctx *context.RoutineContext, data []byte) {
	glog.Infof("[Rtn%d]配置文件%s内容为:\n%s\n", ctx.RoutineId, ctx.FileContext.FileName, string(data))
	glog.Infof("[Rtn%d]开始保存配置文件%s.", ctx.RoutineId, ctx.FileContext.FileName)
	fileutils.WriteFile(ctx.FileContext.Path, data)
	glog.Infof("[Rtn%d]配置文件已保存到:%s.", ctx.RoutineId, ctx.FileContext.Path)
}

//校验配置文件节点是否存在
func checkFileNode(ctx *context.RoutineContext) (bool, error) {
	glog.Infof("[Rtn%d]开始校验Zk上是否存在配置文件节点:%s.", ctx.RoutineId, ctx.FileContext.FileZkPath)

	zkRetryer := retryer.ZkRequestRetryer()
	output := zkRetryer.DoRetry(NodeExists, ctx)
	if exists, ok := output.Result.(bool); ok {
		return exists, output.Err
	} else { //这种情形不会存在
		glog.Errorf("[Rtn%d]调用Zk节点是否存在接口返回值类型异常", ctx.RoutineId)
		return false, output.Err
	}
}
func NodeExists(ctx *context.RoutineContext) *context.OutputContext {
	if isExists, err := zkMgr.ExistsNode(ctx.FileContext.FileZkPath); !isExists && err == nil {
		glog.Errorf("[Rtn%d]配置文件节点:%s,不存在.", ctx.RoutineId, ctx.FileContext.FileZkPath)
		return context.NewSuccessOutputContext(false) //配置文件不存在,说明调用成功，返回false，err为nil
	} else {
		if err != nil {
			glog.Errorf("[Rtn%d]校验Zk上是否存在配置文件节点出现异常.", ctx.RoutineId)
			return context.NewErrorOutputContext(err)
		} else {
			glog.Infof("[Rtn%d]校验成功,Zk上存在配置文件节点:%s.", ctx.RoutineId, ctx.FileContext.FileZkPath)
			return context.NewSuccessOutputContext(true)
		}
	}
}

// 获得实例保存到zk上的路径
func appInstanceNode(prefix, filename string, app *app.Instance) (string, string) {
	fileZkPath := prefix + "/" + app.AppNodePath() + "/file/" + filename
	instanceZkPath := fileZkPath + "/" + app.InstanceNodePath()
	return fileZkPath, instanceZkPath
}

//定时将日志刷到文件中
func flushLog() {
	ticker := time.NewTicker(consts.LogFlushGap)
	go func() {
		for _ = range ticker.C {
			//t.Year()
			glog.Flush()
		}
	}()
}
