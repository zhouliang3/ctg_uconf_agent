package main

import (
	"flag"
	"os"
	"strconv"
	"time"

	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"ctg.com/uconf/agent/app"
	"ctg.com/uconf/agent/consts"
	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/fileutils"
	"ctg.com/uconf/agent/host"
	"ctg.com/uconf/agent/httpclient"
	"ctg.com/uconf/agent/retryer"
	"ctg.com/uconf/agent/strutils"
	"github.com/golang/glog"
)

var autoReload bool

// 1 表示连接上了 0 表示没连上
var zkState int32
var MainRoutineContext *context.RoutineContext
var zooAction, fileAction, appAction, cfglistAction string
var cmd, tag, rootpath, configFile, flagAnd string = "", "", "", "", ""
var prefix string
var Arootpath = flag.String("path", "", "托管应用的绝对路径") //"E:/work/maven/repository/com/ctgae/alogic/alogic-demo-web/0.0.1-SNAPSHOT/t/alogic-demo-web-0.0.1-SNAPSHOT.war"
var AappKey = flag.Int64("key", -1, "托管应用编码")        //int64(2000)
func main() {
	flag.Parse()
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
	appInstance := app.NewInstance(machine.Ip, machine.HoseName, nil)

	//根据命令行传入的key获取应用的详细信息
	loadAppInfoFromEnv(appKey, appInstance)
	//获取应用的根目录的绝对路径
	loadAppRootPath(appInstance)
	//根据应用的详细信息下载应用
	appFilelistLoad(appInstance)
}

//根据Agent命令行参数中的key获取app的[name,tenant,version]信息
func loadAppInfoFromEnv(key int64, appInstance *app.Instance) {
	appRetryer := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
	//根据app.key获取[name,tenant,version]
	glog.Info("从环境变量中获取app的[name,tenant,version,env]")
	appEnv := os.Getenv("UCONF_AGENT_APP")
	envs := strings.Split(appEnv, "|")
	//校验环境变量的正确性
	if len(envs) < 4 {
		glog.Error("环境变量UCONF_AGENT_APP配置的 租户|应用|版本|环境，无效。环境变量UCONF_AGENT_APP=" + appEnv)
		panic("环境变量UCONF_AGENT_APP配置的 租户|应用|版本|环境，无效。环境变量UCONF_AGENT_APP=" + appEnv)
	}
	appInstance.Tenant = envs[0]
	appInstance.AppName = envs[1]
	appInstance.Version = envs[2]
	appInstance.Env = envs[3]
	glog.Infof("获取成功,App信息为[name=%s,tenant=%s,version=%s,env=%s].", appInstance.AppName, appInstance.Tenant, appInstance.Version, appInstance.Env)
	return

}
func loadAppRootPath(appInstance *app.Instance) {
	appRetryer := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
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
					configPath := cfg.ConfigPath
					//TODO 这里需要改变
					if strings.HasPrefix(configPath, "/") { //相对于war包的绝对路径

					} else { //相对于classpath的路径
						configPath = "WEB-INF" + string(filepath.Separator) + "classes" + string(filepath.Separator) + configPath
					}
					filepath := appInstance.Dir + string(filepath.Separator) + configPath + string(filepath.Separator) + fileName
					//保存配置文件到临时目录中
					data := []byte(fileValue)
					save(filepath, fileName, data)
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

func fileDownloadUrl(serverAddr string, appInstance *app.Instance, filename string) string {
	return serverAddr + "?version=" + appInstance.Version + "&app=" + appInstance.AppName + "&key=" + filename + "&tenant=" + appInstance.Tenant + "&type=file" + "&env=" + appInstance.Env
}

func filelistLoadUrl(serverAddr string, appInstance *app.Instance) string {
	return serverAddr + "?configType=file&version=" + appInstance.Version + "&app=" + appInstance.AppName + "&tenant=" + appInstance.Tenant + "&env=" + appInstance.Env
}

func save(filepath, filename string, data []byte) {
	glog.Infof("配置文件%s内容为:\n%s\n", filename, string(data))
	glog.Infof("开始保存配置文件%s.", filename)
	fileutils.WriteFile(filepath, data)
	glog.Infof("配置文件已保存到:%s.", filepath)
}

//定时将日志刷到文件中
func flushLog() {
	ticker := time.NewTicker(consts.LogFlushGap)
	go func() {
		for _ = range ticker.C {
			glog.Flush()
		}
	}()
}
