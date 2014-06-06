// cloudpump project main.go
package cloudpump

import (
	"fmt"
	"github.com/crowdmob/goamz/aws"
	"github.com/crowdmob/goamz/cloudwatch"
	_ "github.com/davecgh/go-spew/spew"
	"github.com/kr/pretty"
	"github.com/marpaia/graphite-golang"
	"github.com/pmylund/sortutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	StringFormatExp = regexp.MustCompile("%\\w")
	StringCustExp   = regexp.MustCompile("%\\d")
)

type MonMon struct {
	Checkers map[string]chan bool
}

type CloudWatchMetric struct {
	Dimensions      []cloudwatch.Dimension
	rawDimensions   [][]string
	Namespace       string
	MetricName      string
	Statistics      []string
	Region          string
	CustomKeyFormat string
	CustomStrings   []string
	CustomKey       string
	AwsSecret       string
	AwsKey          string
	cw              *cloudwatch.CloudWatch
}

type MonResult struct {
	CwMetric *CloudWatchMetric
	Resp     *cloudwatch.GetMetricStatisticsResponse
}

func (cwMetric *CloudWatchMetric) SetDimensions(dimensions [][]string) {
	var newDimensions = make([]cloudwatch.Dimension, 0)
	for _, k := range dimensions {
		newDimensions = append(newDimensions, cloudwatch.Dimension{
			Name:  k[0],
			Value: k[1]})
	}
	cwMetric.rawDimensions = dimensions
	cwMetric.Dimensions = newDimensions
}

func (cwMetric *CloudWatchMetric) GetDimensionValue(name string) string {
	fmt.Println("Getting dimension for: ", name, " in :", cwMetric.Dimensions)
	for _, d := range cwMetric.Dimensions {
		if d.Name == name {
			fmt.Println("Returning: ", d.Value)
			return d.Value
		}
	}
	return ""
}

func (cwMetric *CloudWatchMetric) MonCloudWatch(resultC chan *MonResult, quit chan bool) {
	ticker := time.NewTicker(60 * time.Second)
	for {
		now := time.Now()
		prev := now.Add(time.Duration(120) * time.Second * -1)
		request := &cloudwatch.GetMetricStatisticsRequest{
			Dimensions: cwMetric.Dimensions,
			EndTime:    now,
			StartTime:  prev,
			MetricName: cwMetric.MetricName,
			Period:     60,
			Statistics: cwMetric.Statistics,
			Namespace:  cwMetric.Namespace,
		}
		response, err := cwMetric.CW().GetMetricStatistics(request)
		if err != nil {
			fmt.Println(err.Error())
		}
		fmt.Printf("%+v\n", response.GetMetricStatisticsResult.Datapoints)
		select {
		case resultC <- &MonResult{
			CwMetric: cwMetric,
			Resp:     response}:
		default:
			fmt.Println("Result channel full?")
		}

		select {
		case <-quit:
			return
		default:
			<-ticker.C
		}
	}
}

func (mon *MonMon) AddCloudWatchMetric(cwMetric *CloudWatchMetric, resultC chan *MonResult) {
	var quit = make(chan bool)
	go cwMetric.MonCloudWatch(resultC, quit)
	mon.Checkers[cwMetric.Key()] = quit
}

func (mon *MonMon) RemoveCloudWatchMetric(cwMetric *CloudWatchMetric) bool {
	key := cwMetric.Key()
	if quit, ok := mon.Checkers[key]; ok {
		quit <- true
		delete(mon.Checkers, key)
		return true
	} else {
		return false
	}
}

func (mon *MonMon) MonitorAllFoundFor(cwMetric *CloudWatchMetric, resultC chan *MonResult) {
	listRequest := &cloudwatch.ListMetricsRequest{
		MetricName: cwMetric.MetricName,
		Namespace:  cwMetric.Namespace,
		Dimensions: cwMetric.Dimensions}

	listResponse, _ := cwMetric.CW().ListMetrics(listRequest)
	pretty.Print(listResponse.ListMetricsResult.Metrics)

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
		mon.AddCloudWatchMetric(newCwMetric, resultC)
	}
}

func (cwMetric *CloudWatchMetric) CW() *cloudwatch.CloudWatch {
	if cwMetric.cw != nil {
		return cwMetric.cw
	} else {
		now := time.Now()
		auth, err := aws.GetAuth(cwMetric.AwsKey, cwMetric.AwsSecret, "", now)
		if err != nil {
			fmt.Printf("Error: %+v\n", err)
			os.Exit(1)
		}
		cw, _ := cloudwatch.NewCloudWatch(auth, aws.Regions[cwMetric.Region].CloudWatchServicepoint)
		return cw
	}
}

func (cwMetric *CloudWatchMetric) Key() string {
	var flatDimensions = make([]string, 0.0)
	for _, dimension := range cwMetric.Dimensions {
		flatDimensions = append(flatDimensions, fmt.Sprintf("%s:%s", dimension.Name, dimension.Value))
	}
	sortutil.Asc(flatDimensions)
	return fmt.Sprintf("%s,%s,%s,%s", cwMetric.Region, cwMetric.Namespace, cwMetric.MetricName, strings.Join(flatDimensions, ","))
}

func (cwMetric *CloudWatchMetric) GetCustomKey() string {
	fmt.Println("Getting custom key")
	replacer := func(s string) string {
		fmt.Println("Replacer recieved: ", s)
		if StringCustExp.MatchString(s) {
			index, _ := strconv.ParseInt(s[1:2], 0, 0)
			return cwMetric.GetDimensionValue(cwMetric.rawDimensions[index][0])
		}
		switch s {
		case "%n":
			return cwMetric.Namespace
		case "%m":
			return cwMetric.MetricName
		case "%r":
			return cwMetric.Region
		default:
			return ""
		}
	}
	if cwMetric.CustomKey == "" {
		fmt.Println("CustomKey is blank, expanding: ", cwMetric.CustomKeyFormat)
		cwMetric.CustomKey = StringFormatExp.ReplaceAllStringFunc(cwMetric.CustomKeyFormat, replacer)
	}
	return cwMetric.CustomKey
}

func CloudPumpResultToMetrics(in <-chan *MonResult, out chan *graphite.Metric) {
	for res := range in {
		for _, metric := range res.Resp.GetMetricStatisticsResult.Datapoints {
			graphiteMetric := &graphite.Metric{
				res.CwMetric.GetCustomKey(), strconv.FormatFloat(metric.Sum, 'f', 5, 64), metric.Timestamp.Unix(),
			}
			fmt.Printf("%+v\n", graphiteMetric)
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
