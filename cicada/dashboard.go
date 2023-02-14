package cicada

import (
	"fmt"
	"strings"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/tj/go-spin"
)

type EmojiString string

const (
	CheckMark   EmojiString = "\u2714"
	CrossMark               = "\u274C"
	FastForward             = "\u23E9"
)

type Dashboard struct {
	eventChannel      <-chan *PoppedTestEvent
	statuses          map[string]ScenarioStatus
	sortedStatusNames []string
	spinner           *spin.Spinner
	metricPrinter     MetricPrinter
}

func NewDashboard(
	eventChannel <-chan *PoppedTestEvent,
	metricPrinter MetricPrinter,
) *Dashboard {
	return &Dashboard{
		eventChannel:      eventChannel,
		statuses:          map[string]ScenarioStatus{},
		spinner:           spin.New(),
		sortedStatusNames: []string{},
		metricPrinter:     metricPrinter,
	}
}

// Displays dashboard on console until test is finished
func (d *Dashboard) Display() {
	if err := ui.Init(); err != nil {
		panic(err)
	}

	ticker := time.NewTicker(time.Millisecond * 100)
	defer ui.Close()
	d.render()

	for {
		select {
		case e := <-ui.PollEvents():
			if e.Type == ui.KeyboardEvent {
				return
			}
		case event, isOpen := <-d.eventChannel:
			if !isOpen {
				return
			}

			d.updateStatusDisplay(event.Event.ScenarioStatuses)
		case <-ticker.C:
			d.render()
		}
	}
}

func (d *Dashboard) updateStatusDisplay(statuses map[string]ScenarioStatus) {
	// Preserve order of statuses as they are added
	for name := range statuses {
		hasSeenName := false

		for _, seenName := range d.sortedStatusNames {
			if name == seenName {
				hasSeenName = true
				break
			}
		}

		if !hasSeenName {
			d.sortedStatusNames = append(d.sortedStatusNames, name)
		}
	}

	d.statuses = statuses
}

func (d *Dashboard) renderStatusDisplay() ui.GridItem {
	if len(d.statuses) < 1 {
		return ui.NewCol(0.5, ui.NewRow(1, widgets.NewParagraph()))
	}

	statuses := []string{}

	for _, name := range d.sortedStatusNames {
		status := d.statuses[name]
		var icon string

		if status == SUCCEEDED {
			icon = string(CheckMark)
		} else if status == FAILED {
			icon = CrossMark
		} else if status == SKIPPED {
			icon = FastForward
		} else {
			icon = d.spinner.Next()
		}

		statuses = append(statuses, fmt.Sprintf("%s: %s %s", name, status, icon))
	}

	p := widgets.NewParagraph()
	p.Text = strings.Join(statuses, "\n")
	p.Title = "Statuses"

	return ui.NewCol(0.5, ui.NewRow(1, p))
}

func (d *Dashboard) renderMetricsDisplay() ui.GridItem {
	rows := []interface{}{}

	totalPrintouts := 0
	printoutScenarioNames := []string{}
	printoutParagraphStringGroups := [][]string{}
	// ratio := float64(1) / float64(len(printouts))

	for _, name := range d.sortedStatusNames {
		printouts := d.metricPrinter.GetPrintouts(name)

		if printouts != nil && len(printouts) > 0 {
			printoutParagraphStringGroups = append(printoutParagraphStringGroups, printouts)
			printoutScenarioNames = append(printoutScenarioNames, name)
			totalPrintouts += len(printouts)
		}
	}

	for i, printoutParagraphStringGroup := range printoutParagraphStringGroups {
		p := widgets.NewParagraph()
		p.Text = strings.Join(printoutParagraphStringGroup, "\n")
		p.Border = true
		p.Title = fmt.Sprintf("%s Metrics", printoutScenarioNames[i])

		// ratio is ratio of this scenario's printouts to total printouts
		row := ui.NewRow(float64(len(printoutParagraphStringGroup))/float64(totalPrintouts), p)

		rows = append(rows, row)
	}

	if len(rows) < 1 {
		return ui.NewCol(0.5, ui.NewRow(1, widgets.NewParagraph()))
	}

	return ui.NewCol(0.5, rows...)
}

func (d *Dashboard) render() {
	grid := ui.NewGrid()

	grid.Set(d.renderStatusDisplay(), d.renderMetricsDisplay())

	w, h := ui.TerminalDimensions()
	grid.SetRect(0, 0, w, h)

	ui.Render(grid)
}
