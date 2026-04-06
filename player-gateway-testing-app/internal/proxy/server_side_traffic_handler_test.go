/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"net"
	"testing"
	"time"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

var (
	testAddr1, _ = net.ResolveUDPAddr("udp", "127.0.0.1:1000")
	testAddr2, _ = net.ResolveUDPAddr("udp", "127.0.0.1:2000")
	testAddr3, _ = net.ResolveUDPAddr("udp", "127.0.0.1:3000")
)

type testSetup struct {
	handler *ServerSideProxyTrafficHandler
	socket  *net.UDPConn
	ccPool  *ClientConnectionPool
}

func newTestSetup(t *testing.T) *testSetup {
	socket := testutil.CreateTestUDPSocket(t)
	t.Cleanup(func() { socket.Close() })

	handler := &ServerSideProxyTrafficHandler{
		clientSideProxySources: make(map[int][]*ClientSideProxyInfo),
		lastUsedSource:         make(map[int]*ClientSideProxyInfo),
		nextSourceIndex:        make(map[int]int),
	}

	ccPool := &ClientConnectionPool{clientConnectionPortMap: make(map[int]*ClientConnectionInfo)}
	ccPool.clientConnectionPortMap[testutil.TestReturnTrafficPort] = &ClientConnectionInfo{PlayerNumber: testutil.TestPlayerNumber}

	return &testSetup{handler: handler, socket: socket, ccPool: ccPool}
}

func (s *testSetup) addEndpoint(t *testing.T, age time.Duration) *net.UDPConn {
	proxy := testutil.CreateTestUDPSocket(t)
	t.Cleanup(func() { proxy.Close() })

	info := &ClientSideProxyInfo{
		SourceAddr:            proxy.LocalAddr().(*net.UDPAddr),
		LastReceivedTimestamp: time.Now().Add(-age),
	}
	s.handler.clientSideProxySources[testutil.TestPlayerNumber] = append(
		s.handler.clientSideProxySources[testutil.TestPlayerNumber], info)
	s.handler.lastUsedSource[testutil.TestPlayerNumber] = info
	return proxy
}

func (s *testSetup) send(msg string) error {
	data := ClientBoundData{Data: []byte(msg), ClientConnectionPort: testutil.TestReturnTrafficPort}
	return s.handler.HandleClientBoundTraffic(data, s.ccPool, s.socket)
}

func assertNoMessageReceived(t *testing.T, conn *net.UDPConn) {
	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 1024)
	_, _, err := conn.ReadFromUDP(buf)
	assert.True(t, err != nil, "Expected no message due to connectionDropTimeout")
}

func TestServerSideProxyTrafficHandler_PreprocessServerBoundTraffic(t *testing.T) {
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
	handler := &ServerSideProxyTrafficHandler{
		tokenManager:           tokenManager,
		clientSideProxySources: make(map[int][]*ClientSideProxyInfo),
		lastUsedSource:         make(map[int]*ClientSideProxyInfo),
		nextSourceIndex:        make(map[int]int),
	}

	// Create data with player number token (hash already stripped by client-side proxy)
	data := []byte("1|" + testutil.TestMessage)
	sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)

	// Test first call adds source
	result, err := handler.PreprocessServerBoundTraffic(data, sourceAddr)
	assert.NoError(t, err)
	assert.Equal(t, result.PlayerNumber, 1)

	// Verify entire token was stripped
	assert.Equal(t, string(result.Data), testutil.TestMessage)

	// Verify source was added
	sources := handler.clientSideProxySources[result.PlayerNumber]
	assert.Equal(t, len(sources), 1)
	assert.Equal(t, sources[0].SourceAddr.String(), sourceAddr.String())

	// Verify timestamp was updated
	clientLastReceivedTimestamp := handler.clientSideProxySources[testutil.TestPlayerNumber][0].LastReceivedTimestamp
	assert.True(t, time.Since(clientLastReceivedTimestamp) <= time.Second, "Expected timestamp to be updated to recent time")

	// Verify lastUsedSource was set
	assert.Equal(t, handler.lastUsedSource[result.PlayerNumber].SourceAddr.String(), sourceAddr.String())

	// Test duplicate source is not added but timestamp is refreshed
	firstCallTimestamp := clientLastReceivedTimestamp
	time.Sleep(10 * time.Millisecond)
	_, err = handler.PreprocessServerBoundTraffic(data, sourceAddr)
	assert.NoError(t, err)
	assert.Equal(t, len(handler.clientSideProxySources[result.PlayerNumber]), 1)

	// Verify timestamp was refreshed for duplicate source
	secondCallTimestamp := handler.clientSideProxySources[result.PlayerNumber][0].LastReceivedTimestamp
	assert.True(t, secondCallTimestamp.After(firstCallTimestamp), "Timestamp should be updated on duplicate source")
	assert.Equal(t, handler.lastUsedSource[result.PlayerNumber].SourceAddr.String(), sourceAddr.String())
}

func TestServerSideProxyTrafficHandler_HandleClientBoundTraffic(t *testing.T) {
	t.Run("RoundRobin", func(t *testing.T) {
		s := newTestSetup(t)
		proxy1 := s.addEndpoint(t, 0)
		proxy2 := s.addEndpoint(t, 0)

		s.send(testutil.TestMessage)
		msg, _ := testutil.ReadUDPMessage(t, proxy1)
		assert.Equal(t, msg, testutil.TestMessage)

		s.send(testutil.TestMessage)
		msg, _ = testutil.ReadUDPMessage(t, proxy2)
		assert.Equal(t, msg, testutil.TestMessage)
	})

	t.Run("RoundRobinWrapsAround", func(t *testing.T) {
		s := newTestSetup(t)
		proxy1 := s.addEndpoint(t, 0)
		proxy2 := s.addEndpoint(t, 0)

		// Send 4 messages - should cycle: proxy1, proxy2, proxy1, proxy2
		s.send(testutil.TestMessage)
		msg, _ := testutil.ReadUDPMessage(t, proxy1)
		assert.Equal(t, msg, testutil.TestMessage)

		s.send(testutil.TestMessage)
		msg, _ = testutil.ReadUDPMessage(t, proxy2)
		assert.Equal(t, msg, testutil.TestMessage)

		s.send(testutil.TestMessage)
		msg, _ = testutil.ReadUDPMessage(t, proxy1)
		assert.Equal(t, msg, testutil.TestMessage)

		s.send(testutil.TestMessage)
		msg, _ = testutil.ReadUDPMessage(t, proxy2)
		assert.Equal(t, msg, testutil.TestMessage)
	})

	t.Run("FallbackToLastUsed", func(t *testing.T) {
		s := newTestSetup(t)
		proxy := s.addEndpoint(t, 5*time.Second) // stale for round-robin but within connectionDropTimeout

		err := s.send("fallback")
		assert.NoError(t, err)

		msg, _ := testutil.ReadUDPMessage(t, proxy)
		assert.Equal(t, msg, "fallback")
	})

	t.Run("ConnectionDrop", func(t *testing.T) {
		s := newTestSetup(t)
		proxy := s.addEndpoint(t, 65*time.Second)

		err := s.send("blocked")
		assert.NoError(t, err)

		assertNoMessageReceived(t, proxy)

		_, sourcesExist := s.handler.clientSideProxySources[testutil.TestPlayerNumber]
		_, lastUsedExists := s.handler.lastUsedSource[testutil.TestPlayerNumber]
		_, indexExists := s.handler.nextSourceIndex[testutil.TestPlayerNumber]
		assert.True(t, !sourcesExist, "clientSideProxySources should be cleaned up")
		assert.True(t, !lastUsedExists, "lastUsedSource should be cleaned up")
		assert.True(t, !indexExists, "nextSourceIndex should be cleaned up")
	})

	t.Run("EndpointBecomesStale", func(t *testing.T) {
		s := newTestSetup(t)
		s.addEndpoint(t, 2*time.Second) // stale (>1s)
		proxy2 := s.addEndpoint(t, 0)   // active

		// Both messages should go to proxy2 (only active endpoint)
		s.send(testutil.TestMessage)
		msg, _ := testutil.ReadUDPMessage(t, proxy2)
		assert.Equal(t, msg, testutil.TestMessage)

		s.send(testutil.TestMessage)
		msg, _ = testutil.ReadUDPMessage(t, proxy2)
		assert.Equal(t, msg, testutil.TestMessage)
	})

	t.Run("ConnectionRestoredAfterDrop", func(t *testing.T) {
		tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
		s := newTestSetup(t)
		s.handler.tokenManager = tokenManager
		proxy := s.addEndpoint(t, 65*time.Second)

		s.send("blocked")
		assertNoMessageReceived(t, proxy)

		// Client sends a message - this updates lastUsedSource timestamp
		clientData := []byte("1|hello")
		s.handler.PreprocessServerBoundTraffic(clientData, proxy.LocalAddr().(*net.UDPAddr))

		s.send("restored")

		proxy.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		msg, _ := testutil.ReadUDPMessage(t, proxy)
		assert.Equal(t, msg, "restored")
	})
}

func TestServerSideProxyTrafficHandler_FindClientInfo(t *testing.T) {
	handler := &ServerSideProxyTrafficHandler{}

	sources := []*ClientSideProxyInfo{
		{SourceAddr: testAddr1},
		{SourceAddr: testAddr2},
	}

	info, found := handler.findClientInfo(sources, testAddr1)
	assert.True(t, found, "Should find existing source")
	assert.Equal(t, info.SourceAddr.String(), testAddr1.String())

	_, found = handler.findClientInfo(sources, testAddr3)
	assert.True(t, !found, "Should not find non-existent source")
}

func TestServerSideProxyTrafficHandler_FilterActiveSources(t *testing.T) {
	handler := &ServerSideProxyTrafficHandler{}

	now := time.Now()
	sources := []*ClientSideProxyInfo{
		{SourceAddr: testAddr1, LastReceivedTimestamp: now},                              // active
		{SourceAddr: testAddr2, LastReceivedTimestamp: now.Add(-2 * time.Second)},        // stale
		{SourceAddr: testAddr3, LastReceivedTimestamp: now.Add(-500 * time.Millisecond)}, // active
	}

	active := handler.filterActiveSources(sources)
	assert.Equal(t, len(active), 2)
	assert.Equal(t, active[0].SourceAddr.String(), testAddr1.String())
	assert.Equal(t, active[1].SourceAddr.String(), testAddr3.String())
}
