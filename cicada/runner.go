package cicada

import (
	"fmt"
	"time"
)

func testRunner(testTimeout time.Duration, scenarios []*Scenario, backend TestBackend) {
	startedScenarioStatuses := map[string]ScenarioStatus{}
	state := NewState()
	numFinishedScenarios := 0

	backend.SendTestEvent(
		&TestEvent{
			Text:             "Started Test",
			ScenarioStatuses: startedScenarioStatuses,
		},
	)

	// start scenarios without dependencies
	for _, scenario := range scenarios {
		if scenario.Dependencies == nil || len(scenario.Dependencies) < 1 {
			err := backend.StartScenario(scenario, state)

			if err != nil {
				// skip dependent scenarios, record result in state
				startedScenarioStatuses[scenario.Name] = FAILED
				numFinishedScenarios++

				backend.SendTestEvent(
					&TestEvent{
						Text:             fmt.Sprintf("Failed to start scenario '%s'", scenario.Name),
						ScenarioStatuses: startedScenarioStatuses,
					},
				)
			} else {
				startedScenarioStatuses[scenario.Name] = STARTED

				backend.SendTestEvent(
					&TestEvent{
						Text:             fmt.Sprintf("Started scenario '%s'", scenario.Name),
						ScenarioStatuses: startedScenarioStatuses,
					},
				)
			}
		}
	}

	// as results are collected, launch successful tests with required dependecies
	// collect state as scenarios are completed
	timeout := time.After(testTimeout)
	resultStream := backend.GetScenarioResults()

	for numFinishedScenarios < len(scenarios) {
		select {
		case poppedResult := <-resultStream:
			if poppedResult.Err != nil {
				panic(poppedResult.Err)
			}

			result := poppedResult.Result
			numFinishedScenarios++
			state.results[result.ScenarioName] = result

			if result.Exception != nil {
				startedScenarioStatuses[result.ScenarioName] = FAILED

				backend.SendTestEvent(
					&TestEvent{
						Text:             fmt.Sprintf("Scenario '%s' failed", result.ScenarioName),
						ScenarioStatuses: startedScenarioStatuses,
					},
				)
			} else {
				startedScenarioStatuses[result.ScenarioName] = SUCCEEDED

				backend.SendTestEvent(
					&TestEvent{
						Text:             fmt.Sprintf("Scenario '%s' succeeded", result.ScenarioName),
						ScenarioStatuses: startedScenarioStatuses,
					},
				)
			}

			for _, scenario := range scenarios {
				_, hasBeenStarted := startedScenarioStatuses[scenario.Name]

				if hasBeenStarted {
					continue
				}

				shouldStartScenario := true

				for _, dependency := range scenario.Dependencies {
					dependencyStatus, hasDependencyStatus := startedScenarioStatuses[dependency.Name]

					if !hasDependencyStatus || dependencyStatus == STARTED {
						shouldStartScenario = false
						break
					}

					if dependencyStatus == FAILED || dependencyStatus == SKIPPED {
						shouldStartScenario = false
						startedScenarioStatuses[scenario.Name] = SKIPPED
						numFinishedScenarios++

						backend.SendTestEvent(
							&TestEvent{
								Text:             fmt.Sprintf("Skipped scenario '%s'", scenario.Name),
								ScenarioStatuses: startedScenarioStatuses,
							},
						)

						break
					}
				}

				if !shouldStartScenario {
					continue
				}

				err := backend.StartScenario(scenario, state)

				if err != nil {
					startedScenarioStatuses[scenario.Name] = FAILED
					numFinishedScenarios++

					backend.SendTestEvent(
						&TestEvent{
							Text:             fmt.Sprintf("Failed to start scenario '%s'", scenario.Name),
							ScenarioStatuses: startedScenarioStatuses,
						},
					)
				} else {
					startedScenarioStatuses[scenario.Name] = STARTED

					backend.SendTestEvent(
						&TestEvent{
							Text:             fmt.Sprintf("Started scenario '%s'", scenario.Name),
							ScenarioStatuses: startedScenarioStatuses,
						},
					)
				}
			}
		case <-timeout:
			backend.SendTestEvent(
				&TestEvent{
					Text:             "Test Timed Out",
					ScenarioStatuses: startedScenarioStatuses,
				},
			)

			backend.SetTestResult(&TestResult{State: state})
		}
	}

	backend.SendTestEvent(
		&TestEvent{
			Text:             "Finished Test",
			ScenarioStatuses: startedScenarioStatuses,
		},
	)

	// return state
	backend.SetTestResult(
		&TestResult{
			State:            state,
			ScenarioStatuses: startedScenarioStatuses,
		},
	)
}

func scenarioRunner(scenario *Scenario, backend ScenarioBackend, state *State) {
	scenarioCommands := NewScenarioCommands(scenario, backend)

	start := time.Now()
	output, exception := scenario.LoadModel(scenarioCommands, state)
	duration := time.Since(start)

	if err := scenarioCommands.ScaleUsers(0); err != nil {
		panic(err)
	}

	backend.SetScenarioResult(
		output,
		exception,
		duration,
	)
}

func userRunner(userID string, scenario *Scenario, state *State, backend UserBackend) {
	userCommands := NewUserCommands(userID, backend, scenario)

	scenario.UserLoop(userCommands, state)
}
