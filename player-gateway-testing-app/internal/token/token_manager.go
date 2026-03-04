/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package token

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
)

const (
	TokenDelimiter       = "|" // Delimiter used internally between player number and game data
	GeneratedTokenLength = 28  // Length for generated tokens (hex characters)
)

var (
	ErrMissingDelimiter    = errors.New("missing delimiter in server-side packet")
	ErrHashMismatch        = errors.New("token hash does not match expected value")
	ErrPacketTooShort      = errors.New("packet is shorter than token length")
	ErrInvalidBase64       = errors.New("token is not valid base64")
	ErrBase64TokenInPacket = errors.New("token appears to be base64 encoded - client must decode token before prepending")
	ErrZeroLengthToken     = errors.New("zero-length token not allowed")
	ErrTokenAlreadyExists  = errors.New("token already exists")
	ErrInvalidPlayerNumber = errors.New("player number is invalid")
)

// TokenManager handles token generation, validation, and parsing operations.
// Each token maps to a specific player number (1-indexed).
type TokenManager struct {
	tokenToPlayer map[string]int // Decoded token -> player number
	base64Tokens  []string       // Base64-encoded tokens indexed by (playerNumber - 1)
	decodedTokens []string       // Decoded tokens indexed by (playerNumber - 1)
	playerCount   int
	mu            sync.RWMutex
}

// NewTokenManager creates a new token manager with tokens for the specified player count.
// Generates one unique token per player (1-indexed).
//
// Parameters:
//   - playerCount: Number of players/tokens to generate
//
// Returns:
//   - *TokenManager: Configured token manager instance
func NewTokenManager(playerCount int) *TokenManager {
	tm := &TokenManager{
		tokenToPlayer: make(map[string]int),
		base64Tokens:  make([]string, playerCount),
		decodedTokens: make([]string, playerCount),
		playerCount:   playerCount,
	}

	for i := range playerCount {
		playerNumber := i + 1
		base64Token := GenerateRandomToken()
		decoded, _ := DecodeBase64Token(base64Token)
		tm.tokenToPlayer[decoded] = playerNumber
		tm.base64Tokens[i] = base64Token
		tm.decodedTokens[i] = decoded
	}

	return tm
}

// ReplaceToken replaces the token for a specific player.
//
// Parameters:
//   - playerNumber: Player number (1-indexed)
//   - base64Token: Base64-encoded token string
//
// Returns:
//   - error: ErrInvalidPlayerNumber if out of range, ErrZeroLengthToken if empty,
//     ErrInvalidBase64 if not valid base64, ErrTokenAlreadyExists if token exists for another player
func (tm *TokenManager) ReplaceToken(playerNumber int, base64Token string) error {
	if base64Token == "" {
		return ErrZeroLengthToken
	}

	decoded, err := DecodeBase64Token(base64Token)
	if err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if playerNumber < 1 || playerNumber > tm.playerCount {
		return fmt.Errorf("%w: must be between 1 and %d", ErrInvalidPlayerNumber, tm.playerCount)
	}

	// Check if token already exists for a different player
	if existingPlayer, exists := tm.tokenToPlayer[decoded]; exists && existingPlayer != playerNumber {
		return ErrTokenAlreadyExists
	}

	idx := playerNumber - 1

	// Remove old token mapping
	oldDecoded := tm.decodedTokens[idx]
	delete(tm.tokenToPlayer, oldDecoded)

	// Set new token
	tm.tokenToPlayer[decoded] = playerNumber
	tm.base64Tokens[idx] = base64Token
	tm.decodedTokens[idx] = decoded

	return nil
}

// GetTokenForPlayer returns the base64-encoded token for a specific player.
//
// Parameters:
//   - playerNumber: Player number (1-indexed)
//
// Returns:
//   - string: Base64-encoded token, or empty string if invalid player number
func (tm *TokenManager) GetTokenForPlayer(playerNumber int) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	idx := playerNumber - 1
	if idx < 0 || idx >= len(tm.base64Tokens) {
		return ""
	}
	return tm.base64Tokens[idx]
}

// GetDecodedTokenForPlayer returns the decoded token for a specific player.
//
// Parameters:
//   - playerNumber: Player number (1-indexed)
//
// Returns:
//   - string: Decoded token, or empty string if invalid player number
func (tm *TokenManager) GetDecodedTokenForPlayer(playerNumber int) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	idx := playerNumber - 1
	if idx < 0 || idx >= len(tm.decodedTokens) {
		return ""
	}
	return tm.decodedTokens[idx]
}

// GetValidTokens returns all tokens as a map of player number to base64-encoded token.
//
// Returns:
//   - map[int]string: Player number -> base64-encoded token
func (tm *TokenManager) GetValidTokens() map[int]string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make(map[int]string)
	for i, token := range tm.base64Tokens {
		result[i+1] = token
	}
	return result
}

// GetPlayerCount returns the number of players/tokens.
//
// Returns:
//   - int: Number of players
func (tm *TokenManager) GetPlayerCount() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.playerCount
}

// GenerateRandomToken generates a cryptographically random token and returns it base64-encoded.
//
// Returns:
//   - string: Base64-encoded random token
func GenerateRandomToken() string {
	hashBytes := make([]byte, GeneratedTokenLength/2)
	rand.Read(hashBytes)
	hash := hex.EncodeToString(hashBytes)
	return base64.StdEncoding.EncodeToString([]byte(hash))
}

// DecodeBase64Token decodes a base64-encoded token string.
//
// Parameters:
//   - base64Token: Base64-encoded token string
//
// Returns:
//   - string: Decoded token string
//   - error: ErrInvalidBase64 if decoding fails
func DecodeBase64Token(base64Token string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(base64Token)
	if err != nil {
		return "", ErrInvalidBase64
	}
	return string(decoded), nil
}

// ParseClientSideToken parses data from clients for a valid token.
// Expected format: first 28 bytes are the token hash, remainder is game data.
// Player number is determined by which token was used.
//
// Parameters:
//   - data: Raw packet data containing the token and game data
//
// Returns:
//   - int: Player number associated with the token
//   - []byte: Modified data in format "playerNumber|gameData"
//   - error: ErrPacketTooShort, ErrBase64TokenInPacket, or ErrHashMismatch
func (tm *TokenManager) ParseClientSideToken(data []byte) (int, []byte, error) {
	if len(data) < GeneratedTokenLength {
		return 0, nil, ErrPacketTooShort
	}

	hash := string(data[:GeneratedTokenLength])
	gameData := data[GeneratedTokenLength:]

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Check if hash matches any decoded token
	playerNumber, isValid := tm.tokenToPlayer[hash]
	if isValid {
		// Client correctly sent decoded token
		playerStr := strconv.Itoa(playerNumber)
		modifiedData := make([]byte, 0, len(playerStr)+1+len(gameData))
		modifiedData = append(modifiedData, []byte(playerStr)...)
		modifiedData = append(modifiedData, TokenDelimiter[0])
		modifiedData = append(modifiedData, gameData...)
		return playerNumber, modifiedData, nil
	}

	// Check if client sent base64-encoded token instead of decoded
	for _, b64 := range tm.base64Tokens {
		if hash == b64[:GeneratedTokenLength] {
			return 0, nil, ErrBase64TokenInPacket
		}
	}

	return 0, nil, ErrHashMismatch
}

// ParseServerSideToken parses data from client-side proxies.
// Expected format: "playerNumber|gameData"
//
// Parameters:
//   - data: Raw packet data containing player number and game data
//
// Returns:
//   - int: Player number
//   - []byte: Game data portion
//   - error: ErrPacketTooShort or ErrMissingDelimiter if format is invalid
func (tm *TokenManager) ParseServerSideToken(data []byte) (int, []byte, error) {
	if len(data) < 2 { // Minimum: "1|"
		return 0, nil, ErrPacketTooShort
	}

	delimiterIndex := bytes.IndexByte(data, TokenDelimiter[0])
	if delimiterIndex == -1 {
		return 0, nil, ErrMissingDelimiter
	}

	playerNumber, err := strconv.Atoi(string(data[:delimiterIndex]))
	if err != nil {
		return 0, nil, ErrMissingDelimiter
	}

	return playerNumber, data[delimiterIndex+1:], nil
}
