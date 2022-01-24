#  Fluent Bit Output Plugin for Log Derived Metrics

![ Show image of grafana dashboard using supported Prometheus metric types](https://github.com/neiman-marcus/fluent-bit-out-prometheus-metrics/blob/staging/out-prometheus-metrics-dashboard.png "Demo grafana dashboard showing supported Prometheus metric types")
## Purpose
To add missing Log Derived Metrics functionality to Fluent Bit.  When combined with Grafana Loki, Prometheus, Cortex, and Grafana UI, you get a fully functional Data Observability Platform.  

## Design
The Output plugin leverages the prometheus pushgateway API by pushing the tracked metrics to pushgateway backed with persistent store.  This allows for a fire-and-forget architecture to prevent fluent bit from blocking.   Prometheus treats pushgateway as a metric scrape target.

While the plugin pairs nicely with Grafana Loki's Output plugin, it can be used standalone or coupled with other output plugins.

---
## Configuration Parameters

| Key                      | Description                                                                                                                                                                                                                                          | Required | Default | Valid Options | Notes                                                                                                    |
|:-------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------| :--- | :--- | :--- |:---------------------------------------------------------------------------------------------------------|
| id                       | Plugin instance id                                                                                                                                                                                                                                   | Yes | | | Must be unique per \[OUTPUT\] section in a single fluent-bit.conf                                        |
| job                      | Prometheus job label                                                                                                                                                                                                                                 | Yes | | |                                                                                                          |
| url                      | HTTP Url for destination push gateway                                                                                                                                                                                                                | Yes | | | Ex. http://127.0.0.1:9091                                                                                |
| push_gateway_retries     | Number of retry attempts to connect to push gateway                                                                                                                                                                                                  | No | 3 | |                                                                                                          |
| metric\_type             | Prometheus metric type                                                                                                                                                                                                                               | Yes | none | Counter, Gauge, Summary, Histogram |                                                                                                          |
| metric\_name             | Metric name sent to Prometheus                                                                                                                                                                                                                       | Yes | | |                                                                                                          |
| metric\_help             | Help string associated with metric                                                                                                                                                                                                                   | Yes | | | Enclose in double quotes                                                                                 |
| metric\_grouping         | Grouping of entries, in particular if you have more than one instance! It is a list of *name:value* pairs which lead to a certain grouping of metrics in Prometheus, cf. https://github.com/prometheus/pushgateway/blob/master/README.md#put-method | No | | | Although not required, a grouping by instance is recommended. <br><br>Ex. instance: ${HOSTNAME}          |
| metric\_constant\_labels | Static JSON formatted key\/value pairs to index metric                                                                                                                                                                                               | No | | | Ex. {"environment":"prod", "source":"fluent-bit"}                                                        |
| metric\_variable\_labels | Comma separated list of fluent bit fields to index metric.  This is the key to log derived metrics.  The value of these keys will vary depending on log line content.                                                                                | No | | | Will be appended to any metric\_constant\_labels is configured. <br><br>Ex. origin, status\_code, method |

## Metric Specific Configurations

In addition to keys noted above.<br>


### Counter
See [Prometheus Counter](https://prometheus.io/docs/concepts/metric_types/#counter) for details.
No additional parameters are required Counter at this time.

### Summary
See [Prometheus Summary](https://prometheus.io/docs/concepts/metric_types/#summary) for details.

| Key | Description | Required for Specific Metric Type | Default | Valid Options | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| metric\_summary\_observe\_key | Single fluent bit field to observe for Summary metric type. | Yes | | | |

### Histogram
See [Prometheus Histogram](https://prometheus.io/docs/concepts/metric_types/#histogram) for details.

| Key | Description | Required for Specific Metric Type | Default | Valid Options | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| metric\_histogram\_bucket\_type | Histogram bucket distribution | Yes | | Linear, Exponential | |
| metric\_histogram\_observe\_key | Single fluent bit field to observe for Histogram metric type. | Yes | | | |

#### Histogram with Linear Bucket Type
Creates 'count' buckets, each 'width' wide, where the lowest bucket has an upper bound of 'start'. The final +Inf bucket is not counted and is not included.
See [Prometheus SDK reference - LinearBuckets()](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#LinearBuckets) for details.

| Key | Description | Required for Specific Metric Type | Default | Valid Options | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| metric\_histogram\_linear\_buckets\_count | Count of buckets | Yes | | \> 0 | Ex. 5|
| metric\_histogram\_linear\_buckets\_width | Width of bucket | Yes | | | Ex. 5 |
| metric\_histogram\_linear\_buckets\_start | Lowest bucket has an upper bound of Start | Yes | | | Ex. 20 |

#### Histogram with Exponential Bucket Type
Creates 'count' buckets, where the lowest bucket has an upper bound of 'start' and each following bucket's upper bound is 'factor' times the previous bucket's upper bound. The final +Inf bucket is not counted and not included.
See [Prometheus SDK reference - ExponentialBuckets()](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#ExponentialBuckets) for details.

| Key | Description | Required for Specific Metric Type | Default | Valid Options | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| metric\_histogram\_exponential\_buckets\_count | Count of buckets | Yes | | \> 0 | Ex. 5|
| metric\_histogram\_exponential\_buckets\_factor | Each additional bucket upper bound is Factor times the previous bucket's upper bound | Yes | | | Ex. 1.5 |
| metric\_histogram\_exponential\_buckets\_start | Lowest bucket has an upper bound of Start | Yes | | | Ex. 20 |

### Gauge
See [Prometheus Gauge](https://prometheus.io/docs/concepts/metric_types/#gauge) for details.

| Key | Description | Required for Specific Metric Type | Default | Valid Options | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| metric\_gauge\_method | Method selected to change the value of the Gauge | Yes | | Set, Add, Sub, Inc, Dec | Set, Add, and Sub require a key as input.  Inc and Dec only increment or decrement the gauge by 1.  |
| metric\_gauge\_set\_key | Single fluent bit field as input to Set method. | Yes with Set| | | |
| metric\_gauge\_add\_key | Single fluent bit field as input to Add method. | Yes with Add| | | |
| metric\_gauge\_sub\_key | Single fluent bit field as input to Sub method. | Yes with Sub| | | |

## Example Configurations
The example folder contains a set of configurations showing each type of metric currently supported by the plugin plus Grafana Loki logs.  These were used to create the dashboard pictured above.
* Check out [https://github.com/neiman-marcus/fluent-bit-data-observability-platform](https://github.com/neiman-marcus/fluent-bit-data-observability-platform) for a full environment leveraging the example configuration. 

---
## Using the demo environment
#### Requirements:
* Either Docker Desktop, or docker and docker-compose.
* Make
* Curl

### Quick start
```
make all
```

### Build instructions
```
make build
```

### Start Prometheus Push Gateway and Fluent bit
```
make start
```

### Show current metrics
Execute repeatedly to see the metrics generated using the plugin.
```
make show-metrics
```

### Show current logs
Execute repeatedly to see the latest container logs
```
make logs
```

### Stop both containers
Shutdown demo environment.
```
make stop
```

### Notes
The **build-plugin** configuration in the Makefile is referenced by the docker build environment.

#### Projects referenced above:
https://fluentbit.io/
<br>
https://grafana.com/oss/grafana/
<br>
https://grafana.com/oss/loki/
<br>
https://grafana.com/oss/prometheus/
<br>
https://grafana.com/oss/cortex/


**Reference Talk:** [FluentCon 2021: Fluent Bit - Swiss Army Tool of Observability Data Ingestion](https://sched.co/iKok)

# Author

* [**Michael Marshall**](mailto:michael_marshall@neimanmarcus.com) - Project creation and development.

## Conduct / Contributing / License

* Refer to our contribution guidelines to contribute to this project. See [CONTRIBUTING.md](https://github.com/neiman-marcus/nmg-sonarqube/tree/master/CONTRIBUTING.md).
* All contributions must follow our code of conduct. See [CONDUCT.md](https://github.com/neiman-marcus/nmg-sonarqube/tree/master/CONDUCT.md).
* This project is licensed under the Apache 2.0 license. See [LICENSE](https://github.com/neiman-marcus/nmg-sonarqube/tree/master/LICENSE).

## Acknowledgments
Thank you to the teams who create and maintain the individual projects listed above.