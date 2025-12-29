package config

// Project represents a collection of repositories (a "fleet").
type Project struct {
	Name         string
	Repositories []string // Repository names
	Tags         []string // User-defined tags
}

// ProjectFile is the raw TOML structure for a project.
type ProjectFile struct {
	Name         string   `toml:"name"`
	Repositories []string `toml:"repositories"`
	Tags         []string `toml:"tags,omitempty"`
}

// HasRepository returns true if the project contains the named repository.
func (p *Project) HasRepository(name string) bool {
	for _, r := range p.Repositories {
		if r == name {
			return true
		}
	}
	return false
}

// HasTag returns true if the project has the specified tag.
func (p *Project) HasTag(tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
