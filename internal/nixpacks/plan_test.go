package nixpacks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSummarizePlanJSON_node(t *testing.T) {
	raw := []byte(`{
  "variables": { "NIXPACKS_METADATA": "node", "CI": "true" },
  "phases": { "setup": { "nixPkgs": ["nodejs_18", "npm-9_x"] } }
}`)
	k, l := SummarizePlanJSON(raw)
	if k != "node" || l != "Node" {
		t.Fatalf("want node/Node, got %q / %q", k, l)
	}
}

func TestSummarizePlanJSON_nodeSpa(t *testing.T) {
	raw := []byte(`{
  "variables": {
    "NIXPACKS_METADATA": "node",
    "NIXPACKS_SPA_OUTPUT_DIR": "dist"
  },
  "phases": {}
}`)
	k, l := SummarizePlanJSON(raw)
	if k != "node_spa" || l != "Node · SPA" {
		t.Fatalf("want node_spa/Node · SPA, got %q / %q", k, l)
	}
}

func TestSummarizePlanJSON_go(t *testing.T) {
	raw := []byte(`{ "variables": { "NIXPACKS_METADATA": "go" }, "phases": {} }`)
	k, l := SummarizePlanJSON(raw)
	if k != "go" || l != "Go" {
		t.Fatalf("want go/Go, got %q / %q", k, l)
	}
}

func TestSummarizePlanJSON_python(t *testing.T) {
	raw := []byte(`{ "variables": { "NIXPACKS_METADATA": "python" }, "phases": {} }`)
	k, l := SummarizePlanJSON(raw)
	if k != "python" || l != "Python" {
		t.Fatalf("want python/Python, got %q / %q", k, l)
	}
}

func TestSummarizePlanJSON_inferFromNixPkgs(t *testing.T) {
	raw := []byte(`{
  "variables": {},
  "phases": { "setup": { "nixPkgs": ["nodejs_22", "npm-10_x"] } }
}`)
	k, l := SummarizePlanJSON(raw)
	if k != "node" || l != "Node" {
		t.Fatalf("want node/Node from nixPkgs, got %q / %q", k, l)
	}
}

func TestSummarizePlanJSON_genericMetaStackKind(t *testing.T) {
	raw := []byte(`{ "variables": { "NIXPACKS_METADATA": "haskell" }, "phases": {} }`)
	k, l := SummarizePlanJSON(raw)
	if k != "haskell" || l != "Haskell" {
		t.Fatalf("want haskell/Haskell, got %q / %q", k, l)
	}
}

func TestSummarizePlanJSON_staticfile(t *testing.T) {
	raw := []byte(`{ "variables": { "NIXPACKS_METADATA": "staticfile" }, "phases": {} }`)
	k, l := SummarizePlanJSON(raw)
	if k != "staticfile" || l != "Staticfile" {
		t.Fatalf("want staticfile/Staticfile, got %q / %q", k, l)
	}
}

func TestSummarizePlanJSON_invalid(t *testing.T) {
	k, l := SummarizePlanJSON([]byte(`not json`))
	if k != "" || l != "" {
		t.Fatalf("want empty, got %q / %q", k, l)
	}
}

func TestSummarizePlanWithWorktree_nextFromPlanCache(t *testing.T) {
	raw := []byte(`{
  "variables": { "NIXPACKS_METADATA": "node" },
  "phases": {
    "build": {
      "cmds": ["npm run build"],
      "cacheDirectories": [".next/cache", "node_modules/.cache"]
    }
  },
  "start": { "cmd": "npm run start" }
}`)
	k, l := SummarizePlanWithWorktree("", raw)
	if k != "node_next" || l != "Node · Next.js" {
		t.Fatalf("want node_next, got %q / %q", k, l)
	}
}

func TestSummarizePlanWithWorktree_viteFromPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "dependencies": { "vite": "^5.0.0", "vue": "^3" }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "variables": {
    "NIXPACKS_METADATA": "node",
    "NIXPACKS_SPA_OUTPUT_DIR": "dist"
  },
  "phases": { "build": { "cmds": ["npm run build"] } }
}`)
	k, l := SummarizePlanWithWorktree(dir, raw)
	if k != "node_vite" || l != "Node · Vite" {
		t.Fatalf("want node_vite, got %q / %q", k, l)
	}
}

func TestSummarizePlanWithWorktree_remixBeforeVite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "dependencies": { "@remix-run/react": "^2", "vite": "^5" }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{ "variables": { "NIXPACKS_METADATA": "node" }, "phases": {} }`)
	k, l := SummarizePlanWithWorktree(dir, raw)
	if k != "node_remix" || l != "Node · Remix" {
		t.Fatalf("want node_remix, got %q / %q", k, l)
	}
}

func TestSummarizePlanWithWorktree_nextWinsOverViteDep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "dependencies": { "next": "14", "vite": "5" }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{ "variables": { "NIXPACKS_METADATA": "node" }, "phases": {} }`)
	k, l := SummarizePlanWithWorktree(dir, raw)
	if k != "node_next" || l != "Node · Next.js" {
		t.Fatalf("want node_next, got %q / %q", k, l)
	}
}

func TestSummarizePlanWithWorktree_cra(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "dependencies": { "react-scripts": "5" }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{ "variables": { "NIXPACKS_METADATA": "node" }, "phases": {} }`)
	k, l := SummarizePlanWithWorktree(dir, raw)
	if k != "node_cra" || l != "Node · Create React App" {
		t.Fatalf("want node_cra, got %q / %q", k, l)
	}
}
