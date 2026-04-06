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
	// activeEndpointTimeout is how long an endpoint stays in the round-robin rotation
	// after receiving traffic from the client
	activeEndpointTimeout = 1 * time.Second
	// connectionDropTimeout is how long before blocking all Server->Client traffic
	// when no Client->Server traffic has been received
	connectionDropTimeout = 60 * time.Second
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
	lastUsedSource         map[int]*ClientSideProxyInfo   // Player number -> most recently used endpoint
	nextSourceIndex        map[int]int                    // Player number -> round-robin index
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
	playerNumber, modifiedData, err := s.tokenManager.ParseServerSideToken(data)
	if err != nil {
		return PreprocessServerBoundTrafficResult{0, nil, nil}, err
	}

	sources := s.clientSideProxySources[playerNumber]
	clientInfo, exists := s.findClientInfo(sources, sourceAddr)
	if !exists {
		clientInfo = &ClientSideProxyInfo{SourceAddr: sourceAddr}
		s.clientSideProxySources[playerNumber] = append(sources, clientInfo)
	}
	clientInfo.LastReceivedTimestamp = time.Now()
	s.lastUsedSource[playerNumber] = clientInfo

	return PreprocessServerBoundTrafficResult{playerNumber, modifiedData, nil}, nil
}

// HandleClientBoundTraffic distributes client-bound traffic using round-robin across endpoints
// active within the past activeEndpointTimeout. Falls back to most recently used endpoint if none active.
// Blocks traffic and cleans up player state if no activity for connectionDropTimeout.
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

	// Check connection drop timeout
	lastUsed := s.lastUsedSource[playerNumber]
	if lastUsed == nil || time.Since(lastUsed.LastReceivedTimestamp) >= connectionDropTimeout {
		// Clean up stale player state
		delete(s.clientSideProxySources, playerNumber)
		delete(s.lastUsedSource, playerNumber)
		delete(s.nextSourceIndex, playerNumber)
		return nil // Block traffic
	}

	// Get active endpoints (within activeEndpointTimeout)
	sources := s.clientSideProxySources[playerNumber]
	activeSources := s.filterActiveSources(sources)

	// Fall back to last used if no active endpoints
	if len(activeSources) == 0 {
		activeSources = []*ClientSideProxyInfo{lastUsed}
	}

	// Round-robin
	idx := s.nextSourceIndex[playerNumber]
	s.nextSourceIndex[playerNumber] = (idx + 1) % len(activeSources)

	_, err := socket.WriteToUDP(data.Data, activeSources[idx].SourceAddr)
	return err
}

// findClientInfo finds an existing source in the sources slice.
//
// Parameters:
//   - sources: slice of client-side proxy info to search through
//   - sourceAddr: UDP address to find
//
// Returns:
//   - *ClientSideProxyInfo: the client-side proxy info corresponding to the given sourceAddr if found
//   - bool: true if source exists, false if not found
func (s *ServerSideProxyTrafficHandler) findClientInfo(sources []*ClientSideProxyInfo, sourceAddr *net.UDPAddr) (*ClientSideProxyInfo, bool) {
	for _, info := range sources {
		if info.SourceAddr.String() == sourceAddr.String() {
			return info, true
		}
	}
	return nil, false
}

// filterActiveSources returns sources that have received traffic within activeEndpointTimeout.
//
// Parameters:
//   - sources: slice of client-side proxy info to filter
//
// Returns:
//   - []*ClientSideProxyInfo: slice containing only active sources
func (s *ServerSideProxyTrafficHandler) filterActiveSources(sources []*ClientSideProxyInfo) []*ClientSideProxyInfo {
	var active []*ClientSideProxyInfo
	for _, src := range sources {
		if time.Since(src.LastReceivedTimestamp) < activeEndpointTimeout {
			active = append(active, src)
		}
	}
	return active
}
