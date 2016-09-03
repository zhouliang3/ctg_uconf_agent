package zkMgr

import (
	"fmt"
	"testing"
)

func init() {
	//InitZk([]string{"127.0.0.1:2181"}, "/uconf", "111.11.2.2")
}
func TestIsParentExists(t *testing.T) {
	//	node := "/a/b/c/d"
	//	CreateNodeRecursion(node, "mydata")
	//	if !ExistsNode(node) {
	//		t.Error("递归创建节点出错！")
	//	}
	//	path := "/a"
	//	deleteNodeRecursion(path)
	//	if ExistsNode(path) {
	//		t.Error("递归删除节点出错！")
	//	}
}

func TestConnect(t *testing.T) {
	var c chan int = make(chan int)
	servers = []string{"127.0.0.1:2181"}
	aa := Connect()
	fmt.Println("aa=", aa)
	<-c
}
func TestChan(t *testing.T) {
	//	var c chan int = make(chan int, 2)
	//	var s chan int = make(chan int)
	//	go func() {
	//		time.Sleep(time.Second * 5)
	//		close(c)
	//	}()
	//	fmt.Println("selecting。。。。。。。")
	//	select {
	//	case i := <-c:
	//		fmt.Println("写入啦数据", i)
	//	case <-s:
	//		fmt.Println("退出啦")
	//	}

}

func TestState(t *testing.T) {
	//	_, ech, _ := zk.Connect([]string{"127.0.0.1:2181"}, time.Minute)
	//	for e := range ech {
	//		fmt.Println(e)
	//	}
}
