package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	// "encoding/json"
)

type Deployment struct {
	GitURL      string            `json:"git_url"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	Port        string            `json:"port"`
	ProjectName string            `json:"project_name"`
}

type DeploymentResult struct {
	Status string `json:"status"`
	URL    string `json:"url"`
	Port   string `json:"port"`
	Error  string `json:"error,omitempty"`
}

func (d *DockerSetup) DeployProject(deployment Deployment) (*DeploymentResult, error) {
	// Create workspace directory
	workDir := filepath.Join("/opt/deployments", deployment.ProjectName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %v", err)
	}

	// Clone repository
	if err := d.cloneRepository(deployment.GitURL, workDir); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %v", err)
	}

	// Create Dockerfile if it doesn't exist
	if err := d.ensureDockerfile(workDir); err != nil {
		return nil, fmt.Errorf("failed to create Dockerfile: %v", err)
	}

	// Create docker-compose.yml
	if err := d.createDockerCompose(workDir, deployment); err != nil {
		return nil, fmt.Errorf("failed to create docker-compose.yml: %v", err)
	}

	// Build and run the container
	if err := d.buildAndRun(workDir, deployment); err != nil {
		return nil, fmt.Errorf("failed to build and run: %v", err)
	}

	// Configure Nginx reverse proxy
	if err := d.configureNginx(deployment); err != nil {
		return nil, fmt.Errorf("failed to configure nginx: %v", err)
	}

	return &DeploymentResult{
		Status: "success",
		URL:    fmt.Sprintf("http://%s.localhost", deployment.ProjectName),
		Port:   deployment.Port,
	}, nil
}

func (d *DockerSetup) cloneRepository(gitURL, workDir string) error {
	cmd := exec.Command("git", "clone", gitURL, workDir)
	return cmd.Run()
}

func (d *DockerSetup) ensureDockerfile(workDir string) error {
	dockerfilePath := filepath.Join(workDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		// Create a default Dockerfile for Go applications
		dockerfile := `FROM golang:1.21-alpine
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o main .
EXPOSE 8080
CMD ["./main"]`
		return os.WriteFile(dockerfilePath, []byte(dockerfile), 0644)
	}
	return nil
}

func (d *DockerSetup) createDockerCompose(workDir string, deployment Deployment) error {
	template := `version: '3'
services:
  %s:
    build: .
    ports:
      - "%s:%s"
    environment:
%s
    restart: always
    networks:
      - deployment-network

networks:
  deployment-network:
    external: true`

	// Format environment variables
	var envVars strings.Builder
	for k, v := range deployment.EnvVars {
		envVars.WriteString(fmt.Sprintf("      - %s=%s\n", k, v))
	}

	compose := fmt.Sprintf(template,
		deployment.ProjectName,
		deployment.Port,
		"8080", // assuming the app listens on 8080 internally
		envVars.String(),
	)

	return os.WriteFile(filepath.Join(workDir, "docker-compose.yml"), []byte(compose), 0644)
}

func (d *DockerSetup) buildAndRun(workDir string, deployment Deployment) error {
	// Create network if it doesn't exist
	networkCmd := exec.Command("docker", "network", "create", "deployment-network")
	networkCmd.Run() // Ignore error if network already exists

	// Build and run using docker-compose
	cmd := exec.Command("docker-compose", "up", "--build", "-d")
	cmd.Dir = workDir
	return cmd.Run()
}

func (d *DockerSetup) configureNginx(deployment Deployment) error {
	configTemplate := `
server {
    listen 80;
    server_name %s.localhost;

    location / {
        proxy_pass http://localhost:%s;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}`

	config := fmt.Sprintf(configTemplate, deployment.ProjectName, deployment.Port)
	configPath := fmt.Sprintf("/etc/nginx/sites-available/%s", deployment.ProjectName)

	// Write config file
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return err
	}

	// Create symlink
	symlink := fmt.Sprintf("/etc/nginx/sites-enabled/%s", deployment.ProjectName)
	if err := os.Symlink(configPath, symlink); err != nil && !os.IsExist(err) {
		return err
	}

	// Reload Nginx
	return exec.Command("sudo", "systemctl", "reload", "nginx").Run()
}
