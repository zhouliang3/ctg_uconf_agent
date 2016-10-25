package httpclient

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"

	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/retryer"
	"github.com/golang/glog"
)

type CfgListRespose struct {
	Message map[string]interface{}
	Success string
	State   int32
	Result  []AppConfig
}

type AppConfig struct {
	ConfigDeployId, ConfigId, AppVersionId, EnvId   int64
	ConfigVersion                                   int32
	ConfigValue, ConfigType, ConfigName, ConfigPath string
}

//发送Get请求，返回请求响应结果
func Get(ctx *context.RoutineContext) *context.OutputContext {
	client := &http.Client{}
	req, err := http.NewRequest("GET", ctx.RequestContext.Url, nil)
	if !checkHttpNewReqErr(err, ctx.RequestContext.Url) {
		return context.NewFailOutputContext("新建Get请求异常")
	}
	if ctx.RequestContext.Headers != nil {
		for key, value := range ctx.RequestContext.Headers {
			req.Header.Add(key, value)
		}
	}
	resp, err := client.Do(req)
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	if !checkRequestError(err, resp, ctx.RequestContext.Url) {
		return context.NewFailOutputContext("Http请求异常")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if !checkResultError("读取Http响应体异常", err) {
		return context.NewFailOutputContext("读取Http响应体异常")
	}
	return context.NewSuccessOutputContext(body)
}

func GetValueFromServer(url string) (map[string]interface{}, error) {
	return JsonDataFromServer(url, nil)
}

func RetryableGetJsonData(ctx *context.RoutineContext) *context.OutputContext {
	output, err := JsonDataFromServer(ctx.RequestContext.Url, nil)
	if err == nil {
		return context.NewSuccessOutputContext(output)
	} else {
		return context.NewErrorOutputContext(err)
	}
}

//发送Rest请求，解析返回的json格式数据
func JsonDataFromServer(url string, headers map[string]string) (map[string]interface{}, error) {
	ctx := context.NewRequestRoutineContext(url, nil)
	httpRetryer := retryer.HttpRequestRetryer()
	output := httpRetryer.DoRetry(Get, ctx)
	if output.Err != nil {
		return nil, errors.New("获取Json数据异常")
	}
	if data, ok := output.Result.([]byte); ok {
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			checkResultError("解析从统一配置中心获取到的Json格式数据,出现异常,数据内容为:"+string(data), err)
			return nil, err
		}
		return result, nil
	} else {
		return nil, errors.New("调用Get接口的返回值类型不匹配")
	}

}

func RetryableGetFileList(ctx *context.RoutineContext) *context.OutputContext {
	output, err := FileListFromServer(ctx.RequestContext.Url, nil)
	if err == nil {
		return context.NewSuccessOutputContext(output)
	} else {
		return context.NewErrorOutputContext(err)
	}
}

//发送Rest请求，解析返回的json格式数据
func FileListFromServer(url string, headers map[string]string) (CfgListRespose, error) {
	ctx := context.NewRequestRoutineContext(url, nil)
	httpRetryer := retryer.HttpRequestRetryer()
	output := httpRetryer.DoRetry(Get, ctx)
	if output.Err != nil {
		return CfgListRespose{}, errors.New("获取Json数据异常")
	}
	if data, ok := output.Result.([]byte); ok {
		var result CfgListRespose
		if err := json.Unmarshal(data, &result); err != nil {
			checkResultError("解析从统一配置中心获取到的Json格式数据,出现异常,数据内容为:"+string(data), err)
			return CfgListRespose{}, err
		}
		return result, nil
	} else {
		return CfgListRespose{}, errors.New("调用Get接口的返回值类型不匹配")
	}

}

//下载配置文件
func DownloadFromServer(url string) ([]byte, bool) {
	headers := make(map[string]string)
	headers["Accept-Encoding"] = "gzip, deflate"
	headers["Accept-Language"] = "en-US,en;q=0.5"
	headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	ctx := context.NewRequestRoutineContext(url, headers)
	httpRetryer := retryer.HttpRequestRetryer()
	output := httpRetryer.DoRetry(Get, ctx)
	if output.Err != nil {
		return nil, false
	}
	if data, ok := output.Result.([]byte); ok {
		return data, true
	} else {
		return nil, false
	}
}

func checkResultError(msg string, err error) bool {
	if err != nil {
		glog.Errorf("%s ; 异常详情: %v", msg, err)
		return false
	}
	return true
}

func checkRequestError(err error, resp *http.Response, url string) bool {
	if err != nil {
		glog.Errorf("Http请求异常:[%v],请求地址:%s", err, url)
		return false
	}

	if resp.StatusCode != 200 {
		glog.Errorf("Http请求异常:[%v],请求地址:%s", resp.Status, url)
		return false
	}
	return true
}

func checkHttpNewReqErr(err error, url string) bool {
	if err != nil {
		glog.Errorf("新建Get请求异常: %v，请求地址：%s", err, url)
		return false
	}
	return true
}
