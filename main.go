package main

import (
	"mt_coupons/utils"
	"mt_coupons/worker"
	"os"
	"sync"
)

func main() {
	var task worker.Task
	//初始化日志管理
	logger, logFile := utils.LoggerInit()
	//初始化toml
	config, err := utils.ConfigTomlInit(logger)

	if err != nil {
		logger.Error("toml配置初始化失败")
		os.Exit(1)
	}
	//时间差异
	utils.TimeDiff(&config, logger)
	defer func() {
		// 关闭文件
		logFile.Close()
		// 同步 logger
		logger.Sync()
	}()
	//筛选有效优惠券和用户
	validCoupons := worker.GetValidCoupons(logger)
	validUsers := worker.GetValidUsers(logger)
	var wg sync.WaitGroup
	resultChan := make(chan map[string]string)
	//分券分Cookie下发任务
	for _, validCoupon := range validCoupons {
		for userKey, validUser := range validUsers {
			//用于判断是否停止某个抢卷线程
			var userCoupon worker.UserCoupon
			userCoupon.ID = validCoupon.ID
			userCoupon.Name = validUser.Name
			userCoupon.IsStop = false
			//并发任务
			wg.Add(1)
			go worker.ProcessCouponUserPair(validCoupon, userKey, validUser, &userCoupon, &wg, resultChan, config, logger, &task)
		}
	}
	results := []map[string]string{}
	go func() {
		wg.Wait()
		close(resultChan)

	}()

	for result := range resultChan {
		results = append(results, result)
	}
	utils.SendPush(logger, config, task, results)
	//QueryCoupon()
}
