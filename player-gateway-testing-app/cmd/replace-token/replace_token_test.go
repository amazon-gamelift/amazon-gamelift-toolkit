/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package replacetoken

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
)

func TestRunReplaceToken_ValidInput(t *testing.T) {
	tests := []struct {
		name         string
		playerNumber int
	}{
		{"player 1", 1},
		{"player 5", 5},
		{"player 10", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup UDP listener to receive the message
			addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
			assert.NoError(t, err)

			conn, err := net.ListenUDP("udp", addr)
			assert.NoError(t, err, "Failed to listen on udp address")
			defer conn.Close()

			connAddr, ok := conn.LocalAddr().(*net.UDPAddr)
			assert.True(t, ok, "Failed to get UDP address from connection")

			// Set config values
			replaceTokenCfg.PlayerNumber = tt.playerNumber
			replaceTokenCfg.Port = connAddr.Port

			errChan := make(chan error, 1)
			go func() {
				errChan <- runReplaceToken(nil, nil)
			}()

			// Read the message
			buffer := make([]byte, 1024)
			n, _, err := conn.ReadFromUDP(buffer)
			assert.NoError(t, err)

			// Verify the message format: PlayerGateway:ReplaceToken:<playerNum>:<token>
			actualMessage := string(buffer[:n])
			parts := strings.Split(actualMessage, ":")
			assert.Equal(t, 4, len(parts), "Message should have 4 parts")
			assert.Equal(t, "PlayerGateway", parts[0])
			assert.Equal(t, "ReplaceToken", parts[1])
			assert.Equal(t, fmt.Sprintf("%d", tt.playerNumber), parts[2])
			assert.True(t, len(parts[3]) > 0, "Token should not be empty")

			// Check command result
			err = <-errChan
			assert.NoError(t, err)
		})
	}
}

func TestRunReplaceToken_InvalidPlayerNumber(t *testing.T) {
	tests := []struct {
		name          string
		playerNumber  int
		expectedError string
	}{
		{"zero player number", 0, "player-number must be at least 1"},
		{"negative player number", -1, "player-number must be at least 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replaceTokenCfg.PlayerNumber = tt.playerNumber
			replaceTokenCfg.Port = 8000

			err := runReplaceToken(nil, nil)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

func TestRunReplaceToken_InvalidPort(t *testing.T) {
	tests := []struct {
		name          string
		port          int
		expectedError string
	}{
		{"zero port", 0, "port must be between 1 and 65535, got 0"},
		{"negative port", -1, "port must be between 1 and 65535, got -1"},
		{"port too high", 65536, "port must be between 1 and 65535, got 65536"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replaceTokenCfg.PlayerNumber = 1
			replaceTokenCfg.Port = tt.port

			err := runReplaceToken(nil, nil)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

func TestReplaceTokenCommand_FlagsInitialization(t *testing.T) {
	// Verify the command is properly initialized
	assert.NotNil(t, ReplaceToken)
	assert.Equal(t, "replace-token", ReplaceToken.Use)
	assert.NotNil(t, ReplaceToken.RunE)

	// Verify flags are registered
	playerNumberFlag := ReplaceToken.Flags().Lookup("player-number")
	assert.NotNil(t, playerNumberFlag)
	assert.Equal(t, "0", playerNumberFlag.DefValue)

	portFlag := ReplaceToken.Flags().Lookup("port")
	assert.NotNil(t, portFlag)
	assert.Equal(t, "8000", portFlag.DefValue)
}
