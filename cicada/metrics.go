package cicada

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/wangjia184/sortedset"
)

type ScenarioMetric struct {
	Scenario string
	Name     string
	Value    float64
}

type PoppedScenarioMetric struct {
	Metric *ScenarioMetric
	Err    error
}

type MetricAggregator interface {
	GetScenarioMetricNames() map[string][]string
	GetCount(scenario, name string) int
	GetTotal(scenario, name string) float64
	GetMin(scenario, name string) float64
	GetMedian(scenario, name string) float64
	GetMax(scenario, name string) float64
	GetAverage(scenario, name string) float64
	GetRateAbove(scenario, name string, splitPoint float64) float64
	GetLast(scenario, name string) float64
}

type MetricPrinter interface {
	GetPrintouts(scenario string) []string
}

type StatAggregator struct {
	nameLock            sync.Mutex
	sortedSetLock       sync.Mutex
	totalsLock          sync.Mutex
	countsLock          sync.Mutex
	lastLock            sync.Mutex
	scenarioMetricNames map[string][]string
	sortedSets          map[string]map[string]*sortedset.SortedSet
	totals              map[string]map[string]float64
	counts              map[string]map[string]int
	lasts               map[string]map[string]float64
}

func NewStatAggregator(
	metrics <-chan *PoppedScenarioMetric,
) *StatAggregator {
	agg := StatAggregator{
		scenarioMetricNames: map[string][]string{},
		sortedSets:          map[string]map[string]*sortedset.SortedSet{},
		totals:              map[string]map[string]float64{},
		counts:              map[string]map[string]int{},
		lasts:               map[string]map[string]float64{},
	}

	go func() {
		for event := range metrics {
			agg.putMetricName(event)
			agg.addToSortedSet(event)
			agg.updateCount(event)
			agg.updateTotals(event)
			agg.updateLast(event)
		}
	}()

	return &agg
}

func (agg *StatAggregator) GetScenarioMetricNames() map[string][]string {
	agg.nameLock.Lock()
	defer agg.nameLock.Unlock()

	scenarioNamesCopy := map[string][]string{}

	for scenario, names := range agg.scenarioMetricNames {
		scenarioNamesCopy[scenario] = names
	}

	return scenarioNamesCopy
}

func (agg *StatAggregator) putMetricName(event *PoppedScenarioMetric) {
	agg.nameLock.Lock()
	defer agg.nameLock.Unlock()

	scenario, hasScenario := agg.scenarioMetricNames[event.Metric.Scenario]

	if !hasScenario {
		agg.scenarioMetricNames[event.Metric.Scenario] = []string{event.Metric.Name}
		return
	}

	for _, name := range scenario {
		if name == event.Metric.Name {
			return
		}
	}

	agg.scenarioMetricNames[event.Metric.Scenario] = append(scenario, event.Metric.Name)
}

func (agg *StatAggregator) addToSortedSet(event *PoppedScenarioMetric) {
	agg.sortedSetLock.Lock()
	defer agg.sortedSetLock.Unlock()

	scenarioSets, hasScenario := agg.sortedSets[event.Metric.Scenario]

	if !hasScenario {
		scenarioSets = map[string]*sortedset.SortedSet{}
		agg.sortedSets[event.Metric.Scenario] = scenarioSets
	}

	sortedSet, hasSortedSet := scenarioSets[event.Metric.Name]

	if !hasSortedSet {
		sortedSet = sortedset.New()
		scenarioSets[event.Metric.Name] = sortedSet
	}

	sortedSet.AddOrUpdate(uuid.NewString(), sortedset.SCORE(event.Metric.Value*10_000), event.Metric.Value)
}

func (agg *StatAggregator) updateCount(event *PoppedScenarioMetric) {
	agg.countsLock.Lock()
	defer agg.countsLock.Unlock()

	scenario, hasScenario := agg.counts[event.Metric.Scenario]

	if !hasScenario {
		scenario = map[string]int{}
		agg.counts[event.Metric.Scenario] = scenario
	}

	counts, hasCounts := scenario[event.Metric.Name]

	if !hasCounts {
		scenario[event.Metric.Name] = 1
	} else {
		scenario[event.Metric.Name] = counts + 1
	}
}

func (agg *StatAggregator) updateTotals(event *PoppedScenarioMetric) {
	agg.totalsLock.Lock()
	defer agg.totalsLock.Unlock()

	scenario, hasScenario := agg.totals[event.Metric.Scenario]

	if !hasScenario {
		scenario = map[string]float64{}
		agg.totals[event.Metric.Scenario] = scenario
	}

	totals, hasTotals := scenario[event.Metric.Name]

	if !hasTotals {
		scenario[event.Metric.Name] = 1
	} else {
		scenario[event.Metric.Name] = totals + event.Metric.Value
	}
}

func (agg *StatAggregator) updateLast(event *PoppedScenarioMetric) {
	agg.lastLock.Lock()
	defer agg.lastLock.Unlock()

	scenario, hasScenario := agg.lasts[event.Metric.Scenario]

	if !hasScenario {
		scenario = map[string]float64{}
		agg.lasts[event.Metric.Scenario] = scenario
	}

	scenario[event.Metric.Name] = event.Metric.Value
}

func (agg *StatAggregator) GetCount(scenario, name string) int {
	agg.countsLock.Lock()
	defer agg.countsLock.Unlock()

	scenarioCounts, hasScenario := agg.counts[scenario]

	if !hasScenario {
		return 0
	}

	count, hasCount := scenarioCounts[name]

	if !hasCount {
		return 0
	}

	return count
}

func (agg *StatAggregator) GetTotal(scenario, name string) float64 {
	agg.totalsLock.Lock()
	defer agg.totalsLock.Unlock()

	scenarioTotals, hasScenario := agg.totals[scenario]

	if !hasScenario {
		return 0
	}

	total, hasTotals := scenarioTotals[name]

	if !hasTotals {
		return 0
	}

	return total
}

func (agg *StatAggregator) GetMedian(scenario, name string) float64 {
	agg.sortedSetLock.Lock()
	defer agg.sortedSetLock.Unlock()

	scenarioSets, hasScenario := agg.sortedSets[scenario]

	if !hasScenario {
		return 0
	}

	sortedSet, hasSortedSet := scenarioSets[name]

	if !hasSortedSet {
		return 0
	}

	medIndex := sortedSet.GetCount() / 2

	medianScores := sortedSet.GetByRankRange(medIndex+1, medIndex+1, false)

	if len(medianScores) < 1 {
		return 0
	}

	return medianScores[0].Value.(float64)
}

func (agg *StatAggregator) GetMin(scenario, name string) float64 {
	agg.sortedSetLock.Lock()
	defer agg.sortedSetLock.Unlock()

	scenarioSets, hasScenario := agg.sortedSets[scenario]

	if !hasScenario {
		return 0
	}

	sortedSet, hasSortedSet := scenarioSets[name]

	if !hasSortedSet {
		return 0
	}

	if !hasScenario {
		return 0
	}

	min := sortedSet.PeekMin()

	return min.Value.(float64)
}

func (agg *StatAggregator) GetMax(scenario, name string) float64 {
	agg.sortedSetLock.Lock()
	defer agg.sortedSetLock.Unlock()

	scenarioSets, hasScenario := agg.sortedSets[scenario]

	if !hasScenario {
		return 0
	}

	sortedSet, hasSortedSet := scenarioSets[name]

	if !hasSortedSet {
		return 0
	}

	if !hasScenario {
		return 0
	}

	max := sortedSet.PeekMax()

	return max.Value.(float64)
}

func (agg *StatAggregator) GetAverage(scenario, name string) float64 {
	count := float64(agg.GetCount(scenario, name))

	if count == 0 {
		return 0
	}

	return agg.GetTotal(scenario, name) / count
}

func (agg *StatAggregator) GetRateAbove(scenario, name string, splitPoint float64) float64 {
	agg.sortedSetLock.Lock()
	defer agg.sortedSetLock.Unlock()

	scenarioSets, hasScenario := agg.sortedSets[scenario]

	if !hasScenario {
		return 0
	}

	sortedSet, hasSortedSet := scenarioSets[name]

	if !hasSortedSet {
		return 0
	}

	count := len(sortedSet.GetByScoreRange(sortedset.SCORE(splitPoint*10_000), sortedset.SCORE(-1), nil))

	return float64(count) / float64(agg.GetCount(scenario, name))
}

func (agg *StatAggregator) GetLast(scenario, name string) float64 {
	agg.lastLock.Lock()
	defer agg.lastLock.Unlock()

	lasts, hasLasts := agg.lasts[scenario]

	if !hasLasts {
		return 0
	}

	last, hasLast := lasts[name]

	if !hasLast {
		return 0
	}

	return last
}

type PrinterFunction struct {
	Scenario string
	Name     string
	Printer  func(scenario, name string, agg MetricAggregator) string
}

type StatPrinter struct {
	printerFunctions []*PrinterFunction
	metricAggregator MetricAggregator
}

type StatPrinterConfig struct {
	MetricAggregator MetricAggregator
	PrinterFunctions []*PrinterFunction
}

func NewStatPrinter(config *StatPrinterConfig) *StatPrinter {
	printerFunctions := []*PrinterFunction{}

	printerFunctions = append(printerFunctions, &PrinterFunction{
		Scenario: "*",
		Name:     "rps",
		Printer: func(scenario, name string, agg MetricAggregator) string {
			return fmt.Sprintf(
				"RPS :: Count: %d, Total: %v, Min: %v, Median: %v, Max: %v, Average: %v",
				agg.GetCount(scenario, name),
				agg.GetTotal(scenario, name),
				agg.GetMin(scenario, name),
				agg.GetMedian(scenario, name),
				agg.GetMax(scenario, name),
				agg.GetAverage(scenario, name),
			)
		},
	}, &PrinterFunction{
		Scenario: "*",
		Name:     "success_rate",
		Printer: func(scenario, name string, agg MetricAggregator) string {
			return fmt.Sprintf("Success Rate: %v%%", agg.GetLast(scenario, name)*100)
		},
	})

	statPrinter := StatPrinter{
		printerFunctions: printerFunctions,
		metricAggregator: config.MetricAggregator,
	}

	if config.PrinterFunctions != nil && len(config.PrinterFunctions) > 0 {
		statPrinter.printerFunctions = config.PrinterFunctions
	}

	return &statPrinter
}

func (sp *StatPrinter) AddPrinterFunction(pf *PrinterFunction) {
	sp.printerFunctions = append(sp.printerFunctions, pf)
}

func (sp *StatPrinter) GetPrintouts(scenario string) []string {
	metricNames := sp.metricAggregator.GetScenarioMetricNames()

	scenarioMetrics, hasScenarioMetrics := metricNames[scenario]

	if !hasScenarioMetrics {
		return []string{}
	}

	printouts := []string{}

	for _, name := range scenarioMetrics {
		for _, pf := range sp.printerFunctions {
			if (name == pf.Name || "*" == pf.Name) && (scenario == pf.Scenario || "*" == pf.Scenario) {
				printouts = append(printouts, pf.Printer(scenario, name, sp.metricAggregator))
			}
		}
	}

	return printouts
}
