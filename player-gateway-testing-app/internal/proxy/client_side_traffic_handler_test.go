/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"
)

func TestClientSideProxyTrafficHandler_PreprocessServerBoundTraffic_NormalTraffic(t *testing.T) {
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
	handler := &ClientSideProxyTrafficHandler{
		tokenManager: tokenManager,
	}

	// Create valid token data
	data := []byte(testutil.TestTokenHashDecoded + testutil.TestMessage)
	addr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)

	result, err := handler.PreprocessServerBoundTraffic(data, addr)
	assert.NoError(t, err)
	assert.Equal(t, result.PlayerNumber, 1)

	// Verify hash was stripped but player number remains
	expectedData := "1|" + testutil.TestMessage
	assert.Equal(t, string(result.Data), expectedData)
}

func TestClientSideProxyTrafficHandler_PreprocessServerBoundTraffic_ConfigCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "valid SetDegradation command",
			command: "PlayerGateway:SetDegradation:50",
		},
		{
			name:    "valid ReplaceToken command",
			command: "PlayerGateway:ReplaceToken:1:bmV3dG9rZW4xMjM=",
		},
		{
			name:    "valid ListTokens command",
			command: "PlayerGateway:ListTokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager: tokenManager,
				rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
			}

			addr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
			result, err := handler.PreprocessServerBoundTraffic([]byte(tt.command), addr)

			assert.NoError(t, err)
			assert.Equal(t, result.PlayerNumber, 0, "Expected player number 0 for config command")
			assert.Equal(t, len(result.Data), 0, "Expected nil modified data for config command")
			assert.True(t, result.ConfigCommand != nil, "Expected config command result")
		})
	}
}

func TestClientSideProxyTrafficHandler_HandleClientBoundTraffic(t *testing.T) {
	handler := &ClientSideProxyTrafficHandler{}

	socket := testutil.CreateTestUDPSocket(t)
	defer socket.Close()

	clientSocket := testutil.CreateTestUDPSocket(t)
	defer clientSocket.Close()

	ccPool := &ClientConnectionPool{
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
	}

	ccPool.clientConnectionPortMap[testutil.TestReturnTrafficPort] = &ClientConnectionInfo{
		ClientAddr: clientSocket.LocalAddr().(*net.UDPAddr),
	}

	data := ClientBoundData{
		Data:                 []byte(testutil.TestMessage),
		ClientConnectionPort: testutil.TestReturnTrafficPort,
	}

	err := handler.HandleClientBoundTraffic(data, ccPool, socket)
	assert.NoError(t, err)

	receivedString, _ := testutil.ReadUDPMessage(t, clientSocket)
	assert.Equal(t, receivedString, string(testutil.TestMessage))
}

func TestClientSideProxyTrafficHandler_ConfigCommand_MalformedCommands(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "missing components - only prefix",
			command: "PlayerGateway:",
		},
		{
			name:    "missing components - only command name",
			command: "PlayerGateway:SetDegradation",
		},
		{
			name:    "unknown command name",
			command: "PlayerGateway:InvalidCommand:param",
		},
		{
			name:    "empty command",
			command: "PlayerGateway::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager: tokenManager,
				rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
			}

			_, err := handler.parseConfigCommand([]byte(tt.command))
			assert.Error(t, err)
		})
	}
}

func TestClientSideProxyTrafficHandler_ParseConfigCommand_SetDegradation(t *testing.T) {
	tests := []struct {
		name                       string
		command                    string
		expectedDegradationPercent int
	}{
		{
			name:                       "set degradation to 50%",
			command:                    "PlayerGateway:SetDegradation:50",
			expectedDegradationPercent: 50,
		},
		{
			name:                       "set degradation to 0%",
			command:                    "PlayerGateway:SetDegradation:0",
			expectedDegradationPercent: 0,
		},
		{
			name:                       "set degradation to 100%",
			command:                    "PlayerGateway:SetDegradation:100",
			expectedDegradationPercent: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager: tokenManager,
			}

			cmdName, err := handler.parseConfigCommand([]byte(tt.command))
			assert.NoError(t, err)
			assert.Equal(t, cmdName, "SetDegradation")
			assert.Equal(t, handler.degradationPercentage, tt.expectedDegradationPercent)
		})
	}
}

func TestClientSideProxyTrafficHandler_ParseConfigCommand_SetDegradationClamping(t *testing.T) {
	tests := []struct {
		name                       string
		command                    string
		expectedDegradationPercent int
	}{
		{
			name:                       "clamp negative value to 0",
			command:                    "PlayerGateway:SetDegradation:-10",
			expectedDegradationPercent: 0,
		},
		{
			name:                       "clamp value over 100",
			command:                    "PlayerGateway:SetDegradation:150",
			expectedDegradationPercent: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager: tokenManager,
				rng:          rand.New(rand.NewSource(time.Now().UnixMicro())),
			}

			handler.parseConfigCommand([]byte(tt.command))

			assert.Equal(t, handler.degradationPercentage, tt.expectedDegradationPercent)
		})
	}
}

func TestClientSideProxyTrafficHandler_ParseConfigCommand_SetDegradationInvalidParameter(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "non-numeric parameter",
			command: "PlayerGateway:SetDegradation:abc",
		},
		{
			name:    "empty parameter",
			command: "PlayerGateway:SetDegradation:",
		},
		{
			name:    "decimal value",
			command: "PlayerGateway:SetDegradation:50.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager: tokenManager,
				rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
			}

			_, err := handler.parseConfigCommand([]byte(tt.command))
			assert.Error(t, err, "Expected error but got none")
		})
	}
}

func TestClientSideProxyTrafficHandler_ParseConfigCommand_ReplaceToken(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		playerCount int
		expectError bool
	}{
		{
			name:        "replace token for player 1",
			command:     "PlayerGateway:ReplaceToken:1:YWJjZA==",
			playerCount: 1,
			expectError: false,
		},
		{
			name:        "replace token for player 2 with 2 players",
			command:     "PlayerGateway:ReplaceToken:2:ZGVhZGJlZWY=",
			playerCount: 2,
			expectError: false,
		},
		{
			name:        "replace token for invalid player number",
			command:     "PlayerGateway:ReplaceToken:5:YWJjZA==",
			playerCount: 2,
			expectError: true,
		},
		{
			name:        "replace token with empty token",
			command:     "PlayerGateway:ReplaceToken:1:",
			playerCount: 1,
			expectError: true,
		},
		{
			name:        "replace token with invalid format",
			command:     "PlayerGateway:ReplaceToken:notanumber:YWJjZA==",
			playerCount: 1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := token.NewTokenManager(tt.playerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager: tokenManager,
				rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
			}

			cmdName, err := handler.parseConfigCommand([]byte(tt.command))

			if tt.expectError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, cmdName, ReplaceTokenCommand)
			}
		})
	}
}

func TestClientSideProxyTrafficHandler_DegradationDropping(t *testing.T) {
	tests := []struct {
		name                  string
		degradationPercentage int
		expectDrop            bool
		expectedPlayerNumber  int
		expectedData          string
	}{
		{
			name:                  "100% degradation - drop all packets",
			degradationPercentage: 100,
			expectDrop:            true,
		},
		{
			name:                  "0% degradation - pass all packets",
			degradationPercentage: 0,
			expectDrop:            false,
			expectedPlayerNumber:  1,
			expectedData:          "1|" + testutil.TestMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager:          tokenManager,
				rng:                   rand.New(rand.NewSource(time.Now().UnixMicro())),
				degradationPercentage: tt.degradationPercentage,
			}

			addr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
			data := []byte(testutil.TestTokenHashDecoded + testutil.TestMessage)

			result, err := handler.PreprocessServerBoundTraffic(data, addr)

			if tt.expectDrop {
				assert.Error(t, err, "Expected packet to be dropped")
				assert.ErrorIs(t, err, ErrPacketDropped, "Expected ErrPacketDropped error")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, result.PlayerNumber, tt.expectedPlayerNumber)
				assert.Equal(t, string(result.Data), tt.expectedData)
			}

			socket := testutil.CreateTestUDPSocket(t)
			defer socket.Close()

			clientSocket := testutil.CreateTestUDPSocket(t)
			defer clientSocket.Close()

			ccPool := &ClientConnectionPool{
				clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
			}

			ccPool.clientConnectionPortMap[testutil.TestReturnTrafficPort] = &ClientConnectionInfo{
				ClientAddr: clientSocket.LocalAddr().(*net.UDPAddr),
			}

			clientBoundData := ClientBoundData{
				Data:                 []byte(testutil.TestMessage),
				ClientConnectionPort: testutil.TestReturnTrafficPort,
			}

			err = handler.HandleClientBoundTraffic(clientBoundData, ccPool, socket)

			if tt.expectDrop {
				assert.Error(t, err, "Expected packet to be dropped")
				assert.ErrorIs(t, err, ErrPacketDropped, "Expected ErrPacketDropped error")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClientSideProxyTrafficHandler_DegradationProbability(t *testing.T) {
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)
	handler := &ClientSideProxyTrafficHandler{
		tokenManager:          tokenManager,
		degradationPercentage: 50, // Drop 50% of packets
		rng:                   rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	addr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
	data := []byte(testutil.TestTokenHashDecoded + testutil.TestMessage)

	// Send many packets and verify approximately 50% are dropped
	totalPackets := 1000
	droppedPackets := 0

	for i := 0; i < totalPackets; i++ {
		_, err := handler.PreprocessServerBoundTraffic(data, addr)
		if err != nil {
			assert.ErrorIs(t, err, ErrPacketDropped, "Expected ErrPacketDropped error")
			droppedPackets++
		}
	}

	// Allow for some variance (40-60% range)
	dropRate := float64(droppedPackets) / float64(totalPackets) * 100
	assert.True(t, dropRate >= 40 && dropRate <= 60, "Expected drop rate around 50%, got drop rate outside acceptable range")
}

func TestClientSideProxyTrafficHandler_IndependentDegradation(t *testing.T) {
	tokenManager := testutil.CreateTestTokenManager(testutil.TestPlayerCount)

	// Create two independent handlers
	handler1 := &ClientSideProxyTrafficHandler{
		tokenManager: tokenManager,
		rng:          rand.New(rand.NewSource(time.Now().UnixMicro())),
	}
	handler2 := &ClientSideProxyTrafficHandler{
		tokenManager: tokenManager,
		rng:          rand.New(rand.NewSource(time.Now().UnixMicro())),
	}

	addr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)

	// Set different degradation percentages
	degradationCmd1 := []byte("PlayerGateway:SetDegradation:50")
	handler1.PreprocessServerBoundTraffic(degradationCmd1, addr)

	degradationCmd2 := []byte("PlayerGateway:SetDegradation:75")
	handler2.PreprocessServerBoundTraffic(degradationCmd2, addr)

	// Verify they maintain independent state
	assert.Equal(t, handler1.degradationPercentage, 50)
	assert.Equal(t, handler2.degradationPercentage, 75)
}

func TestClientSideProxyTrafficHandler_ParseConfigCommand_ListTokens(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		playerCount int
	}{
		{
			name:        "list single token",
			command:     "PlayerGateway:ListTokens",
			playerCount: 1,
		},
		{
			name:        "list multiple tokens",
			command:     "PlayerGateway:ListTokens",
			playerCount: 3,
		},
		{
			name:        "list tokens with trailing colon",
			command:     "PlayerGateway:ListTokens:",
			playerCount: 2,
		},
		{
			name:        "list tokens with whitespace",
			command:     "PlayerGateway:ListTokens  ",
			playerCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenManager := token.NewTokenManager(tt.playerCount)
			handler := &ClientSideProxyTrafficHandler{
				tokenManager: tokenManager,
				rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
			}

			cmdName, err := handler.parseConfigCommand([]byte(tt.command))
			assert.NoError(t, err)
			assert.Equal(t, cmdName, ListTokensCommand)

			// Verify token count matches player count
			assert.Equal(t, tokenManager.GetPlayerCount(), tt.playerCount)
		})
	}
}
