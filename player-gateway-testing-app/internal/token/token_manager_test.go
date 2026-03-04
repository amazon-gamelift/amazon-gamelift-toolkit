/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package token

import (
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
)

// Test constants - 28-byte tokens for fixed-length parsing
const (
	testTokenHash        = "dG9rZW4xMTExMTExMTExMTExMTExMTExMTExMQ==" // base64("token11111111111111111111111")
	testTokenHashDecoded = "token11111111111111111111111"              // 28 bytes
)

// TestGenerateRandomToken tests token generation
func TestGenerateRandomToken(t *testing.T) {
	t.Run("generates valid base64 format", func(t *testing.T) {
		token := GenerateRandomToken()
		assert.NotEqual(t, token, "", "GenerateRandomToken() returned empty token")

		// Token should be valid base64
		decoded, err := DecodeBase64Token(token)
		assert.NoError(t, err, "GenerateRandomToken() should return valid base64")
		assert.NotEqual(t, decoded, "", "Decoded token should not be empty")
	})

	t.Run("generates decodable token", func(t *testing.T) {
		token := GenerateRandomToken()
		decoded, err := DecodeBase64Token(token)
		assert.NoError(t, err, "Token should be decodable")
		// Decoded token should be hex-encoded hash (32 chars)
		assert.Equal(t, len(decoded), GeneratedTokenLength, "Decoded token length mismatch")
	})

	t.Run("generates unique tokens", func(t *testing.T) {
		token1 := GenerateRandomToken()
		token2 := GenerateRandomToken()
		assert.NotEqual(t, token1, token2, "Tokens should be unique")
	})
}

// TestParseClientSideToken_SplitBehavior tests extracting token from the raw packet data
func TestParseClientSideToken_SplitBehavior(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantData string
		wantErr  error
	}{
		{
			name:     "valid token and data",
			input:    testTokenHashDecoded + "gamedata",
			wantData: "gamedata",
			wantErr:  nil,
		},
		{
			name:     "valid token with empty data",
			input:    testTokenHashDecoded,
			wantData: "",
			wantErr:  nil,
		},
		{
			name:     "valid token with data containing pipe",
			input:    testTokenHashDecoded + "game|data",
			wantData: "game|data",
			wantErr:  nil,
		},
		{
			name:     "packet too short",
			input:    "short",
			wantData: "",
			wantErr:  ErrPacketTooShort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTokenManager(1)
			tm.ReplaceToken(1, testTokenHash)
			data := []byte(tt.input)

			playerNum, modifiedData, err := tm.ParseClientSideToken(data)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr, "ParseClientSideToken() error mismatch")
			} else {
				assert.NoError(t, err, "ParseClientSideToken() unexpected error")
				assert.Equal(t, playerNum, 1, "ParseClientSideToken() player number mismatch")
				// Modified data should be "playerNumber|gameData"
				assert.Equal(t, string(modifiedData), "1|"+tt.wantData, "ParseClientSideToken() data mismatch")
			}
		})
	}
}

func TestParseClientSideToken(t *testing.T) {
	tests := []struct {
		name             string
		playerCount      int
		setupPlayer      int
		setupToken       string
		input            string
		wantPlayerNumber int
		wantModifiedData string
		wantErr          error
	}{
		{
			name:             "valid token returns player number",
			playerCount:      1,
			setupPlayer:      1,
			setupToken:       testTokenHash,
			input:            testTokenHashDecoded + "gamedata",
			wantPlayerNumber: 1,
			wantModifiedData: "1|gamedata",
			wantErr:          nil,
		},
		{
			name:             "valid token with empty game data",
			playerCount:      1,
			setupPlayer:      1,
			setupToken:       testTokenHash,
			input:            testTokenHashDecoded,
			wantPlayerNumber: 1,
			wantModifiedData: "1|",
			wantErr:          nil,
		},
		{
			name:             "returns correct player for multi-player setup",
			playerCount:      3,
			setupPlayer:      2,
			setupToken:       testTokenHash,
			input:            testTokenHashDecoded + "gamedata",
			wantPlayerNumber: 2,
			wantModifiedData: "2|gamedata",
			wantErr:          nil,
		},
		{
			name:             "hash mismatch",
			playerCount:      1,
			setupPlayer:      1,
			setupToken:       testTokenHash,
			input:            "xxxxxxxxxxxxxxxxxxxxxxxxxxxx" + "gamedata", // 28 wrong chars + data
			wantPlayerNumber: 0,
			wantModifiedData: "",
			wantErr:          ErrHashMismatch,
		},
		{
			name:             "packet too short",
			playerCount:      1,
			setupPlayer:      1,
			setupToken:       testTokenHash,
			input:            "x",
			wantPlayerNumber: 0,
			wantModifiedData: "",
			wantErr:          ErrPacketTooShort,
		},
		{
			name:             "empty data",
			playerCount:      1,
			setupPlayer:      1,
			setupToken:       testTokenHash,
			input:            "",
			wantPlayerNumber: 0,
			wantModifiedData: "",
			wantErr:          ErrPacketTooShort,
		},
		{
			name:             "base64 token in packet - client error",
			playerCount:      1,
			setupPlayer:      1,
			setupToken:       testTokenHash,
			input:            testTokenHash[:GeneratedTokenLength] + "gamedata", // Client sent base64 instead of decoded
			wantPlayerNumber: 0,
			wantModifiedData: "",
			wantErr:          ErrBase64TokenInPacket,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTokenManager(tt.playerCount)
			tm.ReplaceToken(tt.setupPlayer, tt.setupToken)
			data := []byte(tt.input)

			playerNumber, modifiedData, err := tm.ParseClientSideToken(data)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr, "ParseClientSideToken() error mismatch")
			} else {
				assert.NoError(t, err, "ParseClientSideToken() unexpected error")
				assert.Equal(t, playerNumber, tt.wantPlayerNumber, "ParseClientSideToken() playerNumber mismatch")
				assert.Equal(t, string(modifiedData), tt.wantModifiedData, "ParseClientSideToken() modifiedData mismatch")
			}
		})
	}
}

func TestParseServerSideToken(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantPlayerNumber int
		wantGameData     string
		wantErr          error
	}{
		{
			name:             "valid single digit player number",
			input:            "1|gamedata",
			wantPlayerNumber: 1,
			wantGameData:     "gamedata",
			wantErr:          nil,
		},
		{
			name:             "valid multi-digit player number",
			input:            "42|gamedata",
			wantPlayerNumber: 42,
			wantGameData:     "gamedata",
			wantErr:          nil,
		},
		{
			name:             "valid large player number",
			input:            "9999|gamedata",
			wantPlayerNumber: 9999,
			wantGameData:     "gamedata",
			wantErr:          nil,
		},
		{
			name:             "empty game data",
			input:            "1|",
			wantPlayerNumber: 1,
			wantGameData:     "",
			wantErr:          nil,
		},
		{
			name:             "game data with delimiter",
			input:            "1|game|data",
			wantPlayerNumber: 1,
			wantGameData:     "game|data",
			wantErr:          nil,
		},
		{
			name:             "missing delimiter",
			input:            "1gamedata",
			wantPlayerNumber: 0,
			wantGameData:     "",
			wantErr:          ErrMissingDelimiter,
		},
		{
			name:             "invalid player number - not a number",
			input:            "abc|gamedata",
			wantPlayerNumber: 0,
			wantGameData:     "",
			wantErr:          ErrMissingDelimiter,
		},
		{
			name:             "packet too short",
			input:            "x",
			wantPlayerNumber: 0,
			wantGameData:     "",
			wantErr:          ErrPacketTooShort,
		},
		{
			name:             "empty data",
			input:            "",
			wantPlayerNumber: 0,
			wantGameData:     "",
			wantErr:          ErrPacketTooShort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTokenManager(1)
			data := []byte(tt.input)

			playerNumber, gameData, err := tm.ParseServerSideToken(data)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr, "ParseServerSideToken() error mismatch")
			} else {
				assert.NoError(t, err, "ParseServerSideToken() unexpected error")
				assert.Equal(t, playerNumber, tt.wantPlayerNumber, "ParseServerSideToken() playerNumber mismatch")
				assert.Equal(t, string(gameData), tt.wantGameData, "ParseServerSideToken() gameData mismatch")
			}
		})
	}
}

func TestNewTokenManager(t *testing.T) {
	t.Run("creates manager with single player", func(t *testing.T) {
		tm := NewTokenManager(1)
		assert.Equal(t, tm.GetPlayerCount(), 1, "Player count mismatch")
		token := tm.GetTokenForPlayer(1)
		assert.NotEqual(t, token, "", "Player 1 should have a token")
	})

	t.Run("creates manager with multiple players", func(t *testing.T) {
		tm := NewTokenManager(5)
		assert.Equal(t, tm.GetPlayerCount(), 5, "Player count mismatch")
		for i := 1; i <= 5; i++ {
			token := tm.GetTokenForPlayer(i)
			assert.NotEqual(t, token, "", "Player should have a token")
		}
	})

	t.Run("each player has unique token", func(t *testing.T) {
		tm := NewTokenManager(5)
		tokens := tm.GetValidTokens()
		seen := make(map[string]bool)
		for _, token := range tokens {
			assert.False(t, seen[token], "Tokens should be unique")
			seen[token] = true
		}
	})
}

func TestReplaceToken(t *testing.T) {
	t.Run("replaces token for valid player", func(t *testing.T) {
		tm := NewTokenManager(2)
		err := tm.ReplaceToken(1, testTokenHash)
		assert.NoError(t, err, "ReplaceToken() should succeed")
		assert.Equal(t, tm.GetTokenForPlayer(1), testTokenHash, "Token should be replaced")
	})

	t.Run("error for player number too high", func(t *testing.T) {
		tm := NewTokenManager(2)
		err := tm.ReplaceToken(5, testTokenHash)
		assert.Error(t, err, "ReplaceToken() should fail for invalid player")
		assert.ErrorIs(t, err, ErrInvalidPlayerNumber, "Should return ErrInvalidPlayerNumber")
	})

	t.Run("error for player number zero", func(t *testing.T) {
		tm := NewTokenManager(2)
		err := tm.ReplaceToken(0, testTokenHash)
		assert.Error(t, err, "ReplaceToken() should fail for player 0")
		assert.ErrorIs(t, err, ErrInvalidPlayerNumber, "Should return ErrInvalidPlayerNumber")
	})

	t.Run("error for negative player number", func(t *testing.T) {
		tm := NewTokenManager(2)
		err := tm.ReplaceToken(-1, testTokenHash)
		assert.Error(t, err, "ReplaceToken() should fail for negative player")
		assert.ErrorIs(t, err, ErrInvalidPlayerNumber, "Should return ErrInvalidPlayerNumber")
	})

	t.Run("error for empty token", func(t *testing.T) {
		tm := NewTokenManager(1)
		err := tm.ReplaceToken(1, "")
		assert.Error(t, err, "ReplaceToken() should fail for empty token")
		assert.ErrorIs(t, err, ErrZeroLengthToken, "Should return ErrZeroLengthToken")
	})

	t.Run("error for invalid base64", func(t *testing.T) {
		tm := NewTokenManager(1)
		err := tm.ReplaceToken(1, "not-valid-base64!!!")
		assert.Error(t, err, "ReplaceToken() should fail for invalid base64")
		assert.ErrorIs(t, err, ErrInvalidBase64, "Should return ErrInvalidBase64")
	})

	t.Run("error for duplicate token on different player", func(t *testing.T) {
		tm := NewTokenManager(2)
		err := tm.ReplaceToken(1, testTokenHash)
		assert.NoError(t, err, "First ReplaceToken() should succeed")
		err = tm.ReplaceToken(2, testTokenHash)
		assert.Error(t, err, "ReplaceToken() should fail for duplicate token")
		assert.ErrorIs(t, err, ErrTokenAlreadyExists, "Should return ErrTokenAlreadyExists")
	})

	t.Run("allows replacing same player with same token", func(t *testing.T) {
		tm := NewTokenManager(1)
		err := tm.ReplaceToken(1, testTokenHash)
		assert.NoError(t, err, "First ReplaceToken() should succeed")
		err = tm.ReplaceToken(1, testTokenHash)
		assert.NoError(t, err, "Second ReplaceToken() with same token should succeed")
	})

	t.Run("allows replacing same player with different token", func(t *testing.T) {
		tm := NewTokenManager(1)
		err := tm.ReplaceToken(1, testTokenHash)
		assert.NoError(t, err, "First ReplaceToken() should succeed")
		newToken := "bmV3LXRva2Vu" // base64("new-token")
		err = tm.ReplaceToken(1, newToken)
		assert.NoError(t, err, "ReplaceToken() with new token should succeed")
		assert.Equal(t, tm.GetTokenForPlayer(1), newToken, "Token should be updated")
	})
}

func TestGetTokenForPlayer(t *testing.T) {
	t.Run("returns token for valid player", func(t *testing.T) {
		tm := NewTokenManager(2)
		tm.ReplaceToken(1, testTokenHash)
		token := tm.GetTokenForPlayer(1)
		assert.Equal(t, token, testTokenHash, "Should return correct token")
	})

	t.Run("returns empty for player number too high", func(t *testing.T) {
		tm := NewTokenManager(1)
		token := tm.GetTokenForPlayer(5)
		assert.Equal(t, token, "", "Should return empty for invalid player")
	})

	t.Run("returns empty for player zero", func(t *testing.T) {
		tm := NewTokenManager(1)
		token := tm.GetTokenForPlayer(0)
		assert.Equal(t, token, "", "Should return empty for player 0")
	})

	t.Run("returns empty for negative player", func(t *testing.T) {
		tm := NewTokenManager(1)
		token := tm.GetTokenForPlayer(-1)
		assert.Equal(t, token, "", "Should return empty for negative player")
	})
}

func TestGetValidTokens(t *testing.T) {
	t.Run("returns all tokens", func(t *testing.T) {
		tm := NewTokenManager(3)
		tokens := tm.GetValidTokens()
		assert.Equal(t, len(tokens), 3, "Should return 3 tokens")
	})

	t.Run("returns map with correct player numbers", func(t *testing.T) {
		tm := NewTokenManager(2)
		tm.ReplaceToken(1, testTokenHash)
		tokens := tm.GetValidTokens()
		assert.Equal(t, tokens[1], testTokenHash, "Player 1 token should match")
		assert.NotEqual(t, tokens[2], "", "Player 2 should have a token")
	})
}

func TestGetPlayerCount(t *testing.T) {
	t.Run("returns correct count", func(t *testing.T) {
		tm := NewTokenManager(5)
		assert.Equal(t, tm.GetPlayerCount(), 5, "Player count should be 5")
	})

	t.Run("returns 1 for single player", func(t *testing.T) {
		tm := NewTokenManager(1)
		assert.Equal(t, tm.GetPlayerCount(), 1, "Player count should be 1")
	})
}

func TestDecodeBase64Token(t *testing.T) {
	t.Run("decodes valid base64", func(t *testing.T) {
		decoded, err := DecodeBase64Token(testTokenHash)
		assert.NoError(t, err, "Should decode valid base64")
		assert.Equal(t, decoded, testTokenHashDecoded, "Decoded value should match")
	})

	t.Run("error for invalid base64", func(t *testing.T) {
		_, err := DecodeBase64Token("not-valid!!!")
		assert.Error(t, err, "Should fail for invalid base64")
		assert.ErrorIs(t, err, ErrInvalidBase64, "Should return ErrInvalidBase64")
	})

	t.Run("decodes empty string", func(t *testing.T) {
		decoded, err := DecodeBase64Token("")
		assert.NoError(t, err, "Should decode empty string")
		assert.Equal(t, decoded, "", "Decoded empty should be empty")
	})
}

func TestMultipleTokensValidSimultaneously(t *testing.T) {
	tm := NewTokenManager(3)

	// Set up known 28-byte tokens for each player
	token1 := "dG9rZW4xMTExMTExMTExMTExMTExMTExMTExMQ==" // base64("token11111111111111111111111")
	token2 := "dG9rZW4yMjIyMjIyMjIyMjIyMjIyMjIyMjIyMg==" // base64("token22222222222222222222222")
	token3 := "dG9rZW4zMzMzMzMzMzMzMzMzMzMzMzMzMzMzMw==" // base64("token33333333333333333333333")

	tm.ReplaceToken(1, token1)
	tm.ReplaceToken(2, token2)
	tm.ReplaceToken(3, token3)

	// Verify each token maps to correct player
	data1 := []byte("token11111111111111111111111gamedata")
	playerNum, _, err := tm.ParseClientSideToken(data1)
	assert.NoError(t, err, "Token1 should be valid")
	assert.Equal(t, playerNum, 1, "Token1 should map to player 1")

	data2 := []byte("token22222222222222222222222gamedata")
	playerNum, _, err = tm.ParseClientSideToken(data2)
	assert.NoError(t, err, "Token2 should be valid")
	assert.Equal(t, playerNum, 2, "Token2 should map to player 2")

	data3 := []byte("token33333333333333333333333gamedata")
	playerNum, _, err = tm.ParseClientSideToken(data3)
	assert.NoError(t, err, "Token3 should be valid")
	assert.Equal(t, playerNum, 3, "Token3 should map to player 3")
}
