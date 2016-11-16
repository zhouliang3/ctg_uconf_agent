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
	//agent.exe -path=E:/work/maven/repository/com/ctgae/alogic/alogic-demo-web/0.0.1-SNAPSHOT/t/alogic-demo-web-0.0.1-SNAPSHOT.war -key=2000
	//./agent -path=/root/work/src/ctg.com/t/alogic-demo-web-0.0.1-SNAPSHOT.war -key=2000
	flag.Parse()
	defer glog.Flush()
	if "linux" == runtime.GOOS {
		cmd = "/bin/sh"
		tag = "-c"
		flagAnd = " && "
	} else if "windows" == runtime.GOOS { //windows系统
		cmd = "cmd"
		tag = "/C"
		flagAnd = " & "
	} else {
		panic("暂时不支持 " + runtime.GOOS + " 操作系统!")
	}
	//agent -path=ab -key=cd -address=ee
	//
	glog.Info("命令行参数为:", os.Args)

	rootpath := *Arootpath
	appKey := *AappKey
	var ext = filepath.Ext(rootpath)
	if ext != ".jar" && ext != ".war" {
		panic("文件类型必须为jar或者war")
	}
	//先将jar或者war拷贝到临时目录下

	dir, filename := filepath.Split(rootpath)
	idx := strings.LastIndex(rootpath, ":")
	tmpDir := dir + "agentTmp"
	tmpFile := tmpDir + string(filepath.Separator) + filename
	glog.Infof("将应用程序文件:%s,拷贝到临时目录下:%s", rootpath, tmpFile)
	fileutils.CopyFile(tmpFile, rootpath)
	disk := ""
	if idx > 0 {
		//windows下的绝对路径，需要先切换盘符
		disk = strutils.Substring(rootpath, 0, idx+1) + flagAnd
	}

	//先解压,将jar或者war包解压到临时目录
	cmdline := disk + "cd " + tmpDir + flagAnd + " jar -xvf  " + filename
	c := exec.Command(cmd, tag, cmdline)
	glog.Infof("开始执行命令:%s\n", c.Args)

	unzipOut, err1 := c.Output()
	c.Wait()
	if err1 != nil {
		glog.Errorf("压缩为jar/war包出现异常:%s,输出结果:%s", err1, string(unzipOut))
		panic(err1)
	} else {
		glog.Infof("开始解压缩%s\n", tmpFile)
		glog.Info(string(unzipOut))
		glog.Infof("结束解压缩%s\n", tmpFile)

	}
	glog.Infof("删除临时应用程序文件:%s", tmpFile)
	//删除临时war包
	os.Remove(tmpFile)
	//var iaddress = flag.String("address", "", "配置中心上下文地址，形如:10.142.90.23:8082/uconf-web")
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
	appInstance := app.NewInstance(machine.Ip, machine.HoseName, tmpDir)

	//根据命令行传入的key获取应用的详细信息
	loadAppInfoByKey(appKey, appInstance)
	//根据应用的详细信息下载应用
	appFilelistLoad(appInstance)
	glog.Infof("删除原应用程序文件:%s", rootpath)
	os.Remove(rootpath) //删掉源文件

	//压缩为jar或者war
	cmdline = disk + "cd " + tmpDir + flagAnd + " jar -cvf " + filename + " * "
	c = exec.Command(cmd, tag, cmdline)
	//c.Stdout = os.Stdout
	//c.Run()
	//
	glog.Infof("开始执行命令:%s\n", c.Args)

	zipOut, err := c.Output()
	c.Wait()

	if err != nil {
		glog.Error("压缩为jar/war包出现异常", err)
		panic(err)
	} else {
		glog.Infof("开始压缩%s\n", tmpFile)
		glog.Info(string(zipOut))
		glog.Infof("结束压缩%s\n", tmpFile)
	}
	glog.Infof("临时目录中的应用程序文件:%s,拷贝到原目录:%s", tmpFile, rootpath)

	//从临时目录拷贝回去
	fileutils.CopyFile(rootpath, tmpFile)
	glog.Infof("删除临时目录%s", tmpDir)
	//删除临时目录
	os.RemoveAll(tmpDir)
}

//根据Agent命令行参数中的key获取app的[name,tenant,version]信息
func loadAppInfoByKey(key int64, appInstance *app.Instance) {
	appRetryer := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
	//根据app.key获取[name,tenant,version]
	glog.Info("开始根据app key,获取app的[name,tenant,version]")
	url := appAction + "?versionId=" + strconv.FormatInt(key, 10)
	requestContext := context.NewRequestRoutineContext(url, nil)
	output := appRetryer.DoRetry(httpclient.RetryableGetJsonData, requestContext)
	glog.Info(output)
	if data, ok := output.Result.(map[string]interface{}); ok {
		if success, ok := data["success"].(string); ok {
			if success == "true" {
				if result, ok := data["result"].(map[string]interface{}); ok {
					appInstance.AppName = result["appCode"].(string)
					appInstance.Tenant = result["tenantCode"].(string)
					appInstance.Version = result["version"].(string)

					glog.Infof("获取成功,App信息为[name=%s,tenant=%s,version=%s].", appInstance.AppName, appInstance.Tenant, appInstance.Version)
					return
				} else {
					glog.Error("解析返回的result为map类型失败")
				}
			} else {
				glog.Errorf("根据appKey:%d获取[name,tenant,version]失败.", key)
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
					configPath := cfg.ConfigPath

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
	return serverAddr + "?version=" + appInstance.Version + "&app=" + appInstance.AppName + "&key=" + filename + "&tenant=" + appInstance.Tenant + "&type=file" + "&env=rd"
}

func filelistLoadUrl(serverAddr string, appInstance *app.Instance) string {
	return serverAddr + "?configType=file&version=" + appInstance.Version + "&app=" + appInstance.AppName + "&tenant=" + appInstance.Tenant + "&env=rd"
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
