package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	// ConfigFileName is the name of the configuration file.
	ConfigFileName = ".harbormaster.toml"

	// DefaultTimeout is the default operation timeout.
	DefaultTimeout = 10 * time.Minute

	// DefaultBranch is the default git branch.
	DefaultBranch = "main"

	// DefaultCloneDepth is the default shallow clone depth.
	DefaultCloneDepth = 1

	// DefaultRetryAttempts is the default number of HTTP retry attempts.
	DefaultRetryAttempts = 3

	// DefaultRetryDelay is the default delay between retries.
	DefaultRetryDelay = 2 * time.Second
)

// Config represents the parsed and validated configuration.
type Config struct {
	General      GeneralConfig
	HTTP         HTTPConfig
	Git          GitConfig
	Repositories []Repository
	Projects     []Project
	configPath   string // Path to the config file
}

// GeneralConfig holds general settings.
type GeneralConfig struct {
	WorkDir          string
	CacheDir         string
	Timeout          time.Duration
	DefaultBranch    string
	RecurseSubmodule bool
}

// HTTPConfig holds HTTP-specific settings.
type HTTPConfig struct {
	UserAgent     string
	RetryAttempts int
	RetryDelay    time.Duration
}

// GitConfig holds Git-specific settings.
type GitConfig struct {
	ShallowClone bool
	CloneDepth   int
}

// ConfigFile represents the raw TOML structure for file I/O.
type ConfigFile struct {
	General      GeneralConfigFile `toml:"general"`
	HTTP         HTTPConfigFile    `toml:"http"`
	Git          GitConfigFile     `toml:"git"`
	Repositories []RepositoryFile  `toml:"repository"`
	Projects     []ProjectFile     `toml:"project"`
}

// GeneralConfigFile is the raw TOML structure for general settings.
type GeneralConfigFile struct {
	WorkDir          string `toml:"work_dir"`
	CacheDir         string `toml:"cache_dir"`
	Timeout          string `toml:"timeout"`
	DefaultBranch    string `toml:"default_branch"`
	RecurseSubmodule *bool  `toml:"recurse_submodule"`
}

// HTTPConfigFile is the raw TOML structure for HTTP settings.
type HTTPConfigFile struct {
	UserAgent     string `toml:"user_agent"`
	RetryAttempts *int   `toml:"retry_attempts"`
	RetryDelay    string `toml:"retry_delay"`
}

// GitConfigFile is the raw TOML structure for Git settings.
type GitConfigFile struct {
	ShallowClone *bool `toml:"shallow_clone"`
	CloneDepth   *int  `toml:"clone_depth"`
}

// Load reads and parses the configuration file.
func Load(path string) (*Config, error) {
	var cf ConfigFile
	if _, err := toml.DecodeFile(path, &cf); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg, err := parseConfigFile(&cf, path)
	if err != nil {
		return nil, err
	}
	cfg.configPath = path

	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// FindConfigFile searches for the configuration file in the workspace root.
func FindConfigFile() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(cwd, ConfigFileName)
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("config file not found: %s", configPath)
		}
		return "", err
	}

	return configPath, nil
}

// Save writes the configuration to the config file.
func (c *Config) Save() error {
	if c.configPath == "" {
		return fmt.Errorf("config path not set")
	}
	return c.SaveTo(c.configPath)
}

// SaveTo writes the configuration to the specified path.
func (c *Config) SaveTo(path string) error {
	cf := toConfigFile(c)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cf); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to encode config: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	c.configPath = path
	return nil
}

// Path returns the path to the config file.
func (c *Config) Path() string {
	return c.configPath
}

// GetRepository returns a repository by name.
func (c *Config) GetRepository(name string) (*Repository, bool) {
	for i := range c.Repositories {
		if c.Repositories[i].Name == name {
			return &c.Repositories[i], true
		}
	}
	return nil, false
}

// GetProject returns a project by name.
func (c *Config) GetProject(name string) (*Project, bool) {
	for i := range c.Projects {
		if c.Projects[i].Name == name {
			return &c.Projects[i], true
		}
	}
	return nil, false
}

// AddRepository adds a repository to the configuration.
func (c *Config) AddRepository(repo Repository) error {
	if _, exists := c.GetRepository(repo.Name); exists {
		return fmt.Errorf("repository already exists: %s", repo.Name)
	}
	c.Repositories = append(c.Repositories, repo)
	return nil
}

// RemoveRepository removes a repository from the configuration.
func (c *Config) RemoveRepository(name string) error {
	for i, repo := range c.Repositories {
		if repo.Name == name {
			c.Repositories = append(c.Repositories[:i], c.Repositories[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("repository not found: %s", name)
}

// GetRepositoriesForProject returns all repositories in a project.
func (c *Config) GetRepositoriesForProject(projectName string) ([]Repository, error) {
	project, ok := c.GetProject(projectName)
	if !ok {
		return nil, fmt.Errorf("project not found: %s", projectName)
	}

	var repos []Repository
	for _, repoName := range project.Repositories {
		repo, ok := c.GetRepository(repoName)
		if !ok {
			return nil, fmt.Errorf("repository %s not found in project %s", repoName, projectName)
		}
		repos = append(repos, *repo)
	}
	return repos, nil
}

// GetRepositoriesByTag returns all repositories with the specified tag.
func (c *Config) GetRepositoriesByTag(tag string) []Repository {
	var repos []Repository
	for _, repo := range c.Repositories {
		for _, t := range repo.Tags {
			if t == tag {
				repos = append(repos, repo)
				break
			}
		}
	}
	return repos
}

// AddProject adds a new project to the configuration.
func (c *Config) AddProject(proj Project) error {
	if _, exists := c.GetProject(proj.Name); exists {
		return fmt.Errorf("project already exists: %s", proj.Name)
	}
	c.Projects = append(c.Projects, proj)
	return nil
}

// RemoveProject removes a project from the configuration.
func (c *Config) RemoveProject(name string) error {
	for i, proj := range c.Projects {
		if proj.Name == name {
			c.Projects = append(c.Projects[:i], c.Projects[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("project not found: %s", name)
}

// AddRepoToProject adds a repository to an existing project.
func (c *Config) AddRepoToProject(projectName, repoName string) error {
	// Verify repository exists
	if _, ok := c.GetRepository(repoName); !ok {
		return fmt.Errorf("repository not found: %s", repoName)
	}

	// Find project and add repo
	for i := range c.Projects {
		if c.Projects[i].Name == projectName {
			// Check if repo already in project
			if c.Projects[i].HasRepository(repoName) {
				return fmt.Errorf("repository %s already in project %s", repoName, projectName)
			}
			c.Projects[i].Repositories = append(c.Projects[i].Repositories, repoName)
			return nil
		}
	}
	return fmt.Errorf("project not found: %s", projectName)
}

// RemoveRepoFromProject removes a repository from a project.
func (c *Config) RemoveRepoFromProject(projectName, repoName string) error {
	for i := range c.Projects {
		if c.Projects[i].Name == projectName {
			for j, r := range c.Projects[i].Repositories {
				if r == repoName {
					c.Projects[i].Repositories = append(
						c.Projects[i].Repositories[:j],
						c.Projects[i].Repositories[j+1:]...,
					)
					return nil
				}
			}
			return fmt.Errorf("repository %s not in project %s", repoName, projectName)
		}
	}
	return fmt.Errorf("project not found: %s", projectName)
}

func parseConfigFile(cf *ConfigFile, configPath string) (*Config, error) {
	cfg := &Config{}

	// Get the directory containing the config file for resolving relative paths
	configDir := filepath.Dir(configPath)
	if !filepath.IsAbs(configDir) {
		absConfigDir, err := filepath.Abs(configDir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve config directory: %w", err)
		}
		configDir = absConfigDir
	}

	// Parse general config
	if cf.General.WorkDir != "" {
		workDir, err := ExpandPath(cf.General.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("failed to expand work_dir: %w", err)
		}
		// If work_dir is relative, resolve it against the config file's directory
		if !filepath.IsAbs(workDir) {
			workDir = filepath.Join(configDir, workDir)
		}
		cfg.General.WorkDir = filepath.Clean(workDir)
	} else {
		// Default to the config file's directory
		cfg.General.WorkDir = configDir
	}

	if cf.General.CacheDir != "" {
		cacheDir, err := ExpandPath(cf.General.CacheDir)
		if err != nil {
			return nil, fmt.Errorf("failed to expand cache_dir: %w", err)
		}
		// If cache_dir is relative, resolve it against the config file's directory
		if !filepath.IsAbs(cacheDir) {
			cacheDir = filepath.Join(configDir, cacheDir)
		}
		cfg.General.CacheDir = filepath.Clean(cacheDir)
	}

	if cf.General.Timeout != "" {
		timeout, err := time.ParseDuration(cf.General.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timeout: %w", err)
		}
		cfg.General.Timeout = timeout
	} else {
		cfg.General.Timeout = DefaultTimeout
	}

	if cf.General.DefaultBranch != "" {
		cfg.General.DefaultBranch = cf.General.DefaultBranch
	} else {
		cfg.General.DefaultBranch = DefaultBranch
	}

	if cf.General.RecurseSubmodule != nil {
		cfg.General.RecurseSubmodule = *cf.General.RecurseSubmodule
	} else {
		cfg.General.RecurseSubmodule = true
	}

	// Parse HTTP config
	if cf.HTTP.UserAgent != "" {
		cfg.HTTP.UserAgent = cf.HTTP.UserAgent
	} else {
		cfg.HTTP.UserAgent = "Harbormaster/1.0"
	}

	if cf.HTTP.RetryAttempts != nil {
		cfg.HTTP.RetryAttempts = *cf.HTTP.RetryAttempts
	} else {
		cfg.HTTP.RetryAttempts = DefaultRetryAttempts
	}

	if cf.HTTP.RetryDelay != "" {
		delay, err := time.ParseDuration(cf.HTTP.RetryDelay)
		if err != nil {
			return nil, fmt.Errorf("failed to parse retry_delay: %w", err)
		}
		cfg.HTTP.RetryDelay = delay
	} else {
		cfg.HTTP.RetryDelay = DefaultRetryDelay
	}

	// Parse Git config
	if cf.Git.ShallowClone != nil {
		cfg.Git.ShallowClone = *cf.Git.ShallowClone
	} else {
		cfg.Git.ShallowClone = true
	}

	if cf.Git.CloneDepth != nil {
		cfg.Git.CloneDepth = *cf.Git.CloneDepth
	} else {
		cfg.Git.CloneDepth = DefaultCloneDepth
	}

	// Parse repositories
	for _, rf := range cf.Repositories {
		repo := Repository{
			Name:       rf.Name,
			URL:        rf.URL,
			Type:       RepositoryType(rf.Type),
			Path:       rf.Path,
			Branch:     rf.Branch,
			Tag:        rf.Tag,
			Commit:     rf.Commit,
			Shallow:    rf.Shallow,
			Depth:      rf.Depth,
			Submodules: rf.Submodules,
			Tags:       rf.Tags,
		}
		cfg.Repositories = append(cfg.Repositories, repo)
	}

	// Parse projects
	for _, pf := range cf.Projects {
		cfg.Projects = append(cfg.Projects, Project(pf))
	}

	return cfg, nil
}

func toConfigFile(c *Config) *ConfigFile {
	cf := &ConfigFile{}

	// General config
	cf.General.WorkDir = c.General.WorkDir
	cf.General.CacheDir = c.General.CacheDir
	cf.General.Timeout = c.General.Timeout.String()
	cf.General.DefaultBranch = c.General.DefaultBranch
	cf.General.RecurseSubmodule = &c.General.RecurseSubmodule

	// HTTP config
	cf.HTTP.UserAgent = c.HTTP.UserAgent
	cf.HTTP.RetryAttempts = &c.HTTP.RetryAttempts
	cf.HTTP.RetryDelay = c.HTTP.RetryDelay.String()

	// Git config
	cf.Git.ShallowClone = &c.Git.ShallowClone
	cf.Git.CloneDepth = &c.Git.CloneDepth

	// Repositories
	for _, repo := range c.Repositories {
		rf := RepositoryFile{
			Name:       repo.Name,
			URL:        repo.URL,
			Type:       string(repo.Type),
			Path:       repo.Path,
			Branch:     repo.Branch,
			Tag:        repo.Tag,
			Commit:     repo.Commit,
			Shallow:    repo.Shallow,
			Depth:      repo.Depth,
			Submodules: repo.Submodules,
			Tags:       repo.Tags,
		}
		cf.Repositories = append(cf.Repositories, rf)
	}

	// Projects
	for _, proj := range c.Projects {
		cf.Projects = append(cf.Projects, ProjectFile(proj))
	}

	return cf
}

// NewDefaultConfig creates a new configuration with default values.
func NewDefaultConfig() *Config {
	cwd, _ := os.Getwd()
	return &Config{
		General: GeneralConfig{
			WorkDir:          cwd,
			Timeout:          DefaultTimeout,
			DefaultBranch:    DefaultBranch,
			RecurseSubmodule: true,
		},
		HTTP: HTTPConfig{
			UserAgent:     "Harbormaster/1.0",
			RetryAttempts: DefaultRetryAttempts,
			RetryDelay:    DefaultRetryDelay,
		},
		Git: GitConfig{
			ShallowClone: true,
			CloneDepth:   DefaultCloneDepth,
		},
	}
}
