package docker

import (
	"fmt"
	"os/exec"
)

// DockerSetup handles the installation and configuration of Docker
type DockerSetup struct{}

// NewDockerSetup creates a new DockerSetup instance
func NewDockerSetup() *DockerSetup {
	return &DockerSetup{}
}

// ExecuteCommand runs a shell command and logs output in real-time
func (d *DockerSetup) ExecuteCommand(command string) error {
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
	steps := []struct {
		description string
		command     string
	}{
		{
			description: "Updating system",
			command:     "sudo apt-get update && sudo apt-get upgrade -y",
		},
		{
			description: "Installing required packages",
			command:     "sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common",
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
			description: "Installing Docker Engine",
			command:     "sudo apt-get update && sudo apt-get install -y docker-ce docker-ce-cli containerd.io",
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
			command:     `sudo curl -L "https://github.com/docker/compose/releases/download/$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep -Po '"tag_name": "\K.*\d')" -o /usr/local/bin/docker-compose && sudo chmod +x /usr/local/bin/docker-compose`,
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

	for _, step := range steps {
		fmt.Printf("\nExecuting: %s\n", step.description)
		if err := d.ExecuteCommand(step.command); err != nil {
			return fmt.Errorf("%s failed: %v", step.description, err)
		}
	}

	fmt.Println("\nDocker setup completed successfully!")
	return nil
}
