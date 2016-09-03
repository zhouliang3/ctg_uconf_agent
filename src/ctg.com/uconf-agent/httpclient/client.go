package httpclient

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
)

//发送Get请求获取数据
func GetData(url string) []byte {
	res, err := http.Get(url)
	checkRequestError(err, res, url)
	data, err := ioutil.ReadAll(res.Body)
	checkError("从统一配置中心获取数据,出现异常", err)
	return data
}

//发送Rest请求，解析返回的json格式数据
func GetValueFromServer(url string) map[string]interface{} {
	data := GetData(url)
	var dat map[string]interface{}
	if err := json.Unmarshal(data, &dat); err != nil {
		checkError("解析从统一配置中心获取到的Json格式数据,出现异常", err)
	}
	return dat
}

//下载配置文件
func DownloadFromServer(url string) []byte {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	newHttpRequestError(err, url)
	req.Header.Add("Accept-Encoding", "gzip, deflate")
	req.Header.Add("Accept-Language", "en-US,en;q=0.5")
	req.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := client.Do(req)
	defer resp.Body.Close()
	checkRequestError(err, resp, url)
	body, _ := ioutil.ReadAll(resp.Body)

	return body
}
func checkError(msg string, err error) {
	if err != nil {
		glog.Fatalf("%s:%v", msg, err)
		panic(err)
	}
}
func checkRequestError(err error, resp *http.Response, url string) {
	if err != nil {
		glog.Fatalf("下载文件请求异常:[%v],请求地址:%s", err, url)
		panic(err)
	}

	if resp.StatusCode != 200 {
		glog.Fatalf("下载文件请求异常:[%v],请求地址:%s", resp.Status, url)
		panic("下载文件请求异常:" + resp.Status + ",请求地址:" + url)
	}
}
func newHttpRequestError(err error, url string) {
	if err != nil {
		glog.Fatalf("新建Get请求异常: %v，请求地址：%s", err, url)
		panic(err)
	}
}
