package worker

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
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

func GetValidCoupons(logger *zap.Logger) []Coupon {
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

func GetValidUsers(logger *zap.Logger) []User {
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
