package consts

import (
	"time"
)

//agent配置文件名称
const AgentYamlFileName string = "uconf.yml"

//agent配置文件的相对路径
const AgentYamlRelPath string = "conf"

const ZooApiPath string = "/api/zoo"

const FileApiPath string = "/api/config/file"

//zk重连时间间隔
const ZkConnectRetryGap = time.Second * 5

//zk请求重试次数
const ZkCallerRetryTimes int = 3

//zk请求重试时间间隔
const ZkCallerRetryGap = time.Second * 1

//zk连接超时时间
const ZkConnectTimeOut = time.Second * 5

const UnreliableZkRetryTimes int = 3

const UnreliableZkRetryGap = time.Second * 5
