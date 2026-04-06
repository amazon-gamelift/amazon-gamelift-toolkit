/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package stats

import (
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"
	"context"
	"sort"
	"sync"
	"time"
)

// StatsEventType represents the type of statistics event
type StatsEventType int

const (
	EventValidPacketProcessed StatsEventType = iota
	EventMalformedPacketProcessed
	EventPlayerConnected
	EventPlayerDisconnected
	EventDegradationUpdate
)

// StatsEvent represents a statistics event published by proxy components
type StatsEvent struct {
	Type               StatsEventType // Type of event that occurred
	Port               int            // Proxy's listening port (unique identifier)
	PlayerNumber       int            // Player number for connection events
	DegradationPercent int            // Degradation percentage for degradation events
}

// StatsSnapshot represents an immutable snapshot of current metrics
type StatsSnapshot struct {
	Uptime            time.Duration    // Total uptime of application
	IPAddress         string           // IP address the testing app is bound to
	EndpointStats     []ProxyStats     // List of stats per proxy
	PlayerConnections map[int]bool     // Player number -> connected
	ValidTokens       map[int]string   // Player number -> base64-encoded token
	PlayerEndpoints   map[int][]int    // Player number -> list of endpoint ports
}

// StatsCollector aggregates statistics events from proxy components
type StatsCollector struct {
	startTime         time.Time           // start time of application
	ipAddress         string              // IP address the testing app is bound to
	endpointStats     map[int]*ProxyStats // port -> stats
	playerConnections map[int]bool        // player number -> active connection
	tokenManager      *token.TokenManager // token manager used to get info about valid tokens
	eventChan         chan StatsEvent     // channel for receiving events from publishers
	playerEndpoints   map[int][]int       // player number -> list of endpoint ports
	mu                sync.RWMutex
}

// NewStatsCollector creates a new stats collector.
//
// Parameters:
//   - tokenManager: token manager used to retrieve valid token information
//   - startPort: starting port number for proxy endpoints
//   - endpointCount: number of UDP endpoints per player
//   - playerCount: number of players
//   - ipAddress: IP address the testing app is bound to
//
// Returns:
//   - *StatsCollector: configured stats collector ready to start
func NewStatsCollector(tokenManager *token.TokenManager, startPort, endpointCount, playerCount int, ipAddress string) *StatsCollector {
	endpointStats := make(map[int]*ProxyStats)
	playerEndpoints := make(map[int][]int)

	port := startPort
	for playerNum := 1; playerNum <= playerCount; playerNum++ {
		for range endpointCount {
			endpointStats[port] = &ProxyStats{Port: port}
			playerEndpoints[playerNum] = append(playerEndpoints[playerNum], port)
			port++
		}
	}

	return &StatsCollector{
		startTime:         time.Now(),
		ipAddress:         ipAddress,
		endpointStats:     endpointStats,
		playerConnections: make(map[int]bool),
		tokenManager:      tokenManager,
		eventChan:         make(chan StatsEvent, 100),
		playerEndpoints:   playerEndpoints,
	}
}

// Start begins processing events from the event channel.
//
// Parameters:
//   - ctx: context for graceful shutdown
func (sc *StatsCollector) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-sc.eventChan:
			sc.processEvent(event)
		}
	}
}

// processEvent handles a single stats event.
//
// Parameters:
//   - event: stats event to process
func (sc *StatsCollector) processEvent(event StatsEvent) {
	switch event.Type {
	case EventValidPacketProcessed:
		sc.incrementValidPacketCounter(event.Port)
	case EventMalformedPacketProcessed:
		sc.incrementMalformedPacketCounter(event.Port)
	case EventPlayerConnected:
		sc.updatePlayerConnection(event.PlayerNumber, true)
	case EventPlayerDisconnected:
		sc.updatePlayerConnection(event.PlayerNumber, false)
	case EventDegradationUpdate:
		sc.updateDegradationPercentage(event.Port, event.DegradationPercent)
	}
}

// incrementValidPacketCounter atomically increments the packet counter for an endpoint.
//
// Parameters:
//   - port: proxy port number identifying the endpoint
func (sc *StatsCollector) incrementValidPacketCounter(port int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	stats, exists := sc.endpointStats[port]
	if !exists {
		return
	}

	stats.IncrementValidPackets()
}

// incrementMalformedPacketCounter atomically increments the malformed packet counter for an endpoint.
//
// Parameters:
//   - port: proxy port number identifying the endpoint
func (sc *StatsCollector) incrementMalformedPacketCounter(port int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	stats, exists := sc.endpointStats[port]
	if !exists {
		return
	}

	stats.IncrementMalformedPackets()
}

// updateDegradationPercentage updates the degradation percentage for an endpoint.
//
// Parameters:
//   - port: proxy port number identifying the endpoint
//   - percentage: degradation percentage value to set
func (sc *StatsCollector) updateDegradationPercentage(port, percentage int) {
	sc.mu.Lock()
	stats, exists := sc.endpointStats[port]
	sc.mu.Unlock()
	if !exists {
		return
	}

	stats.SetDegradationPercentage(percentage)
}

// updatePlayerConnection updates the total connection count.
//
// Parameters:
//   - playerNumber: player number for the connection
//   - didConnect: true if player connected, false if disconnected
func (sc *StatsCollector) updatePlayerConnection(playerNumber int, didConnect bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if didConnect {
		sc.playerConnections[playerNumber] = true
	} else {
		delete(sc.playerConnections, playerNumber)
	}
}

// GetEventChannel returns a write-only channel for publishing events.
//
// Returns:
//   - chan<- StatsEvent: write-only channel for publishing stats events
func (sc *StatsCollector) GetEventChannel() chan<- StatsEvent {
	return sc.eventChan
}

// GetSnapshot creates an immutable snapshot of current metrics.
//
// Returns:
//   - StatsSnapshot: immutable snapshot containing uptime, endpoint stats, player connections, and valid tokens
func (sc *StatsCollector) GetSnapshot() StatsSnapshot {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	uptime := time.Since(sc.startTime)

	endpointStatsCopy := make([]ProxyStats, 0, len(sc.endpointStats))
	for _, stats := range sc.endpointStats {
		endpointStatsCopy = append(endpointStatsCopy, ProxyStats{
			Port:                  stats.Port,
			ValidPackets:          stats.GetValidPackets(),
			MalformedPackets:      stats.GetMalformedPackets(),
			DegradationPercentage: stats.GetDegradationPercentage(),
		})
	}

	sort.Slice(endpointStatsCopy, func(i, j int) bool {
		return endpointStatsCopy[i].Port < endpointStatsCopy[j].Port
	})

	playerConnectionsCopy := make(map[int]bool, len(sc.playerConnections))
	for playerNum, connected := range sc.playerConnections {
		playerConnectionsCopy[playerNum] = connected
	}

	playerEndpointsCopy := make(map[int][]int, len(sc.playerEndpoints))
	for playerNum, ports := range sc.playerEndpoints {
		portsCopy := make([]int, len(ports))
		copy(portsCopy, ports)
		playerEndpointsCopy[playerNum] = portsCopy
	}

	var validTokens map[int]string
	if sc.tokenManager != nil {
		validTokens = sc.tokenManager.GetValidTokens()
	}

	return StatsSnapshot{
		Uptime:            uptime,
		IPAddress:         sc.ipAddress,
		EndpointStats:     endpointStatsCopy,
		PlayerConnections: playerConnectionsCopy,
		ValidTokens:       validTokens,
		PlayerEndpoints:   playerEndpointsCopy,
	}
}
