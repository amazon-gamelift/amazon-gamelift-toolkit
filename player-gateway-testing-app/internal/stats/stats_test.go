/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package stats

import (
	"sync"
	"testing"
	"time"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

const (
	StartPort = 8080
)

func TestStatsCollector_EventProcessing(t *testing.T) {
	tm := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	collector := NewStatsCollector(tm, StartPort, 3, 1, "127.0.0.1")

	// Test EventPacketProcessed
	collector.processEvent(StatsEvent{Type: EventValidPacketProcessed, Port: 8080})
	collector.processEvent(StatsEvent{Type: EventValidPacketProcessed, Port: 8080})
	collector.processEvent(StatsEvent{Type: EventValidPacketProcessed, Port: 8081})

	snapshot := collector.GetSnapshot()
	assert.Equal(t, len(snapshot.EndpointStats), 3, "Expected 3 endpoints")

	assert.Equal(t, snapshot.EndpointStats[0].ValidPackets, int64(2), "Expected 2 packets processed for port 8080")
	assert.Equal(t, snapshot.EndpointStats[1].ValidPackets, int64(1), "Expected 1 packets processed for port 8081")

	// Test EventMalformedPacket
	collector.processEvent(StatsEvent{Type: EventMalformedPacketProcessed, Port: 8082})
	snapshot = collector.GetSnapshot()

	assert.Equal(t, snapshot.EndpointStats[2].MalformedPackets, int64(1), "Expected 1 malformed packet for port 8082")

	// Test EventPlayerConnected/Disconnected
	collector.processEvent(StatsEvent{Type: EventPlayerConnected, PlayerNumber: 1})
	collector.processEvent(StatsEvent{Type: EventPlayerConnected, PlayerNumber: 2})
	collector.processEvent(StatsEvent{Type: EventPlayerDisconnected, PlayerNumber: 1})

	snapshot = collector.GetSnapshot()
	assert.Equal(t, len(snapshot.PlayerConnections), 1, "Expected 1 total connection")

	// Test EventDegradationUpdate
	collector.processEvent(StatsEvent{Type: EventDegradationUpdate, DegradationPercent: 50, Port: 8080})
	collector.processEvent(StatsEvent{Type: EventDegradationUpdate, DegradationPercent: 20, Port: 8081})

	snapshot = collector.GetSnapshot()
	assert.Equal(t, snapshot.EndpointStats[0].DegradationPercentage, 50, "Expected port 8080 to be 50% degraded")
	assert.Equal(t, snapshot.EndpointStats[1].DegradationPercentage, 20, "Expected port 8081 to be 20% degraded")
}

func TestStatsCollector_ConcurrentEventPublishing(t *testing.T) {
	tm := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
	collector := NewStatsCollector(tm, StartPort, 10, 1, "127.0.0.1")

	// Start the collector
	ctx := t.Context()
	go collector.Start(ctx)

	// Publish events from multiple goroutines
	const numGoroutines = 10
	const eventsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for port := range numGoroutines {
		go func(port int) {
			defer wg.Done()
			for range eventsPerGoroutine {
				collector.GetEventChannel() <- StatsEvent{
					Type: EventValidPacketProcessed,
					Port: 8080 + port,
				}
			}
		}(port)
	}

	wg.Wait()

	// Give some time for events to be processed
	time.Sleep(testutil.TestTimeout)

	snapshot := collector.GetSnapshot()

	// Verify all events were processed
	totalPackets := int64(0)
	for i := range snapshot.EndpointStats {
		totalPackets += snapshot.EndpointStats[i].ValidPackets
	}

	expected := int64(numGoroutines * eventsPerGoroutine)
	assert.Equal(t, totalPackets, expected, "Expected all events to be processed")
}

func TestStatsCollector_SnapshotAccuracy(t *testing.T) {
	tm := testutil.CreateTestTokenManager(2) // 2 players = 2 tokens
	collector := NewStatsCollector(tm, StartPort, 1, 2, "127.0.0.1")

	// Add some events
	collector.processEvent(StatsEvent{Type: EventValidPacketProcessed, Port: 8080})
	collector.processEvent(StatsEvent{Type: EventValidPacketProcessed, Port: 8080})
	collector.processEvent(StatsEvent{Type: EventMalformedPacketProcessed, Port: 8080})
	collector.processEvent(StatsEvent{Type: EventValidPacketProcessed, Port: 8081})
	collector.processEvent(StatsEvent{Type: EventPlayerConnected, PlayerNumber: 1})
	collector.processEvent(StatsEvent{Type: EventPlayerConnected, PlayerNumber: 2})
	collector.processEvent(StatsEvent{Type: EventPlayerConnected, PlayerNumber: 3})
	collector.processEvent(StatsEvent{Type: EventPlayerDisconnected, PlayerNumber: 1})

	time.Sleep(testutil.TestTimeout)
	snapshot := collector.GetSnapshot()

	// Verify snapshot data
	assert.Equal(t, len(snapshot.PlayerConnections), 2, "Expected 2 connections")
	assert.Equal(t, len(snapshot.ValidTokens), 2, "Expected 2 valid tokens")
	assert.Equal(t, len(snapshot.EndpointStats), 2, "Expected 2 endpoints")

	// Verify uptime is positive
	assert.True(t, snapshot.Uptime > 0, "Expected positive uptime")
}

func TestStatsCollector_PlayerEndpoints(t *testing.T) {
	tm := testutil.CreateTestTokenManager(3)
	collector := NewStatsCollector(tm, StartPort, 2, 3, "127.0.0.1")

	snapshot := collector.GetSnapshot()

	// Verify per-player endpoint mapping
	assert.Equal(t, len(snapshot.PlayerEndpoints), 3, "Expected 3 players")
	assert.Equal(t, len(snapshot.PlayerEndpoints[1]), 2, "Player 1 should have 2 endpoints")
	assert.Equal(t, snapshot.PlayerEndpoints[1][0], 8080, "Player 1 first port should be 8080")
	assert.Equal(t, snapshot.PlayerEndpoints[1][1], 8081, "Player 1 second port should be 8081")
	assert.Equal(t, snapshot.PlayerEndpoints[2][0], 8082, "Player 2 first port should be 8082")
	assert.Equal(t, snapshot.PlayerEndpoints[2][1], 8083, "Player 2 second port should be 8083")
	assert.Equal(t, snapshot.PlayerEndpoints[3][0], 8084, "Player 3 first port should be 8084")
	assert.Equal(t, snapshot.PlayerEndpoints[3][1], 8085, "Player 3 second port should be 8085")

	// Verify total endpoints
	assert.Equal(t, len(snapshot.EndpointStats), 6, "Expected 6 total endpoints")
}

func TestStatsCollector_EdgeCases(t *testing.T) {
	t.Run("zero connections", func(t *testing.T) {
		collector := NewStatsCollector(nil, StartPort, 1, 1, "127.0.0.1")
		snapshot := collector.GetSnapshot()

		assert.Equal(t, len(snapshot.PlayerConnections), 0, "Expected 0 connections")
	})

	t.Run("no endpoints", func(t *testing.T) {
		collector := NewStatsCollector(nil, StartPort, 0, 1, "127.0.0.1")
		snapshot := collector.GetSnapshot()

		assert.Equal(t, len(snapshot.EndpointStats), 0, "Expected 0 endpoints")
	})

	t.Run("nil token manager", func(t *testing.T) {
		collector := NewStatsCollector(nil, StartPort, 1, 1, "127.0.0.1")
		snapshot := collector.GetSnapshot()

		assert.Equal(t, len(snapshot.ValidTokens), 0, "Expected empty token list")
	})

	t.Run("update stats for port that doesn't have a proxy", func(t *testing.T) {
		collector := NewStatsCollector(nil, StartPort, 1, 1, "127.0.0.1")
		collector.processEvent(StatsEvent{Type: EventValidPacketProcessed, Port: 9000})
		collector.processEvent(StatsEvent{Type: EventMalformedPacketProcessed, Port: 9000})
		collector.processEvent(StatsEvent{Type: EventPlayerConnected, PlayerNumber: 1})
		collector.processEvent(StatsEvent{Type: EventPlayerDisconnected, PlayerNumber: 1})
		collector.processEvent(StatsEvent{Type: EventDegradationUpdate, DegradationPercent: 50, Port: 9000})
		snapshot := collector.GetSnapshot()

		assert.Equal(t, len(snapshot.EndpointStats), 1, "Expected 1 endpoint stat")
		assert.Equal(t, snapshot.EndpointStats[0].Port, 8080, "Expected port to be 8080")
	})
}
