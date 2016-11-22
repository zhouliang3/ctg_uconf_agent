package main

import (
	"flag"
	"os"
	"time"

	"path/filepath"
	"strings"

	"ctg.com/uconf/agent/app"
	"ctg.com/uconf/agent/consts"
	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/fileutils"
	"ctg.com/uconf/agent/host"
	"ctg.com/uconf/agent/httpclient"
	"ctg.com/uconf/agent/retryer"
	"github.com/golang/glog"
)

var autoReload bool

var MainRoutineContext *context.RoutineContext
var zooAction, fileAction, appRootAction, cfglistAction string
var retryTimes int
var retryInterval int64

func main() {
	flag.Parse()
	defer glog.Flush()
	MainRoutineContext = context.InitMainRoutineContext()
	//定时flush日志
	flushLog()

	//解析配置agent文件
	agentConfig := fileutils.Read()
	retryTimes = agentConfig.Server.Retry.Times
	retryInterval = agentConfig.Server.Retry.Interval
	retryer.InitHttpRequestRetryer(retryTimes, time.Duration(retryInterval)*time.Millisecond)
	autoReload = agentConfig.Enabled
	//获取RESTful API的url地址
	zooAction, fileAction, appRootAction, cfglistAction = agentConfig.Server.ServerActionAddress()
	//获取机器信息
	machine := host.Info(agentConfig.Server.Ip + ":" + agentConfig.Server.Port)
	//初始化zookeeper
	appInstance := app.NewInstance(machine.Ip, machine.HoseName, consts.AppRootDir)

	//根据命令行传入的key获取应用的详细信息
	loadAppInfoFromEnv(appInstance)

	//根据应用的详细信息下载应用配置
	appFilelistLoad(appInstance)

}

//根据Agent命令行参数中的key获取app的[code,tenant,version]信息
func loadAppInfoFromEnv(appInstance *app.Instance) {
	//根据app.key获取[code,tenant,version]
	glog.Info("从环境变量中获取app的[code,tenant,version,env]")
	appEnv := os.Getenv("UCONF_AGENT_APP")
	envs := strings.Split(appEnv, "|")
	//校验环境变量的正确性
	if len(envs) < 4 {
		glog.Error("环境变量UCONF_AGENT_APP配置的 租户|应用|版本|环境，无效。环境变量UCONF_AGENT_APP=" + appEnv)
		panic("环境变量UCONF_AGENT_APP配置的 租户|应用|版本|环境，无效。环境变量UCONF_AGENT_APP=" + appEnv)
	}
	appInstance.Tenant = envs[0]
	appInstance.AppCode = envs[1]
	appInstance.Version = envs[2]
	appInstance.Env = envs[3]
	glog.Infof("获取成功,App信息为[code=%s,tenant=%s,version=%s,env=%s].", appInstance.AppCode, appInstance.Tenant, appInstance.Version, appInstance.Env)
	return
}

//调用配置中心接口，下载应用下的所有已发布的配置文件
func appFilelistLoad(appInstance *app.Instance) {
	listUrl := filelistLoadUrl(cfglistAction, appInstance)
	glog.Infof("准备发送Http请求获取应用[%s]的所有配置文件,请求的Http接口:%s", appInstance.AppCode, listUrl)
	//下载所有的配置
	conflistContext := context.NewRequestRoutineContext(listUrl, nil)
	listOutut := httpclient.RetryableGetFileList(conflistContext)
	if listOutut.Err != nil {
		glog.Error("查询配置信息出现异常!")
		panic("查询配置信息出现异常!")
	}
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
					if len(configPath) > 0 {
						configPath = configPath + string(filepath.Separator)
					} else {
						configPath = ""
					}
					filepath := appInstance.Dir + string(filepath.Separator) + configPath + fileName
					//保存配置文件到/apps/uconf目录中
					data := []byte(fileValue)
					save(filepath, fileName, data)
				}
			} else {
				glog.Warning("应用[%s]未查询到配置文件信息!", appInstance.AppCode)
			}
		} else {
			glog.Error("查询配置信息返回失败!")
			glog.Errorf("返回的详细内容为:%v", cfglist)
			panic("查询配置信息返回失败!")
		}
	} else {
		glog.Error("查询配置信息出现异常!")
		panic("查询配置信息出现异常!")
	}
}

func fileDownloadUrl(serverAddr string, appInstance *app.Instance, filename string) string {
	return serverAddr + "?version=" + appInstance.Version + "&app=" + appInstance.AppCode + "&key=" + filename + "&tenant=" + appInstance.Tenant + "&type=file" + "&env=" + appInstance.Env
}

func filelistLoadUrl(serverAddr string, appInstance *app.Instance) string {
	return serverAddr + "?configType=file&version=" + appInstance.Version + "&app=" + appInstance.AppCode + "&tenant=" + appInstance.Tenant + "&env=" + appInstance.Env
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
