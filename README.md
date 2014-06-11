# cw-engine


## Overview
cw-engine is a bundled monitoring "engine" package and daemon, CloudPump.  

## cw-engine
Engines are envisioned as packages that provide an API to a systems for monitoring a particular resource, CloudWatch in this case, which are then consumed in an application exposing the external API.  This external API may be configuration files, REST endpoints, etc.  They abstract away the concurrency and connection details, provide an easy to use API, and are hot-reconfigurable.

## CloudPump
CloudPump wraps the cw-engine in a daemon and currently provides and external API via a TOML configuration file.  Metrics retrieved from CloudWatch are then fed into a graphite compatible server over TCP.

## Configuration
CloudPump's configuration is currently handled through TOML.  In the future this can easily be extended to JSON support as well.  Bellow you will see an excerpt from the sample config.  Most of it is self explanatory.  Multiple accounts are supported.  Regoins can be specified.  `StatsInterval` determins how often we send cwengine stats to graphite.  `StatsKeyPrefix` will be the graphite prefix for those stats:
``` toml
GraphiteHost = "192.168.1.2"
GraphitePort = 2010
StatsInterval = 10
StatsKeyPrefix = "some_product.dev_environment.cloudpump"

[[Accounts]]
AwsAccessKey = "?"
AwsSecretKey = "?"
	[[Accounts.Templates]]
		Namespace = "AWS/RDS"
		Region = "ap-southeast-2"
		Backfill = 300
		Period = 60
		Interval = 120
		CustomKeyFormat = "some_product.dev_environment.rds-stuff.%0.%m"
		Statistics = ["Sum"]
		DimensionList = [
			["DBInstanceIdentifier",""]]
```

### Templates
When you get down to it, templates are nothing more than partially-filled CloudWatchMetric structs.  That is, they are conceptual within the cwengine package itself.  Here is a brief overview of the values present:
* `Namespace` this it the CloudWatch namespace the metric(s) you would like are located in.
* `Region` is the AWS region your CloudWatch metrics are stored in.
* `Backfill` determines how far back, in seconds, we reach for metrics on the first grab.  Here we get the past 5 minutes on the first fetch.
* `Period` the period you would like in seconds.  This is a CloudWatch term and determines the number of seconds each datapoint represents.
* `Interval` how often we would like to check for this metric.  API calls can get expensive if you are trying to update all your metrics every minute, so crank this up high if you don't need them right away.
* `CustomKeyFormat` allows you to specify a graphite key format string.  Discussed below.
* `Statistics` is another CloudWatch term.  Only "Sum" is supported currently.
* `DimensionList` is an array of string arrays, each size 2.  In the inner arrays [0] is the dimension name with [1] being the value.
* `MetricName` isn't listed, but it could be :)

So what's going on here?  I have not specified the `MetricName`; all MetricNames returned will be tracked.  I specified a dimension name without a value; all metrics that have a dimension of `DBInstanceIdentifier` will be tracked regardless of their value.  I could have specified these things to specify an individual metric though.  That brings us to the `CustomKeyFormat`

#### CustomKeyFormat
This allows you to craft a graphite key using fields from the returned metric(s).  Here is how it works:
* `%m` will be replaced for the metric name
* `%n` will be replaced for the metric namespace
* `%0` will be replaced by the dimension value for the name you specified in index 0 of `DimensionList`.  CloudWatch supports 10 dimensions per metric, so you can use 0-9.
