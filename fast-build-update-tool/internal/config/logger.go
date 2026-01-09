package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/aws/smithy-go/logging"
	"github.com/pterm/pterm"
)

const logFileName = AppName + ".log"

type ApplicationLogger struct {
	Logger      *slog.Logger
	AwsLogger   logging.Logger
	logsDir     string
	prevLogsDir string
	logFile     *os.File
}

// InitializeLogger will initialize the app logger. Use the verbose flag to configure the level of logging that will be used.
func InitializeLogger(verbose bool) (*ApplicationLogger, error) {
	result := &ApplicationLogger{
		AwsLogger:   &awsLogger{},
		logsDir:     logsDir(),
		prevLogsDir: logsDir() + "-prev",
	}

	if err := result.initializeLogDirectories(); err != nil {
		return result, err
	}

	if err := result.initializeSlog(verbose); err != nil {
		return result, err
	}

	return result, nil
}

func (a *ApplicationLogger) initializeLogDirectories() error {
	// Remove an existing prev log dir if we have one
	err := os.RemoveAll(a.prevLogsDir)
	if err != nil {
		return fmt.Errorf("error removing old log directory %w", err)
	}

	// Move existing log dir to prev
	if doesFileExist(a.logsDir) {
		err = os.Rename(a.logsDir, a.prevLogsDir)
		if err != nil {
			return fmt.Errorf("error renaming previous log directory %w", err)
		}
	}

	// Create a folder for storing our logs
	err = os.MkdirAll(a.logsDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("error making new log directory %w", err)
	}

	return nil
}

func (a *ApplicationLogger) initializeSlog(verbose bool) (err error) {
	var handler slog.Handler

	if verbose {
		if a.logFile == nil {
			// If we have verbose logs, use the default slog.TextHandler, and write everything to STDOUT, and a log file
			a.logFile, err = os.OpenFile(filepath.Join(a.logsDir, logFileName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("error creating log file %w", err)
			}
		}

		level := &slog.LevelVar{}
		level.Set(slog.LevelDebug)
		handler = slog.NewTextHandler(io.MultiWriter(a.logFile, os.Stdout), &slog.HandlerOptions{Level: level})
	} else {
		// Otherwise use the pretty logger at Warn level, and only log to STDOUT
		pterm.DefaultLogger.Level = pterm.LogLevelWarn
		handler = pterm.NewSlogHandler(&pterm.DefaultLogger)
	}

	a.Logger = slog.New(handler)
	return nil
}

func (a *ApplicationLogger) Close() {
	if a.logFile != nil {
		err := a.logFile.Close()
		if err != nil {
			fmt.Println("error closing log file", err)
		}
	}
}

func logsDir() string {
	return filepath.Join(".", AppName+"-logs")
}

// GetLogPathForFile will return the proper path where application logs can be written
func GetLogPathForFile(fileName string) string {
	return filepath.Join(logsDir(), fileName)
}

// awsLogger implements the logger interface required by the AWS SDK, and writes logs to the standard slog logger
type awsLogger struct{}

func (a *awsLogger) Logf(classification logging.Classification, format string, v ...interface{}) {
	switch classification {
	case logging.Warn:
		slog.Warn("(AWSSDK) " + fmt.Sprintf(format, v...))

	default:
		slog.Debug("(AWSSDK) " + fmt.Sprintf(format, v...))
	}
}

type errorLogger struct {
	context string
}

func (e *errorLogger) Write(p []byte) (n int, err error) {
	slog.Error(string(p), "context", e.context)
	return n, nil
}

// NewErrorLogger returns an io.Writer that can be used to handle logs that would normally be written to STDERR
func NewErrorLogger(context string) io.Writer {
	return &errorLogger{context: context}
}
