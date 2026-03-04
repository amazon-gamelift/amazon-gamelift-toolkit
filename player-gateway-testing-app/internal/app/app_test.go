/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package app

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

const (
	// testReportFilePath is intentionally different from testutil.TestReportFilePath
	// to avoid conflicts with other tests running concurrently
	testReportFilePath = "test-app-report.txt"
)

func TestNew(t *testing.T) {
	app := New()

	assert.NotNil(t, app, "New should return non-nil app")
}

func TestApp_Run(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()

	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("8000-8010"),
		testutil.WithReportFilePath(testReportFilePath),
		testutil.WithHeadless(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
	defer cancel()

	err := app.Run(ctx, cancel, cfg)
	assert.NoError(t, err, "Unexpected error when running application")

	app.cleanup()
}

// TestSetupProxies_InvalidServerAddress tests setupProxies with invalid server address
func TestSetupProxies_InvalidServerAddress(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithServerIPAddress("invalid-ip-address"),
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.Error(t, err, "Expected error with invalid server address")

	app.cleanup()
}

// TestSetupProxies_InvalidPortRange tests setupProxies with invalid port range
func TestSetupProxies_InvalidPortRange(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("invalid-range"),
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.Error(t, err, "Expected error with invalid port range")

	app.cleanup()
}

// TestSetupProxies_PortRangeOutOfBounds tests setupProxies with out-of-bounds port range
func TestSetupProxies_PortRangeOutOfBounds(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("70000-80000"),
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.Error(t, err, "Expected error with out-of-bounds port range")

	app.cleanup()
}

// TestSetupProxies_Success tests successful proxy setup
func TestSetupProxies_Success(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.NoError(t, err, "Expected no error with valid configuration")
	assert.NotNil(t, app.tokenManager, "Token manager should be initialized")
	assert.NotNil(t, app.serverSideProxy, "Server-side proxy should be created")
	assert.True(t, len(app.clientSideProxies) > 0, "Client-side proxies should be created")

	app.cleanup()
}

// TestInitializeToken_DefaultPlayerCount tests token initialization with default player count
func TestInitializeToken_DefaultPlayerCount(t *testing.T) {
	app := New()
	cfg := testutil.CreateTestConfig()

	err := app.initializeToken(cfg)
	assert.NoError(t, err, "Expected no error when generating token")
	assert.NotNil(t, app.tokenManager, "Token manager should be initialized")
	assert.Equal(t, 1, app.tokenManager.GetPlayerCount(), "Should have 1 player by default")

	token := app.tokenManager.GetTokenForPlayer(1)
	assert.True(t, len(token) > 0, "Generated token should not be empty")

	app.cleanup()
}

// TestInitializeToken_MultiplePlayerCount tests token initialization with multiple players
func TestInitializeToken_MultiplePlayerCount(t *testing.T) {
	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPlayerCount(3),
	)

	err := app.initializeToken(cfg)
	assert.NoError(t, err, "Expected no error with multiple players")
	assert.NotNil(t, app.tokenManager, "Token manager should be initialized")
	assert.Equal(t, 3, app.tokenManager.GetPlayerCount(), "Should have 3 players")

	// Verify each player has a token
	for i := 1; i <= 3; i++ {
		token := app.tokenManager.GetTokenForPlayer(i)
		assert.True(t, len(token) > 0, "Player should have a token")
	}

	app.cleanup()
}

// TestSetupProxies_EmptyPortRange tests client proxy creation with auto-allocation
func TestSetupProxies_EmptyPortRange(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange(""),
		testutil.WithUDPEndpointCount(2),
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.NoError(t, err, "Client proxy creation should succeed with auto-allocation")
	assert.Equal(t, 2, len(app.clientSideProxies), "Should create 2 client proxies")

	app.cleanup()
}

// TestSetupProxies_SpecifiedPortRange tests client proxy creation with specified port range
func TestSetupProxies_SpecifiedPortRange(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("15000-15010"),
		testutil.WithUDPEndpointCount(3),
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.NoError(t, err, "Client proxy creation should succeed with specified port range")
	assert.Equal(t, 3, len(app.clientSideProxies), "Should create 3 client proxies")

	// Verify ports are in expected range
	for i, proxy := range app.clientSideProxies {
		addr := proxy.LocalAddr().(*net.UDPAddr)
		expectedPort := 15000 + i
		assert.Equal(t, addr.Port, expectedPort, "Proxy port should match expected port")
	}

	app.cleanup()
}

// TestSetupProxies_MultipleEndpoints tests client proxy creation with multiple endpoints
func TestSetupProxies_MultipleEndpoints(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithUDPEndpointCount(5),
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.NoError(t, err, "Client proxy creation should succeed w multiple endpoints")
	assert.Equal(t, 5, len(app.clientSideProxies), "Should create 5 client proxies")

	app.cleanup()
}

// TestCleanup_NilProxies tests cleanup with nil proxies
func TestCleanup_NilProxies(t *testing.T) {
	app := New()
	app.clientSideProxies = nil
	app.serverSideProxy = nil

	// Should not panic
	app.cleanup()
}

// TestCleanup_AlreadyClosedProxies tests cleanup with already closed proxies
func TestCleanup_AlreadyClosedProxies(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("11000-11010"),
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.NoError(t, err, "Setup should succeed")

	// Close proxies once
	app.cleanup()

	// Close again - should handle gracefully
	app.cleanup()
}

// TestRun_SetupProxiesFailure tests that Run returns error when setupProxies fails
func TestRun_SetupProxiesFailure(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()

	cfg := testutil.CreateTestConfig(
		testutil.WithIPAddress("invalid-ip-address"),
		testutil.WithReportFilePath(testReportFilePath),
	)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
	defer cancel()

	err := app.Run(ctx, cancel, cfg)
	assert.Error(t, err, "Expected error when setupProxies fails")

	assert.True(t, app.serverSideProxy == nil, "Server-side proxy should not be created on setup failure")
	assert.Equal(t, len(app.clientSideProxies), 0, "Client-side proxies should not be created on setup failure")

	app.cleanup()
}

// TestSetupProxies_PortRangeExhausted tests client proxy creation when port range is exhausted
func TestSetupProxies_PortRangeExhausted(t *testing.T) {
	defer os.Remove(testReportFilePath)

	app := New()

	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange(""),
		testutil.WithUDPEndpointCount(70000), // Request more ports than available
		testutil.WithReportFilePath(testReportFilePath),
	)

	err := app.setupProxies(cfg)
	assert.Error(t, err, "Expected error when port range exhausted")

	app.cleanup()
}
