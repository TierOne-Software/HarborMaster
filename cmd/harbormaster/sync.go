package main

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/manager"
)

var (
	syncLocked   bool
	syncProject  string
	syncTag      string
	syncParallel int
	syncDryRun   bool
	syncForce    bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [repository...]",
	Short: "Synchronize repositories",
	Long: `Synchronize repositories based on the configuration.

Without arguments, syncs all repositories. Positional repository names,
--project, and --tag can be combined; the union of all matching
repositories is synced.

Use --locked to sync to the exact commits recorded in the lock file
for reproducible builds.

Syncing is refused while a work session is active, since it may switch
branches under the session. Use --force to sync anyway.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncLocked, "locked", false, "sync to locked SHAs only")
	syncCmd.Flags().StringVarP(&syncProject, "project", "p", "", "sync repositories in project")
	syncCmd.Flags().StringVarP(&syncTag, "tag", "t", "", "sync repositories with tag")
	syncCmd.Flags().IntVar(&syncParallel, "parallel", 4, "number of concurrent operations")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "show what would be synced")
	syncCmd.Flags().BoolVar(&syncForce, "force", false, "sync even if a work session is active")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	// Refuse to sync while a work session is active: syncing checks out
	// configured refs and would pull repositories off the session branch.
	if ws != nil && !syncForce {
		return fmt.Errorf("a work session '%s' is active; syncing would switch branches\nEnd it with 'hm work end' or use --force to sync anyway", ws.Name)
	}

	// Build filter: positional names, --project, and --tag are unioned.
	filter := buildFilter(args, syncProject, syncTag)

	// Dry run - just show what would be synced
	if syncDryRun {
		mgr := manager.NewRepositoryManager(cfg,
			manager.WithLockFile(lf),
			manager.WithLocked(syncLocked),
		)
		return runSyncDryRun(mgr, filter)
	}

	// Use the interactive (full-screen) UI only when stdout is a
	// terminal; piped output gets plain line-oriented progress.
	interactive := !quiet && isatty.IsTerminal(os.Stdout.Fd())

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
		manager.WithConcurrency(syncParallel),
		manager.WithLocked(syncLocked),
		manager.WithInteractive(interactive),
	)

	// With --quiet, silence stdout for the duration of the sync: the
	// progress UI writes to stdout unconditionally. Failures are still
	// reported on stderr below, so --quiet can never hide an error.
	if quiet {
		if devnull, devErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0); devErr == nil {
			orig := os.Stdout
			os.Stdout = devnull
			defer func() {
				os.Stdout = orig
				_ = devnull.Close()
			}()
		}
	}

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

	// Return error if any operations failed. Details always go to stderr
	// so machine consumers of stdout never see them and --quiet cannot
	// hide failures.
	if result.HasFailures() {
		for _, f := range result.FailedResults() {
			if f.Error != nil {
				fmt.Fprintf(os.Stderr, "  %s: %v\n", f.RepoName, f.Error)
			} else {
				fmt.Fprintf(os.Stderr, "  %s: unknown error\n", f.RepoName)
			}
		}
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
			fmt.Printf("    Current: %s\n", shortSHA(s.CurrentSHA))
		}
		if s.LockedSHA != "" {
			fmt.Printf("    Locked:  %s\n", shortSHA(s.LockedSHA))
		}
	}

	return nil
}
