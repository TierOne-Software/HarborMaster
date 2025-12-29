package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/lockfile"
)

var (
	initForce   bool
	initExample bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Harbormaster workspace",
	Long: `Initialize a new Harbormaster workspace by creating a configuration
file (.harbormaster.toml) and lock file (.harbormaster.lock) in the
current directory.

Use --example to include example repository entries in the configuration.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "overwrite existing configuration")
	initCmd.Flags().BoolVar(&initExample, "example", false, "include example repository entries")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(cwd, config.ConfigFileName)
	lockPath := filepath.Join(cwd, lockfile.LockFileName)

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil && !initForce {
		return fmt.Errorf("config file already exists: %s\nUse --force to overwrite", configPath)
	}

	// Create default config
	cfg := config.NewDefaultConfig()

	// Add example repositories if requested
	if initExample {
		cfg.Repositories = []config.Repository{
			{
				Name:   "example-repo",
				URL:    "https://github.com/user/repo.git",
				Type:   config.RepoTypeGit,
				Branch: "main",
				Path:   "example-repo",
				Tags:   []string{"example"},
			},
		}
		cfg.Projects = []config.Project{
			{
				Name:         "example-project",
				Repositories: []string{"example-repo"},
				Tags:         []string{"example"},
			},
		}
	}

	// Save config
	if err := cfg.SaveTo(configPath); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	// Create empty lock file
	lf := lockfile.New()
	if err := lf.Save(lockPath); err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	if !quiet {
		fmt.Println("Initialized Harbormaster workspace:")
		fmt.Printf("  Config: %s\n", configPath)
		fmt.Printf("  Lock:   %s\n", lockPath)
		if initExample {
			fmt.Println("\nExample configuration created. Edit the config file to add your repositories.")
		} else {
			fmt.Println("\nEdit the config file to add your repositories, then run 'hm sync'.")
		}
	}

	return nil
}
