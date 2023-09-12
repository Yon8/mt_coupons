package utils

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"mt_coupons/worker"
	"time"
)

type TimeResponse struct {
	Data int `json:"data"`
}

func TimeDiff(config *worker.Config, logger *zap.Logger) {
	client := resty.New()
	resp, err := client.R().
		SetHeader("User-Agent", config.UserAgent).
		Get("https://cube.meituan.com/ipromotion/cube/toc/component/base/getServerCurrentTime")
	receiveTime := time.Now()
	if err != nil {
		logger.Error("TIME请求失败", zap.Error(err))
		return
	}

	var respData TimeResponse
	err = json.Unmarshal(resp.Body(), &respData)
	if err != nil {
		logger.Error("TIME响应体Json解析失败", zap.Error(err))
		return
	}

	onlineTime := time.Unix(0, int64(respData.Data)*int64(time.Millisecond))
	diffTime := onlineTime.Sub(receiveTime)
	(*config).AheadFetchMilli += int(diffTime / time.Millisecond)
	logger.Info(fmt.Sprintf("当前与服务器时间差值大约为%s 配置提前时间为%d", diffTime, config.AheadFetchMilli))
}
