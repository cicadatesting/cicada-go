package cicada

import (
	"fmt"
	"testing"
)

func TestMetrics(t *testing.T) {
	c := make(chan *PoppedScenarioMetric)

	agg := NewStatAggregator(c)
	printer := NewStatPrinter(&StatPrinterConfig{
		MetricAggregator: agg,
	})

	c <- &PoppedScenarioMetric{
		Metric: &ScenarioMetric{
			Scenario: "my_scenario",
			Name:     "rps",
			Value:    1,
		},
	}

	c <- &PoppedScenarioMetric{
		Metric: &ScenarioMetric{
			Scenario: "my_scenario",
			Name:     "rps",
			Value:    2,
		},
	}

	c <- &PoppedScenarioMetric{
		Metric: &ScenarioMetric{
			Scenario: "my_scenario",
			Name:     "rps",
			Value:    3,
		},
	}

	c <- &PoppedScenarioMetric{
		Metric: &ScenarioMetric{
			Scenario: "my_scenario",
			Name:     "rps",
			Value:    4,
		},
	}

	c <- &PoppedScenarioMetric{
		Metric: &ScenarioMetric{
			Scenario: "my_scenario",
			Name:     "rps",
			Value:    5,
		},
	}

	printouts := printer.GetPrintouts("my_scenario")

	fmt.Println("printouts:", printouts)
	if len(printouts) != 1 {
		t.Errorf("Expected 1 printout but got %v", len(printouts))
	}

	if printouts[0] != "RPS :: Count: 5, Total: 15, Min: 1, Median: 3, Max: 5, Average: 3" {
		t.Errorf("Recieved incorrect printout: %s", printouts[0])
	}
}
