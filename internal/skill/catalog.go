// Package skill discovers SKILL.md folders and exposes their metadata.
//
// A skill is a directory under the skills root containing a SKILL.md file:
//
//	skills/<name>/
//	  SKILL.md          # YAML frontmatter (name, description) + markdown body
//	  scripts/...       # optional bundled scripts (run via the run_script tool)
//	  REFERENCE.md      # optional bundled docs (read via the read_file tool)
//
// Progressive disclosure (mirrors Anthropic Agent Skills):
//   - L1 metadata: name + description, always injected into the system prompt
//     (see PromptBlock). Cheap — the model only learns a skill exists.
//   - L2 instructions: the SKILL.md body, loaded on demand when the model calls
//     read_file because a request matches the description.
//   - L3 resources/scripts: bundled files, read or executed on demand via the
//     read_file / run_script tools.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Meta struct {
	Name        string
	Description string
	RelPath     string // path to SKILL.md relative to the skills root, e.g. "weather/SKILL.md"
}

type Catalog struct {
	root  string
	metas []Meta
}

// Load scans root for immediate subdirectories containing a SKILL.md. A missing
// root is not an error — it yields an empty catalog. Malformed SKILL.md files
// are skipped with the returned warnings (so one bad skill doesn't block start).
func Load(root string) (*Catalog, []string, error) {
	c := &Catalog{}
	if strings.TrimSpace(root) == "" {
		return c, nil, nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: resolve root: %w", err)
	}
	c.root = abs

	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil, nil
		}
		return nil, nil, fmt.Errorf("skill: read dir: %w", err)
	}

	var warnings []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(abs, e.Name(), "SKILL.md")
		raw, err := os.ReadFile(mdPath)
		if err != nil {
			continue // dir without SKILL.md is not a skill
		}
		name, desc, err := parseFrontmatter(raw)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		if name == "" {
			name = e.Name()
		}
		c.metas = append(c.metas, Meta{
			Name:        name,
			Description: desc,
			RelPath:     filepath.ToSlash(filepath.Join(e.Name(), "SKILL.md")),
		})
	}
	sort.Slice(c.metas, func(i, j int) bool { return c.metas[i].Name < c.metas[j].Name })
	return c, warnings, nil
}

func (c *Catalog) Root() string { return c.root }
func (c *Catalog) List() []Meta { return c.metas }
func (c *Catalog) Empty() bool  { return len(c.metas) == 0 }

// PromptBlock is the system-prompt section that makes the model aware of the
// available skills (L1) and tells it how to load one (L2). Empty when there are
// no skills.
func (c *Catalog) PromptBlock() string {
	if len(c.metas) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Skills\n\n")
	b.WriteString("A skill is a set of on-disk instructions. When a request matches a skill below, ")
	b.WriteString("call the read_file tool on its SKILL.md to load the full instructions, then follow them ")
	b.WriteString("(using tools such as http_get or run_script as the instructions direct). ")
	b.WriteString("Do not guess a skill's steps — read it first.\n\n")
	for _, m := range c.metas {
		fmt.Fprintf(&b, "- %s: %s [read_file: %s]\n", m.Name, m.Description, m.RelPath)
	}
	return b.String()
}

// parseFrontmatter extracts name/description from leading YAML frontmatter
// delimited by --- lines.
func parseFrontmatter(content []byte) (name, description string, err error) {
	s := strings.TrimLeft(string(content), "\uFEFF \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return "", "", fmt.Errorf("SKILL.md missing YAML frontmatter (must start with ---)")
	}
	rest := s[len("---"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("SKILL.md frontmatter not terminated with ---")
	}
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return "", "", fmt.Errorf("SKILL.md frontmatter: %w", err)
	}
	if strings.TrimSpace(fm.Description) == "" {
		return "", "", fmt.Errorf("SKILL.md frontmatter missing 'description'")
	}
	return strings.TrimSpace(fm.Name), strings.TrimSpace(fm.Description), nil
}
