// Package prompts parses and renders OKF-style prompt files: a YAML
// frontmatter block delimited by "---" lines, followed by a markdown body that
// is a Go text/template.
package prompts

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"text/template"

	"go.yaml.in/yaml/v3"

	embedded "github.com/dorkitude/smart_search/prompts"
)

// Well-known prompt names shipped in prompts/.
const (
	RelevanceScore = "relevance-score"
	RelevanceNoul  = "relevance-noul"
	RelevanceBatch = "relevance-batch"
	ThemeCoverage  = "theme-coverage"
	ChunkRelevance = "chunk-relevance"
	SourceQuality  = "source-quality"
)

// Prompt is a parsed OKF prompt file.
type Prompt struct {
	// Frontmatter fields.
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Type        string   `yaml:"type"` // "score" or "noul"
	Model       string   `yaml:"model"`
	Criteria    []string `yaml:"criteria"`
	Batch       bool     `yaml:"batch"`

	// Body is the raw template text after the frontmatter.
	Body string `yaml:"-"`

	tmpl *template.Template
}

// Data is the template context available to prompt bodies.
type Data struct {
	Query    string
	Title    string
	URL      string
	Snippet  string
	Question string // for noul prompts
	Theme    string // for eval coverage prompts
	Chunk    string // for page chunk prompts
	ID       string // for batch prompts
	Index    int    // zero-based position in a batch
}

// Render executes the prompt body with data, returning trimmed text.
func (p *Prompt) Render(data Data) (string, error) {
	var buf bytes.Buffer
	if err := p.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt %q: %w", p.Name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// ErrNoFrontmatter is returned when a prompt file lacks the leading "---" block.
var ErrNoFrontmatter = errors.New("missing YAML frontmatter (file must start with ---)")

// Parse parses OKF prompt content. name is used for error messages and as a
// fallback if the frontmatter omits "name".
func Parse(name string, content []byte) (*Prompt, error) {
	front, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("prompt %s: %w", name, err)
	}
	p := &Prompt{}
	if err := yaml.Unmarshal(front, p); err != nil {
		return nil, fmt.Errorf("prompt %s: parse frontmatter: %w", name, err)
	}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(path.Base(name), ".md")
	}
	switch p.Type {
	case "score":
		if len(p.Criteria) < 2 {
			return nil, fmt.Errorf("prompt %s: score prompts need at least 2 criteria, got %d", name, len(p.Criteria))
		}
	case "noul":
		if len(p.Criteria) > 0 {
			return nil, fmt.Errorf("prompt %s: noul prompts must not declare criteria", name)
		}
	case "":
		return nil, fmt.Errorf("prompt %s: frontmatter is missing required field \"type\"", name)
	default:
		return nil, fmt.Errorf("prompt %s: unknown type %q (expected score or noul)", name, p.Type)
	}
	p.Body = strings.TrimSpace(string(body))
	if p.Body == "" {
		return nil, fmt.Errorf("prompt %s: body is empty", name)
	}
	tmpl, err := template.New(p.Name).Option("missingkey=error").Parse(p.Body)
	if err != nil {
		return nil, fmt.Errorf("prompt %s: parse template: %w", name, err)
	}
	p.tmpl = tmpl
	return p, nil
}

// splitFrontmatter separates the YAML block from the body. It accepts "---"
// with trailing whitespace and both LF and CRLF line endings.
func splitFrontmatter(content []byte) (front, body []byte, err error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimLeft(text, "\uFEFF") // BOM
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, nil, ErrNoFrontmatter
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return []byte(strings.Join(lines[1:i], "\n")),
				[]byte(strings.Join(lines[i+1:], "\n")), nil
		}
	}
	return nil, nil, errors.New("unterminated YAML frontmatter (no closing ---)")
}

// LoadFS reads and parses name (with or without .md) from fsys.
func LoadFS(fsys fs.FS, name string) (*Prompt, error) {
	file := name
	if !strings.HasSuffix(file, ".md") {
		file += ".md"
	}
	content, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, fmt.Errorf("load prompt %q: %w", name, err)
	}
	return Parse(file, content)
}

// ListFS returns the prompt names (without .md) in fsys, sorted.
func ListFS(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(names)
	return names, nil
}

var (
	cacheMu sync.Mutex
	cache   = map[string]*Prompt{}
)

// Load returns the embedded prompt with the given name, parsing it once.
func Load(name string) (*Prompt, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if p, ok := cache[name]; ok {
		return p, nil
	}
	p, err := LoadFS(embedded.FS, name)
	if err != nil {
		return nil, err
	}
	cache[name] = p
	return p, nil
}

// MustLoad is Load for prompts that ship with the binary; a failure is a bug.
func MustLoad(name string) *Prompt {
	p, err := Load(name)
	if err != nil {
		panic(err)
	}
	return p
}

// List returns the names of all embedded prompts.
func List() []string {
	names, _ := ListFS(embedded.FS)
	return names
}
