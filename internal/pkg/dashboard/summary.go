// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dashboard

import (
	"github.com/rivo/tview"

	"github.com/aenix-io/talm/internal/pkg/dashboard/apidata"
	"github.com/aenix-io/talm/internal/pkg/dashboard/components"
	"github.com/aenix-io/talm/internal/pkg/dashboard/resourcedata"
)

// SummaryGrid represents the summary grid with the basic node information and the logs.
type SummaryGrid struct {
	tview.Grid

	app *tview.Application

	apiDataListeners    []APIDataListener
	resourceListeners   []ResourceDataListener
	nodeSelectListeners []NodeSelectListener

	active     bool
	node       string
	logViewers map[string]*components.LogViewer

	diagnostics        *components.Diagnostics
	diagnosticsVisible bool
}

const summaryTopFixedRows = 7

// NewSummaryGrid initializes SummaryGrid.
func NewSummaryGrid(app *tview.Application) *SummaryGrid {
	widget := &SummaryGrid{
		app:        app,
		Grid:       *tview.NewGrid(),
		logViewers: make(map[string]*components.LogViewer),
	}

	widget.SetRows(summaryTopFixedRows, 0).SetColumns(-3, -2, -3)

	talosInfo := components.NewTalosInfo()
	widget.AddItem(talosInfo, 0, 0, 1, 1, 0, 0, false)

	kubernetesInfo := components.NewKubernetesInfo()
	widget.AddItem(kubernetesInfo, 0, 1, 1, 1, 0, 0, false)

	networkInfo := components.NewNetworkInfo()
	widget.AddItem(networkInfo, 0, 2, 1, 1, 0, 0, false)

	widget.diagnostics = components.NewDiagnostics()

	widget.apiDataListeners = []APIDataListener{
		kubernetesInfo,
	}

	widget.resourceListeners = []ResourceDataListener{
		talosInfo,
		kubernetesInfo,
		networkInfo,
		widget.diagnostics,
	}

	widget.nodeSelectListeners = []NodeSelectListener{
		talosInfo,
		kubernetesInfo,
		networkInfo,
		widget.diagnostics,
	}

	return widget
}

// OnNodeSelect implements the NodeSelectListener interface.
func (widget *SummaryGrid) OnNodeSelect(node string) {
	widget.node = node

	widget.updateLogViewer()

	for _, nodeSelectListener := range widget.nodeSelectListeners {
		nodeSelectListener.OnNodeSelect(node)
	}

	widget.updateDiagnostics()
}

// OnAPIDataChange implements the APIDataListener interface.
func (widget *SummaryGrid) OnAPIDataChange(node string, data *apidata.Data) {
	for _, dataWidget := range widget.apiDataListeners {
		dataWidget.OnAPIDataChange(node, data)
	}
}

// OnResourceDataChange implements the ResourceDataListener interface.
func (widget *SummaryGrid) OnResourceDataChange(nodeResource resourcedata.Data) {
	for _, resourceListener := range widget.resourceListeners {
		resourceListener.OnResourceDataChange(nodeResource)
	}

	widget.updateDiagnostics()
}

// OnLogDataChange implements the LogDataListener interface.
func (widget *SummaryGrid) OnLogDataChange(node, logLine, logError string) {
	widget.logViewer(node).WriteLog(logLine, logError)
}

func (widget *SummaryGrid) updateDiagnostics() {
	height := widget.diagnostics.GetCurrentHeight()

	switch {
	case height == 0 && widget.diagnosticsVisible:
		widget.RemoveItem(widget.diagnostics)
		widget.SetRows(summaryTopFixedRows, 0)
		widget.diagnosticsVisible = false
	case height > 0 && !widget.diagnosticsVisible:
		widget.SetRows(summaryTopFixedRows, 0, height)
		widget.AddItem(widget.diagnostics, 2, 0, 1, 3, 0, 0, false)
		widget.diagnosticsVisible = true
	case height > 0:
		widget.SetRows(summaryTopFixedRows, 0, height)
	}
}

func (widget *SummaryGrid) updateLogViewer() {
	if !widget.active {
		return
	}

	widget.logViewer(widget.node)

	for currNode, logViewer := range widget.logViewers {
		if currNode == widget.node {
			widget.AddItem(logViewer, 1, 0, 1, 3, 0, 0, false)

			widget.app.SetFocus(logViewer)

			return
		}

		widget.RemoveItem(logViewer)
	}
}

func (widget *SummaryGrid) logViewer(node string) *components.LogViewer {
	logViewer, ok := widget.logViewers[node]
	if !ok {
		logViewer = components.NewLogViewer()

		widget.logViewers[node] = logViewer
	}

	return logViewer
}

// OnScreenSelect implements the screenSelectListener interface.
func (widget *SummaryGrid) onScreenSelect(active bool) {
	widget.active = active

	widget.updateLogViewer()
}
