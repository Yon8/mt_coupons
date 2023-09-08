package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Sign struct {
	MtgSig        string `json:"mtgsig"`
	MtFingerprint string `json:"mtFingerprint"`
}
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

var resultMap map[string]string

func processCouponUserPair(coupon Coupon, userKey int, user User, userCoupon *UserCoupon, wg *sync.WaitGroup, resultChan chan map[string]string) {
	resultMap = make(map[string]string)
	defer wg.Done()
	logger.Info("抢卷任务运行", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("userKey", userKey))

	login := checkLogin(user, coupon, resultChan)
	if !login {
		return
	}
	grabCoupon(user, userKey, coupon, &userCoupon, resultChan)
}
func generateSign(cookie string, postUrl string) Sign {
	fmt.Println(config.UserAgent)

	//获取签名Json代码
	signJsCode := fmt.Sprintf(`
		fs.readFile('./data/js/mt212.js', 'UTF-8', async (err, data) => {
			if (err) console.log(err);
			
			const url = '%s';
			const dataObj = {
				"cType": "mti",
				"fpPlatform": 3,
				"wxOpenId": "",
				"appVersion": ""
			};
			const h5 = eval(data);
			const h5guard = new h5('%s', '%s');
			console.log(JSON.stringify(await h5guard.sign(url, dataObj)));
			process.exit(0);
		});
	`, postUrl, cookie, config.UserAgent)

	// 调用 nodejs 获取签名
	cmd := exec.Command("node", "-e", signJsCode)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("调用失败!", zap.Error(err))
		return Sign{}
	}
	// 将输出转换为字符串
	outputStr := string(output)
	var signData Sign
	// 检查输出中是否包含Pango警告
	if strings.Contains(outputStr, "Pango-WARNING") {
		// 如果包含警告，则从输出中移除警告消息
		outputStr = removePangoWarning(outputStr)
		if err := json.Unmarshal([]byte(outputStr), &signData); err != nil {
			logger.Error("签名解析失败!", zap.Error(err))
			return Sign{}
		}
	} else {
		if err := json.Unmarshal(output, &signData); err != nil {
			logger.Error("签名解析失败!", zap.Error(err))
			return Sign{}
		}
	}
	fmt.Println(signData.MtgSig)
	return signData
}
func checkLogin(user User, coupon Coupon, resultChan chan map[string]string) bool {
	//postTemplate := `https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/fetchcoupon?couponReferId=%s&actualLng=115.064963&actualLat=35.777563&geoType=2&gdPageId=%s&pageId=%s`
	//postUrl := fmt.Sprintf(postTemplate, coupon.ID, coupon.GdID, coupon.PageID)
	//ua := "Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Mobile Safari/537.36"
	//sign := generateSign(user.Cookie, postUrl, ua)

	var infoUrl string
	if strings.Contains(coupon.Desc, "v2") {
		infoUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/info/v2?couponReferIds=%s&actualLng=%s&actualLat=%s&geoType=2&isInDpEnv=0&sceneId=1&cType=wx_wallet&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			coupon.ID, config.ActualLng, config.ActualLat)
	} else if strings.Contains(coupon.Desc, "v1") {
		infoUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/info?couponReferIds=%s&actualLng=%s&actualLat=%s&geoType=2&isInDpEnv=0&sceneId=1&cType=wx_wallet&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			coupon.ID, config.ActualLng, config.ActualLat)
	} else {
		infoUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/info?couponReferIds=%s",
			coupon.ID)
		//infoUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/info?couponReferIds=%s&actualLng=%s&actualLat=%s&geoType=2&gdPageId=%s&pageId=%s&componentId=%s&yodaReady=h5&csecplatform=4&csecversion=2.1.2&mtgsig=%s",
		//	coupon.ID, config.ActualLng, config.ActualLat, coupon.GdID, coupon.PageID, coupon.InstID, url.QueryEscape("{\"a1\":\"1.1\",\"a2\":1694070847161,\"a3\":\"8472xz73ww8652z1z7746zz0400yuz0y81z3v2w1w26979583w4498y1\",\"a5\":\"GWXOl81ochTUSfEq8Ju0bpN5fk3zdxRc\",\"a6\":\"hs1.4rMLnSSN7/TuDgTJbLhpKNDVnAdjPbCEuFr+W/5Q5JxbHifHeGI0uKyo40lOCzh8qiuMHlhW/EcSsVThBjZrkduyFwUbQ8bT0bSAmvWbYmsI=\",\"x0\":4,\"d1\":\"7dda9f6324f942a9f2a241755923ddc3\"}"))
	}
	client := resty.New()
	//fmt.Println(infoUrl)
	resp, err := client.R().
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("User-Agent", config.UserAgent).
		SetHeader("Origin", "https://market.waimai.meituan.com").
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
		return false
	}

	fmt.Println(string(resp.Body()))

	var response Response
	err = json.Unmarshal(resp.Body(), &response)
	if err != nil {
		logger.Error("Check响应体Json解析失败", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		return false
	}

	if resp.StatusCode() == 200 && strings.Contains(response.Msg, "成功") {
		//v1v2是couponList结构不同 暂不进行判定
		if response.Data.CouponInfo[coupon.ID] == nil && !strings.Contains(coupon.Desc, "v") {
			//fmt.Printf("%s：用户没有资格领取或该优惠券已下架：%s\n", user.Name, coupon.Desc)
			logger.Error("用户没有资格领取或该优惠券已下架！", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
			logger.Warn("详细请求", zap.String("请求", string(resp.Body())))
			resultMap[coupon.Desc] = fmt.Sprintf("%s：%s", user.Name, "用户没有资格领取或该优惠券已下架！")
			resultChan <- resultMap
			return false
		}
		logger.Info("用户登录状态检查成功！", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		return true
	} else {
		logger.Error(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		resultMap[coupon.Desc] = fmt.Sprintf("%s：%s", user.Name, response.Msg)
		resultChan <- resultMap
		return false
	}

}
func grabCoupon(user User, userKey int, coupon Coupon, userCoupon **UserCoupon, resultChan chan map[string]string) {
	var postUrl string
	if strings.Contains(coupon.Desc, "v2") {
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/fetchcomponentcoupon/v2?couponReferId=%s&geoType=2&isInDpEnv=0&gdPageId=%s&pageId=%s&sceneId=1",
			coupon.ID, coupon.GdID, coupon.PageID)
	} else if strings.Contains(coupon.Desc, "v1") {
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/fetchcomponentcoupon?couponReferId=%s&actualLng=%s&actualLat=%s&geoType=2&isInDpEnv=0&gdPageId=%s&pageId=%s",
			coupon.ID, config.ActualLng, config.ActualLat, coupon.GdID, coupon.PageID)
	} else if coupon.InstID == "" {
		//半参
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/fetchcoupon?couponReferId=%s&actualLng=%s&actualLat=%s&geoType=2&gdPageId=%s&pageId=%s",
			coupon.ID, config.ActualLng, config.ActualLat, coupon.GdID, coupon.PageID)
	} else {
		//全参
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/fetchcoupon?couponReferId=%s&actualLng=%s&actualLat=%s&geoType=2&gdPageId=%s&pageId=%s&version=1&utmSource=&utmCampaign=&instanceId=%s&componentId=%s&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			coupon.ID, config.ActualLng, config.ActualLat, coupon.GdID, coupon.PageID, coupon.InstID, coupon.InstID)
	}
	fmt.Println(postUrl)
	// 计算抢卷时间与当前时间的差值
	timeUntilCoupon := coupon.Time.Sub(time.Now())

	//在抢卷时间前n秒生成签名
	signTimer := time.NewTimer(timeUntilCoupon - time.Duration(config.AheadSignSec)*time.Second)
	defer signTimer.Stop()
	//在抢卷时间前前n毫秒开始抢卷任务
	couponTimer := time.NewTimer(timeUntilCoupon - time.Duration(config.AheadFetchMilli)*time.Millisecond)
	defer couponTimer.Stop()

	logger.Info("定时任务启动!", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))

	var signs []Sign
	//非成功情况尝试次数
	var tries int = 1
	//最大尝试次数
	var maxTries int = config.MaxTries

	select {
	case <-signTimer.C:
		signStartTime := time.Now()
		for i := 0; i < maxTries; i++ {
			sign := generateSign(user.Cookie, postUrl)
			signs = append(signs, sign)
		}
		signEndTime := time.Now()
		signTime := signEndTime.Sub(signStartTime)
		logger.Info("签名生成成功", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.String("耗时", signTime.String()), zap.Int("数量", maxTries))

	}
	select {
	case <-couponTimer.C:
		for _, sign := range signs {
			if (*userCoupon).IsStop {
				return
			}
			time.Sleep(time.Duration(userKey*config.ProcessIntervalMilli) * time.Millisecond)
			makeRequest(user, coupon, sign, postUrl, tries, &userCoupon, resultChan)

			tries++
			time.Sleep(time.Duration(config.RequestIntervalMilli) * time.Millisecond)
		}
	}
}

func makeRequest(user User, coupon Coupon, sign Sign, postUrl string, tries int, userCoupon ***UserCoupon, resultChan chan map[string]string) {
	client := resty.New()
	data := Data{
		CType:         "mti",
		FpPlatform:    3,
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
		//SetContentLength(true).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("mtgsig", sign.MtgSig).
		SetHeader("User-Agent", config.UserAgent).
		SetHeader("Content-Type", "application/json").
		SetHeader("Origin", "https://market.waimai.meituan.com").
		//SetHeader("X-Requested-With", "com.tencent.mm").
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
		return
	}
	var response Response
	fmt.Println(string(resp.Body()))
	err = json.Unmarshal(resp.Body(), &response)
	if err != nil {
		logger.Error("响应体Json解析失败", zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		return
	}

	//fmt.Println(response.Msg)
	if response.Msg != "" {
		if strings.Contains(response.Msg, "成功") || strings.Contains(response.Msg, "已领取") || strings.Contains(response.Msg, "已获得") {
			logger.Info(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries), zap.Duration("耗时", resp.Time()))
			resultMap[coupon.Desc] = fmt.Sprintf("%s：%s %s", user.Name, response.Msg, time.Now().Format("2006-01-02 15:04:05.0000"))
			resultChan <- resultMap
			(*(*userCoupon)).IsStop = true
		} else if strings.Contains(response.Msg, "来晚了") || strings.Contains(response.Msg, "已抢光") || strings.Contains(response.Msg, "异常") {
			logger.Error(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries), zap.Duration("耗时", resp.Time()))
			resultMap[coupon.Desc] = fmt.Sprintf("%s：%s %s", user.Name, response.Msg, time.Now().Format("2006-01-02 15:04:05.0000"))
			resultChan <- resultMap
			(*(*userCoupon)).IsStop = true
		} else {
			logger.Warn(response.Msg, zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("请求次数", tries), zap.Duration("耗时", resp.Time()))
			//最后一次尝试结果
			if tries == config.MaxTries-1 {
				resultMap[coupon.Desc] = fmt.Sprintf("%s：%s %s", user.Name, response.Msg, time.Now().Format("2006-01-02 15:04:05.0000"))
			}
		}
	} else {
		logger.Error("请求失败！", zap.Int("statusCode", int(resp.StatusCode())), zap.String("user", user.Name), zap.String("coupon", coupon.Desc))
		resultMap[coupon.Desc] = fmt.Sprintf("%s：%s %s", user.Name, int(resp.StatusCode()), time.Now().Format("2006-01-02 15:04:05.0000"))
		resultChan <- resultMap
		(*(*userCoupon)).IsStop = true
	}
}

// mt212移除Pango警告消息
func removePangoWarning(output string) string {
	lines := strings.Split(output, "\n")
	var filteredLines []string

	for _, line := range lines {
		// 过滤掉包含"Pango-WARNING"的行
		if !strings.Contains(line, "Pango-WARNING") {
			filteredLines = append(filteredLines, line)
		}
	}

	// 重新组合成一个字符串
	return strings.Join(filteredLines, "\n")
}