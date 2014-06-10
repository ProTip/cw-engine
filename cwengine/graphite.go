package cwengine

import (
	"fmt"
	"github.com/marpaia/graphite-golang"
	"strconv"
	"time"
)

func (mon *MonMon) ResultsToGraphite(resultC <-chan *MonResult, host string, port int, bufferSize int) {
	var metricC = make(chan *graphite.Metric, bufferSize)
	go CloudPumpResultToMetrics(resultC, metricC)
	go MetricsToGraphite(metricC, host, port)
	go mon.StatsToGraphite(metricC)
}

func (mon *MonMon) StatsToGraphite(out chan *graphite.Metric) {
	if mon.StatsInterval == 0 {
		mon.StatsInterval = 5
	}
	ticker := time.NewTicker(time.Duration(mon.StatsInterval) * time.Second)
	for _ = range ticker.C {
		for k, v := range mon.StatsMap() {
			graphiteMetric := &graphite.Metric{
				mon.StatsKeyPrefix + "." + k, strconv.FormatFloat(float64(v), 'f', 5, 64), time.Now().Unix(),
			}
			out <- graphiteMetric
		}
	}
}

func CloudPumpResultToMetrics(in <-chan *MonResult, out chan *graphite.Metric) {
	for res := range in {
		for _, metric := range res.Resp.GetMetricStatisticsResult.Datapoints {
			graphiteMetric := &graphite.Metric{
				res.CwMetric.GetCustomKey(), strconv.FormatFloat(metric.Sum, 'f', 5, 64), metric.Timestamp.Unix(),
			}
			out <- graphiteMetric
		}
	}
}

func MetricsToGraphite(c <-chan *graphite.Metric, host string, port int) {
	//Must be able to connect to graphite first or conn will be nill
	//Con is not exported so we can't check it later
	var Graphite *graphite.Graphite
	for {
		Graphite = &graphite.Graphite{Host: host, Port: port}
		err := Graphite.Connect()
		if err != nil {
			fmt.Println("Unable to connect to graphite")
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}
	for metric := range c {
		for {
			fmt.Println("Sending metric: ", metric)
			err := Graphite.SendMetric(*metric)
			if err != nil {
				//Sending the metric has failed, likely due to a connection issue
				//Attempt to reconnect and then restart sending this result
				fmt.Println("Sending failed, attempting to reconnect")
				for {
					if err := Graphite.Connect(); err == nil {
						break
					}
					fmt.Println("Graphite reconnnect failed: ", err.Error())
					time.Sleep(5 * time.Second)
				}
				continue
			}
			break
		}
	}
}
