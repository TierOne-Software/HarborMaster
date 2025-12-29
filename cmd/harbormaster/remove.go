package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/manager"
)

var (
	removeDeleteFiles bool
	removeForce       bool
)

var removeCmd = &cobra.Command{
	Use:     "remove <repository>",
	Aliases: []string{"rm"},
	Short:   "Remove a repository from the configuration",
	Long: `Remove a repository from the Harbormaster configuration.

By default, only removes the repository from the configuration file.
Use --delete-files to also delete the local repository files.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func init() {
	removeCmd.Flags().BoolVar(&removeDeleteFiles, "delete-files", false, "also delete local files")
	removeCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "don't prompt for confirmation")
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Check if repository exists
	repo, ok := cfg.GetRepository(name)
	if !ok {
		return fmt.Errorf("repository not found: %s", name)
	}

	repoPath := filepath.Join(cfg.General.WorkDir, repo.GetEffectivePath())

	// Confirm if not forced
	if !removeForce {
		msg := fmt.Sprintf("Remove repository '%s' from configuration?", name)
		if removeDeleteFiles {
			msg = fmt.Sprintf("Remove repository '%s' and delete files at '%s'?", name, repoPath)
		}

		if !confirm(msg) {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Create manager and remove repository
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.Remove(name); err != nil {
		return err
	}

	// Save config
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Save lock file
	if err := saveLockFile(); err != nil {
		return fmt.Errorf("failed to save lock file: %w", err)
	}

	if !quiet {
		fmt.Printf("Removed repository: %s\n", name)
	}

	// Delete files if requested
	if removeDeleteFiles {
		if _, err := os.Stat(repoPath); err == nil {
			if err := os.RemoveAll(repoPath); err != nil {
				return fmt.Errorf("failed to delete files: %w", err)
			}
			if !quiet {
				fmt.Printf("Deleted files: %s\n", repoPath)
			}
		}
	}

	return nil
}

func confirm(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N]: ", prompt)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
