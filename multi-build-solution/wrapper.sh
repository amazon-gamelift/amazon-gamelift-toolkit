#!/bin/bash

# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

# TODO: Set your hosting port here
PORT=7777
# TODO: Set the relative path to your game server binary within each build version folder
SERVER_BINARY_RELATIVE_PATH="gameserver"

# Shared builds directory (mounted volume)
BUILDS_DIR="/shared/builds"

# Trap termination signals to ensure clean shutdown
trap 'echo "Received termination signal, shutting down..."; kill -SIGINT $WRAPPER_PID 2>/dev/null; exit 0' SIGTERM SIGINT

# Clean up any leftover files from previous sessions
echo "Cleaning up any possible temporary files for build version and activate session"
rm -f /tmp/build_version.txt /tmp/activate_session.txt

echo "Start Go SDK Wrapper in the background and register our port"
./SdkGoWrapper/gameliftwrapper $PORT &
WRAPPER_PID=$!

echo "Container will wait indefinitely until a session is allocated or termination is requested"

# Poll for the build version file created by the Go SDK wrapper
BUILD_VERSION_FILE="/tmp/build_version.txt"

# Wait indefinitely for game session allocation
while [ ! -f "$BUILD_VERSION_FILE" ]; do
    # Check if the Go wrapper is still running
    if ! kill -0 $WRAPPER_PID 2>/dev/null; then
        echo "ERROR: Go wrapper process terminated unexpectedly"
        exit 1
    fi
    sleep 1
done

BUILD_VERSION=$(cat "$BUILD_VERSION_FILE")
echo "Received build version: $BUILD_VERSION"

# Construct the path to the game server binary
SERVER_BINARY_PATH="$BUILDS_DIR/$BUILD_VERSION/$SERVER_BINARY_RELATIVE_PATH"

# Wait for the build to be available (synced by sidecar)
echo "Waiting for build version $BUILD_VERSION to be available..."
echo "Looking for: $SERVER_BINARY_PATH"
SYNC_TIMEOUT=30  # Wait 30 seconds for the build
SYNC_ELAPSED=0

while [ ! -f "$SERVER_BINARY_PATH" ] && [ $SYNC_ELAPSED -lt $SYNC_TIMEOUT ]; do
    if [ $((SYNC_ELAPSED % 10)) -eq 0 ]; then
        echo "Still waiting... ($SYNC_ELAPSED seconds elapsed)"
        echo "Build folder contents:"
        ls -la "$BUILDS_DIR/" 2>/dev/null || echo "  (builds directory not accessible)"
    fi
    sleep 2
    SYNC_ELAPSED=$((SYNC_ELAPSED + 2))
done

if [ ! -f "$SERVER_BINARY_PATH" ]; then
    echo "ERROR: Build version $BUILD_VERSION not available after $SYNC_TIMEOUT seconds"
    echo "Available builds in $BUILDS_DIR:"
    ls -la "$BUILDS_DIR/" 2>/dev/null || echo "  (builds directory not accessible)"
    echo "Terminating..."
    kill -SIGINT $WRAPPER_PID
    exit 1
fi

echo "Build version $BUILD_VERSION is available"

# Wait for download completion marker for max 30 seconds
echo "Verifying build download is complete..."
DOWNLOAD_MARKER="$BUILDS_DIR/$BUILD_VERSION/.download_complete"
MARKER_TIMEOUT=30
MARKER_ELAPSED=0

while [ ! -f "$DOWNLOAD_MARKER" ] && [ $MARKER_ELAPSED -lt $MARKER_TIMEOUT ]; do
    echo "Waiting for download completion marker... ($MARKER_ELAPSED seconds elapsed)"
    sleep 2
    MARKER_ELAPSED=$((MARKER_ELAPSED + 2))
done

if [ ! -f "$DOWNLOAD_MARKER" ]; then
    echo "ERROR: Build version $BUILD_VERSION download not complete (missing .download_complete marker)"
    echo "This build may be incomplete or still uploading"
    kill -SIGINT $WRAPPER_PID
    exit 1
fi

echo "Build version $BUILD_VERSION is fully downloaded and ready"

# Verify the binary is executable
if [ ! -x "$SERVER_BINARY_PATH" ]; then
    echo "ERROR: Binary exists but is not executable: $SERVER_BINARY_PATH"
    ls -la "$SERVER_BINARY_PATH"
    echo "Terminating..."
    kill -SIGINT $WRAPPER_PID
    exit 1
fi

echo "Starting game server from build version $BUILD_VERSION"
cd "$BUILDS_DIR/$BUILD_VERSION" || exit 1

# Start the game server in the background
./"$SERVER_BINARY_RELATIVE_PATH" &
SERVER_PID=$!

# Give the server a moment to initialize
sleep 2

# Check if the server process is still running
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "ERROR: Game server process failed to start immediately"
    echo "Go wrapper will timeout and terminate the container"
    kill -SIGINT $WRAPPER_PID
    exit 1
fi

echo "Game server process started successfully (PID: $SERVER_PID)"

# Signal the Go wrapper that the game server is ready
echo "Signaling Go wrapper to activate game session..."
touch /tmp/activate_session.txt

# Wait for the game server to exit
wait $SERVER_PID
SERVER_EXIT_CODE=$?

echo "Game server terminated with exit code $SERVER_EXIT_CODE"

# Clean up activation file
rm -f /tmp/activate_session.txt

echo "Game server terminated, signal wrapper so it can call ProcessEnding()"
kill -SIGINT $WRAPPER_PID
sleep 0.3
