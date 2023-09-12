package main

import (
	"encoding/json"
	"fmt"
	"github.com/pelletier/go-toml"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io/ioutil"
	"os"
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

type Task struct {
	Total   int
	Success int
	Fail    int
}

var logger *zap.Logger
var config Config
var task Task
var logFile *os.File

func main() {

	//初始化日志管理
	loggerInit()
	//初始化toml
	tomlInit()
	//时间差异
	timeDiff()
	defer func() {
		// 关闭文件
		logFile.Close()
		// 同步 logger
		logger.Sync()
	}()
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
	queryCounpon()
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
	logFile, _ = os.OpenFile("./data/log/mt.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)

	// 创建 Zap 编码器配置
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000000"))
	}
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// 创建文件写入器
	fileWriteSync := zapcore.AddSync(logFile)

	// 创建文件核心（core）以输出到文件
	fileCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), // 或者使用其他适合您的编码器
		fileWriteSync,
		zapcore.InfoLevel, // 或其他日志级别
	)

	// 创建控制台写入器
	consoleWriteSync := zapcore.Lock(os.Stdout)

	// 创建控制台核心（core）以输出到控制台
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig), // 控制台编码器
		consoleWriteSync,
		zapcore.InfoLevel, // 或其他日志级别
	)

	// 创建多核心（multi-core），将日志同时输出到文件和控制台
	cores := []zapcore.Core{fileCore, consoleCore}
	multiCore := zapcore.NewTee(cores...)

	// 创建 Zap Logger
	logger = zap.New(multiCore)

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
