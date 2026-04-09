/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
 
package main

import (
        "aws/amazon-gamelift-go-sdk/model"
        "aws/amazon-gamelift-go-sdk/server"
        "log"
        "time"
        "os"
        "os/signal"
        "syscall"
        "fmt"
        "strconv"
        "net/http"
        "encoding/json"
)

// GameSession data
var GameSession model.GameSession
// UpdateGameSession data
var UpdateGameSession model.UpdateGameSession

type gameProcess struct {
        // Port - port for incoming player connections
        Port int
        // Logs - set of files to upload when the game session ends. We don't use this with container-based deployments
        Logs server.LogParameters
}

func (g gameProcess) OnStartGameSession(session model.GameSession) {
        
        // Store the session in the global variable
        GameSession = session
        
        // Extract build version from game properties
        buildVersion := ""
        if val, ok := session.GameProperties["BuildVersion"]; ok {
                buildVersion = val
                log.Printf("Build version from GameProperties: %s", buildVersion)
        } else {
                log.Print("WARNING: No BuildVersion specified in GameProperties, using 'default'")
                buildVersion = "default"
        }
        
        // Write build version to file for wrapper.sh to read
        err := os.WriteFile("/tmp/build_version.txt", []byte(buildVersion), 0644)
        if err != nil {
                log.Printf("ERROR: Failed to write build version file: %s", err.Error())
        } else {
                log.Printf("Build version written to /tmp/build_version.txt: %s", buildVersion)
        }
        
        // Wait for wrapper.sh to signal that the game server process has started
        log.Print("Waiting for game server to start (60 second timeout)...")
        activationFile := "/tmp/activate_session.txt"
        timeout := time.After(60 * time.Second)
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()
        
        startTime := time.Now()
        for {
                select {
                case <-timeout:
                        log.Printf("ERROR: Game server failed to start within 60 seconds")
                        log.Printf("Elapsed time: %.1f seconds", time.Since(startTime).Seconds())
                        log.Print("Calling ProcessEnding() and terminating container...")
                        server.ProcessEnding()
                        os.Exit(1)
                case <-ticker.C:
                        elapsed := time.Since(startTime).Seconds()
                        log.Printf("Still waiting for activation file... (%.1f seconds elapsed)", elapsed)
                        
                        if _, err := os.Stat(activationFile); err == nil {
                                // File exists, game server is ready
                                log.Printf("Game server ready signal received after %.1f seconds", elapsed)
                                
                                // Activate the game session now that server is running
                                err = server.ActivateGameSession()
                                if err != nil {
                                        log.Fatal(err.Error())
                                }
                                log.Print("Activated game session")
                                
                                // Clean up the activation file
                                os.Remove(activationFile)
                                return
                        }
                }
        }
}

func (g gameProcess) OnUpdateGameSession(session model.UpdateGameSession) {
        // Handle updated game session data (FlexMatch backfilling)

        // Store the update data in the global variable
        UpdateGameSession = session
}

func (g gameProcess) OnProcessTerminate() {

        // Amazon GameLift will invoke this callback before shutting down an instance hosting this game server.
        // This will not happen on instances running game sessions and with fleet instance protection on, unless they are running on Spot and the instance is interrupted

        // First we tell GameLift we're ending the process
        server.ProcessEnding()

        // Send a termination signal to the parent process which is the wrapper.sh (this will terminate both this process, as well as the game server)
        // Get parent process ID
        ppid := os.Getppid()
        fmt.Printf("Parent Process ID: %d\n", ppid)

        // Get handle to parent process
        parent, err := os.FindProcess(ppid)
        if err != nil {
                fmt.Printf("Failed to find parent process: %v\n", err)
                return
        }
        // Send SIGKILL signal to parent
        fmt.Printf("Attempting to terminate parent process %d\n", ppid)
        err = parent.Signal(syscall.SIGKILL)
        if err != nil {
                fmt.Printf("Failed to terminate parent process: %v\n", err)
                return
        }

        // Terminate the wrapper (this should already happen when we terminate the parent process)
        os.Exit(0)
}

func (g gameProcess) OnHealthCheck() bool {
        // We expect the game server to be healthy as long as it's running. For deep health checks, integrate the SDK to the game server itself.
        return true
}

// A HTTP server to provide game session data to the game server process on request. Only runs on localhost for security
func HttpServer(port int) {
        go func() {
                http.HandleFunc("/gamesessiondata", GameSessionData)
                http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port), nil)
        }()
}

func GameSessionData(w http.ResponseWriter, req *http.Request) {

        // If UpdateGameSessionData GameSessionId is not empty, return the updated data
        // NOTE: We're not returning the UpdateReason or BackfillTicketID that are also included in an UpdateGameSession, just the game session data itself
        if UpdateGameSession.GameSession.GameSessionID != "" {
                log.Print("Returning Updated GameSessionData")
                jsonBytes, err := json.Marshal(UpdateGameSession.GameSession)
                if err != nil {
                        fmt.Println("Error:", err)
                        // Set the response status code to 500 (Internal Server Error)
                        w.WriteHeader(http.StatusInternalServerError)
                        fmt.Fprintf(w, "error\n")
                        return
                }
                w.Header().Set("Content-Type", "application/json")
                fmt.Fprintf(w, string(jsonBytes))
                return
        }

        // else try to return the initial game session data (could be empty values but that's fine, client can handle that)
        log.Print("Returning GameSessionData")
        jsonBytes, err := json.Marshal(GameSession)
        if err != nil {
                fmt.Println("Error:", err)
                // Set the response status code to 500 (Internal Server Error)
                w.WriteHeader(http.StatusInternalServerError)
                fmt.Fprintf(w, "error\n")
                return
        }
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintf(w, string(jsonBytes))
}

func main() {

        // Start the localhost server thread to provide information to the game server process on request
        HttpServer(8090)

        // Exit if no port is specified
        if len(os.Args) < 2 {
                fmt.Println("No port specified")
                os.Exit(1)
        }
        // Get the port on cli args
        log.Print("Getting the game server port")
        port, err := strconv.Atoi(os.Args[1])
        if err != nil {
                panic(err)
        }

        log.Print("Game server port is: ", port)

        log.Print("Starting GameLift wrapper")

        // Initialize the Amazon GameLift Server SDK
        err2 := server.InitSDK(server.ServerParameters{})
        if err2 != nil {
                log.Fatal(err2.Error())
        }

        // Make sure to call server.ProcessEnding() when the application quits.
        // This tells GameLift the session has ended
        defer server.ProcessEnding()

        process := gameProcess{
                Port: port,
                Logs: server.LogParameters{
                        // The log path is not actually used with container fleets...
                        LogPaths: []string{"/local/game/logs/myserver.log"},
                },
        }

        // Register our process to the Amazon GameLift service
        err = server.ProcessReady(server.ProcessParameters{
                OnStartGameSession:  process.OnStartGameSession,
                OnUpdateGameSession: process.OnUpdateGameSession,
                OnProcessTerminate:  process.OnProcessTerminate,
                OnHealthCheck:       process.OnHealthCheck,
                LogParameters:       process.Logs,
                Port:                process.Port,
        })
        if err != nil {
                log.Fatal(err.Error())
        }


       // Create channel for SIGINT
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, os.Interrupt, syscall.SIGINT)

        // Wait until we get the interruption signal
        for {
                select {
                case <-sigChan:
                        log.Print("Received SIGINT, shutting down...")
                        return
                default:
                        time.Sleep(50 * time.Millisecond)
                }
        }

}