package cicada

import (
	"testing"
	"time"
)

func TestConstantArrivalRate(t *testing.T) {
	s := NewScenario(
		"test",
		func(state *State) (interface{}, error) {
			// <-time.After(time.Millisecond * 1500)
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

	state := State{}
	eventBus := NewLocalEventBus()

	backend := NewLocalScenarioBackend(s, eventBus, &state)
	sub := eventBus.Subscribe("SCENARIO_test_RESULT")
	timer := time.After(time.Second * 11)

	scenarioRunner(s, backend, &state)

	select {
	case e := <-sub.events:
		result := e.(*ScenarioResult)

		if result.Exception != nil {
			t.Errorf("Scenario failed with exception %v", result.Exception)
		}

		out := result.Result.(*ArrivalRateOutput)

		if out.FinalUsers != 10 {
			t.Errorf("ended with %v users", out.FinalUsers)
		}

		if out.Succeeded != 500 {
			t.Errorf("recieved %v results:", out.Succeeded)
		}
	case <-timer:
		t.Error("timed out")
	}
}

func TestThreshold(t *testing.T) {
	scalingPeriods := 0

	s := NewScenario(
		"test",
		func(state *State) (interface{}, error) {
			return nil, nil
		},
		&ScenarioOptions{
			LoadModel: Threshold(&ThresholdConfig{
				StartingUsers: 1,
				ScalingPeriod: time.Second,
				ScaleOutFunction: func(currentUsers int) int {
					return currentUsers + 1
				},
				ThresholdFunction: func(stats *ThresholdOutput) bool {
					scalingPeriods++

					return scalingPeriods >= 5
				},
			}),
			UserLoop: WhileAlive(),
		},
	)

	state := State{}
	eventBus := NewLocalEventBus()

	backend := NewLocalScenarioBackend(s, eventBus, &state)
	sub := eventBus.Subscribe("SCENARIO_test_RESULT")
	timer := time.After(time.Second * 11)

	scenarioRunner(s, backend, &state)

	select {
	case e := <-sub.events:
		result := e.(*ScenarioResult)

		if result.Exception != nil {
			t.Errorf("Scenario failed with exception %v", result.Exception)
		}

		out := result.Result.(*ThresholdOutput)

		if out.FinalUsers != 5 {
			t.Errorf("ended with %v users", out.FinalUsers)
		}
	case <-timer:
		t.Error("timed out")
	}
}
