package utils

import (
	"github.com/pelletier/go-toml"
	"go.uber.org/zap"
	"io/ioutil"
	"mt_coupons/worker"
	"os"
)

func ConfigTomlInit(logger *zap.Logger) (worker.Config, error) {
	configFile, err := os.Open("./data/config/config.toml")
	if err != nil {
		logger.Error("toml配置文件打开失败！", zap.Error(err))
		return worker.Config{}, err
	}
	defer configFile.Close()

	configBytes, err := ioutil.ReadAll(configFile)
	var config worker.Config
	if err != nil {
		logger.Error("toml配置文件读取失败！", zap.Error(err))
		return worker.Config{}, err
	}
	err = toml.Unmarshal(configBytes, &config)
	if err != nil {
		logger.Error("toml配置文件解析失败！", zap.Error(err))
		return worker.Config{}, err
	}
	return config, nil
}
