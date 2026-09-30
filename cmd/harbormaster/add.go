package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/downloader"
	"github.com/tierone/harbormaster/pkg/manager"
)

var (
	addName     string
	addType     string
	addBranch   string
	addTag      string
	addCommit   string
	addPath     string
	addSync     bool
	addTags     []string
	addProjects []string
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
	addCmd.Flags().StringVarP(&addName, "name", "n", "", "repository name (default: derived from the URL)")
	// Note: --type deliberately has no shorthand; -t means --tag elsewhere
	// in the CLI and binding it to --type here would be a trap.
	addCmd.Flags().StringVar(&addType, "type", "", "repository type (git or http)")
	addCmd.Flags().StringVarP(&addBranch, "branch", "b", "", "git branch")
	addCmd.Flags().StringVar(&addTag, "tag", "", "git tag")
	addCmd.Flags().StringVar(&addCommit, "commit", "", "git commit SHA")
	addCmd.Flags().StringVarP(&addPath, "path", "p", "", "local path (relative to work_dir)")
	addCmd.Flags().BoolVar(&addSync, "sync", false, "sync immediately after adding")
	addCmd.Flags().StringSliceVar(&addTags, "tags", nil, "tags for filtering")
	// No shorthand: -p is --path on this command, and abbreviating --project
	// here would collide with it.
	addCmd.Flags().StringSliceVar(&addProjects, "project", nil, "project(s) to add the repository to (comma-separated; must already exist)")

	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	url := args[0]

	// Default the repository name from the URL when --name is not given.
	nameDerived := false
	if addName == "" {
		addName = downloader.RepoNameFromURL(url)
		if addName == "" {
			return fmt.Errorf("could not derive a repository name from %q; pass --name", url)
		}
		nameDerived = true
	}

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

	// Attach to any requested projects before saving: an unknown project
	// aborts the whole add and nothing is persisted. Projects are never
	// auto-created — a typo'd flag must not silently fork the grouping.
	for _, project := range addProjects {
		if err := mgr.AddRepoToProject(project, repo.Name); err != nil {
			return fmt.Errorf("failed to add repository to project: %w", err)
		}
	}

	// Validate and save config. Validation catches bad input (e.g. an
	// invalid --type) before it is persisted and bricks the workspace.
	if err := saveConfigValidated(); err != nil {
		return err
	}

	if !quiet {
		fmt.Printf("Added repository: %s\n", repo.Name)
		if nameDerived {
			fmt.Println("  (name derived from URL; use --name to override)")
		}
		fmt.Printf("  URL:  %s\n", repo.URL)
		fmt.Printf("  Type: %s\n", repo.Type)
		fmt.Printf("  Path: %s\n", repo.Path)
		if len(addProjects) > 0 {
			fmt.Printf("  Projects: %s\n", strings.Join(addProjects, ", "))
		}
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
