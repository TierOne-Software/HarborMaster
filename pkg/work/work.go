package work

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	// WorkFileName is the name of the work session file.
	WorkFileName = ".harbormaster.work"

	// CurrentVersion is the current work file format version.
	CurrentVersion = 1
)

// WorkSession tracks an active coordinated work session across repositories.
type WorkSession struct {
	Version   int        `toml:"version"`
	Name      string     `toml:"name"`
	Branch    string     `toml:"branch"`
	CreatedAt time.Time  `toml:"created_at"`
	Repos     []WorkRepo `toml:"repo"`
	path      string
}

// WorkRepo tracks a repository enrolled in the work session.
type WorkRepo struct {
	Name           string    `toml:"name"`
	OriginalBranch string    `toml:"original_branch"`
	OriginalSHA    string    `toml:"original_sha"`
	AddedAt        time.Time `toml:"added_at"`
}

// New creates a new work session.
func New(name, branch string) *WorkSession {
	return &WorkSession{
		Version:   CurrentVersion,
		Name:      name,
		Branch:    branch,
		CreatedAt: time.Now(),
		Repos:     []WorkRepo{},
	}
}

// FileExists returns true if a work session file exists at the given path.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Load reads a work session from disk. Returns nil, nil if the file does not exist.
func Load(path string) (*WorkSession, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	ws := &WorkSession{}
	if _, err := toml.DecodeFile(path, ws); err != nil {
		return nil, fmt.Errorf("failed to parse work session file: %w", err)
	}

	ws.path = path
	return ws, nil
}

// Save writes the work session to disk.
func (ws *WorkSession) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create work session file: %w", err)
	}

	// Write header comment
	if _, err := f.WriteString("# Harbormaster Work Session\n"); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.WriteString("# DO NOT EDIT - This file is auto-generated\n"); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.WriteString("# Use 'hm work end' to finish the current session\n\n"); err != nil {
		_ = f.Close()
		return err
	}

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(ws); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to encode work session: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write work session file: %w", err)
	}

	ws.path = path
	return nil
}

// Delete removes the work session file from disk.
func (ws *WorkSession) Delete() error {
	if ws.path == "" {
		return nil
	}
	if _, err := os.Stat(ws.path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(ws.path)
}

// Path returns the file path of the work session.
func (ws *WorkSession) Path() string {
	return ws.path
}

// AddRepo adds a repository to the work session.
func (ws *WorkSession) AddRepo(repo WorkRepo) error {
	if ws.HasRepo(repo.Name) {
		return fmt.Errorf("repository already in work session: %s", repo.Name)
	}
	ws.Repos = append(ws.Repos, repo)
	return nil
}

// RemoveRepo removes a repository from the work session.
func (ws *WorkSession) RemoveRepo(name string) error {
	for i, r := range ws.Repos {
		if r.Name == name {
			ws.Repos = append(ws.Repos[:i], ws.Repos[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("repository not in work session: %s", name)
}

// GetRepo returns a repository from the work session.
func (ws *WorkSession) GetRepo(name string) (*WorkRepo, bool) {
	for i := range ws.Repos {
		if ws.Repos[i].Name == name {
			return &ws.Repos[i], true
		}
	}
	return nil, false
}

// HasRepo returns true if the repository is in the work session.
func (ws *WorkSession) HasRepo(name string) bool {
	_, ok := ws.GetRepo(name)
	return ok
}

// RepoNames returns the names of all repositories in the work session.
func (ws *WorkSession) RepoNames() []string {
	names := make([]string, len(ws.Repos))
	for i, r := range ws.Repos {
		names[i] = r.Name
	}
	return names
}
