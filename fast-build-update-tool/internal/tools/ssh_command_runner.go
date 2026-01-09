package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"github.com/aws/amazon-gamelift-toolkit/fast-build-update-tool/internal/config"
	"github.com/aws/amazon-gamelift-toolkit/fast-build-update-tool/internal/gamelift"
	"golang.org/x/crypto/ssh"
)

// SSHCommandRunner is used to run a shell script on a remote instance over SSH
type SSHCommandRunner struct {
	logger              *slog.Logger
	sshPort             int32
	instanceIpAddress   string
	instanceId          string
	sshKey              ssh.Signer
	updateScriptCommand string
	remoteUserName      string
}

// NewSSHCommandRunner build a new SSHCommandRunner for the provided script, and instance
func NewSSHCommandRunner(logger *slog.Logger, localUpdateScriptPath string, sshPort int32, sshKey ssh.Signer, instance *gamelift.Instance) (*SSHCommandRunner, error) {
	updateScriptCommand, err := generateUpdateScriptCommand(localUpdateScriptPath, instance)
	if err != nil {
		return nil, err
	}

	return &SSHCommandRunner{
		logger:              logger.With("context", "SSHCommandRunner"),
		sshPort:             sshPort,
		instanceIpAddress:   instance.IpAddress,
		instanceId:          instance.InstanceId,
		sshKey:              sshKey,
		updateScriptCommand: updateScriptCommand,
		remoteUserName:      string(config.RemoteUserForOperatingSystem(instance.OperatingSystem)),
	}, nil
}

// Run will open an SSH connection to the remote instance, and run a script command on it
func (s *SSHCommandRunner) Run(ctx context.Context, remotePublicKey ssh.PublicKey) error {
	// Set up the SSH connection to the remote instance
	client, err := ssh.Dial("tcp", net.JoinHostPort(s.instanceIpAddress, fmt.Sprintf("%d", s.sshPort)), &ssh.ClientConfig{
		User:              s.remoteUserName,
		HostKeyCallback:   ssh.FixedHostKey(remotePublicKey),
		HostKeyAlgorithms: []string{remotePublicKey.Type()},
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(s.sshKey),
		},
	})
	if err != nil {
		return fmt.Errorf("error dialing ssh connection: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("error starting ssh session: %w", err)
	}
	defer session.Close()

	logFilePath := config.GetLogPathForFile(fmt.Sprintf("%s-ssh-command.log", s.instanceId))
	// Set up a log file so we log out any remote output we get from the instance
	logFile, err := os.OpenFile(logFilePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("error creating log file for ssh command runner: %w", err)
	}
	defer logFile.Close()

	session.Stdout = logFile
	session.Stderr = config.NewErrorLogger("SSHCommandRunner")

	slog.Debug("running command on instance", "command", s.updateScriptCommand)

	// Run the actual update command on the instance
	err = session.Run(s.updateScriptCommand)
	if err != nil {
		return fmt.Errorf("error running server update script: %w; Check logs in %s for more information", err, logFilePath)
	}

	return nil
}

// generateUpdateScriptCommand will generate the remote command used to run the update script we have generated for a specific instance
func generateUpdateScriptCommand(localUpdateScriptPath string, instance *gamelift.Instance) (string, error) {
	remoteUploadDirectory := string(config.RemoteUploadDirectoryForOperatingSystem(instance.OperatingSystem))

	remoteUpdateScript := remoteUploadDirectory + filepath.Base(localUpdateScriptPath)

	switch instance.OperatingSystem {
	case config.OperatingSystemWindows:
		return fmt.Sprintf("powershell.exe -ExecutionPolicy Bypass -File %s", remoteUpdateScript), nil

	case config.OperatingSystemLinux:
		return fmt.Sprintf("chmod +x %s && %s", remoteUpdateScript, remoteUpdateScript), nil

	default:
		return "", config.UnknownOperatingSystemError(fmt.Sprint(instance.OperatingSystem))
	}
}
