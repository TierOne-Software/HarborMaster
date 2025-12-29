package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/manager"
)

var (
	projectRepos []string
	projectTags  []string
	projectForce bool
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects (repository groups)",
	Long: `Manage projects in Harbormaster.

Projects are collections of repositories that can be operated on together.
Use 'hm sync -p <project>' to sync all repositories in a project.`,
}

var projectAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new project",
	Long: `Create a new project (repository group).

Projects can be created empty and repositories added later, or you can
specify initial repositories with --repos.`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectAdd,
}

var projectRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a project",
	Long: `Remove a project from the configuration.

This only removes the project grouping; repositories are not affected.`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectRemove,
}

var projectAddRepoCmd = &cobra.Command{
	Use:   "add-repo <project> <repository>",
	Short: "Add a repository to a project",
	Args:  cobra.ExactArgs(2),
	RunE:  runProjectAddRepo,
}

var projectRemoveRepoCmd = &cobra.Command{
	Use:     "remove-repo <project> <repository>",
	Aliases: []string{"rm-repo"},
	Short:   "Remove a repository from a project",
	Args:    cobra.ExactArgs(2),
	RunE:    runProjectRemoveRepo,
}

func init() {
	// project add flags
	projectAddCmd.Flags().StringSliceVarP(&projectRepos, "repos", "r", nil,
		"initial repositories (comma-separated)")
	projectAddCmd.Flags().StringSliceVarP(&projectTags, "tags", "t", nil,
		"project tags")

	// project remove flags
	projectRemoveCmd.Flags().BoolVarP(&projectForce, "force", "f", false,
		"don't prompt for confirmation")

	// Build command tree
	projectCmd.AddCommand(projectAddCmd)
	projectCmd.AddCommand(projectRemoveCmd)
	projectCmd.AddCommand(projectAddRepoCmd)
	projectCmd.AddCommand(projectRemoveRepoCmd)
	rootCmd.AddCommand(projectCmd)
}

func runProjectAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Create manager
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	// Create project
	proj := config.Project{
		Name:         name,
		Repositories: projectRepos,
		Tags:         projectTags,
	}

	if err := mgr.AddProject(proj); err != nil {
		return err
	}

	// Save config
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if !quiet {
		fmt.Printf("Created project: %s\n", name)
		if len(projectRepos) > 0 {
			fmt.Printf("  Repositories: %s\n", strings.Join(projectRepos, ", "))
		}
		if len(projectTags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(projectTags, ", "))
		}
	}

	return nil
}

func runProjectRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Check if project exists
	proj, ok := cfg.GetProject(name)
	if !ok {
		return fmt.Errorf("project not found: %s", name)
	}

	// Confirm if not forced
	if !projectForce {
		msg := fmt.Sprintf("Remove project '%s'?", name)
		if len(proj.Repositories) > 0 {
			msg = fmt.Sprintf("Remove project '%s' (contains %d repositories)?",
				name, len(proj.Repositories))
		}

		if !confirm(msg) {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Create manager and remove project
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.RemoveProject(name); err != nil {
		return err
	}

	// Save config
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if !quiet {
		fmt.Printf("Removed project: %s\n", name)
	}

	return nil
}

func runProjectAddRepo(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	repoName := args[1]

	// Create manager
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.AddRepoToProject(projectName, repoName); err != nil {
		return err
	}

	// Save config
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if !quiet {
		fmt.Printf("Added repository '%s' to project '%s'\n", repoName, projectName)
	}

	return nil
}

func runProjectRemoveRepo(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	repoName := args[1]

	// Create manager
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.RemoveRepoFromProject(projectName, repoName); err != nil {
		return err
	}

	// Save config
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if !quiet {
		fmt.Printf("Removed repository '%s' from project '%s'\n", repoName, projectName)
	}

	return nil
}
