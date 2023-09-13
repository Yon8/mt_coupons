package worker

import (
	"fmt"
	"go.uber.org/zap"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	MaxTries             int    `toml:"MaxTries"`
	AheadSignSec         int    `toml:"AheadSignSec"`
	AheadFetchMilli      int    `toml:"AheadFetchMilli"`
	ProcessIntervalMilli int    `toml:"ProcessIntervalMilli"`
	RequestIntervalMilli int    `toml:"RequestIntervalMilli"`
	PushToken            string `toml:"PushToken"`
	ActualLng            string `toml:"ActualLng"`
	ActualLat            string `toml:"ActualLat"`
	UserAgent            string `toml:"UserAgent"`
	EnableTopic          bool   `toml:"EnableTopic"`
}
type UserCoupon struct {
	ID     string
	Name   string
	IsStop bool
}
type Task struct {
	Total   int
	Success int
	Fail    int
}

var resultMap map[string]string
var config Config

func ProcessCouponUserPair(coupon Coupon, userKey int, user User, userCoupon *UserCoupon, wg *sync.WaitGroup, resultChan chan map[string]string, Config Config, logger *zap.Logger, task *Task) {
	resultMap = make(map[string]string)
	config = Config
	defer wg.Done()
	logger.Info("抢卷任务运行", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.Int("userKey", userKey))
	//总数统计
	(*task).Total++
	login := CheckLogin(user, coupon, resultChan, config, logger, &task)
	if !login {
		return
	}
	FetchCoupon(user, userKey, coupon, &userCoupon, resultChan, config, logger, &task)

}

func FetchCoupon(user User, userKey int, coupon Coupon, userCoupon **UserCoupon, resultChan chan map[string]string, config Config, logger *zap.Logger, task **Task) {
	var postUrl string
	var version string
	if strings.Contains(coupon.Desc, "v2") {
		version = "v2"
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/fetchcomponentcoupon/v2?couponReferId=%s&actualLng=%s&actualLat=%s&geoType=2&isInDpEnv=0&gdPageId=%s&pageId=%s&version=1&instanceId=%s&utmSource=appshare&utmCampaign=AgroupBgroupC0D200E0Ghomepage_category1_394__a1__c-1024&needFetchedByUUID=1&sceneId=1&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			url.QueryEscape(coupon.ID),
			url.QueryEscape(config.ActualLng),
			url.QueryEscape(config.ActualLat),
			url.QueryEscape(coupon.GdID),
			url.QueryEscape(coupon.PageID),
			url.QueryEscape(coupon.InstID))
	} else if strings.Contains(coupon.Desc, "v1") {
		version = "v1"
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/couponcomponent/fetchcomponentcoupon?couponReferId=%s&actualLng=%s&actualLat=%s&geoType=2&isInDpEnv=0&gdPageId=%s&pageId=%s",
			url.QueryEscape(coupon.ID),
			url.QueryEscape(config.ActualLng),
			url.QueryEscape(config.ActualLat),
			url.QueryEscape(coupon.GdID),
			url.QueryEscape(coupon.PageID))
	} else if coupon.InstID == "" {
		version = "brief"
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/fetchcoupon?couponReferId=%s&actualLng=%s&actualLat=%s&geoType=2&gdPageId=%s&pageId=%s",
			url.QueryEscape(coupon.ID),
			url.QueryEscape(config.ActualLng),
			url.QueryEscape(config.ActualLat),
			url.QueryEscape(coupon.GdID),
			url.QueryEscape(coupon.PageID))
	} else {
		version = "intact"
		postUrl = fmt.Sprintf("https://promotion.waimai.meituan.com/lottery/limitcouponcomponent/fetchcoupon?couponReferId=%s&actualLng=%s&actualLat=%s&geoType=2&gdPageId=%s&pageId=%s&version=1&utmSource=&utmCampaign=&instanceId=%s&componentId=%s&yodaReady=h5&csecplatform=4&csecversion=2.1.2",
			url.QueryEscape(coupon.ID),
			url.QueryEscape(config.ActualLng),
			url.QueryEscape(config.ActualLat),
			url.QueryEscape(coupon.GdID),
			url.QueryEscape(coupon.PageID),
			url.QueryEscape(coupon.InstID),
			url.QueryEscape(coupon.InstID))
	}
	// 计算抢卷时间与当前时间的差值
	timeUntilCoupon := coupon.Time.Sub(time.Now())

	//在抢卷时间前n秒生成签名
	signTimer := time.NewTimer(timeUntilCoupon - time.Duration(config.AheadSignSec)*time.Second)
	defer signTimer.Stop()
	//在抢卷时间前前n毫秒开始抢卷任务
	couponTimer := time.NewTimer(timeUntilCoupon - time.Duration(config.AheadFetchMilli)*time.Millisecond)
	defer couponTimer.Stop()

	logger.Info("定时任务启动!", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.String("version", version))

	var signs []Sign
	//非成功情况尝试次数
	var tries int = 1
	//最大尝试次数
	var maxTries int = config.MaxTries

	select {
	case <-signTimer.C:
		signStartTime := time.Now()
		for i := 0; i < maxTries; i++ {
			sign := GenerateSign(user, postUrl, logger)
			if sign.MtgSig == "" || sign.MtFingerprint == "" {
				return
			}
			signs = append(signs, sign)
		}
		signEndTime := time.Now()
		signTime := signEndTime.Sub(signStartTime)
		if len(signs) != 0 {
			logger.Info("签名生成成功", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.String("耗时", signTime.String()), zap.Int("数量", maxTries))

		} else {
			logger.Error("签名生成失败", zap.String("user", user.Name), zap.String("coupon", coupon.Desc), zap.String("耗时", signTime.String()), zap.Int("数量", maxTries))
			(*(*task)).Fail++
		}

	}
	select {
	case <-couponTimer.C:
		time.Sleep(time.Duration(userKey*config.ProcessIntervalMilli) * time.Millisecond)
		for _, sign := range signs {
			if (*userCoupon).IsStop {
				return
			}
			MakeRequest(user, coupon, sign, postUrl, tries, &userCoupon, resultChan, config, logger, &task)
			tries++
			//并发时使用
			//time.Sleep(time.Duration(config.RequestIntervalMilli) * time.Millisecond)
		}
	}
}
