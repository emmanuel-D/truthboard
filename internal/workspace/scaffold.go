package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Declare adds spokes to the hub's manifest, creating it when absent and
// merging into it otherwise. All-or-nothing: every declaration is validated
// before anything touches disk. An existing entry is never rewritten —
// re-declaring a spoke identically is a logged no-op, and changing one is
// an explicit edit of the manifest file, not a scaffold side effect. The
// merge appends to the existing YAML document, so hand-written comments
// and ordering survive.
func Declare(hub string, repos []Repo) ([]string, error) {
	seen := map[string]bool{}
	for _, r := range repos {
		if err := ValidName(r.Name); err != nil {
			return nil, err
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("repo %q declared twice", r.Name)
		}
		seen[r.Name] = true
		if r.Remote == "" && r.Path == "" {
			return nil, fmt.Errorf("repo %q needs a remote (name=remote) or a local checkout (--path %s=dir)", r.Name, r.Name)
		}
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("nothing to declare — pass name=remote pairs (and/or --path name=dir)")
	}

	// A malformed existing manifest must fail here, never be appended to.
	existing, err := Load(hub)
	if err != nil {
		return nil, err
	}
	byName := map[string]Repo{}
	if existing != nil {
		for _, r := range existing.Repos {
			byName[r.Name] = r
		}
	}

	var log []string
	var fresh []Repo
	for _, r := range repos {
		cur, ok := byName[r.Name]
		if !ok {
			fresh = append(fresh, r)
			continue
		}
		if cur.Remote != r.Remote || cur.Path != r.Path {
			return nil, fmt.Errorf("repo %q is already declared (remote %q, path %q) — edit %s to change it",
				r.Name, cur.Remote, cur.Path, File)
		}
		log = append(log, fmt.Sprintf("spoke %s already declared — left untouched", r.Name))
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].Name < fresh[j].Name })

	if len(fresh) > 0 {
		if err := writeManifest(hub, fresh); err != nil {
			return nil, err
		}
	}
	for _, r := range fresh {
		switch {
		case r.Remote != "" && r.Path != "":
			log = append(log, fmt.Sprintf("declared spoke %s → %s (local checkout %s)", r.Name, r.Remote, r.Path))
		case r.Remote != "":
			log = append(log, fmt.Sprintf("declared spoke %s → %s (board server will clone)", r.Name, r.Remote))
		default:
			log = append(log, fmt.Sprintf("declared spoke %s → local checkout %s", r.Name, r.Path))
		}
	}
	return log, nil
}

// writeManifest appends repos to the manifest's repos: mapping, creating
// the document when the file does not exist yet.
func writeManifest(hub string, repos []Repo) error {
	path := filepath.Join(hub, filepath.FromSlash(File))
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	if raw, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(raw, doc); err != nil {
			return fmt.Errorf("%s: %w", File, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	root := doc.Content[0]
	var reposNode *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "repos" {
			reposNode = root.Content[i+1]
			break
		}
	}
	if reposNode == nil {
		reposNode = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "repos"}, reposNode)
	}
	for _, r := range repos {
		entry := &yaml.Node{Kind: yaml.MappingNode}
		add := func(k, v string) {
			entry.Content = append(entry.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: k},
				&yaml.Node{Kind: yaml.ScalarNode, Value: v})
		}
		if r.Remote != "" {
			add("remote", r.Remote)
		}
		if r.Integration != "" {
			add("integration", r.Integration)
		}
		if r.Path != "" {
			add("path", r.Path)
		}
		reposNode.Content = append(reposNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: r.Name}, entry)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
