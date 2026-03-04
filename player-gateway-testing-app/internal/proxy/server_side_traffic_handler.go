/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"net"
	"time"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"
)

const (
	// maxRecentMessages is the size of the buffer that tracks recent messages sent from client-side proxy for a given player.
	// If a proxy hasn't sent a message in the last maxRecentMessages, it's considered stale and removed from the active sources list.
	maxRecentMessages = 20
	// sourceTimeoutDuration is the maximum time since last received traffic before a client-side
	// proxy source is considered stale and removed from the active sources list
	sourceTimeoutDuration = 30 * time.Second
)

// ClientSideProxyInfo holds information about a client-side proxy source that sent traffic.
type ClientSideProxyInfo struct {
	SourceAddr            *net.UDPAddr // The address for the client-side proxy
	LastReceivedTimestamp time.Time    // Timestamp of the last received message
}

// ServerSideProxyTrafficHandler handles preprocessing and routing for server-side proxies.
type ServerSideProxyTrafficHandler struct {
	tokenManager           *token.TokenManager            // Token manager for parsing tokens
	clientSideProxySources map[int][]*ClientSideProxyInfo // Player number -> client-side proxy sources info
	nextSourceIndexByPort  map[int]int                    // Return traffic port -> index
	recentMessages         map[int][]*net.UDPAddr         // Last maxRecentMessages addresses per player
}

// PreprocessServerBoundTraffic tracks client-side proxy sources and extracts player information.
// Maintains a list of client-side proxy addresses for each player to enable round-robin distribution.
// Parses the token format "playerNumber|gameData" and strips the entire token.
//
// Parameters:
//   - data: raw UDP packet data from client-side proxy in format "playerNumber|gameData"
//   - sourceAddr: client-side proxy's source UDP address
//
// Returns:
//   - PreprocessServerBoundTrafficResult: contains player number, modified data, and command result
//   - error: nil on success, error if token parsing fails
func (s *ServerSideProxyTrafficHandler) PreprocessServerBoundTraffic(data []byte, sourceAddr *net.UDPAddr) (PreprocessServerBoundTrafficResult, error) {
	// Extract player number and strip entire token from data
	playerNumber, modifiedData, err := s.tokenManager.ParseServerSideToken(data)
	if err != nil {
		return PreprocessServerBoundTrafficResult{0, nil, nil}, err
	}

	sources := s.clientSideProxySources[playerNumber]
	clientInfo, exists := s.findClientInfo(sources, sourceAddr)
	if !exists {
		clientInfo = &ClientSideProxyInfo{
			SourceAddr: sourceAddr,
		}
		s.clientSideProxySources[playerNumber] = append(sources, clientInfo)
	}
	clientInfo.LastReceivedTimestamp = time.Now()

	s.addToRecentMessages(playerNumber, sourceAddr)

	return PreprocessServerBoundTrafficResult{playerNumber, modifiedData, nil}, nil
}

// HandleClientBoundTraffic distributes client-bound traffic across client-side proxies using round-robin.
// Removes stale client-side proxies that haven't received traffic in the last sourceTimeoutDuration or sent a message in the last maxRecentMessages.
//
// Parameters:
//   - data: the data to send to the client and the client connection port it was received on
//   - ccPool: client connection pool for address lookup
//   - socket: the UDP socket used to send data
//
// Returns:
//   - error: nil on success, error if data failed to send
func (s *ServerSideProxyTrafficHandler) HandleClientBoundTraffic(data ClientBoundData, ccPool *ClientConnectionPool, socket *net.UDPConn) error {
	playerNumber, exists := ccPool.GetPlayerNumberFromClientConnectionPort(data.ClientConnectionPort)
	if !exists {
		return nil
	}

	sources := s.clientSideProxySources[playerNumber]
	if len(sources) == 0 {
		return nil
	}

	nextIndex := s.nextSourceIndexByPort[data.ClientConnectionPort]
	sources = s.removeStaleSources(sources, nextIndex, playerNumber)
	if nextIndex >= len(sources) {
		nextIndex = 0
	}

	s.clientSideProxySources[playerNumber] = sources
	s.nextSourceIndexByPort[data.ClientConnectionPort] = (nextIndex + 1) % len(sources)

	if _, err := socket.WriteToUDP(data.Data, sources[nextIndex].SourceAddr); err != nil {
		return err
	}
	return nil
}

// findClientInfo finds an existing source and updates its timestamp.
//
// Parameters:
//   - sources: slice of client-side proxy info to search through
//   - sourceAddr: UDP address to find and update
//
// Returns:
//   - *ClientSideProxyInfo: the client-side proxy info corresponding to the given sourceAddr if found
//   - bool: true if source exists (and was updated), false if not found
func (s *ServerSideProxyTrafficHandler) findClientInfo(sources []*ClientSideProxyInfo, sourceAddr *net.UDPAddr) (*ClientSideProxyInfo, bool) {
	for _, info := range sources {
		if info.SourceAddr.String() == sourceAddr.String() {
			return info, true
		}
	}
	return nil, false
}

// addToRecentMessages adds a source address to the recent messages ring buffer.
// Maintains a maximum of maxRecentMessages entries per player.
//
// Parameters:
//   - playerNumber: the player number
//   - sourceAddr: the source address to add
func (s *ServerSideProxyTrafficHandler) addToRecentMessages(playerNumber int, sourceAddr *net.UDPAddr) {
	s.recentMessages[playerNumber] = append(s.recentMessages[playerNumber], sourceAddr)
	if len(s.recentMessages[playerNumber]) > maxRecentMessages {
		startIndex := len(s.recentMessages[playerNumber]) - maxRecentMessages
		s.recentMessages[playerNumber] = s.recentMessages[playerNumber][startIndex:]
	}
}

// removeStaleSources removes stale sources starting from the given index and returns the next valid source.
// Keeps removing sources until it finds an active one or only one source remains.
//
// Parameters:
//   - sources: slice of client-side proxy sources
//   - index: index to start checking from
//   - playerNumber: the player number
//
// Returns:
//   - []*ClientSideProxyInfo: updated sources slice with stale sources removed
func (s *ServerSideProxyTrafficHandler) removeStaleSources(sources []*ClientSideProxyInfo, index int, playerNumber int) []*ClientSideProxyInfo {
	clientInfo := sources[index]

	for len(sources) > 1 {
		if s.isSourceActive(playerNumber, clientInfo) {
			break
		}

		sources = append(sources[:index], sources[index+1:]...)
		if index >= len(sources) {
			index = 0
		}
		clientInfo = sources[index]
	}

	return sources
}

// isSourceActive checks if a source is still active based on recent messages and timestamp.
// A source is considered active if it has sent a message in the last maxRecentMessages
// AND has received traffic in the past sourceTimeoutDuration.
//
// Parameters:
//   - playerNumber: the player number
//   - clientInfo: the client-side proxy info to check
//
// Returns:
//   - bool: true if source is active, false if stale
func (s *ServerSideProxyTrafficHandler) isSourceActive(playerNumber int, clientInfo *ClientSideProxyInfo) bool {
	return s.isInRecentMessages(playerNumber, clientInfo.SourceAddr) &&
		time.Since(clientInfo.LastReceivedTimestamp) < sourceTimeoutDuration
}

// isInRecentMessages checks if an address is in the last maxRecentMessages for a player.
//
// Parameters:
//   - playerNumber: the player number to check
//   - addr: the UDP address to search for
//
// Returns:
//   - bool: true if address is found in the last maxRecentMessages, false otherwise
func (s *ServerSideProxyTrafficHandler) isInRecentMessages(playerNumber int, addr *net.UDPAddr) bool {
	for _, recentAddr := range s.recentMessages[playerNumber] {
		if recentAddr.String() == addr.String() {
			return true
		}
	}
	return false
}
