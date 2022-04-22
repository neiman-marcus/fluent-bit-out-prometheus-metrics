package main

import (
	"C"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

const pluginName = "prometheus_metrics"

var re *regexp.Regexp

type Histogram struct {
	BucketType string
	ObserveKey string
	LinearBucketData
	ExponentialBucketData
}
type LinearBucketData struct {
	Count string
	Width string
	Start string
}
type ExponentialBucketData struct {
	Count  string
	Factor string
	Start  string
}

type Summary struct {
	ObserveKey string
}

type Gauge struct {
	Method string
	SetKey string
	AddKey string
	SubKey string
}

type Group struct {
	Name  string
	Value string
}

type MetricData struct {
	Gauge
	Summary
	Histogram
	Type           string
	Name           string
	Help           string
	ConstantLabels prometheus.Labels
	Grouping       []Group
	VariableLabels []string
}

// SetMetricType Set context metric_type
// Required: Yes
// Values: Counter, Gauge, Summary, Histogram
func (m *MetricData) SetMetricType(t string) {
	m.Type = ConfigKeyQuoteTrim(t)
}

// SetMetricName Set context metric_name
// Required: Yes
func (m *MetricData) SetMetricName(n string) {
	m.Name = ConfigKeyQuoteTrim(n)
}

// SetMetricHelp Set context metric_help
// Required: Yes
func (m *MetricData) SetMetricHelp(h string) {
	m.Help = ConfigKeyQuoteTrim(h)
}

func (m *MetricData) IsCounter() bool {
	return m.Type == "Counter"
}

func (m *MetricData) IsGauge() bool {
	return m.Type == "Gauge"
}

func (m *MetricData) IsHistogram() bool {
	return m.Type == "Histogram"
}

func (m *MetricData) IsExponentialBucket() bool {
	return m.Histogram.BucketType == "Exponential"
}

func (m *MetricData) IsLinearBucket() bool {
	return m.Histogram.BucketType == "Linear"
}

func (m *MetricData) IsSummary() bool {
	return m.Type == "Summary"
}

// PluginContext Core context data structure unique per instance
type PluginContext struct {
	MetricData
	Metric
	ID                      string
	LogLevel                string
	Job                     string
	URL                     string
	Pusher                  *push.Pusher
	Logger                  log.Logger
	PushGatewayRetries      int64
	PushGatewayRetryCounter int64
}

// SetPluginID Set context id
// Required: Yes
// Note: This Must be unique per [OUTPUT] section in a single fluent-bit.conf
func (p *PluginContext) SetPluginID(i string) {
	p.ID = i
}

// SetPluginLogLevel Set context LogLevel
// Required: No
// Values: info, warn, error, debug
// Default: error
func (p *PluginContext) SetPluginLogLevel(l string) {
	if len(l) != 0 {
		p.LogLevel = strings.ToLower(l)
	} else {
		p.LogLevel = "error"
	}
}

// SetPluginJobName Set context Job
// Required: Yes
func (p *PluginContext) SetPluginJobName(j string) {
	p.Job = j
}

// SetPushGatewayURL Set push gateway context Url
// Required: Yes
func (p *PluginContext) SetPushGatewayURL(u string) {
	p.URL = u
}

// SetPushGatewayRetries Set context Push_gateway_retries
// Required: Yes
func (p *PluginContext) SetPushGatewayRetries(u string, logger log.Logger) {
	if len(u) != 0 {
		var err error
		p.PushGatewayRetries, err = strconv.ParseInt(u, 10, 64)
		if err != nil {
			level.Error(logger).Log("msg", "push_gateway_retries not a valid integer, defaulting to 3.", err)
			p.PushGatewayRetries = 3
		}
	} else {
		level.Error(logger).Log("msg", "push_gateway_retries set to nil, defaulting to 3.")
		p.PushGatewayRetries = 3
	}
}

// SetMetricConstantLabels Set context metric_constant_labels
// Required: No
func (m *MetricData) SetMetricConstantLabels(l string, logger log.Logger) {
	if len(l) != 0 {
		err := json.Unmarshal([]byte(l), &m.ConstantLabels)
		if err != nil {
			level.Error(logger).Log("msg", "Metric Label JSON issue", "input", l, "err", err)
			panic(err)
		}
	}
}

// SetMetricGrouping func (m *MetricData) SetMetricGrouping(g string, logger log.Logger) { Set context metric_grouping
// Required: No
func (m *MetricData) SetMetricGrouping(g string, logger log.Logger) {
	if len(g) != 0 {
		groups := strings.Split(StripWhitespace(g), ",")
		for _, group := range groups {
			nameValue := strings.Split(StripWhitespace(group), ":")
			if len(nameValue) != 2 {
				level.Warn(logger).Log("msg", "Metric Grouping must consist of name:value pairs", "grouping", g, "ignoring", group)
			} else {
				level.Debug(logger).Log("msg", "Append grouping", "name", nameValue[0], "value", nameValue[1])
				m.Grouping = append(m.Grouping, Group{nameValue[0], nameValue[1]})
			}
		}
	}
}

// SetMetricVariableLabels Set context metric_variable_labels
// Required: No
func (p *PluginContext) SetMetricVariableLabels(k string) {
	if len(k) != 0 {
		p.MetricData.VariableLabels = strings.Split(StripWhitespace(k), ",")
	}
}

// SetMetricSummaryObserveKey Set context metric_summary_observe_key
// Required with Summary: Yes
func (p *PluginContext) SetMetricSummaryObserveKey(k string, logger log.Logger) {
	if len(k) != 0 {
		p.MetricData.Summary.ObserveKey = k
	} else {
		level.Error(logger).Log("msg", "metric_summary_observe_key not populated")
		panic(1)
	}

}

// SetMetricHistogramBucketType Set context metric_histogram_bucket_type
// Required with Histogram: Yes
// Values: Linear, Exponential
func (p *PluginContext) SetMetricHistogramBucketType(c string, logger log.Logger) {
	if len(c) != 0 {
		p.MetricData.Histogram.BucketType = c
	} else {
		level.Error(logger).Log("msg", "metric_histogram_bucket_type not populated")
		panic(1)
	}
}

// Linear Buckets

// SetMetricHistogramLinearBucketsCount Set context metric_histogram_linear_buckets_count
// Required with Histogram Linear: Yes
func (p *PluginContext) SetMetricHistogramLinearBucketsCount(c string, logger log.Logger) {
	if len(c) != 0 {
		p.MetricData.Histogram.LinearBucketData.Count = c
	} else {
		level.Error(logger).Log("msg", "metric_histogram_linear_buckets_count not populated")
		panic(1)
	}
}

// SetMetricHistogramLinearBucketsWidth Set context metric_histogram_linear_buckets_width
// Required with Histogram Linear: Yes
func (p *PluginContext) SetMetricHistogramLinearBucketsWidth(w string, logger log.Logger) {
	if len(w) != 0 {
		p.MetricData.Histogram.LinearBucketData.Width = w
	} else {
		level.Error(logger).Log("msg", "metric_histogram_linear_buckets_width not populated")
		panic(1)
	}
}

// SetMetricHistogramLinearBucketsStart Set context metric_histogram_linear_buckets_start
// Required with Histogram Linear: Yes
func (p *PluginContext) SetMetricHistogramLinearBucketsStart(s string, logger log.Logger) {
	if len(s) != 0 {
		p.MetricData.Histogram.LinearBucketData.Start = s
	} else {
		level.Error(logger).Log("msg", "metric_histogram_linear_buckets_start not populated")
		panic(1)
	}
}

// Exponential Buckets

// SetMetricHistogramExponentialBucketsCount Set context metric_histogram_exponential_buckets_count
// Required with Histogram Exponential: Yes
func (p *PluginContext) SetMetricHistogramExponentialBucketsCount(c string, logger log.Logger) {
	if len(c) != 0 {
		p.MetricData.Histogram.ExponentialBucketData.Count = c
	} else {
		level.Error(logger).Log("msg", "metric_histogram_exponential_buckets_count not populated")
		panic(1)
	}
}

// SetMetricHistogramExponentialBucketsFactor Set context metric_histogram_exponential_buckets_factor
// Required with Histogram Exponential: Yes
func (p *PluginContext) SetMetricHistogramExponentialBucketsFactor(w string, logger log.Logger) {
	if len(w) != 0 {
		p.MetricData.Histogram.ExponentialBucketData.Factor = w
	} else {
		level.Error(logger).Log("msg", "metric_histogram_exponential_buckets_factor not populated")
		panic(1)
	}
}

// SetMetricHistogramExponentialBucketsStart Set context metric_histogram_exponential_buckets_start
// Required with Histogram Exponential: Yes
func (p *PluginContext) SetMetricHistogramExponentialBucketsStart(s string, logger log.Logger) {
	if len(s) != 0 {
		p.MetricData.Histogram.ExponentialBucketData.Start = s
	} else {
		level.Error(logger).Log("msg", "metric_histogram_exponential_buckets_start not populated")
		panic(1)
	}
}

// SetMetricHistogramObserveKey Set context metric_histogram_observe_key
// Required with Histogram: Yes
func (p *PluginContext) SetMetricHistogramObserveKey(k string, logger log.Logger) {
	if len(k) != 0 {
		p.MetricData.Histogram.ObserveKey = k
	} else {
		level.Error(logger).Log("msg", "metric_histogram_observe_key not populated")
		panic(1)
	}

}

// SetMetricGaugeMethod Set context metric_gauge_method
// Required with Gauge: Yes
// Values: Set, Add, Sub, Inc, Dec
func (p *PluginContext) SetMetricGaugeMethod(s string, logger log.Logger) {
	if len(s) != 0 {
		p.MetricData.Gauge.Method = s
	} else {
		level.Error(logger).Log("msg", "metric_gauge_method not populated")
		panic(1)
	}
}

// SetMetricGaugeSetKey Set context metric_gauge_set_key
// Required with Gauge: Yes
func (p *PluginContext) SetMetricGaugeSetKey(s string, logger log.Logger) {
	if len(s) != 0 {
		p.MetricData.Gauge.SetKey = s
	} else {
		level.Error(logger).Log("msg", "metric_gauge_set_key not populated")
		panic(1)
	}
}

// SetMetricGaugeAddKey Set context metric_gauge_add_key
// Required with Gauge: Yes
func (p *PluginContext) SetMetricGaugeAddKey(s string, logger log.Logger) {
	if len(s) != 0 {
		p.MetricData.Gauge.AddKey = s
	} else {
		level.Error(logger).Log("msg", "metric_gauge_add_key not populated")
		panic(1)
	}
}

// SetMetricGaugeSubKey Set context metric_gauge_sub_key
// Required with Gauge: Yes
func (p *PluginContext) SetMetricGaugeSubKey(s string, logger log.Logger) {
	if len(s) != 0 {
		p.MetricData.Gauge.SubKey = s
	} else {
		level.Error(logger).Log("msg", "metric_gauge_sub_key not populated")
		panic(1)
	}
}

type FBCounter struct {
	Handle *prometheus.CounterVec
}

type FBGauge struct {
	Handle *prometheus.GaugeVec
}

type FBHistogram struct {
	Handle *prometheus.HistogramVec
}

type FBSummary struct {
	Handle *prometheus.SummaryVec
}

type Metric struct {
	FBCounter
	FBGauge
	FBHistogram
	FBSummary
}

func (c *FBCounter) NewMetric(p *PluginContext) {
	c.Handle = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        p.Name,
		Help:        p.Help,
		ConstLabels: p.ConstantLabels,
	}, p.VariableLabels)
}

func (g *FBGauge) NewMetric(p *PluginContext) {
	g.Handle = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        p.Name,
		Help:        p.Help,
		ConstLabels: p.ConstantLabels,
	}, p.VariableLabels)
}

func (h *FBHistogram) NewMetric(p *PluginContext, Buckets []float64) {
	h.Handle = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        p.Name,
		Help:        p.Help,
		ConstLabels: p.ConstantLabels,
		Buckets:     Buckets,
	}, p.VariableLabels)
}

func (s *FBSummary) NewMetric(p *PluginContext) {
	s.Handle = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:        p.Name,
		Help:        p.Help,
		ConstLabels: p.ConstantLabels,
	}, p.VariableLabels)
}

// ConfigKeyQuoteTrim Trims surrounding double quotes if present
func ConfigKeyQuoteTrim(key string) string {
	if len(key) > 0 && key[0] == '"' {
		key = key[1:]
	}
	if len(key) > 0 && key[len(key)-1] == '"' {
		key = key[:len(key)-1]
	}
	return key
}

// StripWhitespace Strip whitespace if present
func StripWhitespace(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, str)
}

// ExtractFloat Convert string to float64.  Works for Int as well.
func ExtractFloat(str string) (float64, error) {
	var ret float64
	var err error

	if len(re.FindStringIndex(str)) > 0 {
		ret, err = strconv.ParseFloat(re.FindString(str), 0)
	}

	return ret, err
}

// toStringSlice: Code borrowed from Loki
// prevent base64-encoding []byte values (default json.Encoder rule) by
// converting them to strings
func toStringSlice(slice []interface{}) []interface{} {
	var s []interface{}
	for _, v := range slice {
		switch t := v.(type) {
		case []byte:
			s = append(s, string(t))
		case map[interface{}]interface{}:
			s = append(s, toStringMap(t))
		case []interface{}:
			s = append(s, toStringSlice(t))
		default:
			s = append(s, t)
		}
	}
	return s
}

// toStringMap: Code borrowed from Loki
func toStringMap(record map[interface{}]interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range record {
		key, ok := k.(string)
		if !ok {
			continue
		}
		switch t := v.(type) {
		case []byte:
			m[key] = string(t)
		case map[interface{}]interface{}:
			m[key] = toStringMap(t)
		case []interface{}:
			m[key] = toStringSlice(t)
		default:
			m[key] = v
		}
	}

	return m
}

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, pluginName, "Exporting custom Prometheus metrics")
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {

	pCtx := new(PluginContext)

	// Initialize push gateway retry counter to 0
	pCtx.PushGatewayRetryCounter = 0

	// SyncWriter ensures concurrency
	pCtx.Logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))

	// Allowed values are: error, warn, info, debug
	pCtx.SetPluginLogLevel(output.FLBPluginConfigKey(plugin, "LogLevel"))

	switch pCtx.LogLevel {
	case "error":
		pCtx.Logger = level.NewFilter(pCtx.Logger, level.AllowError())
	case "info":
		pCtx.Logger = level.NewFilter(pCtx.Logger, level.AllowInfo())
	case "warn":
		pCtx.Logger = level.NewFilter(pCtx.Logger, level.AllowWarn())
	case "debug":
		pCtx.Logger = level.NewFilter(pCtx.Logger, level.AllowDebug())
	default:
		pCtx.Logger = level.NewFilter(pCtx.Logger, level.AllowInfo())
	}

	// initialize compiled regex a single time
	re = regexp.MustCompile(`(\d+\.?\d+)`)

	pCtx.SetPluginID(output.FLBPluginConfigKey(plugin, "id"))

	pCtx.Logger = log.With(pCtx.Logger, "plugin", pluginName, "caller", log.Caller(3), "id", pCtx.ID)

	pCtx.SetPluginJobName(output.FLBPluginConfigKey(plugin, "job"))
	pCtx.SetPushGatewayURL(output.FLBPluginConfigKey(plugin, "url"))
	pCtx.SetPushGatewayRetries(output.FLBPluginConfigKey(plugin, "push_gateway_retries"), pCtx.Logger)
	pCtx.SetMetricType(output.FLBPluginConfigKey(plugin, "metric_type"))
	pCtx.SetMetricName(output.FLBPluginConfigKey(plugin, "metric_name"))
	pCtx.SetMetricHelp(output.FLBPluginConfigKey(plugin, "metric_help"))
	pCtx.SetMetricConstantLabels(output.FLBPluginConfigKey(plugin, "metric_constant_labels"), pCtx.Logger)
	pCtx.SetMetricGrouping(output.FLBPluginConfigKey(plugin, "metric_grouping"), pCtx.Logger)
	pCtx.SetMetricVariableLabels(output.FLBPluginConfigKey(plugin, "metric_variable_labels"))

	if pCtx.IsSummary() {
		pCtx.SetMetricSummaryObserveKey(output.FLBPluginConfigKey(plugin, "metric_summary_observe_key"), pCtx.Logger)
		pCtx.FBSummary.NewMetric(pCtx)
	}
	if pCtx.IsGauge() {
		pCtx.SetMetricGaugeMethod(output.FLBPluginConfigKey(plugin, "metric_gauge_method"), pCtx.Logger)

		switch pCtx.MetricData.Gauge.Method {
		case "Set":
			pCtx.SetMetricGaugeSetKey(output.FLBPluginConfigKey(plugin, "metric_gauge_set_key"), pCtx.Logger)
		case "Add":
			pCtx.SetMetricGaugeAddKey(output.FLBPluginConfigKey(plugin, "metric_gauge_add_key"), pCtx.Logger)
		case "Sub":
			pCtx.SetMetricGaugeSubKey(output.FLBPluginConfigKey(plugin, "metric_gauge_sub_key"), pCtx.Logger)
		case "Inc":
		case "Dec":
		default:
			level.Error(pCtx.Logger).Log("Unknown metric_gauge_method ", pCtx.MetricData.Gauge.Method)
		}
		pCtx.FBGauge.NewMetric(pCtx)
	}
	if pCtx.IsHistogram() {
		pCtx.SetMetricHistogramBucketType(output.FLBPluginConfigKey(plugin, "metric_histogram_bucket_type"), pCtx.Logger)

		if pCtx.IsLinearBucket() {
			pCtx.SetMetricHistogramLinearBucketsCount(output.FLBPluginConfigKey(plugin, "metric_histogram_linear_buckets_count"), pCtx.Logger)
			pCtx.SetMetricHistogramLinearBucketsWidth(output.FLBPluginConfigKey(plugin, "metric_histogram_linear_buckets_width"), pCtx.Logger)
			pCtx.SetMetricHistogramLinearBucketsStart(output.FLBPluginConfigKey(plugin, "metric_histogram_linear_buckets_start"), pCtx.Logger)
			Start, err := strconv.ParseFloat(pCtx.MetricData.LinearBucketData.Start, 0)
			Width, err := strconv.ParseFloat(pCtx.MetricData.LinearBucketData.Width, 0)
			Count, err := strconv.ParseInt(pCtx.MetricData.LinearBucketData.Count, 10, 0)

			if err == nil {
				LinearBuckets := prometheus.LinearBuckets(Start, Width, int(Count))
				fmt.Println("Linear Buckets", LinearBuckets)
				pCtx.FBHistogram.NewMetric(pCtx, LinearBuckets)
			} else {
				level.Error(pCtx.Logger).Log("msg", "Histogram Linear Buckets failed type conversion", err)
			}
		}
		if pCtx.IsExponentialBucket() {
			pCtx.SetMetricHistogramExponentialBucketsCount(output.FLBPluginConfigKey(plugin, "metric_histogram_exponential_buckets_count"), pCtx.Logger)
			pCtx.SetMetricHistogramExponentialBucketsFactor(output.FLBPluginConfigKey(plugin, "metric_histogram_exponential_buckets_factor"), pCtx.Logger)
			pCtx.SetMetricHistogramExponentialBucketsStart(output.FLBPluginConfigKey(plugin, "metric_histogram_exponential_buckets_start"), pCtx.Logger)
			Start, err := strconv.ParseFloat(pCtx.MetricData.ExponentialBucketData.Start, 0)
			Factor, err := strconv.ParseFloat(pCtx.MetricData.ExponentialBucketData.Factor, 0)
			Count, err := strconv.ParseInt(pCtx.MetricData.ExponentialBucketData.Count, 10, 0)

			if err == nil {
				ExponentialBuckets := prometheus.ExponentialBuckets(Start, Factor, int(Count))
				fmt.Println("Exponential Buckets", ExponentialBuckets)
				pCtx.FBHistogram.NewMetric(pCtx, ExponentialBuckets)
			} else {
				level.Error(pCtx.Logger).Log("msg", "Histogram Exponential Buckets failed type conversion", err)
			}
		}
		pCtx.SetMetricHistogramObserveKey(output.FLBPluginConfigKey(plugin, "metric_histogram_observe_key"), pCtx.Logger)
	}
	if pCtx.IsCounter() {
		pCtx.FBCounter.NewMetric(pCtx)
	}

	if pCtx.IsGauge() {
		pCtx.FBGauge.NewMetric(pCtx)
	}

	level.Info(pCtx.Logger).Log("Job", pCtx.Job)
	level.Info(pCtx.Logger).Log("Url", pCtx.URL)
	level.Info(pCtx.Logger).Log("metric_type", pCtx.Type)
	level.Info(pCtx.Logger).Log("metric_name", pCtx.Name)
	level.Info(pCtx.Logger).Log("metric_help", pCtx.Help)
	j, err := json.Marshal(pCtx.ConstantLabels)

	if err != nil {
		level.Error(pCtx.Logger).Log("msg", "Failed to convert map[string]=string to json", "err", err)
	}

	var g []string
	for _, v := range pCtx.Grouping {
		g = append(g, fmt.Sprintf("%s:%s", v.Name, v.Value))
	}
	level.Info(pCtx.Logger).Log("metric_grouping", strings.Join(g, ","))
	level.Info(pCtx.Logger).Log("metric_constant_labels", j)
	level.Info(pCtx.Logger).Log("metric_variable_labels", strings.Join(pCtx.VariableLabels, ","))

	if pCtx.IsCounter() {
		level.Debug(pCtx.Logger).Log("Handle", fmt.Sprintf("%+v", pCtx.FBCounter.Handle))
	}

	registry := prometheus.NewRegistry()

	if pCtx.IsCounter() {
		registry.MustRegister(pCtx.FBCounter.Handle)
	}

	if pCtx.IsSummary() {
		registry.MustRegister(pCtx.FBSummary.Handle)
	}

	if pCtx.IsHistogram() {
		registry.MustRegister(pCtx.FBHistogram.Handle)
	}

	if pCtx.IsGauge() {
		registry.MustRegister(pCtx.FBGauge.Handle)
	}

	// Initialize new metric with push gateway
	pCtx.Pusher = push.New(pCtx.URL, pCtx.Job).Gatherer(registry)
	for _, group := range pCtx.Grouping {
		level.Debug(pCtx.Logger).Log("Add Group: name", group.Name, ", Value", group.Value)
		pCtx.Pusher = pCtx.Pusher.Grouping(group.Name, group.Value)
	}

	if err := pCtx.Pusher.Add(); err == nil {
		// Reset retry counter to zero and return error
		pCtx.PushGatewayRetryCounter = 0
	} else {
		var ret int

		if pCtx.PushGatewayRetryCounter < pCtx.PushGatewayRetries {
			level.Error(pCtx.Logger).Log("msg", "Could not put push to pushgateway, requesting retry attempt", pCtx.PushGatewayRetryCounter, "err", err)
			pCtx.PushGatewayRetryCounter++
			ret = output.FLB_RETRY
		} else {
			// Reset retry counter to zero and return error
			level.Error(pCtx.Logger).Log("msg", "Could not put push to pushgateway, resetting retry counter, declaring failure data will be lost", "err", err)
			pCtx.PushGatewayRetryCounter = 0
			ret = output.FLB_ERROR
		}
		return ret
	}

	// Set the context to point to any Go variable
	output.FLBPluginSetContext(plugin, pCtx)

	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	fmt.Println("[prometheus_metrics] Flush called for unknown instance")
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	// Type assert context back into the original type for the Go variable
	pCtx := output.FLBPluginGetContext(ctx).(*PluginContext)
	level.Debug(pCtx.Logger).Log("msg", "Flush called", "Tag", C.GoString(tag), "Keys", strings.Join(pCtx.VariableLabels, ","))

	dec := output.NewDecoder(data, int(length))

	count := 0
	for {
		ret, ts, record := output.GetRecord(dec)
		if ret != 0 {
			break
		}

		var timestamp time.Time
		switch t := ts.(type) {
		case output.FLBTime:
			timestamp = ts.(output.FLBTime).Time
		case uint64:
			timestamp = time.Unix(int64(t), 0)
		default:
			level.Warn(pCtx.Logger).Log("msg", "time provided invalid, defaulting to now.")
			timestamp = time.Now()
		}

		// Print record keys and values
		msgPrefix := fmt.Sprintf("[%d] %v: [%s] {", count, C.GoString(tag), timestamp.String())
		records := toStringMap(record)

		var msgRecords string
		if pCtx.LogLevel == "debug" {
			for k, v := range records {
				msgRecords += fmt.Sprintf("\"%s\": %s,", k, v)
			}
		}

		metricLabels := prometheus.Labels{}

		var msgKeys string
		for _, lk := range pCtx.VariableLabels {
			if pCtx.LogLevel == "debug" {
				msgKeys += fmt.Sprintf("|%s=%s|", lk, records[lk])
			}

			// This takes the value of a fluent bit key and assigns it to a GoLang map
			metricLabels[lk] = fmt.Sprintf("%v", records[lk])
		}

		level.Debug(pCtx.Logger).Log("msg", msgPrefix+msgRecords+msgKeys+"}")
		count++
		if pCtx.IsCounter() {
			pCtx.FBCounter.Handle.With(metricLabels).Inc()
			level.Debug(pCtx.Logger).Log("metric_description", pCtx.FBCounter.Handle.With(metricLabels).Desc().String())
		}

		if pCtx.IsGauge() {
			switch pCtx.MetricData.Gauge.Method {
			case "Set":
				s := fmt.Sprintf("%v", records[pCtx.MetricData.Gauge.SetKey])
				v, err := ExtractFloat(s)

				if err == nil {
					pCtx.FBGauge.Handle.With(metricLabels).Set(v)
				} else {
					level.Error(pCtx.Logger).Log("Unable to convert %s into a float64", s, "err", err)
				}

			case "Add":
				s := fmt.Sprintf("%v", records[pCtx.MetricData.Gauge.AddKey])
				v, err := ExtractFloat(s)

				if err == nil {
					level.Debug(pCtx.Logger).Log("Gauge: Add ", v)
					pCtx.FBGauge.Handle.With(metricLabels).Add(v)
				} else {
					level.Error(pCtx.Logger).Log("Unable to convert %s into a float64", s, "err", err)
				}

			case "Sub":
				s := fmt.Sprintf("%v", records[pCtx.MetricData.Gauge.SubKey])
				v, err := ExtractFloat(s)

				if err == nil {
					pCtx.FBGauge.Handle.With(metricLabels).Sub(v)
				} else {
					level.Error(pCtx.Logger).Log("Unable to convert %s into a float64", s, "err", err)
				}

			case "Inc":
				pCtx.FBGauge.Handle.With(metricLabels).Inc()
			case "Dec":
				pCtx.FBGauge.Handle.With(metricLabels).Dec()
			default:
				level.Error(pCtx.Logger).Log("Unknown metric_gauge_method ", pCtx.MetricData.Gauge.Method)
			}
		}
		if pCtx.IsSummary() {
			s := fmt.Sprintf("%v", records[pCtx.MetricData.Summary.ObserveKey])
			v, err := ExtractFloat(s)

			if err == nil {
				pCtx.FBSummary.Handle.With(metricLabels).Observe(v)
			} else {
				level.Error(pCtx.Logger).Log("Unable to convert %s into a float64", s, "err", err)
			}
		}
		if pCtx.IsHistogram() {
			s := fmt.Sprintf("%v", records[pCtx.MetricData.Histogram.ObserveKey])
			v, err := ExtractFloat(s)

			if err == nil {
				pCtx.FBHistogram.Handle.With(metricLabels).Observe(v)
			} else {
				level.Error(pCtx.Logger).Log("Unable to convert %s into a float64", s, "err", err)
			}
		}
	}

	if err := pCtx.Pusher.Add(); err == nil {
		// Reset retry counter to zero and return error
		pCtx.PushGatewayRetryCounter = 0
	} else {
		var ret int

		if pCtx.PushGatewayRetryCounter < pCtx.PushGatewayRetries {
			level.Error(pCtx.Logger).Log("msg", "Could not put push to pushgateway, requesting retry attempt", pCtx.PushGatewayRetryCounter, "err", err)
			pCtx.PushGatewayRetryCounter++
			ret = output.FLB_RETRY
		} else {
			// Reset retry counter to zero and return error
			level.Error(pCtx.Logger).Log("msg", "Could not put push to pushgateway, resetting retry counter, declaring failure data will be lost", "err", err)
			pCtx.PushGatewayRetryCounter = 0
			ret = output.FLB_ERROR
		}
		return ret
	}

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}

func main() {
}
