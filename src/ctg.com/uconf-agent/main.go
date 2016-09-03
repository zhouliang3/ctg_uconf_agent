package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"ctg.com/uconf-agent/app"
	"ctg.com/uconf-agent/fileutils"
	"ctg.com/uconf-agent/host"
	"ctg.com/uconf-agent/httpclient"
	"ctg.com/uconf-agent/zkMgr"
	"github.com/golang/glog"
)

var autoReload bool

func main() {
	flag.Parse()
	defer glog.Flush()
	//定时flush日志
	flushLog()
	//解析配置agent文件
	agentConfig := fileutils.Read()
	autoReload = agentConfig.Enabled
	//调用RESTful API获取zk配置
	zooAction, fileAction := agentConfig.Server.ServerActionAddress()
	zooHostsUrl := zooAction + "/" + "hosts"
	zooPrefixUrl := zooAction + "/" + "prefix"
	hostsResponseMap := httpclient.GetValueFromServer(zooHostsUrl)
	hosts := hostsResponseMap["value"].(string)
	glog.Info("zk服务器地址列表:", hosts)
	prefixResponseMap := httpclient.GetValueFromServer(zooPrefixUrl)
	prefix := prefixResponseMap["value"].(string)
	glog.Info("zk根路径:", prefix)
	//获取机器信息
	machine := host.Info(agentConfig.Server.Ip + ":" + agentConfig.Server.Port)
	//初始化zookeeper
	servers := strings.Split(hosts, ",")
	if autoReload {
		if isConnected := zkMgr.InitZk(servers); isConnected {
			zkMgr.CreateNode(prefix, []byte(machine.Ip))
		}
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
			go dealFileItem(url, path, fileZkPath, instanceZkPath)
		}
	}

	<-stopSignal
}

func fileDownloadUrl(serverAddr string, appIdentity *app.Identity, conf *fileutils.AppConfig) string {
	return serverAddr + "?version=" + appIdentity.Version + "&app=" + appIdentity.AppName + "&env=" + appIdentity.Env + "&key=" + conf.Name + "&tenant=" + appIdentity.Tenant + "&type=file"
}

//处理配置文件，包含：下载配置文件，保存配置文件到指定目录，监听配置文件节点，创建Agent实例临时节点
func dealFileItem(url, path, fileZkPath, instanceZkPath string) {
	defer glog.Flush()

	for {
		data := downloadAndSave(url, path, fileZkPath, instanceZkPath)
		if autoReload { //配置更新是否需要重新下载
			//先判断配置文件zk节点是否存在，不存在则抛出恐慌
			checkFileNode(fileZkPath)
			createOrUpdateInstNode(instanceZkPath, data)
			watchFileNode(fileZkPath, instanceZkPath, data)
		} else {
			break
		}
	}

}

//创建实例临时节点
func createOrUpdateInstNode(instanceZkPath string, data []byte) {
	glog.Infof("开始创建Agent实例临时节点:%s", instanceZkPath)
	zkMgr.CreateOrUpdateEphemeralNode(instanceZkPath, data)
	glog.Infof("Agent实例临时节点创建成功:%s.\n", instanceZkPath)
}

//监听配置文件zk节点
func watchFileNode(fileZkPath, instanceZkPath string, data []byte) {
	glog.Infof("开始监听配置文件节点:%s.\n", fileZkPath)
	//监听配置文件的zk节点
	for {
		glog.Flush()
		watcher := zkMgr.GetNodeWatcher(fileZkPath)
		event := <-watcher
		glog.Infof("监听到节点%s发生[%s]事件", fileZkPath, event.Type.String())
		if zkMgr.IsDataChanged(event) {
			glog.Infof("配置文件节点发生变化:%s,准备重新下载.", fileZkPath)
			break
		}
	}
}

//下载配置文件并保存
func downloadAndSave(url, path, fileZkPath, instanceZkPath string) []byte {
	glog.Info("-----------------开始下载配置-----------------") //下载
	fmt.Println(url)
	data := httpclient.DownloadFromServer(url)
	glog.Info("下载配置成功，开始保存文件")
	glog.Infof("配置文件内容为:\n%s\n", string(data))
	fileutils.WriteFile(path, data)
	glog.Infof("配置文件已保存到:%s.", path)

	return data
}

//校验配置文件节点是否存在，不存在则抛出恐慌
func checkFileNode(fileZkPath string) {
	glog.Info("开始校验Zk上是否有配置信息.")
	if !zkMgr.ExistsNode(fileZkPath) {
		glog.Fatalf("配置文件节点:%s,不存在", fileZkPath)
		panic("配置文件节点:" + fileZkPath + "不存在.")
	}
	glog.Info("开始校验Zk上有配置信息成功.")
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
