package app

import (
	s "strings"

	uuid "github.com/satori/go.uuid"
)

type Identity struct {
	Tenant   string
	AppName  string
	Version  string
	Env      string
	Ip       string
	HostName string
	Uuid     string
}

func NewIdentity(tenant, name, version, env, ip, hostname string) *Identity {
	identity := &Identity{Tenant: tenant, AppName: name, Version: version, Env: env, Ip: ip, HostName: hostname}
	identity.Uuid = newUuid()
	return identity
}

func (app *Identity) AppNodePath() string {
	return app.Tenant + "_" + app.AppName + "_" + app.Version + "_" + app.Env
}
func (app *Identity) InstanceNodePath() string {
	return app.HostName + "_" + app.Ip + "_" + app.Uuid
}
func newUuid() string {
	id := uuid.NewV4().String()
	id = s.Replace(id, "-", "", -1)
	return id
}
