package httpclient

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"ctg.com/uconf-agent/consts"
	"github.com/golang/glog"
)

type RequestContext struct {
	Url     string
	Headers map[string]string
}

//发送Get请求，返回请求响应结果
func Get(requestContext *RequestContext) ([]byte, bool) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", requestContext.Url, nil)
	if !checkHttpNewReqErr(err, requestContext.Url) {
		return nil, false
	}
	if requestContext.Headers != nil {
		for key, value := range requestContext.Headers {
			req.Header.Add(key, value)
		}
	}
	resp, err := client.Do(req)
	defer resp.Body.Close()
	if !checkRequestError(err, resp, requestContext.Url) {
		return nil, false
	}
	body, err := ioutil.ReadAll(resp.Body)
	if !checkError("读取Http响应体异常", err) {
		return nil, false
	}
	return body, true
}

//发送Rest请求，解析返回的json格式数据
func GetValueFromServer(url string) (map[string]interface{}, error) {
	requestContext := &RequestContext{Url: url, Headers: nil}
	data, success := httpRetryCall(Get, requestContext, "获取Json数据异常")
	if !success {
		return nil, errors.New("获取Json数据异常")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		checkError("解析从统一配置中心获取到的Json格式数据,出现异常", err)
		return nil, err
	}
	return result, nil
}

//下载配置文件
func DownloadFromServer(url string) ([]byte, bool) {
	headers := make(map[string]string)
	headers["Accept-Encoding"] = "gzip, deflate"
	headers["Accept-Language"] = "en-US,en;q=0.5"
	headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	requestContext := &RequestContext{Url: url, Headers: headers}
	return httpRetryCall(Get, requestContext, "下载配置文件出现异常")
}

func checkError(msg string, err error) bool {
	if err != nil {
		glog.Errorf("%s : %v", msg, err)
		return false
	}
	return true
}

func checkRequestError(err error, resp *http.Response, url string) bool {
	if err != nil {
		glog.Fatalf("Http请求异常:[%v],请求地址:%s", err, url)
		return false
	}

	if resp.StatusCode != 200 {
		glog.Fatalf("Http请求异常:[%v],请求地址:%s", resp.Status, url)
		return false
	}
	return true
}

func checkHttpNewReqErr(err error, url string) bool {
	if err != nil {
		glog.Fatalf("新建Get请求异常: %v，请求地址：%s", err, url)
		return false
	}
	return true
}

type UnreliableHttpCaller func(ctx *RequestContext) ([]byte, bool)

//传入适配UnreliableHttpCaller类型的方法；调用参数；超时信息，可进行失败重试
func httpRetryCall(caller UnreliableHttpCaller, ctx *RequestContext, msg string) ([]byte, bool) {
	for i := 0; i < consts.UnreliableHttpRetryTimes; i++ {
		data, success := caller(ctx)
		if !success {
			retryRemainTimes := consts.UnreliableHttpRetryTimes - (i + 1)
			if retryRemainTimes > 0 {
				glog.Errorf("%s，将在%d秒后将重试，剩余重试次数:%d,请求地址：%s", msg, consts.UnreliableHttpRetryGap/time.Second, retryRemainTimes, ctx.Url)
				time.Sleep(consts.UnreliableHttpRetryGap)
			} else {
				glog.Errorf("%s，剩余重试次数:%d,请求地址：%s", msg, retryRemainTimes, ctx.Url)

			}
			continue
		}
		return data, true
	}
	return nil, false
}
