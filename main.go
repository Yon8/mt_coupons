package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/pelletier/go-toml"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Coupon struct {
	ID     string `json:"id"`
	GdID   string `json:"gdId"`
	PageID string `json:"pageId"`
	InstID string `json:"instance_id"`
	Desc   string `json:"desc"`
	Active bool   `json:"active"`
	Time   time.Time
}
type User struct {
	Name   string `json:"name"`
	Cookie string `json:"cookie"`
	UA     string `json:"ua"`
	Active bool   `json:"active"`
}
type UserCoupon struct {
	ID     string
	Name   string
	IsStop bool
}
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
}
type PushResponse struct {
	Code int `json:"code"`
}
type Task struct {
	Total   int
	Success int
	Fail    int
}

var logger *zap.Logger
var config Config
var task Task

func main() {

	//初始化日志管理
	loggerInit()
	//初始化toml
	tomlInit()

	defer logger.Sync()

	//筛选有效优惠券和用户
	validCoupons := getValidCoupons()
	validUsers := getValidUsers()
	var wg sync.WaitGroup
	resultChan := make(chan map[string]string)
	//分券分Cookie下发任务
	for _, validCoupon := range validCoupons {
		for userKey, validUser := range validUsers {
			//用于判断是否停止某个抢卷线程
			var userCoupon UserCoupon
			userCoupon.ID = validCoupon.ID
			userCoupon.Name = validUser.Name
			userCoupon.IsStop = false
			//并发任务
			wg.Add(1)
			go processCouponUserPair(validCoupon, userKey, validUser, &userCoupon, &wg, resultChan)
		}
	}
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	//for result := range resultChan {
	//	fmt.Println(result)
	//}
	sendPush(resultChan)
	//queryCounpon()
}

func getValidCoupons() []Coupon {
	// 读取优惠券信息
	file, err := os.Open("./data/config/sq_coupon.json")
	if err != nil {
		logger.Error("优惠券配置文件读取失败！", zap.Error(err))
		return nil
	}
	defer file.Close()

	var couponsByTime map[string][]Coupon
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&couponsByTime)
	if err != nil {
		logger.Error("优惠券配置文件解析失败！", zap.Error(err))
		return nil
	}
	// 时间本地化
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		logger.Error("时区加载失败！", zap.Error(err))
		return nil
	}

	// 遍历优惠券信息获取符合场次信息
	var validCoupons []Coupon
	closestTime, _ := time.ParseInLocation("2006-01-02 15:04:05", "2999-01-02 15:04:05", loc)
	for startTimeStr, coupons := range couponsByTime {
		currentTime := time.Now()
		startTime, err := time.ParseInLocation("2006-01-02 15:04:05", fmt.Sprintf("%s %s", currentTime.Format("2006-01-02"), startTimeStr), loc)
		if err != nil {
			logger.Error("时间解析失败！", zap.Error(err))
			continue
		}
		//如果时间已过 延迟到下一天
		if startTime.Before(currentTime) {
			startTime = startTime.Add(24 * time.Hour)
		}

		//获取符合场次
		if startTime.After(currentTime) && startTime.Before(closestTime) {
			closestTime = startTime
			validCoupons = nil // 清空之前的有效优惠券
			for _, coupon := range coupons {
				coupon.Time = startTime
				if coupon.Active {
					validCoupons = append(validCoupons, Coupon{
						ID:     coupon.ID,
						GdID:   coupon.GdID,
						PageID: coupon.PageID,
						InstID: coupon.InstID,
						Desc:   coupon.Desc,
						Active: coupon.Active,
						Time:   coupon.Time,
					})
				}
			}
		}
	}
	return validCoupons
}

func getValidUsers() []User {
	file, err := os.Open("./data/config/users.json")
	if err != nil {
		logger.Error("用户配置文件读取失败！", zap.Error(err))
		return nil
	}
	defer file.Close()
	var users []User
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&users)
	if err != nil {
		logger.Error("优惠券配置文件解析失败！", zap.Error(err))
		return nil
	}

	// 筛选出 active 为 true 的用户
	var validUsers []User
	for _, user := range users {
		if user.Active {
			validUsers = append(validUsers, user)
		}
	}
	return validUsers
}
func loggerInit() {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000000"))
	}
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	core := zapcore.NewCore(
		encoder,
		zapcore.Lock(os.Stdout),
		zapcore.InfoLevel,
	)

	logger = zap.New(core)

}
func tomlInit() {
	configFile, err := os.Open("./data/config/config.toml")
	if err != nil {
		logger.Error("toml配置文件打开失败！", zap.Error(err))
		return
	}
	defer configFile.Close()

	configBytes, err := ioutil.ReadAll(configFile)
	if err != nil {
		logger.Error("toml配置文件读取失败！", zap.Error(err))
		return
	}
	err = toml.Unmarshal(configBytes, &config)
	if err != nil {
		logger.Error("toml配置文件解析失败！", zap.Error(err))
		return
	}
}
func sendPush(resultChan chan map[string]string) {
	var content []string
	var title string

	for results := range resultChan {
		for couponName, result := range results {
			title = couponName
			content = append(content, result)
		}
	}
	content = append(content, fmt.Sprintf("🎰成功率:%.2f%%	🏆成功:%d	💀失败:%d",
		float64(task.Success)/float64(task.Total)*100.0,
		task.Success, task.Fail))
	//反转原始内容
	var reversedContent []string

	for i := len(content) - 1; i >= 0; i-- {
		reversedContent = append(reversedContent, content[i])
	}
	//转换成字符串
	combinedContent := strings.Join(reversedContent, "\n")

	//pushUrl := fmt.Sprintf("http://www.pushplus.plus/send?token=%s&content=%s&title=%s&topic=%s",
	//	url.QueryEscape(config.PushToken), url.QueryEscape(combinedContent), url.QueryEscape(title), url.QueryEscape("MT_COUPON"))
	pushUrl := fmt.Sprintf("http://www.pushplus.plus/send?token=%s&content=%s&title=%s",
		url.QueryEscape(config.PushToken), url.QueryEscape(combinedContent), url.QueryEscape(title))

	client := resty.New()

	resp, err := client.R().SetHeader("Content-Type", "application/json").Post(pushUrl)

	if err != nil {
		logger.Error("推送失败！", zap.Error(err))
		return
	}
	var response PushResponse
	err = json.Unmarshal(resp.Body(), &response)
	if err != nil {
		logger.Error("推送响应体Json解析失败", zap.Error(err))
		return
	}
	if response.Code == 200 {
		logger.Info("推送成功!", zap.String("响应", string(resp.Body())))
	} else {
		logger.Error("推送失败!", zap.String("响应", string(resp.Body())))
	}
}
