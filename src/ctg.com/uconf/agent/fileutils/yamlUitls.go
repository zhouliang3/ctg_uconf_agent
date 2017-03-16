package fileutils

import (
	"io/ioutil"
	"os"
	"path/filepath"
	s "strings"

	"ctg.com/uconf/agent/consts"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"
)

type RestServer struct {
	Ip      string
	Port    string
	Context string
	Retry   Retryer
}
type Retryer struct {
	Times    int
	Interval int64
}

func (this *RestServer) ServerActionAddress() (string, string, string) {

	if len(this.Context) > 0 {
		if !s.HasPrefix(this.Context, "/") {
			this.Context = "/" + this.Context
		}
	} else {
		this.Context = ""
	}
	url := "http://" + this.Ip + ":" + this.Port + this.Context

	return url + consts.ZooApiPath, url + consts.FileApiPath, url + consts.CfgListpath
}

type AgentConfig struct {
	Enabled bool
	Server  RestServer
}

func Read(cmdserver, cmdport, cmdcontext string) *AgentConfig {
	t := &AgentConfig{}
	data := LoadAgentConfig()
	glog.Infof("开始解析Agent配置文件:%s.", consts.AgentYamlFileName)
	err := yaml.Unmarshal(data, t)
	if err != nil {
		glog.Fatalf("Agent配置文件解析失败: %v", err)
		panic("Agent配置文件解析失败!")
	}
	glog.Info("成功解析Agent配置文件.")
	if len(s.TrimSpace(cmdserver)) > 0 {
		t.Server.Ip = cmdserver
	}
	if len(s.TrimSpace(cmdport)) > 0 {
		t.Server.Port = cmdport
	}
	if len(s.TrimSpace(cmdcontext)) > 0 {
		t.Server.Context = cmdcontext
	}
	checkConfig(t)
	return t
}

//配置校验
func checkConfig(config *AgentConfig) {
	glog.Info("开始校验Agent配置文件.")
	//	if config.Server.Ip == "" {
	//		glog.Fatal("Agent配置文件校验失败,server.ip未配置!")
	//		panic("Agent配置文件校验失败,server.ip未配置!")
	//	}
	//	if config.Server.Port == "" {
	//		glog.Fatal("Agent配置文件校验失败,server.port未配置!")
	//		panic("Agent配置文件校验失败,server.port未配置!")
	//	}
	//	if config.Server.Retry.Times <= 0 {
	//		glog.Fatal("Agent配置文件校验失败,server.retry.times未配置!")
	//		panic("Agent配置文件校验失败,server.retry.times未配置!")
	//	}
	//	if config.Server.Retry.Interval <= 0 {
	//		glog.Fatal("Agent配置文件校验失败,server.retry.interval未配置!")
	//		panic("Agent配置文件校验失败,server.retry.interval未配置!")
	//	}
	glog.Info("校验Agent配置文件通过.")
}

//读取conf目录下的uconf.yml配置文件
func LoadAgentConfig() []byte {
	inputFile := configFilepath()
	glog.Infof("开始读取Agent配置文件%s", inputFile)

	buf, err := ioutil.ReadFile(inputFile)
	if err != nil {
		glog.Fatalf("配置文件读取失败：%s\n错误信息:", inputFile, err)
		panic(err)
	}
	glog.Infof("成功读取Agent配置文件,配置内容如下:\n%s", string(buf))

	return buf
}

func configFilepath() string {

	inputFile := GetExecRootPath() + consts.AgentYamlRelPath + string(filepath.Separator) + consts.AgentYamlFileName
	if _, err := os.Stat(inputFile); err != nil {
		if os.IsNotExist(err) {
			glog.Fatalf("配置文件%s不存在", inputFile)
			panic("配置文件uconf.yml不存在!")
		}
	}
	return inputFile
}

func testconfigpath() string {
	return "E:\\uconf.yml"
}
