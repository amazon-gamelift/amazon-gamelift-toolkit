/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package integrationtest

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/app"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

const (
	testMessage  = "hello"
	testResponse = "world"
)

// TestFullRequestLifecycle tests the complete flow from client to server and back
func TestFullRequestLifecycle(t *testing.T) {
	reportFilePath := fmt.Sprintf("integ-test-full-lifecycle-%d.txt", time.Now().Unix())
	defer os.Remove(reportFilePath)

	// Create game server
	gameServer := testutil.CreateTestUDPSocketWithPort(t, 9000)
	defer gameServer.Close()

	// Start app
	app := app.New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("8300-8310"),
		testutil.WithUDPEndpointCount(3),
		testutil.WithReportFilePath(reportFilePath),
		testutil.WithHeadless(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go app.Run(ctx, cancel, cfg)
	time.Sleep(50 * time.Millisecond)

	// Create client
	client := testutil.CreateTestUDPSocket(t)
	defer client.Close()

	// Test complete lifecycle
	proxyAddr, err := net.ResolveUDPAddr("udp", testutil.TestProxyAddr+":8300")
	assert.NoError(t, err, "Failed to resolve proxy addr")

	// Replace token with known test token
	replaceTokenCmd := fmt.Sprintf("PlayerGateway:ReplaceToken:1:%s", testutil.TestTokenHash)
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(replaceTokenCmd))
	time.Sleep(10 * time.Millisecond)

	// Step 1: Client sends tokenized packet to client-side proxy
	tokenizedMessage := fmt.Sprintf("%s%s", testutil.TestTokenHashDecoded, testMessage)
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(tokenizedMessage))

	// Step 2: Server-side proxy forwards to game server (token stripped)
	message, sourceAddr := testutil.ReadUDPMessage(t, gameServer)
	assert.Equal(t, message, testMessage, "Game server should receive message without token")

	// Step 3: Game server responds
	testutil.SendUDPMessage(t, gameServer, sourceAddr, []byte(testResponse))

	// Step 4: Response flows back through proxies to client
	responseData, _, err := testutil.WaitForUDPMessage(t, client, testutil.TestTimeout)
	assert.NoError(t, err, "Client failed to receive response")
	assert.Equal(t, string(responseData), testResponse, "Client should receive response")

	// Verify data integrity through the pipeline
	assert.Equal(t, testResponse, string(responseData), "Data integrity maintained through pipeline")
}

// TestTokenManagementLifecycle tests dynamic token operations using ReplaceToken
func TestTokenManagementLifecycle(t *testing.T) {
	reportFilePath := fmt.Sprintf("integ-test-token-lifecycle-%d.txt", time.Now().Unix())
	defer os.Remove(reportFilePath)

	// Create game server
	gameServer := testutil.CreateTestUDPSocketWithPort(t, 9000)
	defer gameServer.Close()

	// Start app with 1 player
	app := app.New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("8400-8410"),
		testutil.WithUDPEndpointCount(1),
		testutil.WithReportFilePath(reportFilePath),
		testutil.WithHeadless(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go app.Run(ctx, cancel, cfg)
	time.Sleep(50 * time.Millisecond)

	// Create client
	client := testutil.CreateTestUDPSocket(t)
	defer client.Close()

	proxyAddr, err := net.ResolveUDPAddr("udp", testutil.TestProxyAddr+":8400")
	assert.NoError(t, err, "Failed to resolve proxy addr")

	// Step 1: Get the auto-generated token and verify it works
	originalToken := app.GetDecodedTokenForPlayer(1)
	tokenizedMessage := fmt.Sprintf("%s%s", originalToken, testMessage)
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(tokenizedMessage))

	_, _, err = testutil.WaitForUDPMessage(t, gameServer, testutil.TestTimeout)
	assert.NoError(t, err, "Original token should work")

	// Step 2: Replace with a different token
	newTokenDecoded := "newtoken22222222222222222222"                     // 28 bytes
	newToken := "bmV3dG9rZW4yMjIyMjIyMjIyMjIyMjIyMjIyMg=="               // base64 of above
	replaceTokenCmd := fmt.Sprintf("PlayerGateway:ReplaceToken:1:%s", newToken)
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(replaceTokenCmd))
	time.Sleep(10 * time.Millisecond)

	// Step 3: Verify old token no longer works
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(tokenizedMessage))
	_, _, err = testutil.WaitForUDPMessage(t, gameServer, testutil.TestTimeout)
	assert.Error(t, err, "Old token should not work after replacement")

	// Step 4: Verify new token works
	newTokenizedMessage := fmt.Sprintf("%s%s", newTokenDecoded, testMessage)
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(newTokenizedMessage))
	_, _, err = testutil.WaitForUDPMessage(t, gameServer, testutil.TestTimeout)
	assert.NoError(t, err, "New token should work after replacement")
}

// TestDegradationAndPacketLoss tests packet degradation functionality
func TestDegradationAndPacketLoss(t *testing.T) {
	reportFilePath := fmt.Sprintf("integ-test-degradation-%d.txt", time.Now().Unix())
	defer os.Remove(reportFilePath)

	// Create game server
	gameServer := testutil.CreateTestUDPSocketWithPort(t, 9000)
	defer gameServer.Close()

	// Start app
	app := app.New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("8500-8510"),
		testutil.WithUDPEndpointCount(1),
		testutil.WithReportFilePath(reportFilePath),
		testutil.WithHeadless(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go app.Run(ctx, cancel, cfg)
	time.Sleep(50 * time.Millisecond)

	// Create client
	client := testutil.CreateTestUDPSocket(t)
	defer client.Close()

	proxyAddr, err := net.ResolveUDPAddr("udp", testutil.TestProxyAddr+":8500")
	assert.NoError(t, err, "Failed to resolve proxy addr")

	// Replace token with known test token
	replaceTokenCmd := fmt.Sprintf("PlayerGateway:ReplaceToken:1:%s", testutil.TestTokenHash)
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(replaceTokenCmd))
	time.Sleep(10 * time.Millisecond)

	tokenizedMessage := fmt.Sprintf("%s%s", testutil.TestTokenHashDecoded, testMessage)

	// Test 1: 0% degradation (all packets pass)
	setDegradationCmd := "PlayerGateway:SetDegradation:0"
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(setDegradationCmd))

	successCount := 0
	for i := 0; i < 10; i++ {
		testutil.SendUDPMessage(t, client, proxyAddr, []byte(tokenizedMessage))

		_, _, err = testutil.WaitForUDPMessage(t, gameServer, 50*time.Millisecond)
		if err == nil {
			successCount++
		}
	}
	assert.Equal(t, 10, successCount, "With 0% degradation, all packets should pass")

	// Test 2: 100% degradation (all packets drop)
	setDegradationCmd = "PlayerGateway:SetDegradation:100"
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(setDegradationCmd))

	successCount = 0
	for i := 0; i < 10; i++ {
		testutil.SendUDPMessage(t, client, proxyAddr, []byte(tokenizedMessage))

		_, _, err = testutil.WaitForUDPMessage(t, gameServer, 50*time.Millisecond)
		if err == nil {
			successCount++
		}
	}
	assert.Equal(t, 0, successCount, "With 100% degradation, all packets should drop")

	// Test 3: 50% degradation (statistical validation)
	setDegradationCmd = "PlayerGateway:SetDegradation:50"
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(setDegradationCmd))

	successCount = 0
	totalAttempts := 100
	for i := 0; i < totalAttempts; i++ {
		testutil.SendUDPMessage(t, client, proxyAddr, []byte(tokenizedMessage))

		_, _, err = testutil.WaitForUDPMessage(t, gameServer, 20*time.Millisecond)
		if err == nil {
			successCount++
		}
	}

	// With 50% degradation, expect roughly 50% success (allow 20% margin)
	successRate := float64(successCount) / float64(totalAttempts)
	assert.True(t, successRate >= 0.30 && successRate <= 0.70,
		fmt.Sprintf("With 50%% degradation, success rate should be ~50%%, got %.2f%%", successRate*100))

	// Verify dropped packets are logged
	content, err := os.ReadFile(reportFilePath)
	assert.NoError(t, err, "Failed to read log file")

	logContent := string(content)
	assert.True(t, strings.Contains(logContent, "packet dropped due to degradation"),
		"Dropped packets should be logged")
}

// TestMultipleConcurrentClients tests handling of multiple simultaneous client connections
func TestMultipleConcurrentClients(t *testing.T) {
	reportFilePath := fmt.Sprintf("integ-test-concurrent-clients-%d.txt", time.Now().Unix())
	defer os.Remove(reportFilePath)

	// Create game server
	gameServer := testutil.CreateTestUDPSocketWithPort(t, 9000)
	defer gameServer.Close()

	// Start app
	app := app.New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("8600-8610"),
		testutil.WithUDPEndpointCount(1),
		testutil.WithReportFilePath(reportFilePath),
		testutil.WithHeadless(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go app.Run(ctx, cancel, cfg)
	time.Sleep(50 * time.Millisecond)

	proxyAddr, err := net.ResolveUDPAddr("udp", testutil.TestProxyAddr+":8600")
	assert.NoError(t, err, "Failed to resolve proxy addr")

	// Replace token with known test token
	setupClient := testutil.CreateTestUDPSocket(t)
	replaceTokenCmd := fmt.Sprintf("PlayerGateway:ReplaceToken:1:%s", testutil.TestTokenHash)
	testutil.SendUDPMessage(t, setupClient, proxyAddr, []byte(replaceTokenCmd))
	setupClient.Close()
	time.Sleep(10 * time.Millisecond)

	// Create multiple clients
	numClients := 5
	clients := make([]*net.UDPConn, numClients)
	for i := 0; i < numClients; i++ {
		client := testutil.CreateTestUDPSocket(t)
		defer client.Close()
		clients[i] = client
	}

	// Send packets from all clients simultaneously
	var wg sync.WaitGroup
	receivedMessages := make(map[string]bool)
	var mu sync.Mutex

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientIndex int) {
			defer wg.Done()

			client := clients[clientIndex]
			message := fmt.Sprintf("message-from-client-%d", clientIndex)
			tokenizedMessage := fmt.Sprintf("%s%s", testutil.TestTokenHashDecoded, message)

			testutil.SendUDPMessage(t, client, proxyAddr, []byte(tokenizedMessage))
		}(i)
	}

	// Game server receives all messages
	for i := 0; i < numClients; i++ {
		messageData, sourceAddr, err := testutil.WaitForUDPMessage(t, gameServer, 200*time.Millisecond)
		assert.NoError(t, err, "Game server failed to receive message")

		message := string(messageData)
		mu.Lock()
		receivedMessages[message] = true
		mu.Unlock()

		// Send response back
		response := fmt.Sprintf("response-to-%s", message)
		testutil.SendUDPMessage(t, gameServer, sourceAddr, []byte(response))
	}

	// Verify all messages were received
	assert.Equal(t, numClients, len(receivedMessages), "All client messages should be received")

	wg.Wait()

	// Test connection cleanup by canceling context
	cancel()
}

// TestErrorHandlingAndRecovery tests various error scenarios
func TestErrorHandlingAndRecovery(t *testing.T) {
	reportFilePath := fmt.Sprintf("integ-test-error-handling-%d.txt", time.Now().Unix())
	defer os.Remove(reportFilePath)

	// Create game server
	gameServer := testutil.CreateTestUDPSocketWithPort(t, 9000)
	defer gameServer.Close()

	// Start app
	app := app.New()
	cfg := testutil.CreateTestConfig(
		testutil.WithPortRange("8700-8710"),
		testutil.WithUDPEndpointCount(1),
		testutil.WithReportFilePath(reportFilePath),
		testutil.WithHeadless(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go app.Run(ctx, cancel, cfg)
	time.Sleep(50 * time.Millisecond)

	// Create client
	client := testutil.CreateTestUDPSocket(t)
	defer client.Close()

	proxyAddr, err := net.ResolveUDPAddr("udp", testutil.TestProxyAddr+":8700")
	assert.NoError(t, err, "Failed to resolve proxy addr")

	// Replace token with known test token
	replaceTokenCmd := fmt.Sprintf("PlayerGateway:ReplaceToken:1:%s", testutil.TestTokenHash)
	testutil.SendUDPMessage(t, client, proxyAddr, []byte(replaceTokenCmd))
	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		name          string
		message       string
		shouldForward bool
		errorInLog    string
	}{
		{
			name:          "invalid token format - wrong hash",
			message:       "xxxxxxxxxxxxxxxxxxxxxxxxxxxx" + "data", // 28 wrong chars + data
			shouldForward: false,
			errorInLog:    "token hash does not match expected value",
		},
		{
			name:          "malformed packet - too short",
			message:       "x",
			shouldForward: false,
			errorInLog:    "packet is shorter than token length",
		},
		{
			name:          "valid token",
			message:       testutil.TestTokenHashDecoded + "valid-data",
			shouldForward: true,
			errorInLog:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Send message
			testutil.SendUDPMessage(t, client, proxyAddr, []byte(tt.message))

			// Check if game server receives it
			_, _, err := testutil.WaitForUDPMessage(t, gameServer, testutil.TestTimeout)

			if tt.shouldForward {
				assert.NoError(t, err, "Valid message should be forwarded")
			} else {
				assert.Error(t, err, "Invalid message should not be forwarded")

				// Verify error is logged
				if tt.errorInLog != "" {
					content, err := os.ReadFile(reportFilePath)
					assert.NoError(t, err, "Failed to read log file")

					logContent := string(content)
					assert.True(t, strings.Contains(logContent, tt.errorInLog),
						fmt.Sprintf("Log should contain error: %s", tt.errorInLog))
				}
			}
		})
	}
}

// TestConfigurationValidation tests various configuration scenarios
func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name        string
		configOpts  []testutil.ConfigOption
		shouldStart bool
	}{
		{
			name: "valid configuration",
			configOpts: []testutil.ConfigOption{
				testutil.WithPortRange("8800-8810"),
				testutil.WithUDPEndpointCount(3),
			},
			shouldStart: true,
		},
		{
			name: "valid configuration with auto port allocation",
			configOpts: []testutil.ConfigOption{
				testutil.WithPortRange(""), // Empty port range triggers auto-allocation
				testutil.WithUDPEndpointCount(2),
			},
			shouldStart: true,
		},
		{
			name: "invalid port range format",
			configOpts: []testutil.ConfigOption{
				testutil.WithPortRange("invalid"),
				testutil.WithUDPEndpointCount(1),
			},
			shouldStart: false,
		},
		{
			name: "out of bounds port number",
			configOpts: []testutil.ConfigOption{
				testutil.WithPortRange("99999-100000"),
				testutil.WithUDPEndpointCount(1),
			},
			shouldStart: false,
		},
		{
			name: "invalid IP address",
			configOpts: []testutil.ConfigOption{
				testutil.WithIPAddress("999.999.999.999"),
				testutil.WithPortRange("9100-9110"),
				testutil.WithUDPEndpointCount(1),
			},
			shouldStart: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reportFilePath := fmt.Sprintf("test-config-validation-%s-%d.txt", strings.ReplaceAll(tt.name, " ", "-"), time.Now().UnixNano())
			defer os.Remove(reportFilePath)

			opts := append(tt.configOpts, testutil.WithReportFilePath(reportFilePath), testutil.WithHeadless())
			cfg := testutil.CreateTestConfig(opts...)

			// Create game server if needed
			var gameServer *net.UDPConn
			if tt.shouldStart {
				gameServer = testutil.CreateTestUDPSocketWithPort(t, 9000)
				defer gameServer.Close()
			}

			app := app.New()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			err := app.Run(ctx, cancel, cfg)

			if tt.shouldStart {
				// For valid configs, error should be nil or context deadline exceeded
				if err != nil {
					assert.True(t, err == context.DeadlineExceeded || strings.Contains(err.Error(), "context deadline exceeded"),
						"Valid config should start successfully")
				}
			} else {
				// For invalid configs, should get an error before context deadline
				assert.Error(t, err, "Invalid config should return error")
				assert.True(t, err != context.DeadlineExceeded,
					"Invalid config should fail before timeout")
			}
		})
	}
}
