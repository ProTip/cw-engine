package main

import (
	"fmt"
	"github.com/burntsushi/toml"
	"github.com/kr/pretty"
	"github.com/protip/cw-engine/cwengine"
)

type Config struct {
	GraphiteHost   string
	GraphitePort   int
	GraphiteBuffer int
	StatsInterval  int
	StatsKeyPrefix string
	Accounts       []Account
}

type Account struct {
	AwsAccessKey string
	AwsSecretKey string
	Templates    []Template
}

type Template struct {
	cwengine.CloudWatchMetric
	DimensionList [][]string
}

var (
	conf     Config
	cwMonMon *cwengine.MonMon
	resultC  chan *cwengine.MonResult
)

func init() {
	if _, err := toml.DecodeFile("config.toml", &conf); err != nil {
		panic(err)
	}
	cwMonMon = cwengine.NewMonMon()
	resultC = make(chan *cwengine.MonResult, 100000)
	cwMonMon.ResultsToGraphite(resultC, conf.GraphiteHost, conf.GraphitePort, conf.GraphiteBuffer)
	cwMonMon.StatsInterval = conf.StatsInterval
	cwMonMon.StatsKeyPrefix = conf.StatsKeyPrefix
}

func main() {
	fmt.Println("Hello World!!!!!")
	pretty.Print(conf)
	PrimeEngine()
	select {}
}

func PrimeEngine() {
	for _, account := range conf.Accounts {
		ProcessAccount(account)
	}
}

func ProcessAccount(account Account) {
	for _, template := range account.Templates {
		template.AwsKey = account.AwsAccessKey
		template.AwsSecret = account.AwsSecretKey
		ProcessTemplate(template)
	}
}

func ProcessTemplate(template Template) {
	template.SetDimensions(template.DimensionList)
	cwMonMon.MonitorAllFoundFor(&template.CloudWatchMetric, resultC)
}
