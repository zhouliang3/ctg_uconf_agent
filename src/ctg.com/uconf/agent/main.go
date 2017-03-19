package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"bufio"
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
	"github.com/magiconair/properties"
)

var autoReload bool

var MainRoutineContext *context.RoutineContext
var zooAction, fileAction, appRootAction, cfglistAction string
var retryTimes int
var retryInterval int64

var cmdserver = flag.String("server", "", "服务器地址")
var cmdport = flag.String("port", "", "服务器端口")
var cmdcontext = flag.String("context", "", "服务端上下文")
var cmdapp = flag.String("app", "", "应用编码")
var cmdtenant = flag.String("tenant", "", "租户编码")
var cmdversion = flag.String("version", "", "版本信息")
var cmdkeys = flag.String("keys", "", "待替换的键值")
var keys []string
var cmdFilepath = flag.String("filepath", "", "待替换的properties文件绝对路径")
var op string
var cmdCmd = flag.String("cmd", "", "命令")
var changed = false
var p *properties.Properties

//window测试命令：e: & cd E:\GitHub\ctg_uconf_agent\src\ctg.com\uconf & agent.exe &echo success download config & c: & cd C:\Home\Programs Files\apache-tomcat-6.0.43\apache-tomcat-6.0.43\bin & startup.bat
//> agent.exe -replace=true -server=10.142.90.23 -port=9090  -context=uconf-web -app=uconf_demo -tenant=ctg -version=1_0_0_0 -keys=password,server,acbd,url -filepath=E:\\GitHub\\ctg_uconf_agent\\src\\ctg.com\\uconf\\agent\\myserver.properties

func main() {
	flag.Parse()
	defer glog.Flush()
	fmt.Print(*cmdCmd)
	if "replace" == strings.TrimSpace(*cmdCmd) {
		fmt.Println("true--")
		if len(*cmdkeys) < 1 {
			return
		}
		if _, err := os.Stat(*cmdFilepath); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("-filepath 配置的文件绝对路径不存在。")
				os.Exit(1)
			} else {
				panic(err)
			}
		} else {
			p = properties.MustLoadFile(*cmdFilepath, properties.UTF8)
		}
		keys = strings.Split(*cmdkeys, ",")
	}
	MainRoutineContext = context.InitMainRoutineContext()
	//定时flush日志
	flushLog()

	//解析配置agent文件
	agentConfig := fileutils.Read(*cmdserver, *cmdport, *cmdcontext)
	retryTimes = agentConfig.Server.Retry.Times
	retryInterval = agentConfig.Server.Retry.Interval
	retryer.InitHttpRequestRetryer(retryTimes, time.Duration(retryInterval)*time.Millisecond)
	autoReload = agentConfig.Enabled

	//获取RESTful API的url地址
	zooAction, fileAction, cfglistAction = agentConfig.Server.ServerActionAddress()
	//获取机器信息
	machine := host.Info(agentConfig.Server.Ip + ":" + agentConfig.Server.Port)
	//初始化zookeeper
	appInstance := app.NewInstance(machine.Ip, machine.HoseName)
	//根据命令行传入的key获取应用的详细信息,fz|uconf_demo|1_0_0_0|1
	loadAppInfoFromEnv(appInstance)
	if "replace" == strings.TrimSpace(*cmdCmd) {
		appUpdateItems(appInstance)
		if changed {
			os.Remove(*cmdFilepath)
			f, _ := os.Create(*cmdFilepath)
			bufwriter := bufio.NewWriter(f)
			p.Write(bufwriter, properties.UTF8)
			bufwriter.Flush()
		}
	} else {
		//根据应用的详细信息下载应用配置
		appFilelistLoad(appInstance)
	}

}

//根据Agent命令行参数中的key获取app的[code,tenant,version]信息
func loadAppInfoFromEnv(appInstance *app.Instance) {
	//先获取命令行的[code,tenant,version]参数
	appCode := strings.TrimSpace(*cmdapp)
	tenantCode := strings.TrimSpace(*cmdtenant)
	appVersion := strings.TrimSpace(*cmdversion)
	//根据app.key获取[code,tenant,version]

	glog.Info("从环境变量中获取app的[code,tenant,version,env]")

	//校验环境变量的正确性
	if len(tenantCode) <= 0 {
		tenantCode = strings.TrimSpace(os.Getenv("TENANT_CODE"))
		if len(tenantCode) <= 0 {
			glog.Error("启动失败，请先配置环境变量TENANT_CODE")
			panic("启动失败，请先配置环境变量TENANT_CODE")
		}
	}
	if len(appCode) <= 0 {
		appCode = strings.TrimSpace(os.Getenv("SERVICE_CODE"))
		if len(appCode) <= 0 {
			glog.Error("启动失败，请配置命令行参数-app的值或者配置环境变量SERVICE_CODE")
			panic("启动失败，请配置命令行参数-app的值或者配置环境变量SERVICE_CODE")
		}
	}
	if len(appVersion) <= 0 {
		appVersion = strings.TrimSpace(os.Getenv("SERVICE_VER"))
		if len(appVersion) <= 0 {
			glog.Error("启动失败，请先配置环境变量SERVICE_VER")
			panic("启动失败，请先配置环境变量SERVICE_VER")
		}
	}

	appInstance.Tenant = tenantCode
	appInstance.AppCode = appCode
	appInstance.Version = appVersion
	glog.Infof("获取成功,App信息为[code=%s,tenant=%s,version=%s,env=%s].", appInstance.AppCode, appInstance.Tenant, appInstance.Version, appInstance.Env)
	return
}

//调用配置中心接口，下载应用下的所有已发布的配置文件
func appUpdateItems(appInstance *app.Instance) {
	listUrl := itemlistLoadUrl(cfglistAction, appInstance)
	glog.Infof("准备发送Http请求获取应用[%s]的所有配置文件,请求的Http接口:%s", appInstance.AppCode, listUrl)
	//下载所有的配置
	conflistContext := context.NewRequestRoutineContext(listUrl, nil)
	listOutut := httpclient.RetryableGetFileList(conflistContext)
	fmt.Println(listOutut)
	if listOutut.Err != nil {
		glog.Error("查询配置信息出现异常!")
		panic("查询配置信息出现异常!")
	}
	if cfglist, ok := listOutut.Result.(httpclient.CfgListRespose); ok {
		if "true" == cfglist.Success {
			if len(cfglist.Result) > 0 {
				for _, cfg := range cfglist.Result {
					if "item" != cfg.ConfigType {
						continue
					}
					itemName := cfg.ConfigName
					itemValue := cfg.ConfigValue
					if hasKey(itemName) {
						p.Set(itemName, itemValue)
						changed = true
					}
				}
			} else {
				glog.Warningf("应用[%s]未查询到配置文件信息!", appInstance.AppCode)
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
func hasKey(itemName string) bool {
	for _, v := range keys {
		if strings.TrimSpace(itemName) == strings.TrimSpace(v) {
			return true
		}
	}
	return false
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

						configPath = fileutils.GetExecRootPath()
					}
					filepath := configPath + fileName
					//保存配置文件到/apps/uconf目录中
					data := []byte(fileValue)
					save(filepath, fileName, data)
				}
			} else {
				glog.Warningf("应用[%s]未查询到配置文件信息!", appInstance.AppCode)
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

func itemlistLoadUrl(serverAddr string, appInstance *app.Instance) string {
	return serverAddr + "?configType=item&version=" + appInstance.Version + "&app=" + appInstance.AppCode + "&tenant=" + appInstance.Tenant + "&env=" + appInstance.Env
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
