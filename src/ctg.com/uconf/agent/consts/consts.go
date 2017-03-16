package consts

import (
	"time"
)

//每过多久将日志刷到文件中去
const LogFlushGap = time.Millisecond * 200

//agent配置文件名称
const AgentYamlFileName string = "uconf.yml"

//agent配置文件的相对路径
const AgentYamlRelPath string = "conf"

const ZooApiPath string = "/api/zoo"

const FileApiPath string = "/api/config/file"

const AppRootPath string = "/api/config/appDir"

const CfgListpath string = "/api/config/list"

//const AppRootDir string = "/apps/uconf"

//zk重连时间间隔
const ZkConnectRetryGap = time.Second * 5

//zk请求重试次数
const ZkCallerRetryTimes int = 3

//zk请求重试时间间隔
const ZkCallerRetryGap = time.Second * 1

//zk连接超时时间
const ZkConnectTimeOut = time.Second * 5

//调用zkMgr的接口重试次数
const UnreliableZkRetryTimes int = 3

//调用zkMgr的接口重试时间间隔
const UnreliableZkRetryGap = time.Second * 5

//http请求重试次数
const UnreliableHttpRetryTimes int = 3

//http请求重试时间间隔
const UnreliableHttpRetryGap = time.Second * 5

//获取zookeeper连接地址的重试时间间隔
const HttpFetchInfoRetryGap = time.Second * 90

//开启的线程数
const MaxRoutineNums = 10
