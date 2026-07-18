package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func openFullyMigratedDB(t *testing.T, name string) (*sql.DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), name)
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_busy_timeout=5000", filepath.ToSlash(dbPath)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, ctx
}

func TestApplyMigrationsCreatesFinalServiceSchema(t *testing.T) {
	t.Parallel()
	db, ctx := openFullyMigratedDB(t, "final.db")

	for table, columns := range map[string][]string{
		"deployments":  {"service_id", "environment_id", "builder_kind", "trigger_kind", "rollback_of", "branch"},
		"domains":      {"application_id", "environment_id", "service_id", "kind", "last_cert_message", "cert_checked_at"},
		"deploy_steps": {"deployment_id", "service_id", "environment_id", "request_id"},
	} {
		for _, column := range columns {
			var count int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("expected %s.%s", table, column)
			}
		}
		var projectColumns int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name='project_id'`, table).Scan(&projectColumns); err != nil {
			t.Fatal(err)
		}
		if projectColumns != 0 {
			t.Fatalf("legacy project_id remains on %s", table)
		}
	}

	for _, table := range []string{"projects", "project_env_vars", "project_git_auth", "project_ssh_keys"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("legacy table %s remains", table)
		}
	}
	for _, table := range []string{"applications", "environments", "services", "service_environments", "environment_variables", "platform_events", "service_metric_samples", "github_app", "github_app_installations", "http_requests"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("required table %s missing: count=%d err=%v", table, count, err)
		}
	}
	for _, table := range []string{
		"database_services", "database_instances", "database_credentials", "database_bindings",
		"backup_destinations", "database_backup_policies", "database_backups", "database_operations", "database_restore_jobs", "database_upgrade_jobs",
		"database_gateway_endpoints", "database_gateway_routes", "database_external_connections",
		"database_external_credentials", "database_external_connection_cidrs", "database_gateway_operations",
	} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("database foundation table %s missing: count=%d err=%v", table, count, err)
		}
	}
	var serviceTypeColumns int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('services') WHERE name='service_type'`).Scan(&serviceTypeColumns); err != nil || serviceTypeColumns != 1 {
		t.Fatalf("expected services.service_type: count=%d err=%v", serviceTypeColumns, err)
	}
	for _, column := range []string{"application_id", "service_id", "environment_id"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('http_requests') WHERE name=?`, column).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected http_requests.%s: count=%d err=%v", column, count, err)
		}
	}
}

func TestPopulatedProjectCutoverCreatesBackupAndPreservesRelationships(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_busy_timeout=5000", filepath.ToSlash(dbPath)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations(version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "0014_application_service_environments.sql" {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if err := applyMigration(ctx, db, entry.Name(), string(body)); err != nil {
			t.Fatal(err)
		}
	}

	stamp := "2026-01-02T03:04:05Z"
	statements := []string{
		`INSERT INTO projects(id,name,repo_url,branch,git_source,github_installation_id,created_at,updated_at) VALUES('project-1','Legacy API','https://github.com/acme/legacy.git','main','github_app',42,?,?)`,
		`INSERT INTO deployments(id,project_id,status,commit_hash,image_ref,created_at,updated_at) VALUES('deploy-1','project-1','SUCCESS','abc123','hostforge/legacy:1',?,?)`,
		`INSERT INTO containers(id,deployment_id,docker_container_id,internal_port,host_port,status,created_at,updated_at) VALUES('container-1','deploy-1','docker-1',3000,32000,'RUNNING',?,?)`,
		`INSERT INTO domains(id,project_id,domain_name,ssl_status,created_at,updated_at) VALUES('domain-1','project-1','legacy.example.test','ACTIVE',?,?)`,
		`INSERT INTO project_env_vars(id,project_id,key,value_ct,value_last4,created_at,updated_at) VALUES('var-1','project-1','DATABASE_URL',X'0102','last',?,?)`,
		`INSERT INTO deploy_steps(deployment_id,project_id,request_id,step,status,duration_ms,started_at,ended_at) VALUES('deploy-1','project-1','request-1','deploy_total','ok',120,?,?)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	if _, err := os.Stat(dbPath + ".pre-application-model.bak"); err != nil {
		t.Fatalf("migration backup missing: %v", err)
	}

	var appID, serviceID, serviceRepo, productionBranch, stagingBranch, activeDeployment string
	var githubInstallationID int64
	if err := migrated.QueryRowContext(ctx, `SELECT id FROM applications WHERE id='project-1'`).Scan(&appID); err != nil {
		t.Fatal(err)
	}
	if err := migrated.QueryRowContext(ctx, `SELECT id,repo_url,github_installation_id FROM services WHERE id='project-1' AND application_id='project-1'`).Scan(&serviceID, &serviceRepo, &githubInstallationID); err != nil {
		t.Fatal(err)
	}
	if err := migrated.QueryRowContext(ctx, `SELECT branch,active_deployment_id FROM service_environments WHERE service_id='project-1' AND environment_id='project-1_production'`).Scan(&productionBranch, &activeDeployment); err != nil {
		t.Fatal(err)
	}
	if err := migrated.QueryRowContext(ctx, `SELECT branch FROM service_environments WHERE service_id='project-1' AND environment_id='project-1_staging'`).Scan(&stagingBranch); err != nil {
		t.Fatal(err)
	}
	if appID != "project-1" || serviceID != "project-1" || serviceRepo != "https://github.com/acme/legacy.git" || githubInstallationID != 42 || productionBranch != "main" || stagingBranch != "" || activeDeployment != "deploy-1" {
		t.Fatalf("cutover mismatch app=%q service=%q repo=%q installation=%d production=%q staging=%q active=%q", appID, serviceID, serviceRepo, githubInstallationID, productionBranch, stagingBranch, activeDeployment)
	}

	for table, id := range map[string]string{"deployments": "deploy-1", "containers": "container-1", "domains": "domain-1", "environment_variables": "var-1"} {
		var count int
		if err := migrated.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+table+" WHERE id=?", id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s relationship not preserved: count=%d err=%v", table, count, err)
		}
	}
	var deploymentService, deploymentEnvironment, deploymentBranch, stepService, stepEnvironment string
	if err := migrated.QueryRowContext(ctx, `SELECT service_id,environment_id,branch FROM deployments WHERE id='deploy-1'`).Scan(&deploymentService, &deploymentEnvironment, &deploymentBranch); err != nil {
		t.Fatal(err)
	}
	if err := migrated.QueryRowContext(ctx, `SELECT service_id,environment_id FROM deploy_steps WHERE deployment_id='deploy-1'`).Scan(&stepService, &stepEnvironment); err != nil {
		t.Fatal(err)
	}
	if deploymentService != "project-1" || deploymentEnvironment != "project-1_production" || deploymentBranch != "main" || stepService != deploymentService || stepEnvironment != deploymentEnvironment {
		t.Fatalf("ownership not reassociated deployment=%s/%s branch=%s step=%s/%s", deploymentService, deploymentEnvironment, deploymentBranch, stepService, stepEnvironment)
	}
	var containerDeployment string
	if err := migrated.QueryRowContext(ctx, `SELECT deployment_id FROM containers WHERE id='container-1'`).Scan(&containerDeployment); err != nil {
		t.Fatal(err)
	}
	if containerDeployment != "deploy-1" {
		t.Fatalf("container reassociated to deployment %q", containerDeployment)
	}
	var domainApplication, domainEnvironment, domainService string
	if err := migrated.QueryRowContext(ctx, `SELECT application_id,environment_id,service_id FROM domains WHERE id='domain-1'`).Scan(&domainApplication, &domainEnvironment, &domainService); err != nil {
		t.Fatal(err)
	}
	if domainApplication != "project-1" || domainEnvironment != "project-1_production" || domainService != "project-1" {
		t.Fatalf("domain ownership not reassociated: %s/%s/%s", domainApplication, domainEnvironment, domainService)
	}
	var variableApplication, variableEnvironment, variableLast4 string
	var variableService sql.NullString
	var variableCiphertext []byte
	if err := migrated.QueryRowContext(ctx, `SELECT application_id,environment_id,service_id,value_ct,value_last4 FROM environment_variables WHERE id='var-1'`).Scan(&variableApplication, &variableEnvironment, &variableService, &variableCiphertext, &variableLast4); err != nil {
		t.Fatal(err)
	}
	if variableApplication != "project-1" || variableEnvironment != "project-1_production" || variableService.Valid || string(variableCiphertext) != string([]byte{1, 2}) || variableLast4 != "last" {
		t.Fatalf("variable migration mismatch app=%q environment=%q service=%v ciphertext=%v last4=%q", variableApplication, variableEnvironment, variableService, variableCiphertext, variableLast4)
	}
	for _, table := range []string{"projects", "project_env_vars", "project_git_auth", "project_ssh_keys"} {
		var count int
		if err := migrated.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("legacy table %s remains: count=%d err=%v", table, count, err)
		}
	}
}
