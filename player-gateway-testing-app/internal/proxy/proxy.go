/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/stats"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"
)

const (
	TenMilliseconds = 10 * time.Millisecond
	BufferSize      = 8192
	UDP             = "udp"
)

// Proxy handles UDP traffic forwarding between clients and a destination server.
type Proxy struct {
	port                 int                     // The port the proxy is listening on
	socket               *net.UDPConn            // Main proxy socket for listening to game-bound traffic
	destinationAddr      *net.UDPAddr            // Address to forward game-bound traffic to
	clientConnectionPool *ClientConnectionPool   // Pool for each client connection that has connected via this proxy
	forwardToPlayerChan  chan ClientBoundData    // Channel for forwarding player-bound traffic
	trafficHandler       TrafficHandler          // Handler for traffic on client/server proxy
	packetLogger         *PacketLogger           // Logger for failed packets
	statsEventChan       chan<- stats.StatsEvent // Channel for publishing stats events
}

// NewClientSideProxy creates a new proxy with a client-side traffic handler.
//
// Parameters:
//   - proxyIP: IP address for the proxy to bind to
//   - port: port number for the proxy to listen on
//   - destinationAddr: target address to forward incoming traffic to
//   - tokenManager: token manager for validating and parsing tokens
//   - reportFilePath: path to file for logging failed packets
//   - statsEventChan: optional channel for publishing stats events (can be nil)
//
// Returns:
//   - *Proxy: configured client-side proxy ready to start
//   - error: nil on success, error if socket creation fails
func NewClientSideProxy(proxyIP string, port int, destinationAddr *net.UDPAddr, tokenManager *token.TokenManager, reportFilePath string, statsEventChan chan<- stats.StatsEvent) (*Proxy, error) {
	if tokenManager == nil {
		return nil, fmt.Errorf("token manager is nil")
	}
	handler := &ClientSideProxyTrafficHandler{
		tokenManager: tokenManager,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	proxy, err := newProxy(proxyIP, port, destinationAddr, handler, reportFilePath, statsEventChan)
	if err != nil {
		return nil, err
	}
	return proxy, nil
}

// NewServerSideProxy creates a new proxy with a server-side traffic handler.
//
// Parameters:
//   - proxyIP: IP address for the proxy to bind to
//   - port: port number for the proxy to listen on
//   - destinationAddr: target address to forward incoming traffic to
//   - tokenManager: token manager for parsing tokens
//   - reportFilePath: path to file for logging failed packets
//   - statsEventChan: channel for publishing stats events
//
// Returns:
//   - *Proxy: configured server-side proxy ready to start
//   - error: nil on success, error if socket creation fails
func NewServerSideProxy(proxyIP string, port int, destinationAddr *net.UDPAddr, tokenManager *token.TokenManager, reportFilePath string, statsEventChan chan<- stats.StatsEvent) (*Proxy, error) {
	if tokenManager == nil {
		return nil, fmt.Errorf("token manager is nil")
	}
	handler := &ServerSideProxyTrafficHandler{
		tokenManager:           tokenManager,
		clientSideProxySources: make(map[int][]*ClientSideProxyInfo),
		nextSourceIndexByPort:  make(map[int]int),
		recentMessages:         make(map[int][]*net.UDPAddr),
	}
	proxy, err := newProxy(proxyIP, port, destinationAddr, handler, reportFilePath, statsEventChan)
	if err != nil {
		return nil, err
	}
	return proxy, nil
}

// newProxy creates a new UDP proxy that forwards traffic between clients and a target address.
//
// Parameters:
//   - proxyIP: IP address for the proxy to bind to
//   - port: port number for the proxy to listen on
//   - destinationAddr: target address to forward incoming traffic to
//   - handler: traffic handler for the proxy to use
//   - reportFilePath: path to file for logging failed packets
//   - statsEventChan: optional channel for publishing stats events (can be nil)
//
// Returns:
//   - *Proxy: configured proxy ready to start
//   - error: nil on success, error if socket creation fails
func newProxy(proxyIP string, port int, destinationAddr *net.UDPAddr, handler TrafficHandler, reportFilePath string, statsEventChan chan<- stats.StatsEvent) (*Proxy, error) {
	addr, err := net.ResolveUDPAddr(UDP, net.JoinHostPort(proxyIP, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP(UDP, addr)
	if err != nil {
		return nil, err
	}

	packetLogger, err := NewPacketLogger(reportFilePath)
	if err != nil {
		conn.Close()
		return nil, err
	}

	forwardToPlayerChan := make(chan ClientBoundData, BufferSize)
	clientConnectionPool := &ClientConnectionPool{
		clientConnectionMap:     make(map[int]*ClientConnection),
		clientConnectionPortMap: make(map[int]*ClientConnectionInfo),
		destinationAddr:         destinationAddr,
		forwardToPlayerChan:     forwardToPlayerChan,
		statsEventChan:          statsEventChan,
	}

	return &Proxy{
		port:                 port,
		socket:               conn,
		destinationAddr:      destinationAddr,
		clientConnectionPool: clientConnectionPool,
		forwardToPlayerChan:  forwardToPlayerChan,
		trafficHandler:       handler,
		packetLogger:         packetLogger,
		statsEventChan:       statsEventChan,
	}, nil
}

// Start begins the proxy's main event loop, handling incoming and return traffic.
//
// Parameters:
//   - ctx: context for graceful shutdown
//
// Returns:
//   - error: nil on graceful shutdown, error if UDP operations fail
func (p *Proxy) Start(ctx context.Context) error {
	buf := make([]byte, BufferSize)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Forwarding stopped for endpoint %s", p.socket.LocalAddr().String())
			return nil
		case data := <-p.forwardToPlayerChan:
			if err := p.trafficHandler.HandleClientBoundTraffic(data, p.clientConnectionPool, p.socket); err != nil {
				if errors.Is(err, ErrPacketDropped) {
					continue
				}
				return err
			}
		default:
			if err := p.handleIncomingTraffic(ctx, buf); err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}
				return err
			}
		}
	}
}

// handleIncomingTraffic processes incoming client traffic and forwards to appropriate client connection.
//
// Parameters:
//   - ctx: context for graceful shutdown
//   - buf: buffer to read incoming data into
//
// Returns:
//   - error: nil on success, error if UDP read or client connection operations fail
func (p *Proxy) handleIncomingTraffic(ctx context.Context, buf []byte) error {
	n, sourceAddr, err := readUDPWithTimeout(p.socket, buf, TenMilliseconds)
	if err != nil {
		return err
	}

	dataCopy := make([]byte, len(buf[:n]))
	copy(dataCopy, buf[:n])

	result, err := p.trafficHandler.PreprocessServerBoundTraffic(dataCopy, sourceAddr)
	if err != nil {
		p.packetLogger.LogFailedPacket(sourceAddr, dataCopy, err.Error())
		// Only send malformed packet event if it's not a dropped packet
		if !errors.Is(err, ErrPacketDropped) {
			p.publishStatsEvent(stats.StatsEvent{
				Type: stats.EventMalformedPacketProcessed,
				Port: p.port,
			})
		}
		return nil
	}

	if result.ConfigCommand != nil {
		if deg, ok := result.ConfigCommand.(DegradationResult); ok {
			p.publishStatsEvent(stats.StatsEvent{
				Type:               stats.EventDegradationUpdate,
				Port:               p.port,
				DegradationPercent: deg.Percentage,
			})
		}
		return nil
	}

	// Publish packet processed event
	p.publishStatsEvent(stats.StatsEvent{
		Type: stats.EventValidPacketProcessed,
		Port: p.port,
	})

	clientConn, err := p.clientConnectionPool.GetOrCreateClientConnection(ctx, result.PlayerNumber, sourceAddr)
	if err != nil {
		return err
	}

	select {
	case clientConn.serverBoundTrafficChan <- result.Data:
	default:
		log.Printf("Dropped packet for player number %d", result.PlayerNumber)
	}

	return nil
}

// publishStatsEvent publishes a stats event using non-blocking select-default pattern
func (p *Proxy) publishStatsEvent(event stats.StatsEvent) {
	if p.statsEventChan != nil {
		select {
		case p.statsEventChan <- event:
			// Event published successfully
		default:
			// Channel full, drop event
		}
	}
}

// readUDPWithTimeout reads from UDP connection with a timeout.
//
// Parameters:
//   - conn: UDP connection to read from
//   - buf: buffer to read data into
//   - timeout: read timeout duration
//
// Returns:
//   - int: number of bytes read
//   - *net.UDPAddr: source address of the packet
//   - error: nil on success, error if read fails or times out
func readUDPWithTimeout(conn *net.UDPConn, buf []byte, timeout time.Duration) (int, *net.UDPAddr, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	return conn.ReadFromUDP(buf)
}

// Close shuts down the proxy and cleans up all resources.
//
// Returns:
//   - error: nil on success, error if socket close fails
func (p *Proxy) Close() error {
	playerNumbers := maps.Keys(p.clientConnectionPool.clientConnectionMap)
	for playerNumber := range playerNumbers {
		p.clientConnectionPool.CloseClientConnection(playerNumber)
	}

	if p.packetLogger != nil {
		p.packetLogger.Close()
	}

	return p.socket.Close()
}

// LocalAddr returns the local network address of the proxy socket.
//
// Returns:
//   - net.Addr: local address of the proxy socket
func (p *Proxy) LocalAddr() net.Addr {
	return p.socket.LocalAddr()
}
