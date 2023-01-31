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
