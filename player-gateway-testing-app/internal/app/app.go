/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/common"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/config"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/proxy"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/renderer"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/stats"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"

	tea "github.com/charmbracelet/bubbletea"
)

// App manages the UDP proxy application with client and server proxy instances.
type App struct {
	clientSideProxies []*proxy.Proxy        // Proxy endpoints that clients connect to
	serverSideProxy   *proxy.Proxy          // Server endpoint that the game server connect to
	tokenManager      *token.TokenManager   // Token manager for validation and parsing
	statsCollector    *stats.StatsCollector // Stats collector for aggregating metrics
}

// New creates a new App instance
//
// Returns:
//   - *App: configured App ready to run
func New() *App {
	return &App{}
}

// Run starts the proxy application with the given configuration.
//
// Parameters:
//   - ctx: context for graceful shutdown
//   - cancel: cancel function to trigger shutdown from UI
//   - c: configuration containing server and client settings
//
// Returns:
//   - error: nil on success, error if proxy setup or startup fails
func (a *App) Run(ctx context.Context, cancel context.CancelFunc, c config.Config) error {
	defer a.cleanup()

	if !c.Headless {
		f, err := tea.LogToFile("application.log", "")
		if err != nil {
			return err
		}
		defer f.Close()
	}

	if err := a.setupProxies(c); err != nil {
		return err
	}

	wg := a.startProxies(ctx, cancel, c.Headless)

	<-ctx.Done()
	wg.Wait()
	return nil
}

// GetTokenForPlayer returns the base64-encoded token for a specific player.
func (a *App) GetTokenForPlayer(playerNum int) string {
	return a.tokenManager.GetTokenForPlayer(playerNum)
}

// GetDecodedTokenForPlayer returns the decoded token for a specific player.
func (a *App) GetDecodedTokenForPlayer(playerNum int) string {
	return a.tokenManager.GetDecodedTokenForPlayer(playerNum)
}

// setupProxies creates both server and client proxies from config.
//
// Parameters:
//   - c: configuration for proxy endpoints
//
// Returns:
//   - error: nil on success, error if proxy creation fails
func (a *App) setupProxies(c config.Config) error {
	if err := a.initializeToken(c); err != nil {
		return err
	}

	startPort, err := a.findClientsideProxyPortRangeStart(c)
	if err != nil {
		return err
	}
	a.statsCollector = stats.NewStatsCollector(a.tokenManager, startPort, c.UDPEndpointCount, c.PlayerCount, c.IPAddress)

	if err := a.createServerSideProxy(c); err != nil {
		return err
	}

	if err := a.createClientSideProxies(c, startPort); err != nil {
		return err
	}

	return nil
}

// initializeToken creates TokenManager with tokens for the specified player count.
//
// Parameters:
//   - c: configuration containing player count
//
// Returns:
//   - error: nil on success, error if player count is invalid
func (a *App) initializeToken(c config.Config) error {
	if c.PlayerCount < 1 {
		return fmt.Errorf("player-count must be 1 or more")
	}
	a.tokenManager = token.NewTokenManager(c.PlayerCount)
	return nil
}

// createServerSideProxy creates the server-facing proxy that forwards to the game server.
//
// Parameters:
//   - c: configuration containing server IP address and port
//
// Returns:
//   - error: nil on success, error if server proxy creation fails
func (a *App) createServerSideProxy(c config.Config) error {
	gameSessionAddr, err := net.ResolveUDPAddr(proxy.UDP, net.JoinHostPort(c.ServerIPAddress, strconv.Itoa(c.ServerPort)))
	if err != nil {
		return err
	}

	anyAvailablePort := 0
	a.serverSideProxy, err = proxy.NewServerSideProxy(
		c.IPAddress,
		anyAvailablePort,
		gameSessionAddr,
		a.tokenManager,
		c.ReportFilePath,
		a.statsCollector.GetEventChannel(),
	)
	if err != nil {
		return err
	}

	return nil
}

// createClientSideProxies creates multiple client-facing proxies that accept client connections.
// Each player gets their own set of endpoints.
//
// Parameters:
//   - c: configuration containing client IP address, port range, and endpoint count
//   - startPort: starting port number for the first player's endpoints
//
// Returns:
//   - error: nil on success, error if client proxy creation fails
func (a *App) createClientSideProxies(c config.Config, startPort int) error {
	destinationAddr, ok := a.serverSideProxy.LocalAddr().(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("unable to parse UDP address when creating client-side proxy")
	}

	totalEndpoints := c.UDPEndpointCount * c.PlayerCount
	a.clientSideProxies = make([]*proxy.Proxy, 0, totalEndpoints)

	port := startPort
	for playerNum := 1; playerNum <= c.PlayerCount; playerNum++ {
		for i := range c.UDPEndpointCount {
			csp, err := proxy.NewClientSideProxy(
				c.IPAddress,
				port,
				destinationAddr,
				a.tokenManager,
				playerNum,
				c.ReportFilePath,
				a.statsCollector.GetEventChannel(),
				a.statsCollector.GetSnapshot,
			)
			if err != nil {
				return fmt.Errorf("failed to create client-side proxy for player %d endpoint %d: %w", playerNum, i+1, err)
			}
			a.clientSideProxies = append(a.clientSideProxies, csp)
			port++
		}
	}

	return nil
}

func (a *App) findClientsideProxyPortRangeStart(c config.Config) (int, error) {
	var startPort int
	var err error

	totalEndpoints := c.UDPEndpointCount * c.PlayerCount
	if c.PortRange == "" {
		const defaultStartSearchPort = 8000
		const maxSearchPort = 65535
		startPort, err = common.FindAvailablePortRange(defaultStartSearchPort, totalEndpoints, maxSearchPort)
		if err != nil {
			return 0, fmt.Errorf("failed to find available port range: %w", err)
		}
		log.Printf("Auto-selected port range starting at %d for %d players with %d endpoints each (%d total)",
			startPort, c.PlayerCount, c.UDPEndpointCount, totalEndpoints)
	} else {
		startPort, _, err = common.ParsePortRange(c.PortRange)
		if err != nil {
			return 0, err
		}
	}
	return startPort, nil
}

// startProxies starts all client and server proxy goroutines.
//
// Parameters:
//   - ctx: context for graceful shutdown of proxy goroutines
//   - cancel: cancel function to allow UI to trigger shutdown
//   - runHeadless: if the application should run without UI
//
// Returns:
//   - *sync.WaitGroup: WaitGroup to wait for all proxies to stop
func (a *App) startProxies(ctx context.Context, cancel context.CancelFunc, runHeadless bool) *sync.WaitGroup {
	var wg sync.WaitGroup

	// Start stats collector
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.statsCollector.Start(ctx)
	}()

	// Start display rendering loop
	if !runHeadless {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.runDisplayLoop(cancel); err != nil {
				log.Printf("Error encountered from UI: %v", err)
			}
		}()
	}

	for i, clientProxy := range a.clientSideProxies {
		wg.Add(1)
		go func(proxy *proxy.Proxy, index int) {
			defer wg.Done()
			if err := proxy.Start(ctx); err != nil {
				log.Printf("Client-side proxy %d error: %v", index+1, err)
			}
		}(clientProxy, i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.serverSideProxy.Start(ctx); err != nil {
			log.Printf("Server-side proxy error: %v", err)
		}
	}()

	return &wg
}

// runDisplayLoop runs the display rendering loop until context cancellation.
//
// Parameters:
//   - cancel: cancel function to allow UI to trigger shutdown
func (a *App) runDisplayLoop(cancel context.CancelFunc) error {
	model, err := renderer.NewModel(a.statsCollector, cancel)
	if err != nil {
		return err
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// cleanup closes all proxy connections and releases resources.
func (a *App) cleanup() {
	for i, clientProxy := range a.clientSideProxies {
		if clientProxy != nil {
			if err := clientProxy.Close(); err != nil {
				log.Printf("Error closing client-side proxy %d: %v", i+1, err)
			}
		}
	}

	if a.serverSideProxy != nil {
		if err := a.serverSideProxy.Close(); err != nil {
			log.Printf("Error closing server-side proxy: %v", err)
		}
	}
}
