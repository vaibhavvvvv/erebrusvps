package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DockerSetup handles the installation and configuration of Docker
type DockerSetup struct{}

// NewDockerSetup creates a new DockerSetup instance
func NewDockerSetup() *DockerSetup {
	return &DockerSetup{}
}

// ExecuteCommand runs a shell command and logs output in real-time
func (d *DockerSetup) ExecuteCommand(command string) error {
	// Modify commands that need automatic yes responses
	if strings.Contains(command, "apt-get") {
		command = strings.Replace(command, "apt-get", "DEBIAN_FRONTEND=noninteractive apt-get -y", 1)
	}

	fmt.Printf("\n[COMMAND] Executing: %s\n", command)

	cmd := exec.Command("sh", "-c", command)

	// Set up pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	// Create a channel to signal when we're done reading output
	done := make(chan bool)

	// Read stdout in a goroutine
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := stdout.Read(buffer)
			if n > 0 {
				fmt.Printf("[STDOUT] %s", buffer[:n])
			}
			if err != nil {
				break
			}
		}
		done <- true
	}()

	// Read stderr in a goroutine
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := stderr.Read(buffer)
			if n > 0 {
				fmt.Printf("[STDERR] %s", buffer[:n])
			}
			if err != nil {
				break
			}
		}
		done <- true
	}()

	// Wait for both stdout and stderr to be fully read
	<-done
	<-done

	// Wait for the command to complete
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command failed: %v", err)
	}

	fmt.Printf("[COMMAND] Completed successfully\n")
	return nil
}

// Install performs the Docker installation and setup
func (d *DockerSetup) Install() error {
	// First, handle kernel updates and potential reboot
	kernelSteps := []struct {
		description string
		command     string
	}{
		{
			description: "Updating package list",
			command:     "DEBIAN_FRONTEND=noninteractive apt-get update",
		},
		{
			description: "Upgrading all packages",
			command:     "DEBIAN_FRONTEND=noninteractive apt-get upgrade -y",
		},
		{
			description: "Performing distribution upgrade (including kernel)",
			command:     "DEBIAN_FRONTEND=noninteractive apt-get dist-upgrade -y",
		},
	}

	// Execute kernel updates
	for _, step := range kernelSteps {
		fmt.Printf("\nExecuting: %s\n", step.description)
		if err := d.ExecuteCommand(step.command); err != nil {
			return fmt.Errorf("%s failed: %v", step.description, err)
		}
	}

	// Check if reboot is needed
	if _, err := os.Stat("/var/run/reboot-required"); err == nil {
		fmt.Println("\n[SYSTEM] Kernel update detected, system requires reboot")
		fmt.Println("[SYSTEM] Scheduling reboot in 1 minute...")

		if err := d.ExecuteCommand("sudo shutdown -r +1"); err != nil {
			return fmt.Errorf("failed to schedule reboot: %v", err)
		}

		fmt.Println("[SYSTEM] Reboot scheduled. Please wait for the system to restart and run this installer again.")
		fmt.Println("[SYSTEM] The program will now exit. Please reconnect after ~2 minutes and run again.")
		os.Exit(0)
	}

	// If no reboot needed or after reboot, proceed with Docker installation
	dockerSteps := []struct {
		description string
		command     string
	}{
		{
			description: "Installing required packages",
			command:     "DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https ca-certificates curl software-properties-common",
		},
		{
			description: "Adding Docker's GPG key",
			command:     "curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg",
		},
		{
			description: "Setting up Docker repository",
			command:     `echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null`,
		},
		{
			description: "Updating package list with Docker repository",
			command:     "DEBIAN_FRONTEND=noninteractive apt-get update",
		},
		{
			description: "Installing Docker Engine",
			command:     "DEBIAN_FRONTEND=noninteractive apt-get install -y docker-ce docker-ce-cli containerd.io",
		},
		{
			description: "Setting up Docker group",
			command:     "sudo groupadd docker || true && sudo usermod -aG docker $USER",
		},
		{
			description: "Enabling Docker service",
			command:     "sudo systemctl enable docker && sudo systemctl start docker",
		},
		{
			description: "Installing Docker Compose",
			command:     `sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose && sudo chmod +x /usr/local/bin/docker-compose`,
		},
		{
			description: "Setting correct permissions for Docker socket",
			command:     "sudo chmod 666 /var/run/docker.sock",
		},
		{
			description: "Verifying Docker installation",
			command:     "docker run hello-world",
		},
	}

	// Execute Docker installation steps
	for _, step := range dockerSteps {
		fmt.Printf("\nExecuting: %s\n", step.description)
		if err := d.ExecuteCommand(step.command); err != nil {
			return fmt.Errorf("%s failed: %v", step.description, err)
		}
	}

	fmt.Println("\nDocker setup completed successfully!")
	return nil
}
