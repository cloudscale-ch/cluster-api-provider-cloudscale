//go:build e2e

/*
Copyright 2026 cloudscale.ch.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const sshUser = "capi"

// CloudscaleLogCollector collects logs from cloudscale VMs via SSH.
type CloudscaleLogCollector struct{}

func (c CloudscaleLogCollector) CollectMachineLog(ctx context.Context, _ client.Client, m *clusterv1.Machine, outputPath string) error {
	logger := log.FromContext(ctx).WithValues("machine", m.Name)

	ip := machineExternalIP(m)
	if ip == "" {
		return fmt.Errorf("no external IP found for machine %s", m.Name)
	}

	sshClient, err := sshDial(ip)
	if err != nil {
		return fmt.Errorf("SSH dial to %s (%s): %w", m.Name, ip, err)
	}
	defer func(sshClient *ssh.Client) {
		_ = sshClient.Close()
	}(sshClient)

	if err := os.MkdirAll(outputPath, 0750); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	commands := []struct {
		outputFile string
		command    string
	}{
		{"journal.log", "sudo journalctl --no-pager --output=short-precise"},
		{"kern.log", "sudo journalctl --no-pager --output=short-precise -k"},
		{"kubelet.log", "sudo journalctl --no-pager --output=short-precise -u kubelet.service"},
		{"containerd.log", "sudo journalctl --no-pager --output=short-precise -u containerd.service"},
		{"cloud-init.log", "sudo cat /var/log/cloud-init.log"},
		{"cloud-init-output.log", "sudo cat /var/log/cloud-init-output.log"},
		{"crictl-info.txt", "sudo crictl info"},
	}

	for _, cmd := range commands {
		if err := sshRunToFile(sshClient, cmd.command, filepath.Join(outputPath, cmd.outputFile)); err != nil {
			logger.V(1).Info("Failed to collect log", "file", cmd.outputFile, "error", err)
		}
	}

	// Collect /var/log/pods as a tarball and extract locally
	if err := sshCollectPods(ctx, sshClient, filepath.Join(outputPath, "pods")); err != nil {
		logger.V(1).Info("Failed to collect pod logs", "error", err)
	}

	return nil
}

func (c CloudscaleLogCollector) CollectMachinePoolLog(_ context.Context, _ client.Client, _ *clusterv1.MachinePool, _ string) error {
	return nil
}

func (c CloudscaleLogCollector) CollectInfrastructureLogs(_ context.Context, _ client.Client, _ *clusterv1.Cluster, _ string) error {
	return nil
}

// machineExternalIP returns the first ExternalIP address from the Machine status.
func machineExternalIP(m *clusterv1.Machine) string {
	for _, addr := range m.Status.Addresses {
		if addr.Type == clusterv1.MachineExternalIP {
			return addr.Address
		}
	}
	return ""
}

// sshDial connects to a host via SSH using the ssh-agent.
func sshDial(host string) (*ssh.Client, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set; ssh-agent is required for log collection")
	}

	conn, err := net.Dial("unix", sock) //nolint:gosec,noctx // G704 does not apply, unix filesystem. This is test code
	if err != nil {
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	}

	agentClient := agent.NewClient(conn)
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // E2E test machines are ephemeral.
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(host, "22"), config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

// sshRunToFile runs a command over SSH and writes stdout to a local file.
func sshRunToFile(client *ssh.Client, command, outputFile string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func(session *ssh.Session) {
		_ = session.Close()
	}(session)

	output, err := session.CombinedOutput(command)
	if err != nil {
		// Still write partial output if available
		if len(output) > 0 {
			_ = writeFile(outputFile, output)
		}
		return err
	}

	return writeFile(outputFile, output)
}

// sshCollectPods tars /var/log/pods on the remote and extracts it locally.
func sshCollectPods(ctx context.Context, client *ssh.Client, outputDir string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func(session *ssh.Session) {
		_ = session.Close()
	}(session)

	tarData, err := session.CombinedOutput("sudo tar -cf - -C /var/log/pods . 2>/dev/null")
	if err != nil {
		return fmt.Errorf("tar pods: %w", err)
	}

	if len(tarData) == 0 {
		return nil
	}

	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return err
	}

	// Write tar to temp file and extract
	tmpFile, err := os.CreateTemp("", "pods-*.tar")
	if err != nil {
		return err
	}
	defer func(name string) {
		_ = os.Remove(name)
	}(tmpFile.Name())

	if _, err := tmpFile.Write(tarData); err != nil {
		_ = tmpFile.Close()
		return err
	}
	_ = tmpFile.Close()

	// Use local tar to extract
	cmd := fmt.Sprintf("tar -xf %s -C %s", tmpFile.Name(), outputDir)
	return runLocalCommand(ctx, cmd)
}

// writeFile writes data to a file, creating parent directories as needed.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// runLocalCommand runs a shell command on the local machine.
func runLocalCommand(ctx context.Context, command string) error {
	return exec.CommandContext(ctx, "sh", "-c", command).Run() //nolint:gosec // E2E test helper with controlled input.
}
