/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package testutil

import (
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"
)

// CreateTestTokenManager creates a token manager with a known token for testing.
// It creates a manager with the specified player count and replaces player 1's token
// with TestTokenHash for predictable testing.
func CreateTestTokenManager(playerCount int) *token.TokenManager {
	tm := token.NewTokenManager(playerCount)
	tm.ReplaceToken(1, TestTokenHash)
	return tm
}
