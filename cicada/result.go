package cicada

import (
	"fmt"
	"time"
)

type ScenarioStatus string

const (
	STARTED   ScenarioStatus = "STARTED"
	SUCCEEDED                = "SUCCEEDED"
	FAILED                   = "FAILED"
	SKIPPED                  = "SKIPPED"
)

type TestResult struct {
	State            *State
	ScenarioStatuses map[string]ScenarioStatus
}

func printScenarioResult(result *ScenarioResult) {
	fmt.Printf("- Time Taken: %v ms\n", result.TimeTaken.Milliseconds())

	output, isDefault := result.Result.(*DefaultScenarioSummary)
	exception := result.Exception

	if result.Result != nil {
		fmt.Printf("- Output:\n")

		if isDefault {
			fmt.Printf("  * Succeeded: %d\n", output.Succeeded)
			fmt.Printf("  * Failed: %d\n", output.Failed)
			fmt.Printf("  * Last Output: %v\n", output.LastOutput)
		} else {
			// TODO: pretty print output
			fmt.Println(result.Result)
		}
	}

	if exception != nil {
		fmt.Printf("- Exception:\n")
		// TODO: pretty print exception
		fmt.Println(exception)
	}
}

// Prints formatted test result
func PrintResult(r *TestResult) {
	// TODO: test result should include list of statuses in order they were collected
	for scenario, status := range r.ScenarioStatuses {
		fmt.Printf("%s :: %s\n", scenario, status)

		result, hasResult := r.State.results[scenario]

		if hasResult {
			printScenarioResult(result)
		}
	}
}

// Prints formatted test result and metrics for each scenario in test
func PrintMetricsAndResults(r *TestResult, printer MetricPrinter) {
	for scenario, status := range r.ScenarioStatuses {
		fmt.Printf("%s :: %s\n", scenario, status)

		result, hasResult := r.State.results[scenario]
		metrics := printer.GetPrintouts(scenario)

		if hasResult {
			printScenarioResult(result)
		}

		if len(metrics) > 0 {
			fmt.Printf("- Metrics:\n")

			for _, metric := range metrics {
				fmt.Printf("  * %s\n", metric)
			}
		}
	}
}

type ScenarioResult struct {
	ScenarioName string
	Result       interface{}
	Exception    error
	TimeTaken    time.Duration
}

type UserResult struct {
	Output    interface{}
	Exception error
	// Logs      string
	TimeTaken time.Duration
}

type PoppedTestResult struct {
	Result *TestResult
	Err    error
}

type PoppedScenarioResult struct {
	Result *ScenarioResult
	Err    error
}

type PoppedUserResult struct {
	Result *UserResult
	Err    error
}
