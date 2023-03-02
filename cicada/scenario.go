package cicada

import (
	"fmt"
	"math"
	"time"
)

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

// Takes an output and produces a map of metrics (name:value) derived from that output
type ResultMetricConverter = func(output interface{}) map[string]float64

type NSecondsConfig struct {
	Users                 int
	Duration              time.Duration
	ResultMetricConverter ResultMetricConverter
}

// built in load models

// Runs a scenario for a set duration. Use with the WhileAlive user loop
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

				if finalResult.Succeeded+finalResult.Failed > 0 {
					sc.AddDatastoreMetric("success_rate", float64(finalResult.Succeeded/(finalResult.Succeeded+finalResult.Failed)))
				}
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

// Runs the scenario a set number of times. Use with the WhileHasWork user loop
func NIterations(config *NIterationsConfig) LoadModel {
	return func(sc ScenarioCommands, state *State) (interface{}, error) {
		resultsCollected := 0
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

				if finalResult.Succeeded+finalResult.Failed > 0 {
					sc.AddDatastoreMetric("success_rate", float64(finalResult.Succeeded/(finalResult.Succeeded+finalResult.Failed)))
				}
				// TODO: collect time_taken stats
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

		// TODO: option to skip scaledown
		sc.ScaleUsers(0)
		return &finalResult, nil
	}
}

// Runs scenario consecutively with a single user until a successful result is collected
//
// Use the WhileHasWork user loop with this load model
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

type ArrivalRateOutput struct {
	Succeeded  int
	Failed     int
	FinalUsers int
	FinalRPS   int
	LastOutput interface{}
}

type ConstantArrivalRateConfig struct {
	TargetRPS             int
	MinUsers              int
	MaxUsers              int
	Duration              time.Duration
	ResultMetricConverter ResultMetricConverter
}

// Runs scenario a fixed number of times per second and scales user pool to compensate for differences in actual rate
func ConstantArrivalRate(config *ConstantArrivalRateConfig) LoadModel {
	return func(sc ScenarioCommands, state *State) (interface{}, error) {
		resultStream := sc.GetLatestResults()
		shutdownStream := sc.ShutdownChannel()

		finalResult := ArrivalRateOutput{
			Succeeded:  0,
			Failed:     0,
			FinalUsers: config.TargetRPS,
			LastOutput: nil,
		}

		resultBuffer := []*UserResult{}
		resultsCollected := 0
		lastResultsCollected := 0

		timeout := time.After(config.Duration)
		ticker := time.NewTicker(time.Millisecond * 100)
		secondTicker := time.NewTicker(time.Second)

		currentUsers := config.MinUsers

		sc.ScaleUsers(currentUsers)
		sc.AddWork(config.TargetRPS)

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
			case <-secondTicker.C:
				rps := resultsCollected - lastResultsCollected
				lastResultsCollected = resultsCollected

				sc.AddDatastoreMetric("rps", float64(rps))
				sc.AddDatastoreMetric("num_users", float64(currentUsers))

				if finalResult.Succeeded+finalResult.Failed > 0 {
					sc.AddDatastoreMetric("success_rate", float64(finalResult.Succeeded/(finalResult.Succeeded+finalResult.Failed)))
				}

				// if rps is below target, add target rps - actual rps amount of workers
				// if rps is above target, reduce workers by actual rps - target
				if rps < config.TargetRPS {
					currentUsers += int(math.Min(float64(config.TargetRPS-rps), float64(config.MaxUsers-currentUsers)))
				} else {
					currentUsers -= int(math.Min(float64(rps-config.TargetRPS), float64(currentUsers-config.MinUsers)))
				}

				sc.ScaleUsers(currentUsers)
				sc.AddWork(config.TargetRPS)

				finalResult.FinalUsers = currentUsers
				finalResult.FinalRPS = rps
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

type ThresholdOutput struct {
	ResultsCollected int64
	Rps              float64
	SuccessRate      float64
	Aggregate        interface{}
	FinalUsers       int
}

// Determine new amount of users for iteration
type ScaleOutFunction func(currentUsers int) int

// Returns true if test has reached threshold yet based on provided stats
type ThresholdFunction func(stats *ThresholdOutput) bool

// Produces an aggregated result for all results gathered by scenario
type ResultAggregator func(result *UserResult, previous interface{}) interface{}

type ThresholdConfig struct {
	StartingUsers         int
	ScalingPeriod         time.Duration
	ScaleOutFunction      ScaleOutFunction
	ResultMetricConverter ResultMetricConverter
	ResultAggregator      ResultAggregator
	ThresholdFunction     ThresholdFunction
}

// Continuously add users until threshold is reached
func Threshold(config *ThresholdConfig) LoadModel {
	if config.ScaleOutFunction == nil || config.ThresholdFunction == nil {
		panic(fmt.Errorf("Must specify scale out function and threshold function"))
	}

	return func(sc ScenarioCommands, state *State) (interface{}, error) {
		resultStream := sc.GetLatestResults()
		shutdownStream := sc.ShutdownChannel()

		finalResult := ThresholdOutput{
			ResultsCollected: 0,
			Rps:              0,
			SuccessRate:      0,
			Aggregate:        nil,
			FinalUsers:       config.StartingUsers,
		}

		resultBuffer := []*UserResult{}

		ticker := time.NewTicker(time.Millisecond * 100)
		secondTicker := time.NewTicker(time.Second)
		scaleOutTicker := time.NewTicker(config.ScalingPeriod)

		var lastResultsCollected int64
		succeeded := 0
		failed := 0

		sc.ScaleUsers(finalResult.FinalUsers)

		for {
			select {
			case err := <-shutdownStream:
				if err != nil {
					return nil, err
				}

				sc.ScaleUsers(0)
				return nil, fmt.Errorf("Scenario shutdown by test")
			case <-scaleOutTicker.C:
				if config.ThresholdFunction(&finalResult) {
					sc.ScaleUsers(0)
					return &finalResult, nil
				} else {
					finalResult.FinalUsers = config.ScaleOutFunction(finalResult.FinalUsers)
					sc.ScaleUsers(finalResult.FinalUsers)
				}
			case <-secondTicker.C:
				if succeeded+failed > 0 {
					finalResult.SuccessRate = float64(succeeded / (succeeded + failed))
				}

				finalResult.Rps = float64(finalResult.ResultsCollected - lastResultsCollected)
				lastResultsCollected = finalResult.ResultsCollected

				sc.AddDatastoreMetric("rps", finalResult.Rps)
				sc.AddDatastoreMetric("success_rate", finalResult.SuccessRate)
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
				finalResult.ResultsCollected += int64(len(resultBuffer))

				for _, result := range resultBuffer {
					if result.Exception != nil {
						failed++
					} else {
						succeeded++

						if config.ResultMetricConverter != nil {
							resultMetrics := config.ResultMetricConverter(result.Output)

							for name, value := range resultMetrics {
								sc.AddDatastoreMetric(name, value)
							}
						}

						if config.ResultAggregator != nil {
							finalResult.Aggregate = config.ResultAggregator(
								result,
								finalResult.Aggregate,
							)
						}
					}
				}

				resultBuffer = []*UserResult{}
			}
		}
	}
}

// Iterates through multiple load models and accumulates results
// func LoadStages() LoadModel {
// 	return func(sc ScenarioCommands, state *State) (interface{}, error) {
// 		finalResult := DefaultScenarioSummary{Succeeded: 0, Failed: 0, LastOutput: nil}

// 		// TODO: take list of load models that return DefaultScenarioSummary
// 		// TODO: run through each load model, accumulate results, signal users on change

// 		return &finalResult, nil
// 	}
// }

// Built-in user loops

// Run scenario function continuously until deactivated by load model
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

// Run scenario function only when directed to by load model
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
