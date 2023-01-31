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

	resp, err := http.Post("http://localhost:8080/books", "application/json", bytes.NewBuffer(b))

	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	if resp.StatusCode != 201 {
		return nil, fmt.Errorf("Recieved response code %d", resp.StatusCode)
	}

	return nil, nil
}

func getBooks(state *cicada.State) (interface{}, error) {
	fmt.Println("create book result:", state.GetResult("createBook").Result.(*cicada.UserResult))

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

	s1 := cicada.NewScenario("createBook", createBook, nil)
	s2 := cicada.NewScenario("getBooks", getBooks, &cicada.ScenarioOptions{
		Dependencies: []*cicada.Scenario{s1},
	})

	engine.AddScenario(s1)
	engine.AddScenario(s2)

	test := engine.RunTest(time.Second * 30)

	// agg := cicada.NewStatAggregator(test.Metrics())

	// cicada.NewDashboard(
	// 	test.Events(),
	// 	cicada.NewStatPrinter(&cicada.StatPrinterConfig{MetricAggregator: agg}),
	// ).Display()

	result := <-test.Result()

	cicada.PrintResult(result.Result)
}
