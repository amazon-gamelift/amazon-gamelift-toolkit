/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

func TestNewClientSideProxy(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	proxy, err := NewClientSideProxy(testutil.TestProxyAddr, 0, destinationAddr, tokenManager, 1, testutil.TestReportFilePath, nil, nil)
	assert.NoError(t, err)
	defer proxy.Close()

	assert.NotNil(t, proxy.socket, "Socket should not be nil")
	assert.NotNil(t, proxy.clientConnectionPool, "ClientConnectionPool should not be nil")
	assert.NotNil(t, proxy.LocalAddr(), "LocalAddr should not be nil")
}

func TestNewClientProxy_NilTokenManager(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)

	proxy, err := NewClientSideProxy(testutil.TestProxyAddr, 0, destinationAddr, nil, 1, testutil.TestReportFilePath, nil, nil)
	assert.Error(t, err, "Expected error but didn't receive one")
	assert.Equal(t, proxy, (*Proxy)(nil), "Expected client-side proxy to be nil")
}

func TestNewServerSideProxy(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	proxy, err := NewServerSideProxy(testutil.TestProxyAddr, 0, destinationAddr, tokenManager, testutil.TestReportFilePath, nil)
	assert.NoError(t, err)
	defer proxy.Close()

	assert.NotNil(t, proxy.socket, "Socket should not be nil")
	assert.NotNil(t, proxy.clientConnectionPool, "ClientConnectionPool should not be nil")
	assert.NotNil(t, proxy.LocalAddr(), "LocalAddr should not be nil")
}

func TestNewServerSideProxy_NilTokenManager(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)

	proxy, err := NewServerSideProxy(testutil.TestProxyAddr, 0, destinationAddr, nil, testutil.TestReportFilePath, nil)
	assert.Error(t, err, "Expected error but didn't receive one")
	assert.Equal(t, proxy, (*Proxy)(nil), "Expected server-side proxy to be nil")
}

func TestProxy_LocalAddr(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	proxy, err := NewClientSideProxy(testutil.TestProxyAddr, 0, destinationAddr, tokenManager, 1, testutil.TestReportFilePath, nil, nil)
	assert.NoError(t, err)
	defer proxy.Close()

	addr := proxy.LocalAddr()
	assert.NotNil(t, addr, "LocalAddr should return valid address")

	udpAddr, ok := addr.(*net.UDPAddr)
	assert.True(t, ok, "LocalAddr should return UDP address")
	assert.NotEqual(t, udpAddr.Port, 0, "Port should be assigned")
}

func TestProxy_Close(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	proxy, err := NewClientSideProxy(testutil.TestProxyAddr, 0, destinationAddr, tokenManager, 1, testutil.TestReportFilePath, nil, nil)
	assert.NoError(t, err)

	err = proxy.Close()
	assert.NoError(t, err, "Close should not return error")
}

func TestProxy_StartStop(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	proxy, err := NewClientSideProxy(testutil.TestProxyAddr, 0, destinationAddr, tokenManager, 1, testutil.TestReportFilePath, nil, nil)
	assert.NoError(t, err)
	defer proxy.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := proxy.Start(ctx)
		assert.NoError(t, err, "Unexpected error from Start")
	}()

	// Wait for context timeout
	<-ctx.Done()
	wg.Wait()
}

func TestProxy_ClientCanSendAndReceiveThroughProxy(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	// Create mock game server
	gameServer := testutil.CreateTestUDPSocket(t)
	defer gameServer.Close()

	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	// Create proxy
	proxy, err := NewClientSideProxy(testutil.TestProxyAddr, 0, gameServer.LocalAddr().(*net.UDPAddr), tokenManager, 1, testutil.TestReportFilePath, nil, nil)
	assert.NoError(t, err)
	defer proxy.Close()

	// Start proxy in background
	ctx, cancel := context.WithTimeout(context.Background(), 5*testutil.TestTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := proxy.Start(ctx)
		assert.NoError(t, err, "Unexpected error from Start")
	}()

	// Create client
	client := testutil.CreateTestUDPSocket(t)
	defer client.Close()

	// Test forward traffic: client -> proxy -> game server
	// Prepend valid token (decoded) to test data
	testData := []byte(testutil.TestTokenHashDecoded + testutil.TestMessage)
	testutil.SendUDPMessage(t, client, proxy.LocalAddr().(*net.UDPAddr), testData)

	// Game server receives forward traffic
	receivedMessage, sourceAddr := testutil.ReadUDPMessage(t, gameServer)

	expectedMessage := "1|" + testutil.TestMessage
	assert.Equal(t, receivedMessage, expectedMessage)

	// Test return traffic: game server -> proxy -> client
	responseData := []byte(testutil.TestResponse)
	testutil.SendUDPMessage(t, gameServer, sourceAddr, responseData)

	// Client receives return traffic
	receivedString, _ := testutil.ReadUDPMessage(t, client)
	assert.Equal(t, receivedString, string(responseData))

	cancel()
	wg.Wait()
}

func TestProxy_Start_ContinuesOnPacketDropped(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	destinationAddr, _ := net.ResolveUDPAddr("udp", testutil.TestDestinationAddr)
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	proxy, err := NewClientSideProxy(testutil.TestProxyAddr, 0, destinationAddr, tokenManager, 1, testutil.TestReportFilePath, nil, nil)
	assert.NoError(t, err)
	defer proxy.Close()

	// Create a mock traffic handler that returns ErrPacketDropped
	mockHandler := &mockTrafficHandlerDropsPackets{}
	proxy.trafficHandler = mockHandler

	ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := proxy.Start(ctx)
		assert.NoError(t, err, "Start should not return error when ErrPacketDropped is encountered")
	}()

	// Send data to the forwardToPlayerChan to trigger the ErrPacketDropped branch
	proxy.forwardToPlayerChan <- ClientBoundData{
		ClientConnectionPort: 12345,
		Data:                 []byte("test data"),
	}

	// Wait for context timeout to ensure the proxy continues running
	<-ctx.Done()
	wg.Wait()

	// Verify that HandleClientBoundTraffic was called
	assert.True(t, mockHandler.called, "HandleClientBoundTraffic should have been called")
}

// mockTrafficHandlerDropsPackets is a mock traffic handler that always returns ErrPacketDropped
type mockTrafficHandlerDropsPackets struct {
	called bool
}

func (m *mockTrafficHandlerDropsPackets) PreprocessServerBoundTraffic(data []byte, sourceAddr *net.UDPAddr) (PreprocessServerBoundTrafficResult, error) {
	return PreprocessServerBoundTrafficResult{}, nil
}

func (m *mockTrafficHandlerDropsPackets) HandleClientBoundTraffic(data ClientBoundData, ccPool *ClientConnectionPool, socket *net.UDPConn) error {
	m.called = true
	return ErrPacketDropped
}
