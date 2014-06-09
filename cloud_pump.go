// cloudpump project main.go
package cloudpump

import (
	"fmt"
	_ "github.com/crowdmob/goamz/aws"
	"github.com/crowdmob/goamz/cloudwatch"
	_ "github.com/davecgh/go-spew/spew"
	"github.com/kr/pretty"
	_ "github.com/pmylund/sortutil"
	_ "os"
	"regexp"
	_ "strconv"
	_ "strings"
	"sync"
	"time"
)

var (
	StringFormatExp = regexp.MustCompile("%\\w")
	StringCustExp   = regexp.MustCompile("%\\d")
)

type MonMon struct {
	Checkers       map[string]chan bool
	Searchers      map[string]chan bool
	GenerateStats  bool
	StatsKeyPrefix string
	StatsInterval  int
	lock           sync.RWMutex
}

type MetricSearcher struct {
	Template   *CloudWatchMetric
	MetricKeys map[string]string
}

type MonResult struct {
	CwMetric *CloudWatchMetric
	Resp     *cloudwatch.GetMetricStatisticsResponse
}

func NewMonMon() *MonMon {
	return &MonMon{
		Checkers:  make(map[string]chan bool),
		Searchers: make(map[string]chan bool),
	}
}

func (mon *MonMon) AddCloudWatchMetric(cwMetric *CloudWatchMetric, resultC chan *MonResult) bool {
	mon.lock.Lock()
	defer mon.lock.Unlock()
	metricKey := cwMetric.Key()
	fmt.Println("<---->\nAsked to add: ", metricKey, "\n<---->")
	if _, ok := mon.Checkers[metricKey]; !ok {
		var quit = make(chan bool)
		go cwMetric.MonCloudWatch(resultC, quit)
		mon.Checkers[metricKey] = quit
		return true
	} else {
		fmt.Println("Metric already being monitored: ", metricKey)
		return false
	}
}

func (mon *MonMon) RemoveCloudWatchMetric(cwMetric *CloudWatchMetric) bool {
	return mon.RemoveCloudWatchMetricByKey(cwMetric.Key())
}

func (mon *MonMon) RemoveCloudWatchMetricByKey(key string) bool {
	fmt.Println("<---->\nAsked to remove: ", key, "\n<---->")
	mon.lock.Lock()
	defer mon.lock.Unlock()
	if quit, ok := mon.Checkers[key]; ok {
		quit <- true
		delete(mon.Checkers, key)
		return true
	} else {
		return false
	}
}

func (mon *MonMon) MonitorAllFoundFor(cwMetric *CloudWatchMetric, resultC chan *MonResult) {
	key := cwMetric.Key()
	if _, ok := mon.Searchers[key]; ok {
		fmt.Println("Already watching this template!")
		return
	}

	quit := make(chan bool)
	go mon.SearchMetrics(cwMetric, resultC, quit)
	mon.Searchers[key] = quit
}

func (mon *MonMon) SearchMetrics(cwMetric *CloudWatchMetric, resultC chan *MonResult, quit chan bool) {
	ticker := time.NewTicker(60 * time.Second)
	currentMetrics := make(map[string]bool)
	listRequest := &cloudwatch.ListMetricsRequest{
		MetricName: cwMetric.MetricName,
		Namespace:  cwMetric.Namespace,
		Dimensions: cwMetric.Dimensions}
	for {
		select {
		case <-quit:
			return
		default:
			listResponse, err := cwMetric.CW().ListMetrics(listRequest)
			if err != nil {
				fmt.Println(err.Error())
				<-ticker.C
				continue
			}
			pretty.Print(listResponse.ListMetricsResult.Metrics)
			newMetrics := make(map[string]bool)
			for _, metric := range listResponse.ListMetricsResult.Metrics {
				newCwMetric := &CloudWatchMetric{
					Dimensions:      metric.Dimensions,
					Namespace:       metric.Namespace,
					MetricName:      metric.MetricName,
					rawDimensions:   cwMetric.rawDimensions,
					Statistics:      cwMetric.Statistics,
					Region:          cwMetric.Region,
					CustomKey:       cwMetric.CustomKey,
					CustomKeyFormat: cwMetric.CustomKeyFormat,
					AwsSecret:       cwMetric.AwsSecret,
					AwsKey:          cwMetric.AwsKey}
				newMetrics[cwMetric.Key()] = true
				mon.AddCloudWatchMetric(newCwMetric, resultC)
			}

			for key, _ := range currentMetrics {
				if _, ok := newMetrics[key]; !ok {
					mon.RemoveCloudWatchMetricByKey(key)
				}
			}
			currentMetrics = newMetrics
			<-ticker.C
		}
	}
}

func (mon *MonMon) StatsMap() map[string]int {
	stats := make(map[string]int)
	stats["templates_tracked"] = mon.SearcherCount()
	stats["metrics_tracked"] = mon.CheckerCount()
	return stats
}

func (mon *MonMon) SearcherCount() int {
	mon.lock.RLock()
	defer mon.lock.RUnlock()
	return len(mon.Searchers)
}

func (mon *MonMon) CheckerCount() int {
	mon.lock.RLock()
	defer mon.lock.RUnlock()
	return len(mon.Checkers)
}
