package cicada

import (
	"fmt"
	"time"
)

// TODO: docs
type ScenarioFunction = func(state *State) (interface{}, error)
type LoadModel = func(sc ScenarioCommands, state *State) (interface{}, error)
type UserLoop = func(uc UserCommands, state *State)
type MetricsCollector func(results []*UserResult) []*ScenarioMetric

type Scenario struct {
	Name         string
	Fn           ScenarioFunction
	Dependencies []*Scenario
	LoadModel    LoadModel
	UserLoop     UserLoop
}

type ScenarioOptions struct {
	Dependencies []*Scenario
	LoadModel    LoadModel
	UserLoop     UserLoop
}

func NewScenario(name string, fn ScenarioFunction, options *ScenarioOptions) *Scenario {
	finalOptions := &ScenarioOptions{
		Dependencies: nil,
		// UserLoop:     WhileAlive(),
		UserLoop: WhileHasWork(),
		// LoadModel: NSeconds(1, time.Second*5),
		// LoadModel:        NIterations(2000, 100),
		LoadModel: RunUntilSuccess(time.Second * 15),
	}

	if options != nil {
		if options.Dependencies != nil {
			finalOptions.Dependencies = options.Dependencies
		}

		if options.LoadModel != nil {
			finalOptions.LoadModel = options.LoadModel
		}

		if options.UserLoop != nil {
			finalOptions.UserLoop = options.UserLoop
		}
	}

	return &Scenario{
		Name:         name,
		Fn:           fn,
		Dependencies: finalOptions.Dependencies,
		UserLoop:     finalOptions.UserLoop,
		LoadModel:    finalOptions.LoadModel,
	}
}

type DefaultScenarioSummary struct {
	Succeeded  int
	Failed     int
	LastOutput interface{}
}

type ResultMetricConverter = func(output interface{}) map[string]float64

type NSecondsConfig struct {
	Users                 int
	Duration              time.Duration
	ResultMetricConverter ResultMetricConverter
}

// Built-in load models
func NSeconds(config *NSecondsConfig) LoadModel {
	return func(sc ScenarioCommands, state *State) (interface{}, error) {
		resultStream := sc.GetLatestResults()
		finalResult := DefaultScenarioSummary{Succeeded: 0, Failed: 0, LastOutput: nil}
		resultBuffer := []*UserResult{}
		ticker := time.NewTicker(time.Millisecond * 100)
		secondTicker := time.NewTicker(time.Second)
		timeout := time.After(config.Duration)
		shutdownStream := sc.ShutdownChannel()
		resultsCollected := 0
		lastResultsCollected := 0

		sc.ScaleUsers(config.Users)

		for {
			select {
			case err := <-shutdownStream:
				if err != nil {
					return nil, err
				}

				sc.ScaleUsers(0)
				return nil, fmt.Errorf("Scenario shutdown by test")
			case <-timeout:
				sc.ScaleUsers(0)
				return &finalResult, nil
			case <-secondTicker.C:
				rps := resultsCollected - lastResultsCollected
				lastResultsCollected = resultsCollected

				sc.AddDatastoreMetric("rps", float64(rps))
				sc.AddDatastoreMetric("success_rate", float64(finalResult.Succeeded/(finalResult.Succeeded+finalResult.Failed)))
			case result, open := <-resultStream:
				if !open {
					sc.ScaleUsers(0)
					return &finalResult, nil
				}

				if result.Err != nil {
					sc.ScaleUsers(0)
					panic(result.Err)
				}

				resultBuffer = append(resultBuffer, result.Result)
			case <-ticker.C:
				resultsCollected += len(resultBuffer)

				for _, result := range resultBuffer {
					if result.Exception != nil {
						finalResult.Failed++
					} else {
						finalResult.Succeeded++

						if config.ResultMetricConverter != nil {
							resultMetrics := config.ResultMetricConverter(result.Output)

							for name, value := range resultMetrics {
								sc.AddDatastoreMetric(name, value)
							}
						}
					}

					finalResult.LastOutput = result.Output
				}

				resultBuffer = []*UserResult{}
			}
		}
	}
}

type NIterationsConfig struct {
	Iterations            int
	Users                 int
	ResultMetricConverter ResultMetricConverter
}

func NIterations(config *NIterationsConfig) LoadModel {
	return func(sc ScenarioCommands, state *State) (interface{}, error) {
		resultsCollected := 0
		errors := []error{}
		finalResult := DefaultScenarioSummary{Succeeded: 0, Failed: 0, LastOutput: nil}
		resultBuffer := []*UserResult{}
		ticker := time.NewTicker(time.Millisecond * 100)
		secondTicker := time.NewTicker(time.Second)
		shutdownStream := sc.ShutdownChannel()
		resultStream := sc.GetLatestResults()
		lastResultsCollected := 0

		sc.ScaleUsers(config.Users)
		sc.AddWork(config.Iterations)

		for resultsCollected < config.Iterations {
			select {
			case err := <-shutdownStream:
				if err != nil {
					return nil, err
				}

				sc.ScaleUsers(0)
				return nil, fmt.Errorf("Scenario shutdown by test")
			case result := <-resultStream:
				if result.Err != nil {
					sc.ScaleUsers(0)
					panic(result.Err)
				}

				resultBuffer = append(resultBuffer, result.Result)
			case <-secondTicker.C:
				rps := resultsCollected - lastResultsCollected
				lastResultsCollected = resultsCollected

				sc.AddDatastoreMetric("rps", float64(rps))
				sc.AddDatastoreMetric("success_rate", float64(finalResult.Succeeded/(finalResult.Succeeded+finalResult.Failed)))
			case <-ticker.C:
				resultsCollected += len(resultBuffer)

				for _, result := range resultBuffer {
					if result.Exception != nil {
						errors = append(errors, result.Exception)
						finalResult.Failed++
					} else {
						finalResult.Succeeded++

						if config.ResultMetricConverter != nil {
							resultMetrics := config.ResultMetricConverter(result.Output)

							for name, value := range resultMetrics {
								sc.AddDatastoreMetric(name, value)
							}
						}
					}

					finalResult.LastOutput = result.Output
				}

				resultBuffer = []*UserResult{}
			}
		}

		// TODO: option to skip scaledown
		sc.ScaleUsers(0)
		return &finalResult, nil
	}
}

func RunUntilSuccess(timeout time.Duration) LoadModel {
	return func(sc ScenarioCommands, state *State) (interface{}, error) {
		shutdownStream := sc.ShutdownChannel()
		resultStream := sc.GetLatestResults()
		timeoutStream := time.After(timeout)

		sc.ScaleUsers(1)
		sc.AddWork(1)

		var lastResult *UserResult

		for {
			select {
			case <-shutdownStream:
				sc.ScaleUsers(0)
				return nil, fmt.Errorf("Scenario shutdown by test")
			case <-timeoutStream:
				sc.ScaleUsers(0)
				return lastResult, fmt.Errorf("Scenario timed out without successful results")
			case event := <-resultStream:
				if event.Err != nil {
					sc.ScaleUsers(0)
					panic(event.Err)
				}

				if event.Result.Exception == nil {
					return event.Result, nil
				}

				lastResult = event.Result
				sc.AddWork(1)
			}
		}
	}
}

// TODO: load stages
// TODO: constant arrival rate
// TODO: threshold

// Built-in user loops
func WhileAlive() UserLoop {
	return func(uc UserCommands, state *State) {
		shutdownStream := uc.ShutdownChannel()

		for {
			select {
			case err := <-shutdownStream:
				if err != nil {
					panic(err)
				}

				return
			default:
				start := time.Now()
				result, err := uc.Run(state)
				duration := time.Since(start)

				uc.ReportResult(result, err, duration)
			}
		}
	}
}

func WhileHasWork() UserLoop {
	return func(uc UserCommands, state *State) {
		workStream := uc.GetWork()
		shutdownStream := uc.ShutdownChannel()

		for {
			select {
			case err := <-shutdownStream:
				if err != nil {
					panic(err)
				}

				return
			case poppedWork, open := <-workStream:
				if !open {
					return
				}

				if poppedWork.Err != nil {
					panic(poppedWork.Err)
				}

				start := time.Now()
				result, err := uc.Run(state)
				duration := time.Since(start)
				uc.ReportResult(result, err, duration)
			}
		}
	}
}
