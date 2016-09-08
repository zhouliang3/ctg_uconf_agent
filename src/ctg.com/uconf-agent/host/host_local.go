package host

import (
	"net"
	"os"
	s "strings"
)

type Machine struct {
	Ip       string
	HoseName string
}

var m *Machine

func Info(serverAddr string) *Machine {
	if m != nil {
		return m
	}
	m = &Machine{}
	m.HoseName, _ = os.Hostname()
	m.Ip = localIp(serverAddr)
	return m
}

//获取本地IP地址
func localIp(serverAddr string) string {
	conn, err := net.Dial("udp", serverAddr)
	if err != nil {
		//		fmt.Println(err.Error())
	}
	defer conn.Close()
	ip := s.Split(conn.LocalAddr().String(), ":")[0]
	//	fmt.Println("local ip is :", ip)
	return ip
}
