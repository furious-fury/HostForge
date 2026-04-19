package services

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hostforge/hostforge/internal/models"
)

func TestRenderWorktreeNixpacksToml_AutoNoOverrides(t *testing.T) {
	t.Parallel()
	_, w := RenderWorktreeNixpacksToml(models.Project{DeployRuntime: models.DeployRuntimeAuto})
	if w {
		t.Fatal("expected no write for auto without overrides")
	}
}

func TestRenderWorktreeNixpacksToml_AutoPartial(t *testing.T) {
	t.Parallel()
	body, w := RenderWorktreeNixpacksToml(models.Project{
		DeployRuntime:    models.DeployRuntimeAuto,
		DeployInstallCmd: "npm ci",
	})
	if !w {
		t.Fatal("expected write")
	}
	if !strings.Contains(body, `[phases.install]`) || !strings.Contains(body, `"npm ci"`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if strings.Contains(strings.ToLower(body), "nodejs_18") {
		t.Fatal("auto partial must not inject nodejs_18")
	}
}

func TestRenderWorktreeNixpacksToml_BunPinsNode20Not18(t *testing.T) {
	t.Parallel()
	body, w := RenderWorktreeNixpacksToml(models.Project{DeployRuntime: models.DeployRuntimeBun})
	if !w {
		t.Fatal("expected write for bun runtime")
	}
	if strings.Contains(body, "nodejs_18") {
		t.Fatalf("bun preset must not reference nodejs_18, got:\n%s", body)
	}
	if !strings.Contains(body, "nodejs_20") {
		t.Fatalf("expected nodejs_20 in setup, got:\n%s", body)
	}
	if matched, _ := regexp.MatchString(`(?m)^\s*providers\s*=\s*\[\s*"bun"\s*\]`, body); matched {
		t.Fatal("must not declare a bun-only providers entry (unsupported on many Nixpacks versions)")
	}
	if !strings.Contains(body, "NIXPACKS_NODE_VERSION") || !strings.Contains(body, "nixPkgs = [\"bun\", \"nodejs_20\"") {
		t.Fatalf("expected Node 20 pin and bun nixPkgs, got:\n%s", body)
	}
}

func TestValidateDeployFields(t *testing.T) {
	t.Parallel()
	rt, i, b, st, code := ValidateDeployFields("BUN", "  a  ", "", "start")
	if code != "" {
		t.Fatalf("unexpected code %q", code)
	}
	if rt != models.DeployRuntimeBun || i != "a" || b != "" || st != "start" {
		t.Fatalf("got %q %q %q %q", rt, i, b, st)
	}
	_, _, _, _, code = ValidateDeployFields("rust", "", "", "")
	if code != "invalid_deploy_runtime" {
		t.Fatalf("got %q", code)
	}
}
