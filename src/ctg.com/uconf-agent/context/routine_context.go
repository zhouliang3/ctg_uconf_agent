package context

import (
	"sync/atomic"
)

var idx int32 = 0

type RoutineContext struct {
	FileName, Url, Path, FileZkPath, InstanceZkPath string
	RoutineId                                       int32
}

func NewRoutineContext(FileName, Url, Path, FileZkPath, InstanceZkPath string) *RoutineContext {
	return &RoutineContext{FileName, Url, Path, FileZkPath, InstanceZkPath, newRoutineId()}
}
func newRoutineId() int32 {
	return atomic.AddInt32(&idx, 1)
}
