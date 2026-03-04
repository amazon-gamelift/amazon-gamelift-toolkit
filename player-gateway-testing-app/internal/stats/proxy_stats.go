/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package stats

import (
	"sync"
	"sync/atomic"
)

// ProxyStats holds statistics for a single endpoint with atomic counters
type ProxyStats struct {
	Port                  int
	ValidPackets          int64 // atomic counter
	MalformedPackets      int64 // atomic counter
	DegradationPercentage int   // protected by mutex
	mu                    sync.RWMutex
}

// IncrementValidPackets atomically increments the valid packets counter
func (ps *ProxyStats) IncrementValidPackets() {
	atomic.AddInt64(&ps.ValidPackets, 1)
}

// IncrementMalformedPackets atomically increments the malformed packets counter
func (ps *ProxyStats) IncrementMalformedPackets() {
	atomic.AddInt64(&ps.MalformedPackets, 1)
}

// GetValidPackets atomically reads the valid packets processed
func (ps *ProxyStats) GetValidPackets() int64 {
	return atomic.LoadInt64(&ps.ValidPackets)
}

// GetMalformedPackets atomically reads the malformed packets counter
func (ps *ProxyStats) GetMalformedPackets() int64 {
	return atomic.LoadInt64(&ps.MalformedPackets)
}

// SetDegradationPercentage sets the degradation percentage with mutex protection
func (ps *ProxyStats) SetDegradationPercentage(percentage int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.DegradationPercentage = percentage
}

// GetDegradationPercentage reads the degradation percentage with mutex protection
func (ps *ProxyStats) GetDegradationPercentage() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.DegradationPercentage
}
