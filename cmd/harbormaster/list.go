package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	listJSON    bool
	listProject string
	listTag     string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List repositories and projects",
	Long:  `List configured repositories and projects.`,
}

var listReposCmd = &cobra.Command{
	Use:     "repos",
	Aliases: []string{"repositories", "r"},
	Short:   "List repositories",
	RunE:    runListRepos,
}

var listProjectsCmd = &cobra.Command{
	Use:     "projects",
	Aliases: []string{"p"},
	Short:   "List projects",
	RunE:    runListProjects,
}

var listTagsCmd = &cobra.Command{
	Use:     "tags",
	Aliases: []string{"t"},
	Short:   "List all tags",
	RunE:    runListTags,
}

func init() {
	listCmd.PersistentFlags().BoolVar(&listJSON, "json", false, "output as JSON")
	listCmd.PersistentFlags().StringVarP(&listProject, "project", "p", "", "filter by project")
	listCmd.PersistentFlags().StringVarP(&listTag, "tag", "t", "", "filter by tag")

	listCmd.AddCommand(listReposCmd)
	listCmd.AddCommand(listProjectsCmd)
	listCmd.AddCommand(listTagsCmd)
	rootCmd.AddCommand(listCmd)
}

func runListRepos(cmd *cobra.Command, args []string) error {
	repos := cfg.Repositories

	// Filter by project
	if listProject != "" {
		var err error
		repos, err = cfg.GetRepositoriesForProject(listProject)
		if err != nil {
			return err
		}
	}

	// Filter by tag
	if listTag != "" {
		repos = cfg.GetRepositoriesByTag(listTag)
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found")
		return nil
	}

	if listJSON {
		type jsonRepo struct {
			Name   string   `json:"name"`
			URL    string   `json:"url"`
			Type   string   `json:"type"`
			Path   string   `json:"path"`
			Branch string   `json:"branch,omitempty"`
			Tag    string   `json:"tag,omitempty"`
			Commit string   `json:"commit,omitempty"`
			Tags   []string `json:"tags,omitempty"`
		}

		output := make([]jsonRepo, len(repos))
		for i, r := range repos {
			output[i] = jsonRepo{
				Name:   r.Name,
				URL:    r.URL,
				Type:   string(r.Type),
				Path:   r.GetEffectivePath(),
				Branch: r.Branch,
				Tag:    r.Tag,
				Commit: r.Commit,
				Tags:   r.Tags,
			}
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tTYPE\tREF\tPATH\tTAGS")

	for _, r := range repos {
		ref := r.Branch
		if r.Tag != "" {
			ref = "tag:" + r.Tag
		} else if r.Commit != "" {
			if len(r.Commit) > 8 {
				ref = r.Commit[:8]
			} else {
				ref = r.Commit
			}
		}
		if ref == "" {
			ref = cfg.General.DefaultBranch
		}

		tags := "-"
		if len(r.Tags) > 0 {
			tags = strings.Join(r.Tags, ", ")
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.Name,
			r.Type,
			ref,
			r.GetEffectivePath(),
			tags,
		)
	}

	return w.Flush()
}

func runListProjects(cmd *cobra.Command, args []string) error {
	projects := cfg.Projects

	if len(projects) == 0 {
		fmt.Println("No projects configured")
		return nil
	}

	if listJSON {
		type jsonProject struct {
			Name         string   `json:"name"`
			Repositories []string `json:"repositories"`
			Tags         []string `json:"tags,omitempty"`
		}

		output := make([]jsonProject, len(projects))
		for i, p := range projects {
			output[i] = jsonProject{
				Name:         p.Name,
				Repositories: p.Repositories,
				Tags:         p.Tags,
			}
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tREPOSITORIES\tTAGS")

	for _, p := range projects {
		repos := strings.Join(p.Repositories, ", ")
		tags := "-"
		if len(p.Tags) > 0 {
			tags = strings.Join(p.Tags, ", ")
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, repos, tags)
	}

	return w.Flush()
}

func runListTags(cmd *cobra.Command, args []string) error {
	tagSet := make(map[string]int)

	// Collect tags from repositories
	for _, r := range cfg.Repositories {
		for _, t := range r.Tags {
			tagSet[t]++
		}
	}

	// Collect tags from projects
	for _, p := range cfg.Projects {
		for _, t := range p.Tags {
			tagSet[t]++
		}
	}

	if len(tagSet) == 0 {
		fmt.Println("No tags found")
		return nil
	}

	if listJSON {
		type jsonTag struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		output := make([]jsonTag, 0, len(tagSet))
		for name, count := range tagSet {
			output = append(output, jsonTag{Name: name, Count: count})
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TAG\tCOUNT")

	for name, count := range tagSet {
		_, _ = fmt.Fprintf(w, "%s\t%d\n", name, count)
	}

	return w.Flush()
}
