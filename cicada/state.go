package cicada

type State struct {
	results map[string]*ScenarioResult
}

func NewState() *State {
	return &State{results: map[string]*ScenarioResult{}}
}

// Gets result of a completed scenario
func (c *State) GetResult(name string) *ScenarioResult {
	return c.results[name]
}
