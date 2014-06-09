package cloudpump

import (
	"fmt"
	"github.com/crowdmob/goamz/aws"
	"github.com/crowdmob/goamz/cloudwatch"
	"github.com/pmylund/sortutil"
	"os"
	"strconv"
	"strings"
	"time"
)

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
	Backfill        int
	Period          int
	Priority        bool
	Interval        int
	currentInterval int
	cw              *cloudwatch.CloudWatch
	lastUpdated     time.Time
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
	ticker := time.NewTicker(time.Duration(cwMetric.Interval) * time.Second)
	if cwMetric.Period == 0 {
		cwMetric.Period = 60
	}

	for {
		now := time.Now()
		request := &cloudwatch.GetMetricStatisticsRequest{
			Dimensions: cwMetric.Dimensions,
			EndTime:    now,
			StartTime:  cwMetric.UpdateFrom(),
			MetricName: cwMetric.MetricName,
			Period:     cwMetric.Period,
			Statistics: cwMetric.Statistics,
			Namespace:  cwMetric.Namespace,
		}
		response, err := cwMetric.CW().GetMetricStatistics(request)
		if err != nil {
			fmt.Println(err.Error())
		} else {
			cwMetric.lastUpdated = now
			fmt.Printf("%+v\n", response.GetMetricStatisticsResult.Datapoints)
			select {
			case resultC <- &MonResult{
				CwMetric: cwMetric,
				Resp:     response}:
			default:
				fmt.Println("Result channel full?")
			}
		}
		select {
		case <-quit:
			return
		default:
			c := ticker.C
			<-c
		}
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

func (cwMetric *CloudWatchMetric) UpdateFrom() time.Time {
	if cwMetric.lastUpdated.IsZero() {
		cwMetric.lastUpdated = (time.Now().Add(time.Duration(cwMetric.Backfill) * time.Second * -1))
	}
	return cwMetric.lastUpdated
}
