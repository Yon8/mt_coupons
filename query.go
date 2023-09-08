package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"log"
	"net/url"
	"strings"
)

func queryCounpon() {
	validUsers := getValidUsers()
	var content []string
	for _, validUser := range validUsers {

		queryURL := "https://wx.waimai.meituan.com/weapp/v1/user/coupons/list"
		token := extractTokenFromCookie(validUser.Cookie)
		formData := map[string]string{
			"wm_logintoken": token,
			"page_size":     "40",
			"page_index":    "0",
		}

		client := resty.New()
		resp, err := client.R().
			SetFormData(formData).
			SetHeader("Accept", "application/json, text/plain, */*").
			SetHeader("User-Agent", config.UserAgent).
			SetHeader("Connection", "keep-alive").
			SetHeader("Accept-Encoding", "gzip,compress,br,deflate").
			SetHeader("Content-Type", "application/x-www-form-urlencoded").
			SetHeader("charset", "utf-8").
			SetHeader("wm-ctype", "wxapp").
			SetHeader("charset", "utf-8").
			Post(queryURL)
		if err != nil {
			log.Fatal(err)
		}

		// 解析JSON
		var responseData map[string]interface{}
		if err := json.Unmarshal(resp.Body(), &responseData); err != nil {
			fmt.Println("解析JSON失败：", err)
			return
		}

		couponList, ok := responseData["data"].(map[string]interface{})["coupon_listIterator"].([]interface{})
		if !ok {
			fmt.Println("找不到coupon_listIterator")
			return
		}
		content = append(content, fmt.Sprintf("\t\t用户：%s", validUser.Name))
		content = append(content, "| 券名        | 金额   |  描述  |")
		content = append(content, "| --------   | -----:  | :----:  |")

		for _, coupon := range couponList {
			couponData, ok := coupon.(map[string]interface{})
			if !ok {
				fmt.Println("无法解析coupon数据")
				continue
			}
			if couponData["coupon_logo_type"].(float64) == 2 && couponData["status"].(float64) == 1 {
				content = append(content, fmt.Sprintf("| %s      | %s   |  %s     |", couponData["title"].(string),
					fmt.Sprintf("%s减%.0f ", couponData["price_limit"].(string), couponData["amount"].(float64)),
					couponData["valid_time_desc"].(string)))
			}

		}
	}
	// 将切片内容合并为一个字符串，以换行分隔
	combinedContent := strings.Join(content, "\n")
	pushUrl := fmt.Sprintf("http://www.pushplus.plus/send?token=%s&content=%s&title=%s&topic=%s&template=markdown",
		url.QueryEscape(config.PushToken), url.QueryEscape(combinedContent), url.QueryEscape("MagicCard"), url.QueryEscape("MT_COUPON"))
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
func extractTokenFromCookie(cookie string) string {
	// 使用分号分割 cookie 字符串
	cookieParts := strings.Split(cookie, ";")

	// 遍历分割后的部分，寻找包含 "token=" 的部分
	for _, part := range cookieParts {
		trimmedPart := strings.TrimSpace(part)
		if strings.HasPrefix(trimmedPart, "token=") {
			return strings.TrimPrefix(trimmedPart, "token=")
		}
	}

	return ""
}
