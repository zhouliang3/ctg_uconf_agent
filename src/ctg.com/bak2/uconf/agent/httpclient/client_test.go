package httpclient

import (
	"fmt"
	"testing"

	"ctg.com/uconf/agent/consts"
	"ctg.com/uconf/agent/context"
	"ctg.com/uconf/agent/retryer"
)

func TestSomething(t *testing.T) {
}

func TestGetAllConfig(t *testing.T) {
	appRetryer := retryer.NewEndlessRetryer(consts.HttpFetchInfoRetryGap)
	conflistContext := context.NewRequestRoutineContext("http://localhost:8080/api/config/list?version=1_0_0_0&app=uconf_demo&env=rd&tenant=fj", nil)
	listOutut := appRetryer.DoRetry(RetryableGetFileList, conflistContext)
	if cfglist, ok := listOutut.Result.(CfgListRespose); ok {
		if "true" == cfglist.Success {
			if len(cfglist.Result) > 0 {
				for _, cfg := range cfglist.Result {
					fmt.Printf("config name is:\n %s\n", cfg.ConfigName)
					fmt.Printf("config value is:\n %s\n", cfg.ConfigValue)
				}
			}
		}
	}
	//	if listData, ok := listOutut.Result.(map[string]interface{}); ok {
	//		result := listData["result"]
	//		if configs, ok := result.([]interface{}); ok {
	//			for _, config := range configs {
	//				if _, ok := config.(map[string]interface{}); ok {
	//					//fmt.Println("-------------config Item--------------\n", configItem)
	//					//fmt.Printf("config name is:\n %s\n", configItem["configName"].(string))
	//					//fmt.Printf("config value is:\n %s\n", configItem["configValue"].(string))
	//				}
	//			}
	//		}
	//	}
}
