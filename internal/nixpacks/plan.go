package nixpacks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PlanJSON runs `nixpacks plan . -f json` in workDir and returns stdout (JSON).
func PlanJSON(ctx context.Context, workDir string) ([]byte, error) {
	planArgs := append([]string{"plan", ".", "-f", "json"}, defaultNixpacksFlags()...)
	cmd := exec.CommandContext(ctx, "nixpacks", planArgs...)
	cmd.Dir = workDir
	cmd.Env = nixpacksEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nixpacks plan in %s: %w (stderr: %s)", workDir, err, strings.TrimSpace(stderr.String()))
	}
	out := bytes.TrimSpace(stdout.Bytes())
	if len(out) == 0 {
		return nil, fmt.Errorf("nixpacks plan returned empty stdout")
	}
	return out, nil
}

type planDoc struct {
	Variables map[string]json.RawMessage `json:"variables"`
	Phases    map[string]phaseDoc         `json:"phases"`
	Start     *struct {
		Cmd string `json:"cmd"`
	} `json:"start"`
}

type phaseDoc struct {
	NixPkgs          []string `json:"nixPkgs"`
	Cmds             []string `json:"cmds"`
	CacheDirectories []string `json:"cacheDirectories"`
}

type pkgDoc struct {
	Dependencies    map[string]json.RawMessage `json:"dependencies"`
	DevDependencies map[string]json.RawMessage `json:"devDependencies"`
}

// SummarizePlanJSON returns stable stack_kind and stack_label from nixpacks plan JSON only (no package.json).
func SummarizePlanJSON(raw []byte) (kind string, label string) {
	return SummarizePlanWithWorktree("", raw)
}

// SummarizePlanWithWorktree parses plan JSON and, for Node projects, refines the stack using package.json
// in workDir when present (Next.js, Vite, Remix, etc.). Pass empty workDir to skip file reads.
func SummarizePlanWithWorktree(workDir string, raw []byte) (kind string, label string) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", ""
	}
	var doc planDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", ""
	}

	meta := strings.ToLower(strings.TrimSpace(stringJSON(doc.Variables["NIXPACKS_METADATA"])))
	spaOut := strings.TrimSpace(stringJSON(doc.Variables["NIXPACKS_SPA_OUTPUT_DIR"])) != ""

	if meta == "" {
		meta = inferMetaFromNixPkgs(doc.Phases)
	}
	if meta == "" {
		return "unknown", "Unknown"
	}

	switch meta {
	case "node":
		return refineNodeStack(workDir, doc, spaOut)
	case "go":
		return "go", "Go"
	case "python":
		return "python", "Python"
	case "ruby":
		return "ruby", "Ruby"
	case "php":
		return "php", "PHP"
	case "rust":
		return "rust", "Rust"
	case "deno":
		return "deno", "Deno"
	case "staticfile":
		return "staticfile", "Staticfile"
	default:
		// stack_kind matches NIXPACKS_METADATA (lowercased) so UI icons can use `<kind>.png`.
		return meta, titleMeta(meta)
	}
}

func planCmdAndCacheHaystack(doc planDoc) string {
	var b strings.Builder
	for _, ph := range doc.Phases {
		for _, c := range ph.Cmds {
			b.WriteString(strings.ToLower(c))
			b.WriteByte(' ')
		}
		for _, cd := range ph.CacheDirectories {
			b.WriteString(strings.ToLower(cd))
			b.WriteByte(' ')
		}
	}
	if doc.Start != nil {
		b.WriteString(strings.ToLower(doc.Start.Cmd))
		b.WriteByte(' ')
	}
	return b.String()
}

func readPackageJSON(dir string) *pkgDoc {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var p pkgDoc
	if err := json.Unmarshal(b, &p); err != nil {
		return nil
	}
	return &p
}

func hasPkgKey(pkg *pkgDoc, key string) bool {
	if pkg == nil {
		return false
	}
	if pkg.Dependencies != nil {
		if _, ok := pkg.Dependencies[key]; ok {
			return true
		}
	}
	if pkg.DevDependencies != nil {
		if _, ok := pkg.DevDependencies[key]; ok {
			return true
		}
	}
	return false
}

func refineNodeStack(workDir string, doc planDoc, spaOut bool) (kind string, label string) {
	hay := planCmdAndCacheHaystack(doc)
	pkg := readPackageJSON(workDir)

	// Next.js: Nixpacks caches .next; package.json usually lists "next".
	if strings.Contains(hay, ".next/cache") || hasPkgKey(pkg, "next") {
		return "node_next", "Node · Next.js"
	}
	// Remix before generic Vite (Remix often depends on vite).
	if hasPkgKey(pkg, "@remix-run/react") || hasPkgKey(pkg, "@remix-run/node") || hasPkgKey(pkg, "remix") {
		return "node_remix", "Node · Remix"
	}
	if hasPkgKey(pkg, "nuxt") || hasPkgKey(pkg, "@nuxt/schema") {
		return "node_nuxt", "Node · Nuxt"
	}
	if hasPkgKey(pkg, "@sveltejs/kit") {
		return "node_svelte", "Node · SvelteKit"
	}
	if hasPkgKey(pkg, "astro") {
		return "node_astro", "Node · Astro"
	}
	if hasPkgKey(pkg, "vite") {
		return "node_vite", "Node · Vite"
	}
	if hasPkgKey(pkg, "react-scripts") || hasPkgKey(pkg, "craco") {
		return "node_cra", "Node · Create React App"
	}

	// Plan text fallbacks when package.json is missing or non-standard.
	if strings.Contains(hay, "next ") || strings.Contains(hay, "next build") || strings.Contains(hay, "next start") {
		return "node_next", "Node · Next.js"
	}
	if strings.Contains(hay, " remix") || strings.Contains(hay, "remix ") || strings.Contains(hay, "remix-vite") {
		return "node_remix", "Node · Remix"
	}
	if strings.Contains(hay, "vite ") || strings.Contains(hay, "vite build") {
		return "node_vite", "Node · Vite"
	}
	if strings.Contains(hay, "react-scripts") {
		return "node_cra", "Node · Create React App"
	}

	if spaOut {
		return "node_spa", "Node · SPA"
	}
	return "node", "Node"
}

func stringJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func inferMetaFromNixPkgs(phases map[string]phaseDoc) string {
	setup, ok := phases["setup"]
	if !ok {
		return ""
	}
	for _, p := range setup.NixPkgs {
		pl := strings.ToLower(p)
		switch {
		case strings.Contains(pl, "nodejs"), strings.Contains(pl, "node_"):
			return "node"
		case strings.Contains(pl, "go_"), pl == "go":
			return "go"
		case strings.Contains(pl, "python"):
			return "python"
		case strings.Contains(pl, "ruby"):
			return "ruby"
		case strings.Contains(pl, "php"):
			return "php"
		case strings.Contains(pl, "rust"), strings.Contains(pl, "cargo"):
			return "rust"
		case strings.Contains(pl, "deno"):
			return "deno"
		}
	}
	return ""
}

func titleMeta(meta string) string {
	if meta == "" {
		return "Unknown"
	}
	return strings.ToUpper(meta[:1]) + meta[1:]
}
