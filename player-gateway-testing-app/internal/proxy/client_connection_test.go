/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"context"
	"net"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

func TestClientConnectionPool_GetClientConnection(t *testing.T) {
	ccPool := &ClientConnectionPool{
		clientConnectionMap: make(map[int]*ClientConnection),
	}

	// Test non-existent client connection
	_, exists := ccPool.GetClientConnection(testutil.TestPlayerNumber)
	assert.False(t, exists, "Client connection should not exist")

	// Add client connection and test
	clientConn := &ClientConnection{returnTrafficPort: testutil.TestReturnTrafficPort, serverBoundTrafficChan: make(chan []byte, 1)}
	ccPool.clientConnectionMap[testutil.TestPlayerNumber] = clientConn

	_, exists = ccPool.GetClientConnection(testutil.TestPlayerNumber)
	assert.True(t, exists, "Client connection should exist")
}

func TestClientConnection_ReturnTrafficPort(t *testing.T) {
	clientConn := &ClientConnection{returnTrafficPort: testutil.TestReturnTrafficPort}

	assert.Equal(t, clientConn.ReturnTrafficPort(), testutil.TestReturnTrafficPort)
}

func TestClientConnectionPool_GetSourceFromReturnPort(t *testing.T) {
	ccPool := &ClientConnectionPool{
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
	}

	// Test non-existent port
	_, exists := ccPool.GetSourceAddrFromClientConnectionPort(testutil.TestReturnTrafficPort)
	assert.False(t, exists, "Source should not exist")

	// Add source and test
	addr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
	ccPool.clientConnectionPortMap[testutil.TestReturnTrafficPort] = &ClientConnectionInfo{
		ClientAddr:   addr,
		PlayerNumber: testutil.TestPlayerNumber,
	}

	result, exists := ccPool.GetSourceAddrFromClientConnectionPort(testutil.TestReturnTrafficPort)
	assert.True(t, exists, "Source should exist")
	assert.Equal(t, result.String(), testutil.TestSourceAddr)
}

func TestClientConnectionPool_CreateClientConnection(t *testing.T) {
	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	returnChan := make(chan ClientBoundData, testutil.TestChannelSize)

	ccPool := &ClientConnectionPool{
		clientConnectionMap:     make(map[int]*ClientConnection),
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
		destinationAddr:         destinationAddr,
		forwardToPlayerChan:     returnChan,
	}

	sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)

	clientConn, err := ccPool.CreateClientConnection(context.Background(), testutil.TestPlayerNumber, sourceAddr)
	assert.NoError(t, err)
	assert.NotNil(t, clientConn, "Client connection should not be nil")
	assert.NotEqual(t, clientConn.ReturnTrafficPort(), 0, "Return traffic port should be assigned")

	// Check client connection is stored
	storedClientConn, exists := ccPool.GetClientConnection(testutil.TestPlayerNumber)
	assert.True(t, exists, "Client connection should be stored in pool")
	assert.Equal(t, storedClientConn, clientConn, "Stored client connection should match created client connection")

	// Check source mapping
	storedSource, exists := ccPool.GetSourceAddrFromClientConnectionPort(clientConn.ReturnTrafficPort())
	assert.True(t, exists, "Source mapping should exist")
	assert.Equal(t, storedSource.String(), sourceAddr.String())

	// Cleanup
	ccPool.CloseClientConnection(testutil.TestPlayerNumber)
}

func TestClientConnectionPool_GetOrCreateClientConnection(t *testing.T) {
	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	returnChan := make(chan ClientBoundData, testutil.TestChannelSize)

	ccPool := &ClientConnectionPool{
		clientConnectionMap:     make(map[int]*ClientConnection),
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
		destinationAddr:         destinationAddr,
		forwardToPlayerChan:     returnChan,
	}

	sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)

	// First call should create client connection
	clientConn1, err := ccPool.GetOrCreateClientConnection(context.Background(), testutil.TestPlayerNumber, sourceAddr)
	assert.NoError(t, err)

	// Second call should return existing client connection
	clientConn2, err := ccPool.GetOrCreateClientConnection(context.Background(), testutil.TestPlayerNumber, sourceAddr)
	assert.NoError(t, err)
	assert.Equal(t, clientConn1, clientConn2, "Should return same client connection instance")

	// Cleanup
	ccPool.CloseClientConnection(testutil.TestPlayerNumber)
}

func TestClientConnectionPool_CloseClientConnection(t *testing.T) {
	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	returnChan := make(chan ClientBoundData, testutil.TestChannelSize)

	ccPool := &ClientConnectionPool{
		clientConnectionMap:     make(map[int]*ClientConnection),
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
		destinationAddr:         destinationAddr,
		forwardToPlayerChan:     returnChan,
	}

	sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)

	// Create client connection
	clientConn, err := ccPool.CreateClientConnection(context.Background(), testutil.TestPlayerNumber, sourceAddr)
	assert.NoError(t, err)

	port := clientConn.ReturnTrafficPort()

	// Verify client connection exists
	_, exists := ccPool.GetClientConnection(testutil.TestPlayerNumber)
	assert.True(t, exists, "Client connection should exist before close")

	// Close client connection
	ccPool.CloseClientConnection(testutil.TestPlayerNumber)

	// Verify client connection is removed
	_, exists = ccPool.GetClientConnection(testutil.TestPlayerNumber)
	assert.False(t, exists, "Client connection should not exist after close")

	// Verify source mapping is removed
	_, exists = ccPool.GetSourceAddrFromClientConnectionPort(port)
	assert.False(t, exists, "Source mapping should not exist after close")
}

func TestClientConnectionPool_AllocateSocket(t *testing.T) {
	ccPool := &ClientConnectionPool{}

	socket, err := ccPool.allocateSocket()
	assert.NoError(t, err)
	defer socket.Close()

	assert.NotNil(t, socket, "Socket should not be nil")

	localAddr := socket.LocalAddr()
	assert.NotNil(t, localAddr, "Local address should not be nil")

	addr, ok := localAddr.(*net.UDPAddr)
	assert.True(t, ok, "Failed to cast local address to UDP address")
	assert.NotEqual(t, addr.Port, 0, "Port should be assigned")
}

// TestClientConnectionPool_GetPlayerNumberFromClientConnectionPort tests retrieving player number from port
func TestClientConnectionPool_GetPlayerNumberFromClientConnectionPort(t *testing.T) {
	ccPool := &ClientConnectionPool{
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
	}

	// Test non-existent port
	playerNumber, exists := ccPool.GetPlayerNumberFromClientConnectionPort(testutil.TestReturnTrafficPort)
	assert.False(t, exists, "Player number should not exist for non-existent port")
	assert.Equal(t, 0, playerNumber, "Player number should be 0 when not found")

	addr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
	ccPool.clientConnectionPortMap[testutil.TestReturnTrafficPort] = &ClientConnectionInfo{
		ClientAddr:   addr,
		PlayerNumber: testutil.TestPlayerNumber,
	}

	playerNumber, exists = ccPool.GetPlayerNumberFromClientConnectionPort(testutil.TestReturnTrafficPort)
	assert.True(t, exists, "Player number should exist")
	assert.Equal(t, testutil.TestPlayerNumber, playerNumber, "Player number should match")
}
