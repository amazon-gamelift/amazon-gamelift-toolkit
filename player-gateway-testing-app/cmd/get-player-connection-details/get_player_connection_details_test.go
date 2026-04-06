/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package getplayerconnectiondetails

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/proxy"
)

func TestRunGetPlayerConnectionDetails_ValidInput(t *testing.T) {
	tests := []struct {
		name          string
		playerNumbers string
	}{
		{"single player", "1"},
		{"multiple players", "1,2,3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup UDP listener to receive and respond
			addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
			assert.NoError(t, err)

			conn, err := net.ListenUDP("udp", addr)
			assert.NoError(t, err)
			defer conn.Close()

			connAddr := conn.LocalAddr().(*net.UDPAddr)

			// Set config values
			playerNumbersStr = tt.playerNumbers
			port = connAddr.Port

			errChan := make(chan error, 1)
			go func() {
				errChan <- runGetPlayerConnectionDetails(nil, nil)
			}()

			// Read the message and send response
			buffer := make([]byte, 1024)
			n, clientAddr, err := conn.ReadFromUDP(buffer)
			assert.NoError(t, err)

			// Verify message format
			expectedMessage := proxy.ConfigCommandPrefix + proxy.GetPlayerConnectionDetailsCommand + ":" + tt.playerNumbers
			assert.Equal(t, expectedMessage, string(buffer[:n]))

			// Send mock response
			mockResponse := proxy.PlayerConnectionDetailsResponse{
				PlayerConnectionDetails: []proxy.PlayerConnectionDetail{
					{PlayerNumber: "1", PlayerGatewayToken: "token123", Endpoints: []proxy.PlayerConnectionEndpoint{{IpAddress: "127.0.0.1", Port: 8000}}},
				},
			}
			responseBytes, _ := json.Marshal(mockResponse)
			conn.WriteToUDP(responseBytes, clientAddr)

			err = <-errChan
			assert.NoError(t, err)
		})
	}
}

func TestRunGetPlayerConnectionDetails_InvalidPort(t *testing.T) {
	tests := []struct {
		name          string
		portValue     int
		expectedError string
	}{
		{"zero port", 0, "port must be between 1 and 65535"},
		{"negative port", -1, "port must be between 1 and 65535"},
		{"port too high", 65536, "port must be between 1 and 65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			playerNumbersStr = "1"
			port = tt.portValue

			err := runGetPlayerConnectionDetails(nil, nil)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

func TestRunGetPlayerConnectionDetails_InvalidPlayerNumbers(t *testing.T) {
	tests := []struct {
		name          string
		playerNumbers string
		expectedError string
	}{
		{"non-numeric", "abc", "invalid player number: abc"},
		{"mixed valid and invalid", "1,abc,3", "invalid player number: abc"},
		{"empty in list", "1,,3", "invalid player number: "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			playerNumbersStr = tt.playerNumbers
			port = 8000

			err := runGetPlayerConnectionDetails(nil, nil)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

func TestRunGetPlayerConnectionDetails_EmptyPlayerNumbers(t *testing.T) {
	tests := []struct {
		name          string
		playerNumbers string
	}{
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			playerNumbersStr = tt.playerNumbers
			port = 8000

			err := runGetPlayerConnectionDetails(nil, nil)
			assert.Error(t, err)
			assert.Equal(t, "at least one player number is required", err.Error())
		})
	}
}

func TestGetPlayerConnectionDetailsCommand_FlagsInitialization(t *testing.T) {
	assert.NotNil(t, GetPlayerConnectionDetails)
	assert.Equal(t, "get-player-connection-details", GetPlayerConnectionDetails.Use)
	assert.NotNil(t, GetPlayerConnectionDetails.RunE)

	playerNumbersFlag := GetPlayerConnectionDetails.Flags().Lookup("player-numbers")
	assert.NotNil(t, playerNumbersFlag)
	assert.Equal(t, "", playerNumbersFlag.DefValue)

	portFlag := GetPlayerConnectionDetails.Flags().Lookup("port")
	assert.NotNil(t, portFlag)
	assert.Equal(t, "8000", portFlag.DefValue)
}
