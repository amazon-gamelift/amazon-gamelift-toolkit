/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"
)

var (
	// ErrPacketDropped is returned when a packet is intentionally dropped due to degradation
	ErrPacketDropped = errors.New("packet dropped due to degradation")
)

const (
	MinimumDegradationPercentage = 0
	MaximumDegradationPercentage = 100

	ConfigCommandDelimiter = ":"
	SetDegradationCommand  = "SetDegradation"
	ReplaceTokenCommand    = "ReplaceToken"
	ListTokensCommand      = "ListTokens"
	ConfigCommandPrefix    = "PlayerGateway:"
)

var (
	ConfigCommandPrefixByteArray = []byte(ConfigCommandPrefix)
	ParameterRequiringCommands   = []string{SetDegradationCommand, ReplaceTokenCommand}
)

// ClientSideProxyTrafficHandler handles preprocessing and routing for client-side proxies.
type ClientSideProxyTrafficHandler struct {
	tokenManager          *token.TokenManager
	degradationPercentage int
	rng                   *rand.Rand
}

// PreprocessServerBoundTraffic processes server-bound traffic from clients.
//
// Handles two types of packets:
//  1. Configuration commands (starting with "PlayerGateway:") - parsed and executed immediately,
//     no further processing occurs.
//  2. Game traffic - validates token format (hash:playerNumber|gameData), applies degradation,
//     and strips the hash portion before forwarding.
//
// Parameters:
//   - data: raw UDP packet data, either config command or game traffic
//   - sourceAddr: client's source UDP address
//
// Returns:
//   - PreprocessServerBoundTrafficResult: contains player number, modified data, and command result
//   - error: nil on success, error if config command is malformed, token validation fails, or packet is dropped
func (c *ClientSideProxyTrafficHandler) PreprocessServerBoundTraffic(data []byte, sourceAddr *net.UDPAddr) (PreprocessServerBoundTrafficResult, error) {
	if bytes.HasPrefix(data, ConfigCommandPrefixByteArray) {
		cmdName, err := c.parseConfigCommand(data)
		if err != nil {
			return PreprocessServerBoundTrafficResult{0, nil, nil}, err
		}

		switch cmdName {
		case SetDegradationCommand:
			return PreprocessServerBoundTrafficResult{0, nil, DegradationResult{c.degradationPercentage}}, nil
		default:
			return PreprocessServerBoundTrafficResult{0, nil, GenericCommandResult{}}, nil
		}
	}

	// Apply degradation if active
	if c.shouldDropPacket() {
		return PreprocessServerBoundTrafficResult{0, nil, nil}, ErrPacketDropped
	}

	playerNumber, modifiedData, err := c.tokenManager.ParseClientSideToken(data)
	if err != nil {
		return PreprocessServerBoundTrafficResult{0, nil, nil}, fmt.Errorf("token validation failed: %w", err)
	}
	return PreprocessServerBoundTrafficResult{playerNumber, modifiedData, nil}, nil
}

// HandleClientBoundTraffic routes client-bound traffic to the original client source address.
//
// Parameters:
//   - data: the data to send to the client and the client connection port it was received on
//   - ccPool: client connection pool for address lookup
//   - socket: the UDP socket used to send data
//
// Returns:
//   - error: nil on success, error if data failed to send
func (c *ClientSideProxyTrafficHandler) HandleClientBoundTraffic(data ClientBoundData, ccPool *ClientConnectionPool, socket *net.UDPConn) error {
	if sourceAddr, ok := ccPool.GetSourceAddrFromClientConnectionPort(data.ClientConnectionPort); ok {
		if c.shouldDropPacket() {
			return ErrPacketDropped
		}

		if _, err := socket.WriteToUDP(data.Data, sourceAddr); err != nil {
			return err
		}
	}
	return nil
}

// parseConfigCommand parses and executes configuration commands sent via UDP packets.
// Expected format: "PlayerGateway:CommandName:Parameter"
//
// Supported commands:
//   - SetDegradation: sets packet drop percentage (0-100)
//   - ReplaceToken: replaces the token for a specific player
//   - ListTokens: prints all player tokens to stdout
//
// Parameters:
//   - data: raw UDP packet data starting with "PlayerGateway:"
//
// Returns:
//   - string: command name
//   - error: nil on success, error if command is malformed or execution fails
func (c *ClientSideProxyTrafficHandler) parseConfigCommand(data []byte) (string, error) {
	rest := data[len(ConfigCommandPrefixByteArray):]
	if len(rest) == 0 {
		return "", fmt.Errorf("missing command name, got %s", string(data))
	}

	cmdName, cmdParameter, _ := strings.Cut(string(rest), ConfigCommandDelimiter)
	cmdName = strings.TrimSpace(cmdName)
	cmdParameter = strings.TrimSpace(cmdParameter)

	if len(cmdParameter) == 0 && slices.Contains(ParameterRequiringCommands, cmdName) {
		return "", fmt.Errorf("passed command %s requires a parameter but found nothing", cmdName)
	}

	switch cmdName {
	case SetDegradationCommand:
		return SetDegradationCommand, c.setDegradation(cmdParameter)
	case ReplaceTokenCommand:
		return ReplaceTokenCommand, c.replaceToken(cmdParameter)
	case ListTokensCommand:
		tokens := c.tokenManager.GetValidTokens()
		for playerNum := 1; playerNum <= c.tokenManager.GetPlayerCount(); playerNum++ {
			log.Printf("Player %d: %s", playerNum, tokens[playerNum])
		}
		return ListTokensCommand, nil
	default:
		return cmdName, fmt.Errorf("unrecognized command, got %s", cmdName)
	}
}

// replaceToken parses and executes a ReplaceToken command.
// Expected format: "playerNumber:base64Token"
//
// Parameters:
//   - cmdParameter: string in format "playerNumber:base64Token"
//
// Returns:
//   - error: nil on success, error if format is invalid or token replacement fails
func (c *ClientSideProxyTrafficHandler) replaceToken(cmdParameter string) error {
	playerNumberStr, tokenStr, found := strings.Cut(cmdParameter, ConfigCommandDelimiter)
	if !found {
		return fmt.Errorf("invalid ReplaceToken format, expected playerNumber:token")
	}

	playerNumber, err := strconv.Atoi(playerNumberStr)
	if err != nil {
		return fmt.Errorf("invalid player number: %s", playerNumberStr)
	}

	return c.tokenManager.ReplaceToken(playerNumber, tokenStr)
}

// setDegradation sets the requested degradation percentage if its a valid integer.
//
// Parameters:
//   - requestedPercentage: string representation of the percentage
//
// Returns:
//   - error: error encountered while parsing percentage if any
func (c *ClientSideProxyTrafficHandler) setDegradation(requestedPercentage string) error {
	// Extract the percentage value after the prefix
	percentage, err := strconv.Atoi(requestedPercentage)
	if err != nil {
		return err
	}

	// Clamp to valid range
	percentage = max(MinimumDegradationPercentage, min(percentage, MaximumDegradationPercentage))
	c.degradationPercentage = percentage

	return nil
}

// shouldDropPacket determines if a packet should be dropped based on current degradation percentage.
//
// Returns:
//   - bool: true if packet should be dropped, false otherwise
func (c *ClientSideProxyTrafficHandler) shouldDropPacket() bool {
	if c.degradationPercentage == MinimumDegradationPercentage {
		return false
	}
	if c.degradationPercentage == MaximumDegradationPercentage {
		return true
	}
	return c.rng.Intn(MaximumDegradationPercentage) < c.degradationPercentage
}
