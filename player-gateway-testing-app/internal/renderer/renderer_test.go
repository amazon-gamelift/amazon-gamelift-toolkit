/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package renderer

import (
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/stats"
	"context"
	"testing"
)

func TestNewModel(t *testing.T) {
	statsCollector := stats.NewStatsCollector(nil, 8000, 3, 1, "127.0.0.1")
	_, cancel := context.WithCancel(context.Background())

	model, err := NewModel(statsCollector, cancel)

	assert.NoError(t, err)
	assert.NotNil(t, model)
	assert.NotEqual(t, model.width, 0)
}

func TestNewModel_NilStatsCollector(t *testing.T) {
	var statsCollector *stats.StatsCollector = nil
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	model, err := NewModel(statsCollector, cancel)

	assert.Error(t, err)
	assert.Equal(t, model, (*Model)(nil))
}
