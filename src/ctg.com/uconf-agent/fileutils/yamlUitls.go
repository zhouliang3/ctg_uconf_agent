package fileutils

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	s "strings"

	"ctg.com/uconf-agent/consts"
	"github.com/golang/glog"
	"gopkg.in/yaml.v2"
)

type RestServer struct {
	Ip      string
	Port    string
	Context string
}

func (this *RestServer) ServerActionAddress() (string, string) {
	if len(this.Context) > 0 {
		if !s.HasPrefix(this.Context, "/") {
			this.Context = "/" + this.Context
		}
	} else {
		this.Context = ""
	}
	srvAddr := "http://" + this.Ip + ":" + this.Port + this.Context
	return srvAddr + consts.ZooApiPath, srvAddr + consts.FileApiPath

}

type App struct {
	Name    string
	Tenant  string
	Version string
	Env     string
	Tmpdir  string
	Appdir  string
	Configs []AppConfig //`yaml:"configs,flow"`
}
type AppConfig struct {
	Name string
	Dir  string
}
type AgentConfig struct {
	Enabled bool
	Server  RestServer
	Apps    []App //`yaml:"apps,flow"`
}

func Read() *AgentConfig {
	t := &AgentConfig{}
	data := LoadAgentConfig()
	glog.Infof("开始解析Agent配置文件:%s.", consts.AgentYamlFileName)
	err := yaml.Unmarshal(data, t)
	if err != nil {
		glog.Fatalf("Agent配置文件解析失败: %v", err)
		panic("Agent配置文件解析失败!")
	}
	glog.Info("成功解析Agent配置文件.")

	return t
}

//读取conf目录下的uconf.yml配置文件
func LoadAgentConfig() []byte {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	dir, _ := filepath.Split(path)
	inputFile := dir + consts.AgentYamlRelPath + string(filepath.Separator) + consts.AgentYamlFileName
	glog.Infof("开始读取Agent配置文件%s", inputFile)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			glog.Fatalf("配置文件%s不存在", inputFile)
			panic("配置文件uconf.yml不存在!")
		}
	}
	buf, err := ioutil.ReadFile(inputFile)
	if err != nil {
		glog.Fatalf("配置文件读取失败：%s\n错误信息:", inputFile, err)
		panic(err)
	}
	glog.Infof("成功读取Agent配置文件,配置内容如下:\n%s", string(buf))

	return buf
}
