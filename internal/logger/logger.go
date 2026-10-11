package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"refundtool/internal/config"
)

// L is the application logger.
var L *zap.Logger

func Init() {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	level := zapcore.InfoLevel
	if config.Debug {
		level = zapcore.DebugLevel
	}

	cores := []zapcore.Core{
		zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), zapcore.Lock(os.Stdout), level),
	}

	if logFile := config.Log.File; logFile != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    config.Log.MaxSize,
			MaxBackups: config.Log.MaxBackups,
			MaxAge:     config.Log.MaxAge,
			Compress:   config.Log.Compress,
			LocalTime:  true,
		}
		cores = append(cores, zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(fileWriter), level))
	}

	L = zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}
