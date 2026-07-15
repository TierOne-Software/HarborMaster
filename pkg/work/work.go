package work

import (
	"fmt"
	"os"
	"strings"
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
	md, err := toml.DecodeFile(path, ws)
	if err != nil {
		return nil, fmt.Errorf("failed to parse work session file: %w", err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("unknown key(s) in work session file %s: %s", path, strings.Join(keys, ", "))
	}

	// An empty or truncated file decodes without error but leaves Version at
	// zero; treat that (and unknown future versions) as an error instead of
	// silently loading an empty session.
	if ws.Version < 1 {
		return nil, fmt.Errorf("work session file %s has invalid version %d (file may be corrupt or truncated)", path, ws.Version)
	}
	if ws.Version > CurrentVersion {
		return nil, fmt.Errorf("work session file %s has version %d, but this version of harbormaster only supports up to version %d", path, ws.Version, CurrentVersion)
	}

	ws.path = path
	return ws, nil
}

// Save writes the work session to disk. The file is written atomically
// (temp file + rename), so an existing session file is never truncated by a
// failed save.
func (ws *WorkSession) Save(path string) error {
	err := writeFileAtomic(path, func(f *os.File) error {
		// Write header comment
		header := "# Harbormaster Work Session\n" +
			"# DO NOT EDIT - This file is auto-generated\n" +
			"# Use 'hm work end' to finish the current session\n\n"
		if _, err := f.WriteString(header); err != nil {
			return err
		}
		if err := toml.NewEncoder(f).Encode(ws); err != nil {
			return fmt.Errorf("failed to encode work session: %w", err)
		}
		return nil
	})
	if err != nil {
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
