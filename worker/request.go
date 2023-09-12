package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"net/url"
	"strings"
	"time"
)

type Response struct {
	Msg  string `json:"msg"`
	Code int    `json:"code"`
	Data struct {
		CouponInfo map[string]interface{} `json:"couponInfo"`
	} `json:"data"`
}
type Data struct {
	CType         string `json:"cType"`
	FpPlatform    int    `json:"fpPlatform"`
	WxOpenId      string `json:"wxOpenId"`
	AppVersion    string `json:"appVersion"`
	MtFingerprint string `json:"mtFingerprint"`
}

func CheckLogin(user User, coupon Coupon, resultChan chan map[string]string, config Config, logger *zap.Logger, task **Task) bool {
	var infoUrl string
	if strings.Contains(coupon.Desc, "v2") {
		infoUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/info/v2?couponReferIds=%s&actualLng=%s&actualLat=%s&geoType=2&isInDpEnv=0&sceneId=1&cType=wx_wallet&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			url.QueryEscape(coupon.ID),
			url.QueryEscape(config.ActualLng),
			url.QueryEscape(config.ActualLat))
	} else if strings.Contains(coupon.Desc, "v1") {
		infoUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/info?couponReferIds=%s&actualLng=%s&actualLat=%s&geoType=2&isInDpEnv=0&sceneId=1&cType=wx_wallet&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			url.QueryEscape(coupon.ID),
			url.QueryEscape(config.ActualLng),
			url.QueryEscape(config.ActualLat))
	} else {
		infoUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/info?couponReferIds=%s&actualLng=%s&actualLat=%s&geoType=2&gdPageId=%s&pageId=%s&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			url.QueryEscape(coupon.ID),
			url.QueryEscape(config.ActualLng),
			url.QueryEscape(config.ActualLat),
			url.QueryEscape(coupon.GdID),
			url.QueryEscape(coupon.PageID))
	}
	client := resty.New()
	resp, err := client.R().
		SetHeader("Host", "promotion.waimai.meituan.com").
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("User-Agent", user.UA).
		SetHeader("Origin", "https://market.waimai.meituan.com").
		SetHeader("X-Requested-With", "com.tencent.mm").
		SetHeader("Sec-Fetch-Site", "same-site").
		SetHeader("Sec-Fetch-Mode", "cors").
		SetHeader("Sec-Fetch-Dest", "empty").
		SetHeader("Referer", "https://market.waimai.meituan.com/").
		SetHeader("Accept-Encoding", "gzip, deflate").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7").
		SetHeader("Cookie", user.Cookie).
		Get(infoUrl)

	if err != nil {
		logger.Error("登陆状态检查失败！", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		(*(*task)).Fail++
		return false
	}
	var response Response
	err = json.Unmarshal(resp.Body(), &response)
	if err != nil {
		logger.Error("Check响应体Json解析失败", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		(*(*task)).Fail++
		return false
	}
	logger.Debug(string(resp.Body()), zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
	if resp.StatusCode() == 200 && strings.Contains(response.Msg, "成功") {
		if response.Data.CouponInfo[coupon.ID] == nil {
			logger.Error("用户没有资格领取或该优惠券已下架！", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
			logger.Warn("详细请求", zap.String("请求", string(resp.Body())))
			(*(*task)).Fail++
			resultMap[coupon.Desc] = fmt.Sprintf("❌%s：%s", user.Name, "用户没有资格领取或该优惠券已下架！")
			resultChan <- resultMap
			return false
		}
		//v1v2结构不同
		if strings.Contains(coupon.Desc, "v") {
			logger.Info("登录状态检查成功！",
				zap.String("user", user.Name),
				zap.String("coupon", coupon.Desc))
			return true
		}
		couponData := response.Data.CouponInfo[coupon.ID].(map[string]interface{})
		progressPercent := couponData["progressPercent"].(float64)
		priceLimit := couponData["priceLimit"].(float64)
		couponValue := couponData["couponValue"].(float64)
		couponName := couponData["couponName"].(string)
		couponReferId := couponData["couponReferId"].(string)[:4]
		logger.Info("登录状态检查成功！",
			zap.String("user", user.Name),
			zap.String("ID", couponReferId),
			zap.String("online", fmt.Sprintf("%s %.0f-%.0f", couponName, priceLimit, couponValue)),
			zap.Float64("progress", progressPercent))
		return true

	} else {
		logger.Error(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		(*(*task)).Fail++
		resultMap[coupon.Desc] = fmt.Sprintf("❌%s：%s", user.Name, response.Msg)
		resultChan <- resultMap
		return false
	}

}
func MakeRequest(user User, coupon Coupon, sign Sign, postUrl string, tries int, userCoupon ***UserCoupon, resultChan chan map[string]string, config Config, logger *zap.Logger, task ***Task) {
	client := resty.New()
	data := Data{
		CType:         "wx_wallet",
		FpPlatform:    13,
		WxOpenId:      "",
		AppVersion:    "",
		MtFingerprint: sign.MtFingerprint,
	}
	//logger.Info("抢卷请求发送！", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries))
	postData, err := json.Marshal(data)
	resp, err := client.R().
		SetBody(bytes.NewBuffer(postData)).
		SetHeader("Host", "promotion.waimai.meituan.com").
		SetHeader("Connection", "keep-alive").
		SetContentLength(true).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("mtgsig", sign.MtgSig).
		SetHeader("User-Agent", user.UA).
		SetHeader("Content-Type", "application/json").
		SetHeader("Origin", "https://market.waimai.meituan.com").
		SetHeader("X-Requested-With", "com.tencent.mm").
		SetHeader("Sec-Fetch-Site", "same-site").
		SetHeader("Sec-Fetch-Mode", "cors").
		SetHeader("Sec-Fetch-Dest", "empty").
		SetHeader("Referer", "https://market.waimai.meituan.com/").
		SetHeader("Accept-Encoding", "gzip, deflate").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7").
		SetHeader("Cookie", user.Cookie).
		Post(postUrl)
	if err != nil {
		logger.Error("请求失败！", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries))
		(*(*task)).Fail++
		return
	}
	var response Response

	err = json.Unmarshal(resp.Body(), &response)
	if err != nil {
		logger.Error("响应体Json解析失败", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		return
	}
	logger.Debug(string(resp.Body()), zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
	//fmt.Println(response.Msg)
	if response.Msg != "" {
		if strings.Contains(response.Msg, "成功") || strings.Contains(response.Msg, "已领取") || strings.Contains(response.Msg, "已获得") {
			logger.Info(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries), zap.Duration("耗时", resp.Time()))
			(*(*task)).Success++
			resultMap[coupon.Desc] = fmt.Sprintf("✅%s：%s %s", user.Name, response.Msg, time.Now().Format("2006-01-02 15:04:05.0000"))
			resultChan <- resultMap
			(*(*userCoupon)).IsStop = true
		} else if strings.Contains(response.Msg, "来晚了") || strings.Contains(response.Msg, "已抢光") || strings.Contains(response.Msg, "异常") {
			logger.Error(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries), zap.Duration("耗时", resp.Time()))
			(*(*task)).Fail++
			//resultMap[coupon.Desc] = fmt.Sprintf("❌%s：%s %s", user.Name, response.Msg, time.Now().Format("2006-01-02 15:04:05.0000"))
			//resultChan <- resultMap
			(*(*userCoupon)).IsStop = true
		} else {
			logger.Warn(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries), zap.Duration("耗时", resp.Time()))
			//最后一次尝试结果
			if tries == config.MaxTries {
				(*(*task)).Fail++
				resultMap[coupon.Desc] = fmt.Sprintf("⚠️%s：%s %s", user.Name, response.Msg, time.Now().Format("2006-01-02 15:04:05.0000"))
				resultChan <- resultMap

			}
		}
	} else {
		logger.Error("请求失败！", zap.Int("statusCode", int(resp.StatusCode())), zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		(*(*task)).Fail++
		resultMap[coupon.Desc] = fmt.Sprintf("❌%s：%s %s", user.Name, int(resp.StatusCode()), time.Now().Format("2006-01-02 15:04:05.0000"))
		resultChan <- resultMap
		(*(*userCoupon)).IsStop = true
	}
}
