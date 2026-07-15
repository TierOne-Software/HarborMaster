package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	WorkDir          string // Expanded absolute path for use at runtime
	WorkDirOriginal  string // Original value from config (for saving back)
	CacheDir         string
	CacheDirOriginal string // Original value from config (for saving back)
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
	// Resolve the path immediately so that later saves are not affected by
	// working-directory changes when a relative path was passed in.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path: %w", err)
	}

	var cf ConfigFile
	md, err := toml.DecodeFile(absPath, &cf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	if err := checkUndecodedKeys(md, "config file"); err != nil {
		return nil, err
	}

	cfg, err := parseConfigFile(&cf, absPath)
	if err != nil {
		return nil, err
	}
	cfg.configPath = absPath

	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// checkUndecodedKeys returns an error if the TOML document contained keys
// that do not map to any known field. This catches typos such as
// "default_brnach" or "[[repositories]]" that would otherwise be silently
// ignored.
func checkUndecodedKeys(md toml.MetaData, what string) error {
	undecoded := md.Undecoded()
	if len(undecoded) == 0 {
		return nil
	}
	keys := make([]string, len(undecoded))
	for i, k := range undecoded {
		keys[i] = k.String()
	}
	return fmt.Errorf("unknown key(s) in %s: %s", what, strings.Join(keys, ", "))
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

// SaveTo writes the configuration to the specified path. The file is
// written atomically (temp file + rename), so an existing config file is
// never truncated by a failed save.
func (c *Config) SaveTo(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	cf := toConfigFile(c, filepath.Dir(absPath))

	err = writeFileAtomic(absPath, func(f *os.File) error {
		if err := toml.NewEncoder(f).Encode(cf); err != nil {
			return fmt.Errorf("failed to encode config: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	c.configPath = absPath
	// Keep the "original" path values in sync with what was written, so
	// that in-memory state matches a subsequent Load of the saved file.
	c.General.WorkDirOriginal = cf.General.WorkDir
	c.General.CacheDirOriginal = cf.General.CacheDir
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
	// Store original value for saving back to file
	cfg.General.WorkDirOriginal = cf.General.WorkDir
	if cf.General.WorkDir != "" {
		workDir, err := resolvePathValue(cf.General.WorkDir, configDir)
		if err != nil {
			return nil, fmt.Errorf("failed to expand work_dir: %w", err)
		}
		cfg.General.WorkDir = workDir
	} else {
		// Default to the config file's directory
		cfg.General.WorkDir = configDir
		cfg.General.WorkDirOriginal = "./"
	}

	// Store original value for saving back to file
	cfg.General.CacheDirOriginal = cf.General.CacheDir
	if cf.General.CacheDir != "" {
		cacheDir, err := resolvePathValue(cf.General.CacheDir, configDir)
		if err != nil {
			return nil, fmt.Errorf("failed to expand cache_dir: %w", err)
		}
		cfg.General.CacheDir = cacheDir
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

// resolvePathValue expands value (~, environment variables) and resolves it
// against baseDir if it is relative, returning a cleaned absolute path.
func resolvePathValue(value, baseDir string) (string, error) {
	expanded, err := ExpandPath(value)
	if err != nil {
		return "", err
	}
	if expanded == "" {
		return "", nil
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(baseDir, expanded)
	}
	return filepath.Clean(expanded), nil
}

// pathValueForSave decides which value to write back to the config file for
// a directory setting. The original (possibly relative or ~/$VAR-based)
// value is preserved as long as it still resolves to the current runtime
// value; otherwise the runtime value was changed programmatically and must
// be written out so the change is not silently discarded.
func pathValueForSave(original, current, configDir string) string {
	if original != "" {
		if resolved, err := resolvePathValue(original, configDir); err == nil && resolved == filepath.Clean(current) {
			return original
		}
	}
	return current
}

func toConfigFile(c *Config, configDir string) *ConfigFile {
	cf := &ConfigFile{}

	// General config - preserve original (relative) values when they still
	// match the runtime paths, but write programmatic changes through.
	cf.General.WorkDir = pathValueForSave(c.General.WorkDirOriginal, c.General.WorkDir, configDir)
	cf.General.CacheDir = pathValueForSave(c.General.CacheDirOriginal, c.General.CacheDir, configDir)
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
// If the current working directory cannot be determined, WorkDir falls back
// to "." (it is resolved to an absolute path on save/load anyway); use
// NewDefaultConfigE to observe the error instead.
func NewDefaultConfig() *Config {
	cfg, err := NewDefaultConfigE()
	if err != nil {
		return newDefaultConfig(".")
	}
	return cfg
}

// NewDefaultConfigE creates a new configuration with default values,
// returning an error if the current working directory cannot be determined.
func NewDefaultConfigE() (*Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to determine working directory: %w", err)
	}
	return newDefaultConfig(cwd), nil
}

func newDefaultConfig(workDir string) *Config {
	return &Config{
		General: GeneralConfig{
			WorkDir:          workDir,
			WorkDirOriginal:  "./", // Use relative path for portability
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
