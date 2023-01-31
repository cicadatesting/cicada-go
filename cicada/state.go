package cicada

type State struct {
	results map[string]*ScenarioResult
}

func NewState() *State {
	return &State{results: map[string]*ScenarioResult{}}
}

func (c *State) GetResult(name string) *ScenarioResult {
	return c.results[name]
}
