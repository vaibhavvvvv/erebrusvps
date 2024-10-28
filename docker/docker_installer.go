package docker

import (
	"fmt"
	"os/exec"
	"strings"
)

// DockerSetup handles the installation and configuration of Docker
type DockerSetup struct {
	sudoPassword string
}

// NewDockerSetup creates a new DockerSetup instance
func NewDockerSetup(sudoPassword string) *DockerSetup {
	return &DockerSetup{
		sudoPassword: sudoPassword,
	}
}

// executeCommand runs a shell command with sudo if required
func (d *DockerSetup) executeCommand(command string, needsSudo bool) error {
	var cmd *exec.Cmd

	if needsSudo {
		cmd = exec.Command("sudo", "-S", "sh", "-c", command)
		cmd.Stdin = strings.NewReader(d.sudoPassword + "\n")
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %v\nOutput: %s", err, string(output))
	}
	fmt.Println(string(output))
	return nil
}

// Install performs the Docker installation and setup
func (d *DockerSetup) Install() error {
	steps := []struct {
		description string
		command     string
		needsSudo   bool
	}{
		{
			description: "Updating system",
			command:     "apt-get update && apt-get upgrade -y",
			needsSudo:   true,
		},
		{
			description: "Installing required packages",
			command:     "apt-get install -y apt-transport-https ca-certificates curl software-properties-common",
			needsSudo:   true,
		},
		{
			description: "Adding Docker's GPG key",
			command:     "curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg",
			needsSudo:   true,
		},
		{
			description: "Setting up Docker repository",
			command:     `echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null`,
			needsSudo:   true,
		},
		{
			description: "Installing Docker Engine",
			command:     "apt-get update && apt-get install -y docker-ce docker-ce-cli containerd.io",
			needsSudo:   true,
		},
		{
			description: "Setting up Docker group",
			command:     "groupadd docker || true && usermod -aG docker $USER",
			needsSudo:   true,
		},
		{
			description: "Enabling Docker service",
			command:     "systemctl enable docker",
			needsSudo:   true,
		},
		{
			description: "Installing Docker Compose",
			command:     `curl -L "https://github.com/docker/compose/releases/download/$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep -Po '"tag_name": "\K.*\d')" -o /usr/local/bin/docker-compose && chmod +x /usr/local/bin/docker-compose`,
			needsSudo:   true,
		},
		{
			description: "Verifying Docker installation",
			command:     "docker run hello-world",
			needsSudo:   false,
		},
	}

	for _, step := range steps {
		fmt.Printf("\nExecuting: %s\n", step.description)
		if err := d.executeCommand(step.command, step.needsSudo); err != nil {
			return fmt.Errorf("%s failed: %v", step.description, err)
		}
	}

	fmt.Println("\nDocker setup completed successfully!")
	return nil
}
