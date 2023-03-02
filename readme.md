# Cicada Go

Cicada Go is a framework for writing performance and integration tests in Go.
Cicada is different because it is unopinionated in which protocols are used,
allowing you to write tests for HTTP based REST API's, websockets, gRPC, Kafka,
etc. Simply write a script for a user's behavior, and Cicada will take care of
scaling the virtual users, collecting metrics, and generating load.

## Installation

To install this module, run

```bash
go get github.com/cicadatesting/cicada-go/cicada
```

## Concepts

### Scenarios

Scenarios represent the case being tested against the system. Each scenario
has a Load Model which describes how the test is being run against the system.
Built in load models include:

- NSeconds: Create a group of virtual users and have them run continuously for a
  desired amount of time
- NIterations: Create a group of users and have them run a fixed amount of times
- RunUntilSuccess: Create a single user and have it run sequentially until a
  successful result is collected
- ConstantArrivalRate: Limit the number of user loops executed per second and
  scale up or down the number of users to meet target load
- Threshold: Increase the number of users until a load threshold is met

### Users

Each scenario starts virtual users which represent the behavior of a client
interacting with an application. Users operate with a User Loop, which tells the
user how to go about executing its behavior, responding to events, and reporting
results. Built in user loops include:

- WhileAlive: Perform the user behavior and report results continuously until
  shutdown by the scenario.
- WhileHasWork: Only execute user behavior if given permission to by scenario.

### Metrics

Scenarios can collect metrics in their load models. Once reported, these metrics
can be aggregated and displayed, or sent to a remote backend to be visualized.

### State

Once the result of scenario is collected, it can be passed to dependent
scenarios as state. This is useful for breaking up integration tests. For
example, you may make a test that needs to create a user, and another one that
checks if a user can be successfully retrieved from a database. This allows you
to pass the API response from user creation to the get user scenario if needed.

## Examples

### Running an integration test

This test creates a scenario called `getBooks` that fetches books from an API.
If it recieves a `200` status code, then the test is successful. By default,
a scenario uses the `RunUntilSuccess` load model so it will call the endpoint
once or until it gets the desired response until it times out (15 seconds by
default).

```go
package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cicadatesting/cicada-go/cicada"
)

func getBooks(state *cicada.State) (interface{}, error) {
	resp, err := http.Get("http://localhost:8080/books")

	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Recieved response code %d", resp.StatusCode)
	}

	return nil, nil
}

func main() {
	engine := cicada.NewEngine(nil)

	s := cicada.NewScenario("getBooks", getBooks, nil)
	engine.AddScenario(s)

	test := engine.RunTest(time.Second * 30)

	result := <-test.Result()
	cicada.PrintResult(result.Result)
}
```

### Creating 10 users to hit an endpoint for 30 Seconds

This test will create 10 users to make POST requests against the books endpoint
for 30 seconds. In order to make unlimited requests in the time alotted, the
user loop needs to be set to `WhileAlive`.

By default, the dashboard will display metrics about success rate and requests
per second.

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cicadatesting/cicada-go/cicada"
)

type Book struct {
	Title string `json:"title"`
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

	client := &http.Client{}
	res, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != 201 {
		return nil, fmt.Errorf("Recieved response code %d", res.StatusCode)
	}

	return nil, nil
}

func main() {
	engine := cicada.NewEngine(nil)

	s := cicada.NewScenario("createBook", createBook, &cicada.ScenarioOptions{
		LoadModel: cicada.NSeconds(&cicada.NSecondsConfig{
			Users:    10,
			Duration: time.Second * 30,
		}),
		UserLoop: cicada.WhileAlive(),
	})

	engine.AddScenario(s)

	test := engine.RunTest(time.Second * 35)

	agg := cicada.NewStatAggregator(test.Metrics())

	printer := cicada.NewStatPrinter(
		&cicada.StatPrinterConfig{
			MetricAggregator: agg,
		},
	)

	cicada.NewDashboard(test.Events(), printer).Display()

	result := <-test.Result()

	cicada.PrintMetricsAndResults(result.Result, printer)
}
```

### Reporting metrics based on user results

In this example, the createBook function is modified to perform tracing on
requests, and includes the times in a result that is reported by users.

The NSeconds load model includes a ResultMetricConverter which parses results
and determines which metrics to send back from the scenario.

A StatPrinter is included to display these results on the console, which takes
and metric reported by `createBook` and prints its minimum value, median,
maximum, and average.

```go
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

func main() {
	engine := cicada.NewEngine(nil)

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

```

### Scenario dependencies and state

In this example, the getBooks scenario is configured to depend on the
`createBook` scenario. `getBooks` has also been modified to print out the
scenario result of `createBook`

```go
func getBooks(state *cicada.State) (interface{}, error) {
	fmt.Println("create book result:", state.GetResult("createBook").Result.(*cicada.UserResult))

	...
}

func main() {
	engine := cicada.NewEngine(nil)

	s1 := cicada.NewScenario("createBook", createBook, nil)
	s2 := cicada.NewScenario("getBooks", getBooks, &cicada.ScenarioOptions{
		Dependencies: []*cicada.Scenario{s1},
	})

	engine.AddScenario(s1)
	engine.AddScenario(s2)

    ...
}
```

### A Constant Arrival Rate Scenario

Performs 50 iterations per minute starting with 10 users and scaling up to 50
if necessary for 10 seconds

```go
s := NewScenario(
	"test",
	func(state *State) (interface{}, error) {
		...
		return nil, nil
	},
	&ScenarioOptions{
		LoadModel: ConstantArrivalRate(&ConstantArrivalRateConfig{
			TargetRPS: 50,
			MinUsers:  10,
			MaxUsers:  50,
			Duration:  time.Second * 10,
		}),
		UserLoop: WhileHasWork(),
	},
)
```

### A Threshold Scenario

Increase number of users by 5 per second until the RPS goes below 5

```go
s := NewScenario(
	"test",
	func(state *State) (interface{}, error) {
		...
		return nil, nil
	},
	&ScenarioOptions{
		LoadModel: Threshold(&ThresholdConfig{
			StartingUsers: 5,
			ScalingPeriod: time.Second,
			ScaleOutFunction: func(currentUsers int) int {
				return currentUsers + 5
			},
			ThresholdFunction: func(stats *ThresholdOutput) bool {
				scalingPeriods++

				return stats.rps < 5
			},
		}),
		UserLoop: WhileAlive(),
	},
)
```
