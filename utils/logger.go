package utils

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"time"
)

func LoggerInit() (*zap.Logger, *os.File) {
	logFile, err := os.OpenFile("./data/log/mt.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic("无法打开日志文件：" + err.Error())
	}

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
		zapcore.DebugLevel, // 或其他日志级别
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
	logger := zap.New(multiCore)
	logger.Debug("--------------------------------------------------------------------------------")

	return logger, logFile
}
