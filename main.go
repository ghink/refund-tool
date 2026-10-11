package main

import (
	"flag"

	"refundtool/internal/config"
	"refundtool/internal/logger"
	"refundtool/internal/ui"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径；默认取程序目录或当前目录下的 config.yaml")
	flag.Parse()

	loadErr := config.Load(*configPath)
	logger.Init()

	ui.Run(*configPath, loadErr)
}
