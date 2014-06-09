package main

import (
	"fmt"
	"github.com/burntsushi/toml"
	"github.com/protip/cw-engine/cwengine"
)

type Config struct {
	GraphiteHost string
	GraphitePort int

	Accounts []Account
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
	conf Config
)

func init() {
	if _, err := toml.DecodeFile("config.toml", &conf); err != nil {
		panic(err)
	}
}

func main() {
	fmt.Printf("%+v\n", conf)
}
