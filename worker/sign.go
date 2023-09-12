package worker

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os/exec"
	"strings"
)

type Sign struct {
	MtgSig        string `json:"mtgsig"`
	MtFingerprint string `json:"mtFingerprint"`
}

func GenerateSign(user User, postUrl string, logger *zap.Logger) Sign {

	//获取签名Json代码
	signJsCode := fmt.Sprintf(`
		fs.readFile('./data/js/mt212.js', 'UTF-8', async (err, data) => {
			if (err) console.log(err);
			
			const url = '%s';
			const dataObj = {
				"cType": "wx_wallet",
				"fpPlatform": 13,
				"wxOpenId": "",
				"appVersion": "",
				"mtFingerprint": ""
			};
			const h5 = eval(data);
			const h5guard = new h5('%s', '%s');
			console.log(JSON.stringify(await h5guard.sign(url, dataObj)));
			process.exit(0);
		});
	`, postUrl, user.Cookie, user.UA)

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
		outputStr = RemovePangoWarning(outputStr)
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
	return signData
}
func RemovePangoWarning(output string) string {
	// mt212移除Pango警告消息
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
