package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/cicadatesting/cicada-go/cicada"
)

type Book struct {
	Title string `json:"title"`
}

type RequestMetrics struct {
	DNSTime          time.Duration
	TLSHandshakeTime time.Duration
	ConnectionTime   time.Duration
	FirstByteTime    time.Duration
	TotalRequestTime time.Duration
}

func createBook(state *cicada.State) (interface{}, error) {
	book := Book{
		Title: "abcd",
	}

	b, _ := json.Marshal(book)

	req, err := http.NewRequest("POST", "http://localhost:8080/books", bytes.NewBuffer(b))

	if err != nil {
		return nil, err
	}

	var start, connect, dns, tlsHandshake time.Time
	requestMetrics := RequestMetrics{}

	trace := &httptrace.ClientTrace{
		DNSStart: func(dsi httptrace.DNSStartInfo) { dns = time.Now() },
		DNSDone: func(ddi httptrace.DNSDoneInfo) {
			requestMetrics.DNSTime = time.Since(dns)
		},

		TLSHandshakeStart: func() { tlsHandshake = time.Now() },
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			requestMetrics.TLSHandshakeTime = time.Since(tlsHandshake)
		},

		ConnectStart: func(network, addr string) { connect = time.Now() },
		ConnectDone: func(network, addr string, err error) {
			requestMetrics.ConnectionTime = time.Since(connect)
		},

		GotFirstResponseByte: func() {
			requestMetrics.FirstByteTime = time.Since(start)
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	start = time.Now()

	res, err := http.DefaultTransport.RoundTrip(req)

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	requestMetrics.TotalRequestTime = time.Since(start)

	if res.StatusCode != 201 {
		return nil, fmt.Errorf("Recieved response code %d", res.StatusCode)
	}

	return requestMetrics, nil
}

func getBooks(state *cicada.State) (interface{}, error) {
	resp, err := http.Get("http://localhost:8080/books")

	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Recieved response code %d", resp.StatusCode)
	}

	return "foo", nil
}

func main() {
	engine := cicada.NewEngine(nil)

	// s := cicada.NewScenario("getBooks", getBooks, &cicada.ScenarioOptions{
	// 	LoadModel: cicada.NSeconds(&cicada.NSecondsConfig{Users: 10, Duration: time.Second * 30}),
	// 	UserLoop:  cicada.WhileAlive(),
	// })

	s := cicada.NewScenario("createBook", createBook, &cicada.ScenarioOptions{
		LoadModel: cicada.NSeconds(&cicada.NSecondsConfig{
			Users:    10,
			Duration: time.Second * 10,
			ResultMetricConverter: func(output interface{}) map[string]float64 {
				metrics := map[string]float64{}

				requestMetrics, isRequestMetrics := output.(RequestMetrics)

				if isRequestMetrics {
					metrics["DNSTime"] = float64(requestMetrics.DNSTime.Nanoseconds()) / float64(1000)
					metrics["TLSHandshakeTime"] = float64(requestMetrics.TLSHandshakeTime.Nanoseconds()) / float64(1000)
					metrics["ConnectionTime"] = float64(requestMetrics.ConnectionTime.Nanoseconds()) / float64(1000)
					metrics["FirstByteTime"] = float64(requestMetrics.FirstByteTime.Nanoseconds()) / float64(1000)
					metrics["TotalRequestTime"] = float64(requestMetrics.TotalRequestTime.Nanoseconds()) / float64(1000)
				}

				return metrics
			},
		}),
		UserLoop: cicada.WhileAlive(),
	})

	engine.AddScenario(s)

	test := engine.RunTest(time.Minute * 5)

	agg := cicada.NewStatAggregator(test.Metrics())

	printer := cicada.NewStatPrinter(
		&cicada.StatPrinterConfig{
			MetricAggregator: agg,
			PrinterFunctions: []*cicada.PrinterFunction{
				{
					Scenario: "createBook",
					Name:     "*",
					Printer: func(scenario, name string, agg cicada.MetricAggregator) string {
						return fmt.Sprintf(
							"%s: Min: %vus, Median: %vus, Max: %vus, Average: %vus",
							name,
							agg.GetMin(scenario, name),
							agg.GetMedian(scenario, name),
							agg.GetMax(scenario, name),
							agg.GetAverage(scenario, name),
						)
					},
				},
			},
		},
	)

	cicada.NewDashboard(test.Events(), printer).Display()

	result := <-test.Result()

	cicada.PrintMetricsAndResults(result.Result, printer)
}
