package app

import (
	s "strings"

	uuid "github.com/satori/go.uuid"
)

type Instance struct {
	Tenant   string
	AppCode  string
	Version  string
	Env      string
	Ip       string
	HostName string
	Uuid     string
}

func NewInstance(ip, hostname string) *Instance {
	Instance := &Instance{Ip: ip, HostName: hostname}
	Instance.Uuid = newUuid()
	return Instance
}

func (app *Instance) AppNodePath() string {
	return app.Tenant + "_" + app.AppCode + "_" + app.Version
}
func (app *Instance) InstanceNodePath() string {
	return app.HostName + "_" + app.Ip + "_" + app.Uuid
}
func newUuid() string {
	id := uuid.NewV4().String()
	id = s.Replace(id, "-", "", -1)
	return id
}
