/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/stats"
)

// ClientConnection represents a client connection through the proxy, maintaining
// a dedicated return port and send channel for proper traffic routing.
type ClientConnection struct {
	returnTrafficPort      int         // Port for listening for player-bound traffic and sending game-bound traffic on
	serverBoundTrafficChan chan []byte // Channel for listening to game server bound traffic on sent from owning proxy
}

// ClientBoundData contains response data from the destination server with port information.
type ClientBoundData struct {
	Data                 []byte // Data received from game server
	ClientConnectionPort int    // Client connection port that received data
}

// ClientConnectionInfo holds data related to a client connection.
type ClientConnectionInfo struct {
	Socket       *net.UDPConn // UDP socket for this connection
	ClientAddr   *net.UDPAddr // Address to return traffic to
	PlayerNumber int          // Associated player number
}

// ClientConnectionPool manages multiple client connections and their associated resources.
type ClientConnectionPool struct {
	clientConnectionMap     map[int]*ClientConnection     // Player number -> client connection
	clientConnectionPortMap map[int]*ClientConnectionInfo // Return port -> client connection info
	destinationAddr         *net.UDPAddr                  // Address to forward incoming game-bound traffic to
	forwardToPlayerChan     chan ClientBoundData          // Channel for sending client-bound traffic on to owning proxy
	statsEventChan          chan<- stats.StatsEvent       // Channel for publishing stats events
	proxyPort               int                           // Port of the owning proxy for stats events
}

// GetClientConnection retrieves an existing client connection for the given player number.
//
// Parameters:
//   - playerNumber: unique identifier for the player
//
// Returns:
//   - *ClientConnection: the connection if found
//   - bool: true if connection exists, false otherwise
func (ccPool *ClientConnectionPool) GetClientConnection(playerNumber int) (*ClientConnection, bool) {
	clientConnection, exists := ccPool.clientConnectionMap[playerNumber]
	return clientConnection, exists
}

// GetOrCreateClientConnection retrieves an existing connection or creates a new one if it doesn't exist.
//
// Parameters:
//   - ctx: context for graceful shutdown
//   - playerNumber: unique identifier for the player
//   - sourceAddr: client's source address for return traffic
//
// Returns:
//   - *ClientConnection: existing or newly created connection
//   - error: nil on success, error if client connection creation fails
func (ccPool *ClientConnectionPool) GetOrCreateClientConnection(ctx context.Context, playerNumber int, sourceAddr *net.UDPAddr) (*ClientConnection, error) {
	if clientConn, exists := ccPool.GetClientConnection(playerNumber); exists {
		return clientConn, nil
	}
	return ccPool.CreateClientConnection(ctx, playerNumber, sourceAddr)
}

// CreateClientConnection creates a new connection for a player with a dedicated return traffic socket.
// It starts listening on the new socket.
//
// Parameters:
//   - ctx: context for graceful shutdown
//   - playerNumber: unique identifier for the player
//   - sourceAddr: client's source address for return traffic
//
// Returns:
//   - *ClientConnection: newly created client connection
//   - error: nil on success, error if socket allocation fails
func (ccPool *ClientConnectionPool) CreateClientConnection(ctx context.Context, playerNumber int, sourceAddr *net.UDPAddr) (*ClientConnection, error) {
	socket, err := ccPool.allocateSocket()
	if err != nil {
		return nil, err
	}

	returnTrafficAddr, ok := socket.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("unable to parse UDP address when creating client connection")
	}

	clientConn := &ClientConnection{
		returnTrafficPort:      returnTrafficAddr.Port,
		serverBoundTrafficChan: make(chan []byte, BufferSize),
	}

	ccPool.clientConnectionMap[playerNumber] = clientConn
	ccPool.clientConnectionPortMap[clientConn.returnTrafficPort] = &ClientConnectionInfo{
		Socket:       socket,
		ClientAddr:   sourceAddr,
		PlayerNumber: playerNumber,
	}

	// Publish connection created event
	ccPool.publishStatsEvent(stats.StatsEvent{
		Type:         stats.EventPlayerConnected,
		PlayerNumber: playerNumber,
	})

	go ccPool.handleSocketTraffic(ctx, socket, clientConn)

	return clientConn, nil
}

// GetSourceAddrFromClientConnectionPort retrieves the original client address for a given return port.
//
// Parameters:
//   - port: return traffic port number
//
// Returns:
//   - *net.UDPAddr: client's source address if found
//   - bool: true if mapping exists, false otherwise
func (ccPool *ClientConnectionPool) GetSourceAddrFromClientConnectionPort(port int) (*net.UDPAddr, bool) {
	if info, exists := ccPool.clientConnectionPortMap[port]; exists {
		return info.ClientAddr, true
	}
	return nil, false
}

// GetPlayerNumberFromClientConnectionPort retrieves the player number associated with a given return port.
//
// Parameters:
//   - port: return traffic port number
//
// Returns:
//   - int: the associated player number if found
//   - bool: true if mapping exists, false otherwise
func (ccPool *ClientConnectionPool) GetPlayerNumberFromClientConnectionPort(port int) (int, bool) {
	if info, exists := ccPool.clientConnectionPortMap[port]; exists {
		return info.PlayerNumber, true
	}
	return 0, false
}

// CloseClientConnection closes and cleans up a client connection and its associated resources.
//
// Parameters:
//   - playerNumber: unique identifier for the client connection to close
func (ccPool *ClientConnectionPool) CloseClientConnection(playerNumber int) {
	if clientConn, exists := ccPool.clientConnectionMap[playerNumber]; exists {
		if info, ok := ccPool.clientConnectionPortMap[clientConn.returnTrafficPort]; ok {
			info.Socket.Close()
			delete(ccPool.clientConnectionPortMap, clientConn.returnTrafficPort)
		}

		close(clientConn.serverBoundTrafficChan)
		delete(ccPool.clientConnectionMap, playerNumber)

		// Publish connection closed event
		ccPool.publishStatsEvent(stats.StatsEvent{
			Type:         stats.EventPlayerDisconnected,
			PlayerNumber: playerNumber,
		})
	}
}

// allocateSocket creates a new UDP socket bound to any available port.
//
// Returns:
//   - *net.UDPConn: newly allocated UDP connection
//   - error: nil on success, error if socket creation fails
func (ccPool *ClientConnectionPool) allocateSocket() (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr(UDP, ":0")
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP(UDP, addr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// handleSocketTraffic handles bidirectional traffic for a connection's dedicated socket.
//
// Parameters:
//   - ctx: context for graceful shutdown
//   - socket: UDP socket for this connection's return traffic
//   - clientConn: client connection object containing send channel and port information
//
// Returns:
//   - error: nil on graceful shutdown, error if UDP operations fail
func (ccPool *ClientConnectionPool) handleSocketTraffic(ctx context.Context, socket *net.UDPConn, clientConn *ClientConnection) error {
	buf := make([]byte, BufferSize)

	for {
		select {
		case <-ctx.Done():
			return nil
		case data := <-clientConn.serverBoundTrafficChan:
			if _, err := socket.WriteToUDP(data, ccPool.destinationAddr); err != nil {
				return err
			}
		default:
			n, _, err := readUDPWithTimeout(socket, buf, TenMilliseconds)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}
				return err
			}

			dataCopy := make([]byte, n)
			copy(dataCopy, buf[:n])

			ccPool.forwardToPlayerChan <- ClientBoundData{
				Data:                 dataCopy,
				ClientConnectionPort: clientConn.ReturnTrafficPort(),
			}
		}
	}
}

// ReturnTrafficPort returns the port number used for return traffic for this client connection.
//
// Returns:
//   - int: port number for return traffic
func (cc *ClientConnection) ReturnTrafficPort() int {
	return cc.returnTrafficPort
}

// publishStatsEvent publishes a stats event using non-blocking select-default pattern
func (ccPool *ClientConnectionPool) publishStatsEvent(event stats.StatsEvent) {
	if ccPool.statsEventChan != nil {
		select {
		case ccPool.statsEventChan <- event:
			// Event published successfully
		default:
			// Channel full, drop event (stats are best-effort)
		}
	}
}
