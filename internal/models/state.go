package models

import "time"

const (
	DeploymentQueued   = "QUEUED"
	DeploymentBuilding = "BUILDING"
	DeploymentSuccess  = "SUCCESS"
	DeploymentFailed   = "FAILED"
)

type Project struct {
	ID        string
	Name      string
	RepoURL   string
	Branch    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Deployment struct {
	ID           string
	ProjectID    string
	Status       string
	CommitHash   string
	LogsPath     string
	ImageRef     string
	Worktree     string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Container struct {
	ID                string
	DeploymentID      string
	DockerContainerID string
	InternalPort      int
	HostPort          int
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
