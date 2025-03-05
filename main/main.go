package main

import (
	"fmt"
	"time"

	"github.com/FurmanovVitaliy/logger"
	config "github.com/FurmanovVitaliy/logger/main/connfig"
)

type data struct {
	Dirt string
	Old  time.Duration
}
type json struct {
	Waight int
	Data   data
}

func main() {
	d := &data{
		Dirt: "sdfsdf",
		Old:  time.Minute * 15,
	}

	j := &json{
		Waight: 15,
		Data:   *d,
	}

	log := logger.NewLogger(
		logger.WithLevel("debug"), logger.IsJSON(false),
		logger.WithSource(true), logger.IsPrettyOut(true))

	cfg := config.MustLoadByPath("./main/connfig/local.yaml")

	log.Error("Hello World", logger.ErrAttr(fmt.Errorf("error")))
	log = log.WithGroup("config")
	log.Info("Hello World")

	log.Debug("configuration loaded", "config", cfg.LogValue())
	log.Info("anu atr", logger.AnyAttr("struct ", j))

}
