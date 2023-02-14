package cicada

import (
	"fmt"
	"time"
)

type Engine struct {
	scenarios      map[string]*Scenario
	backendBuilder BackendBuilder
}

type EngineConfig struct {
	BackendBuilder BackendBuilder
}

func NewEngine(config *EngineConfig) *Engine {
	engine := Engine{
		scenarios:      map[string]*Scenario{},
		backendBuilder: NewLocalBackendBuilder(),
	}

	if config != nil {
		if config.BackendBuilder != nil {
			engine.backendBuilder = config.BackendBuilder
		}
	}

	return &engine
}

func (e *Engine) getScenario(name string) *Scenario {
	scenario, hasScenario := e.scenarios[name]

	if !hasScenario {
		panic(fmt.Errorf("Scenario '%s' not found", name))
	}

	return scenario
}

// Adds scenario to engine
func (e *Engine) AddScenario(scenario *Scenario) {
	_, hasName := e.scenarios[scenario.Name]

	if hasName {
		panic(fmt.Errorf("Cannot add scenario with name '%s' more than once", scenario.Name))
	}

	e.scenarios[scenario.Name] = scenario
}

// Starts test with scenarios currently in engine
func (e *Engine) RunTest(timeout time.Duration) *RunningTest {
	testBackend := e.backendBuilder.GetTestBackend()
	testEventBackend := e.backendBuilder.GetTestEventBackend()
	scenarios := []*Scenario{}

	runningTest := NewRunningTest(testEventBackend)

	for _, scenario := range e.scenarios {
		scenarios = append(scenarios, scenario)
	}

	go func() {
		testRunner(timeout, scenarios, testBackend)
		testBackend.Stop()
		testEventBackend.Stop()
	}()

	return runningTest
}

// func (e *Engine) RunScenario(scenarioName string, context *Context) {
// 	scenario := e.getScenario(scenarioName)

// 	backend := e.backendBuilder.GetScenarioBackend(scenario, context)

// 	go func() {
// 		ScenarioRunner(scenario, backend, context)
// 		backend.Stop()
// 	}()

// 	// TODO: return running scenario
// }
