package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/manager"
	"github.com/tierone/harbormaster/pkg/ui"
)

var (
	syncLocked   bool
	syncProject  string
	syncTag      string
	syncParallel int
	syncDryRun   bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [repository...]",
	Short: "Synchronize repositories",
	Long: `Synchronize repositories based on the configuration.

Without arguments, syncs all repositories. Specify repository names
to sync specific ones, or use --project to sync a project's repositories.

Use --locked to sync to the exact commits recorded in the lock file
for reproducible builds.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncLocked, "locked", false, "sync to locked SHAs only")
	syncCmd.Flags().StringVarP(&syncProject, "project", "p", "", "sync repositories in project")
	syncCmd.Flags().StringVarP(&syncTag, "tag", "t", "", "sync repositories with tag")
	syncCmd.Flags().IntVar(&syncParallel, "parallel", 4, "number of concurrent operations")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "show what would be synced")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	// Build filter
	filter := manager.Filter{}
	if len(args) > 0 {
		filter.Names = args
	} else if syncProject != "" {
		filter.Projects = []string{syncProject}
	} else if syncTag != "" {
		filter.Tags = []string{syncTag}
	} else {
		filter.All = true
	}

	// Create manager
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
		manager.WithConcurrency(syncParallel),
		manager.WithLocked(syncLocked),
		manager.WithInteractive(!quiet),
	)

	// Dry run - just show what would be synced
	if syncDryRun {
		return runSyncDryRun(mgr, filter)
	}

	// Create and start UI
	uiMgr := ui.NewProgressManager(!quiet)
	if err := uiMgr.Start(); err != nil {
		return fmt.Errorf("failed to start UI: %w", err)
	}

	mgr = manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
		manager.WithConcurrency(syncParallel),
		manager.WithLocked(syncLocked),
		manager.WithInteractive(!quiet),
		manager.WithUI(uiMgr),
	)

	// Run sync
	result, err := mgr.Sync(filter)
	if err != nil {
		return err
	}

	// Save lock file
	if !syncLocked {
		if err := saveLockFile(); err != nil {
			return fmt.Errorf("failed to save lock file: %w", err)
		}
	}

	// Return error if any operations failed
	if result.HasFailures() {
		return fmt.Errorf("%d of %d repositories failed to sync", result.FailureCount, result.TotalRepos)
	}

	return nil
}

func runSyncDryRun(mgr *manager.RepositoryManager, filter manager.Filter) error {
	statuses, err := mgr.Status(filter)
	if err != nil {
		return err
	}

	if len(statuses) == 0 {
		fmt.Println("No repositories to sync")
		return nil
	}

	fmt.Println("Would sync the following repositories:")
	fmt.Println()

	for _, s := range statuses {
		action := "update"
		if !s.Exists {
			action = "clone"
		}

		fmt.Printf("  %s: %s (%s)\n", s.Name, action, s.RequestedRef)
		if s.Exists && s.CurrentSHA != "" {
			fmt.Printf("    Current: %s\n", s.CurrentSHA[:8])
		}
		if s.LockedSHA != "" {
			fmt.Printf("    Locked:  %s\n", s.LockedSHA[:8])
		}
	}

	return nil
}
