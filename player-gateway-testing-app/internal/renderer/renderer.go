/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package renderer

import (
	"context"
	"fmt"
	"time"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/stats"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	DefaultWidth = 150
)

var (
	lineHeading = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	statusBar   = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}).Foreground(lipgloss.AdaptiveColor{Light: "#5A5A5A", Dark: "#B0B0B0"})
)

type TickMsg time.Time

// Model stores the state for the UI
type Model struct {
	statsCollector *stats.StatsCollector // StatsCollector for retrieving snapshots of stats
	snapshot       stats.StatsSnapshot   // The most recent snapshot of stats
	cancelFunc     context.CancelFunc    // CancelFunc used for terminating the application

	endpointTable         table.Model // Table model for displaying client-side endpoints stats
	totalValidPackets     int64       // Total valid packets across all endpoints
	totalMalformedPackets int64       // Total malformed packets across all endpoints
	width                 int         // Current width of the UI based on window size
}

// NewModel creates and initializes a new Model with the provided stats collector and cancel function.
// It sets up the endpoint table with appropriate columns and styling, and returns the configured model.
//
// Parameters:
//   - statsCollector: A pointer to the StatsCollector for retrieving snapshots of stats
//   - cancelFunc: A context.CancelFunc used for terminating the application
//
// Returns:
//   - Model: A configured Model instance
func NewModel(statsCollector *stats.StatsCollector, cancelFunc context.CancelFunc) (*Model, error) {
	if statsCollector == nil {
		return nil, fmt.Errorf("StatsCollector is nil")
	}
	endpointColumns := []table.Column{
		{Title: "Player", Width: 6},
		{Title: "Endpoint", Width: 10},
		{Title: "Port", Width: 5},
		{Title: "Packets with Valid Token", Width: 25},
		{Title: "Packets with Malformed Token", Width: 30},
		{Title: "Degradation %", Width: 15},
	}

	endpointTable := table.New(
		table.WithColumns(endpointColumns),
		table.WithHeight(13),
	)

	s := table.DefaultStyles()
	s.Selected = s.Selected.Foreground(lipgloss.NoColor{}).Bold(false)
	endpointTable.SetStyles(s)

	m := &Model{
		statsCollector:        statsCollector,
		endpointTable:         endpointTable,
		snapshot:              statsCollector.GetSnapshot(),
		cancelFunc:            cancelFunc,
		width:                 DefaultWidth,
		totalValidPackets:     0,
		totalMalformedPackets: 0,
	}

	return m, nil
}

// Init initializes the model and returns the initial commands to run.
// It starts the periodic tick timer.
//
// Returns:
//   - tea.Cmd: A batch command containing the tick timer and screen clear commands
func (m Model) Init() tea.Cmd {
	return tea.Batch(tick())
}

// Update handles incoming messages and updates the model state accordingly.
// It processes keyboard input for quitting the application, tick messages for refreshing stats,
// and window resize messages for adjusting the UI width.
//
// Parameters:
//   - msg: The incoming tea.Msg to process
//
// Returns:
//   - tea.Model: The updated model
//   - tea.Cmd: Any commands to execute as a result of the update
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, tea.Quit
		}
	case TickMsg:
		m.snapshot = m.statsCollector.GetSnapshot()

		// Calculate endpoints per player from the snapshot
		endpointsPerPlayer := 0
		if len(m.snapshot.PlayerEndpoints) > 0 {
			endpointsPerPlayer = len(m.snapshot.PlayerEndpoints[1])
		}

		// Update table rows and packet totals
		var endpointRows []table.Row
		var totalValidPackets, totalMalformedPackets int64
		for i := range m.snapshot.EndpointStats {
			// Only show player number on first endpoint for that player
			playerStr := ""
			if endpointsPerPlayer > 0 && i%endpointsPerPlayer == 0 {
				playerStr = fmt.Sprintf("%d", i/endpointsPerPlayer+1)
			}

			endpointRows = append(endpointRows, table.Row{
				playerStr,
				fmt.Sprintf("%d", i+1),
				fmt.Sprintf("%d", m.snapshot.EndpointStats[i].Port),
				fmt.Sprintf("%d", m.snapshot.EndpointStats[i].ValidPackets),
				fmt.Sprintf("%d", m.snapshot.EndpointStats[i].MalformedPackets),
				fmt.Sprintf("%d", m.snapshot.EndpointStats[i].DegradationPercentage),
			})
			totalValidPackets += m.snapshot.EndpointStats[i].ValidPackets
			totalMalformedPackets += m.snapshot.EndpointStats[i].MalformedPackets
		}

		m.endpointTable.SetRows(endpointRows)
		m.totalValidPackets = totalValidPackets
		m.totalMalformedPackets = totalMalformedPackets

		return m, tick()
	case tea.WindowSizeMsg:
		m.width = msg.Width
	}

	return m, nil
}

// View renders the current state of the model as a string for display in the terminal.
// It displays the application title, uptime, number of connected players, valid tokens,
// the endpoint statistics table, and a status bar with quit instructions.
//
// Returns:
//   - string: The rendered view of the current model state
func (m Model) View() string {
	s := lipgloss.NewStyle().Bold(true).Render("Player gateway testing tool") + "\n\n"

	s += lineHeading.Render("Uptime") + fmt.Sprintf(": %s\n", m.snapshot.Uptime.Round(time.Second).String())
	s += lineHeading.Render("IP Address") + fmt.Sprintf(": %s\n", m.snapshot.IPAddress)
	s += lineHeading.Render("Players connected") + fmt.Sprintf(": %d\n", len(m.snapshot.PlayerConnections))

	// Format tokens as "Player N: token" list, one per line
	s += lineHeading.Render("Tokens") + ":\n"
	for playerNum := 1; playerNum <= len(m.snapshot.ValidTokens); playerNum++ {
		if token, ok := m.snapshot.ValidTokens[playerNum]; ok {
			s += fmt.Sprintf("  Player %d: %s\n", playerNum, token)
		}
	}

	var validTokenPercentage float64
	totalPackets := m.totalValidPackets + m.totalMalformedPackets
	if totalPackets > 0 {
		validTokenPercentage = (float64(m.totalValidPackets) / float64(totalPackets)) * 100
	}
	s += lineHeading.Render("% of packets with valid token") + fmt.Sprintf(": %.2f%%\n\n", validTokenPercentage)

	s += m.endpointTable.View() + "\n"

	s += statusBar.Width(m.width).Render("Press q / ctrl+c to quit")

	return s
}

// tick creates a command that sends a TickMsg every second.
// This is used to periodically refresh the stats display in the UI.
//
// Returns:
//   - tea.Cmd: A command that emits TickMsg at one-second intervals
func tick() tea.Cmd {
	return tea.Every(time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}
