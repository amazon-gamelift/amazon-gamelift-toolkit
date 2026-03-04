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

func TestServerSideProxyTrafficHandler_PreprocessServerBoundTraffic(t *testing.T) {
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
	handler := &ServerSideProxyTrafficHandler{
		tokenManager:           tokenManager,
		clientSideProxySources: make(map[int][]*ClientSideProxyInfo),
		nextSourceIndexByPort:  make(map[int]int),
		recentMessages:         make(map[int][]*net.UDPAddr),
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

	clientLastReceivedTimestamp := handler.clientSideProxySources[testutil.TestPlayerNumber][0].LastReceivedTimestamp
	assert.True(t, time.Since(clientLastReceivedTimestamp) <= time.Second, "Expected timestamp to be updated to recent time")

	assert.Equal(t, sources[0].SourceAddr.String(), sourceAddr.String())

	// Test duplicate source is not added
	_, err = handler.PreprocessServerBoundTraffic(data, sourceAddr)
	assert.NoError(t, err)

	sources = handler.clientSideProxySources[result.PlayerNumber]
	assert.Equal(t, len(sources), 1)
}

func TestServerSideProxyTrafficHandler_HandleClientBoundTraffic(t *testing.T) {
	handler := &ServerSideProxyTrafficHandler{
		clientSideProxySources: make(map[int][]*ClientSideProxyInfo),
		nextSourceIndexByPort:  make(map[int]int),
		recentMessages:         make(map[int][]*net.UDPAddr),
	}

	socket := testutil.CreateTestUDPSocket(t)
	defer socket.Close()

	proxySocket := testutil.CreateTestUDPSocket(t)
	defer proxySocket.Close()

	proxyAddr := proxySocket.LocalAddr().(*net.UDPAddr)

	handler.clientSideProxySources[testutil.TestPlayerNumber] = []*ClientSideProxyInfo{
		{
			SourceAddr:            proxyAddr,
			LastReceivedTimestamp: time.Now().Add(-100 * time.Second),
		},
	}
	handler.recentMessages[testutil.TestPlayerNumber] = []*net.UDPAddr{proxyAddr}

	ccPool := &ClientConnectionPool{
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
	}
	ccPool.clientConnectionPortMap[testutil.TestReturnTrafficPort] = &ClientConnectionInfo{
		PlayerNumber: testutil.TestPlayerNumber,
	}

	data := ClientBoundData{
		Data:                 []byte(testutil.TestMessage),
		ClientConnectionPort: testutil.TestReturnTrafficPort,
	}

	// Test successful handling
	err := handler.HandleClientBoundTraffic(data, ccPool, socket)
	assert.NoError(t, err)

	// Verify round robin index stays at 0 for single source
	assert.Equal(t, handler.nextSourceIndexByPort[testutil.TestReturnTrafficPort], 0)

	receivedString, _ := testutil.ReadUDPMessage(t, proxySocket)
	assert.Equal(t, receivedString, string(testutil.TestMessage))

	// Test with no sources (should not error)
	handler.clientSideProxySources[testutil.TestPlayerNumber] = []*ClientSideProxyInfo{}
	err = handler.HandleClientBoundTraffic(data, ccPool, socket)
	assert.NoError(t, err)
}

func TestServerSideProxyTrafficHandler_FindClientInfo(t *testing.T) {

	tests := []struct {
		name          string
		sources       []*ClientSideProxyInfo
		searchAddr    *net.UDPAddr
		expectedFound bool
	}{
		{
			name: "find existing source",
			sources: []*ClientSideProxyInfo{
				{SourceAddr: testAddr1, LastReceivedTimestamp: time.Now().Add(-10 * time.Second)},
				{SourceAddr: testAddr2, LastReceivedTimestamp: time.Now().Add(-5 * time.Second)},
			},
			searchAddr:    testAddr1,
			expectedFound: true,
		},
		{
			name: "not finding non-existent source",
			sources: []*ClientSideProxyInfo{
				{SourceAddr: testAddr1, LastReceivedTimestamp: time.Now().Add(-10 * time.Second)},
				{SourceAddr: testAddr2, LastReceivedTimestamp: time.Now().Add(-5 * time.Second)},
			},
			searchAddr:    testAddr3,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ServerSideProxyTrafficHandler{}
			_, found := handler.findClientInfo(tt.sources, tt.searchAddr)

			assert.Equal(t, found, tt.expectedFound)
		})
	}
}

func TestServerSideProxyTrafficHandler_AddToRecentMessages(t *testing.T) {
	handler := &ServerSideProxyTrafficHandler{
		recentMessages: make(map[int][]*net.UDPAddr),
	}

	// Add first message
	handler.addToRecentMessages(testutil.TestPlayerNumber, testAddr1)
	assert.Equal(t, len(handler.recentMessages[testutil.TestPlayerNumber]), 1)

	// Add more messages up to the limit
	for i := 0; i < 19; i++ {
		handler.addToRecentMessages(testutil.TestPlayerNumber, testAddr2)
	}

	assert.Equal(t, len(handler.recentMessages[testutil.TestPlayerNumber]), 20)

	// Add one more to trigger trimming
	handler.addToRecentMessages(testutil.TestPlayerNumber, testAddr1)
	assert.Equal(t, len(handler.recentMessages[testutil.TestPlayerNumber]), 20)

	// Verify oldest message was removed (first testAddr1 should be gone)
	assert.Equal(t, handler.recentMessages[testutil.TestPlayerNumber][0].String(), testAddr2.String())
}

func TestServerSideProxyTrafficHandler_IsInRecentMessages(t *testing.T) {
	tests := []struct {
		name           string
		recentMessages map[int][]*net.UDPAddr
		playerNumber   int
		searchAddr     *net.UDPAddr
		expectedFound  bool
	}{
		{
			name:           "find existing address addr1",
			recentMessages: map[int][]*net.UDPAddr{testutil.TestPlayerNumber: {testAddr1, testAddr2}},
			playerNumber:   testutil.TestPlayerNumber,
			searchAddr:     testAddr1,
			expectedFound:  true,
		},
		{
			name:           "find existing address addr2",
			recentMessages: map[int][]*net.UDPAddr{testutil.TestPlayerNumber: {testAddr1, testAddr2}},
			playerNumber:   testutil.TestPlayerNumber,
			searchAddr:     testAddr2,
			expectedFound:  true,
		},
		{
			name:           "not finding non-existent address",
			recentMessages: map[int][]*net.UDPAddr{testutil.TestPlayerNumber: {testAddr1, testAddr2}},
			playerNumber:   testutil.TestPlayerNumber,
			searchAddr:     testAddr3,
			expectedFound:  false,
		},
		{
			name:           "empty recent messages for non-existent player",
			recentMessages: map[int][]*net.UDPAddr{testutil.TestPlayerNumber: {testAddr1, testAddr2}},
			playerNumber:   999,
			searchAddr:     testAddr1,
			expectedFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ServerSideProxyTrafficHandler{
				recentMessages: tt.recentMessages,
			}

			found := handler.isInRecentMessages(tt.playerNumber, tt.searchAddr)
			assert.Equal(t, found, tt.expectedFound)
		})
	}
}

func TestServerSideProxyTrafficHandler_IsSourceActive(t *testing.T) {
	tests := []struct {
		name           string
		recentMessages []*net.UDPAddr
		clientInfo     *ClientSideProxyInfo
		expectedActive bool
	}{
		{
			name:           "active source - in recent messages and recent timestamp",
			recentMessages: []*net.UDPAddr{testAddr1},
			clientInfo: &ClientSideProxyInfo{
				SourceAddr:            testAddr1,
				LastReceivedTimestamp: time.Now(),
			},
			expectedActive: true,
		},
		{
			name:           "inactive source - not in recent messages",
			recentMessages: []*net.UDPAddr{testAddr1},
			clientInfo: &ClientSideProxyInfo{
				SourceAddr:            testAddr2,
				LastReceivedTimestamp: time.Now(),
			},
			expectedActive: false,
		},
		{
			name:           "inactive source - old timestamp",
			recentMessages: []*net.UDPAddr{testAddr1},
			clientInfo: &ClientSideProxyInfo{
				SourceAddr:            testAddr1,
				LastReceivedTimestamp: time.Now().Add(-35 * time.Second),
			},
			expectedActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ServerSideProxyTrafficHandler{
				recentMessages: map[int][]*net.UDPAddr{testutil.TestPlayerNumber: tt.recentMessages},
			}

			active := handler.isSourceActive(testutil.TestPlayerNumber, tt.clientInfo)
			assert.Equal(t, active, tt.expectedActive)
		})
	}
}

func TestServerSideProxyTrafficHandler_RemoveStaleSources(t *testing.T) {
	tests := []struct {
		name                string
		recentMessages      []*net.UDPAddr
		sources             []*ClientSideProxyInfo
		startIndex          int
		expectedSourceCount int
		expectedFirstAddr   string
	}{
		{
			name:           "remove stale source at index 0",
			recentMessages: []*net.UDPAddr{testAddr2, testAddr3},
			sources: []*ClientSideProxyInfo{
				{SourceAddr: testAddr1, LastReceivedTimestamp: time.Now().Add(-35 * time.Second)},
				{SourceAddr: testAddr2, LastReceivedTimestamp: time.Now()},
				{SourceAddr: testAddr3, LastReceivedTimestamp: time.Now()},
			},
			startIndex:          0,
			expectedSourceCount: 2,
			expectedFirstAddr:   testAddr2.String(),
		},
		{
			name:           "all active sources - no removal",
			recentMessages: []*net.UDPAddr{testAddr2, testAddr3},
			sources: []*ClientSideProxyInfo{
				{SourceAddr: testAddr2, LastReceivedTimestamp: time.Now()},
				{SourceAddr: testAddr3, LastReceivedTimestamp: time.Now()},
			},
			startIndex:          1,
			expectedSourceCount: 2,
			expectedFirstAddr:   testAddr2.String(),
		},
		{
			name:           "single stale source - should not remove",
			recentMessages: []*net.UDPAddr{},
			sources: []*ClientSideProxyInfo{
				{SourceAddr: testAddr1, LastReceivedTimestamp: time.Now().Add(-31 * time.Second)},
			},
			startIndex:          0,
			expectedSourceCount: 1,
			expectedFirstAddr:   testAddr1.String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ServerSideProxyTrafficHandler{
				recentMessages: map[int][]*net.UDPAddr{testutil.TestPlayerNumber: tt.recentMessages},
			}

			updatedSources := handler.removeStaleSources(tt.sources, tt.startIndex, testutil.TestPlayerNumber)

			assert.Equal(t, len(updatedSources), tt.expectedSourceCount)

			if len(updatedSources) > 0 {
				assert.Equal(t, updatedSources[0].SourceAddr.String(), tt.expectedFirstAddr)
			}
		})
	}
}
