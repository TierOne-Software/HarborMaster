package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/downloader"
	"github.com/tierone/harbormaster/pkg/manager"
)

var (
	addName   string
	addType   string
	addBranch string
	addTag    string
	addCommit string
	addPath   string
	addSync   bool
	addTags   []string
)

var addCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a repository to the configuration",
	Long: `Add a new repository to the Harbormaster configuration.

The repository type is auto-detected from the URL, but can be
overridden with --type. Use --sync to immediately sync the
repository after adding.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().StringVarP(&addName, "name", "n", "", "repository name (required)")
	// Note: --type deliberately has no shorthand; -t means --tag elsewhere
	// in the CLI and binding it to --type here would be a trap.
	addCmd.Flags().StringVar(&addType, "type", "", "repository type (git or http)")
	addCmd.Flags().StringVarP(&addBranch, "branch", "b", "", "git branch")
	addCmd.Flags().StringVar(&addTag, "tag", "", "git tag")
	addCmd.Flags().StringVar(&addCommit, "commit", "", "git commit SHA")
	addCmd.Flags().StringVarP(&addPath, "path", "p", "", "local path (relative to work_dir)")
	addCmd.Flags().BoolVar(&addSync, "sync", false, "sync immediately after adding")
	addCmd.Flags().StringSliceVar(&addTags, "tags", nil, "tags for filtering")

	_ = addCmd.MarkFlagRequired("name") // Safe to ignore - panics caught at startup
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	url := args[0]

	// Determine type
	repoType := config.RepositoryType(addType)
	if repoType == "" {
		repoType = downloader.DetectType(url)
	}

	// Create repository
	repo := config.Repository{
		Name:   addName,
		URL:    url,
		Type:   repoType,
		Path:   addPath,
		Branch: addBranch,
		Tag:    addTag,
		Commit: addCommit,
		Tags:   addTags,
	}

	// Set default path if not specified
	if repo.Path == "" {
		repo.Path = repo.Name
	}

	// Validate ref options
	refCount := 0
	if addBranch != "" {
		refCount++
	}
	if addTag != "" {
		refCount++
	}
	if addCommit != "" {
		refCount++
	}
	if refCount > 1 {
		return fmt.Errorf("only one of --branch, --tag, or --commit can be specified")
	}

	// Create manager and add repository
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.Add(repo); err != nil {
		return err
	}

	// Validate and save config. Validation catches bad input (e.g. an
	// invalid --type) before it is persisted and bricks the workspace.
	if err := saveConfigValidated(); err != nil {
		return err
	}

	if !quiet {
		fmt.Printf("Added repository: %s\n", repo.Name)
		fmt.Printf("  URL:  %s\n", repo.URL)
		fmt.Printf("  Type: %s\n", repo.Type)
		fmt.Printf("  Path: %s\n", repo.Path)
	}

	// Sync if requested
	if addSync {
		if !quiet {
			fmt.Println("\nSyncing repository...")
		}

		result, err := mgr.SyncOne(repo.Name)
		if err != nil {
			return err
		}

		if !result.Success {
			return fmt.Errorf("sync failed: %v", result.Error)
		}

		// Save lock file
		if err := saveLockFile(); err != nil {
			return fmt.Errorf("failed to save lock file: %w", err)
		}

		if !quiet {
			if result.CommitSHA != "" {
				fmt.Printf("Synced at %s\n", shortSHA(result.CommitSHA))
			} else {
				fmt.Println("Synced")
			}
		}
	}

	return nil
}
