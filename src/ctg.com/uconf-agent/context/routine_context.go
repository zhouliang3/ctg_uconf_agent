package context

import (
	"errors"
	"sync/atomic"
)

var idx int32 = 0

//RoutineContext是一个大而全的上下文结构体，目前程序中都是通过指针传递，对资源的消耗很小。
type RoutineContext struct {
	FileContext    *FileContext
	RoutineId      int32
	ZooActionMeta  *ZooActionMeta
	RequestContext *RequestContext
}

//配置文件上下文
type FileContext struct {
	FileName, Url, Path, FileZkPath, InstanceZkPath string
	Data                                            []byte
}
type RequestContext struct {
	Url     string
	Headers map[string]string
}

func NewFileContext(FileName, Url, Path, FileZkPath, InstanceZkPath string, data []byte) *FileContext {
	return &FileContext{FileName, Url, Path, FileZkPath, InstanceZkPath, data}
}

func NewRoutineContext(FileName, Url, Path, FileZkPath, InstanceZkPath string) *RoutineContext {
	zooContext := NewFileContext(FileName, Url, Path, FileZkPath, InstanceZkPath, nil)
	return &RoutineContext{zooContext, newRoutineId(), nil, nil}
}

func NewRequestRoutineContext(url string, headers map[string]string) *RoutineContext {
	return &RoutineContext{nil, newRoutineId(), nil, &RequestContext{url, headers}}
}

func NewZooActionMetaContext(zoo *ZooActionMeta) *RoutineContext {
	return &RoutineContext{nil, newRoutineId(), zoo, nil}
}

func InitMainRoutineContext() *RoutineContext {
	return &RoutineContext{nil, newRoutineId(), nil, nil}
}

func newRoutineId() int32 {
	return atomic.AddInt32(&idx, 1)
}

type OutputContext struct {
	Err    error
	Result interface{}
}

func NewSuccessOutputContext(result interface{}) *OutputContext {
	return &OutputContext{nil, result}
}

func NewFailOutputContext(msg string) *OutputContext {
	return &OutputContext{errors.New(msg), nil}
}

func NewErrorOutputContext(err error) *OutputContext {
	return &OutputContext{err, nil}
}

type ZooActionMeta struct {
	ZooAction string
}

func NewZooServiceMeta(zooAction string) *ZooActionMeta {
	return &ZooActionMeta{zooAction}
}
func (zoo *ZooActionMeta) HostsActionUrl() string {
	return zoo.ZooAction + "/" + "hosts"
}
func (zoo *ZooActionMeta) PrefixActionUrl() string {
	return zoo.ZooAction + "/" + "prefix"
}
