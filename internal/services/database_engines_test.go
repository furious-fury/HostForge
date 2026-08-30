package services

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/databases"
	"github.com/furious-fury/HostForge/internal/repository"
)

func TestEveryCatalogEngineHasContainerAndHealthConfiguration(t *testing.T) {
	credential := repository.DatabaseCredential{DatabaseName: "hostforge_app", Username: "hf_app"}
	for _, engine := range databases.Catalog() {
		t.Run(engine.ID, func(t *testing.T) {
			candidate := credential
			if engine.ID == "redis" || engine.ID == "valkey" {
				candidate.DatabaseName, candidate.Username = "0", "default"
			}
			spec, err := databaseContainerConfiguration(engine.ID, candidate, []byte("application-secret"), []byte("admin-secret"))
			if err != nil {
				t.Fatal(err)
			}
			if len(spec.Env) == 0 && len(spec.Command) == 0 {
				t.Fatalf("engine has no startup configuration: %+v", spec)
			}
			if engine.StopTimeoutSeconds <= 0 {
				t.Fatalf("engine has no graceful stop timeout: %+v", engine)
			}
			command, _, err := databaseHealthCommand(engine.ID, candidate, []byte("application-secret"), []byte("admin-secret"))
			if err != nil || len(command) == 0 {
				t.Fatalf("engine has no health command: command=%v err=%v", command, err)
			}
		})
	}
}

func TestPrepareManagedDatabaseSupportsEveryCatalogEngine(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := repository.New(db)
	sealer, err := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	app, _ := store.CreateApplication(ctx, "Engine catalog", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	for _, engine := range databases.Catalog() {
		t.Run(engine.ID, func(t *testing.T) {
			version := engine.Versions[0]
			created, err := PrepareManagedDatabase(ctx, store, sealer, CreateManagedDatabaseInput{
				ApplicationID: app.ID, Name: engine.ID + " database", Engine: engine.ID, Version: version.Version,
				EnvironmentIDs: []string{environments[0].ID}, ResourcePreset: "standard", Actor: "test",
			})
			if err != nil {
				t.Fatal(err)
			}
			if created.Database.Engine != engine.ID || len(created.Instances) != 1 || created.Instances[0].ImageRef != version.ImageRef {
				t.Fatalf("unexpected prepared database: %+v", created)
			}
			credential, err := store.GetDatabaseCredentialSealed(ctx, created.Instances[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			password, _ := sealer.Open(credential.PasswordCT)
			admin, _ := sealer.Open(credential.AdminPasswordCT)
			if len(password) == 0 || len(admin) == 0 || string(password) == string(admin) {
				t.Fatal("application and administrator credentials must be separately generated")
			}
		})
	}
}

func TestPreparedSQLIdentitiesIncludePerEnvironmentEntropy(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "identities.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := repository.New(db)
	sealer, _ := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	app, _ := store.CreateApplication(ctx, "Identity catalog", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := PrepareManagedDatabase(ctx, store, sealer, CreateManagedDatabaseInput{ApplicationID: app.ID, Name: "Customer Records", Engine: "postgresql", Version: "18", EnvironmentIDs: []string{environments[0].ID, environments[1].ID}, ResourcePreset: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := store.GetDatabaseCredentialSealed(ctx, created.Instances[0].ID)
	second, _ := store.GetDatabaseCredentialSealed(ctx, created.Instances[1].ID)
	for _, credential := range []repository.DatabaseCredential{first, second} {
		if !strings.HasPrefix(credential.DatabaseName, "customer_records_") || !strings.HasPrefix(credential.Username, "hf_customer_records_") {
			t.Fatalf("generated identity did not retain the safe slug and entropy: %+v", credential)
		}
	}
	if first.DatabaseName == second.DatabaseName || first.Username == second.Username {
		t.Fatalf("production and staging reused database identities: first=%+v second=%+v", first, second)
	}
}

func TestMySQLFamilyKeepsAdminCredentialSeparate(t *testing.T) {
	credential := repository.DatabaseCredential{DatabaseName: "app", Username: "hf_app"}
	for _, engine := range []string{"mysql", "mariadb"} {
		spec, err := databaseContainerConfiguration(engine, credential, []byte("application-secret"), []byte("different-admin-secret"))
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(spec.Env, "\n")
		if !strings.Contains(joined, "different-admin-secret") || !strings.Contains(joined, "application-secret") {
			t.Fatalf("%s did not receive separate admin and application credentials", engine)
		}
	}
}

func TestPostgreSQLKeepsAdminCredentialSeparate(t *testing.T) {
	credential := repository.DatabaseCredential{DatabaseName: "app", Username: "hf_app"}
	spec, err := databaseContainerConfiguration("postgresql", credential, []byte("application-secret"), []byte("different-admin-secret"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(spec.Env, "\n")
	if !strings.Contains(joined, "POSTGRES_USER=hostforge_admin") || !strings.Contains(joined, "POSTGRES_PASSWORD=different-admin-secret") {
		t.Fatalf("PostgreSQL did not receive the separate administrator identity: %s", joined)
	}
	if strings.Contains(joined, "application-secret") || strings.Contains(joined, "POSTGRES_USER="+credential.Username) {
		t.Fatalf("PostgreSQL promoted application credentials into the bootstrap administrator: %s", joined)
	}
}

func TestPrepareManagedDatabaseAcceptsValidatedCustomResources(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "custom.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := repository.New(db)
	sealer, _ := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	app, _ := store.CreateApplication(ctx, "Custom resources", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := PrepareManagedDatabase(ctx, store, sealer, CreateManagedDatabaseInput{ApplicationID: app.ID, Name: "custom", Engine: "postgresql", Version: "18", EnvironmentIDs: []string{environments[0].ID}, ResourcePreset: "custom", CustomCPUMillis: 1500, CustomMemoryBytes: 3 * 1024 * 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if created.Instances[0].ResourcePreset != "custom" || created.Instances[0].CPULimitMillis != 1500 || created.Instances[0].MemoryLimitBytes != 3*1024*1024*1024 {
		t.Fatalf("custom limits were not persisted: %+v", created.Instances[0])
	}
}

func TestRedisFamilyPersistsRotatedConfigurationOnVolume(t *testing.T) {
	credential := repository.DatabaseCredential{DatabaseName: "0", Username: "default"}
	for _, engine := range []string{"redis", "valkey"} {
		spec, err := databaseContainerConfiguration(engine, credential, []byte("secret"), []byte("admin"))
		if err != nil {
			t.Fatal(err)
		}
		command := strings.Join(spec.Command, " ")
		if !strings.Contains(command, "/data/hostforge.conf") || !strings.Contains(command, "[ ! -f") {
			t.Fatalf("%s startup would overwrite retained credentials: %s", engine, command)
		}
	}
}

func TestRedisFamilyBackupsStreamRDBWithoutTemporaryFile(t *testing.T) {
	instance := repository.DatabaseInstance{NetworkAlias: "cache-staging", InternalPort: 6379}
	credential := repository.DatabaseCredential{DatabaseName: "0", Username: "default"}
	for _, engine := range []string{"redis", "valkey"} {
		t.Run(engine, func(t *testing.T) {
			command, env, format, err := databaseBackupCommand(engine, instance, credential, []byte("secret"))
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(command, " ")
			if !strings.Contains(joined, "--rdb -") || strings.Contains(joined, "/tmp/dump.rdb") {
				t.Fatalf("%s backup must stream RDB to stdout: %v", engine, command)
			}
			if !strings.Contains(strings.Join(env, "\n"), "REDISCLI_AUTH=secret") {
				t.Fatalf("%s backup did not use memory-only CLI authentication", engine)
			}
			if format != engine+"-rdb+gzip+aead" {
				t.Fatalf("unexpected archive format %q", format)
			}
		})
	}
}

func TestPostgreSQLRestoreDoesNotRequireDatabaseCreationPrivilege(t *testing.T) {
	instance := repository.DatabaseInstance{NetworkAlias: "postgres-staging", InternalPort: 5432}
	credential := repository.DatabaseCredential{DatabaseName: "hostforge_app", Username: "hf_app"}
	command, _, err := databaseRestoreCommand("postgresql", instance, credential, []byte("secret"), credential.DatabaseName)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command, " ")
	if strings.Contains(joined, "dropdb") || strings.Contains(joined, "createdb") {
		t.Fatalf("least-privilege PostgreSQL restore attempted database-level lifecycle operations: %s", joined)
	}
	for _, required := range []string{"pg_restore", "--clean", "--if-exists", "--no-owner", "--no-privileges"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("PostgreSQL restore is missing %q: %s", required, joined)
		}
	}
}

func TestDatabaseAdapterCommandsNeverContainPlaintextPasswords(t *testing.T) {
	instance := repository.DatabaseInstance{NetworkAlias: "database-staging", InternalPort: 5432}
	credential := repository.DatabaseCredential{DatabaseName: "hostforge_app", Username: "hf_app"}
	password := []byte("application-secret-marker")
	adminPassword := []byte("admin-secret-marker")
	for _, engine := range databases.Catalog() {
		t.Run(engine.ID, func(t *testing.T) {
			candidate := credential
			instance.InternalPort = engine.InternalPort
			if engine.ID == "redis" || engine.ID == "valkey" {
				candidate.DatabaseName, candidate.Username = "0", "default"
			}
			backupCommand, _, _, err := databaseBackupCommand(engine.ID, instance, candidate, password)
			if err != nil {
				t.Fatal(err)
			}
			restoreCommand, _, err := databaseRestoreCommand(engine.ID, instance, candidate, password, candidate.DatabaseName)
			if err != nil {
				t.Fatal(err)
			}
			healthCommand, _, err := databaseHealthCommand(engine.ID, candidate, password, adminPassword)
			if err != nil {
				t.Fatal(err)
			}
			applicationCommand, _, err := databaseApplicationCredentialCommand(engine.ID, candidate, password)
			if err != nil {
				t.Fatal(err)
			}
			commands := strings.Join(append(append(append(backupCommand, restoreCommand...), healthCommand...), applicationCommand...), " ")
			if strings.Contains(commands, string(password)) || strings.Contains(commands, string(adminPassword)) {
				t.Fatalf("%s adapter embedded a password in command metadata: %s", engine.ID, commands)
			}
		})
	}
}

func TestPostgreSQLHealthCheckWaitsForAuthenticatedTCP(t *testing.T) {
	credential := repository.DatabaseCredential{DatabaseName: "hostforge_app", Username: "hf_app"}
	command, env, err := databaseHealthCommand("postgresql", credential, []byte("application-secret"), []byte("admin-secret"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command, " ")
	for _, expected := range []string{"psql", "--host 127.0.0.1", "--username hostforge_admin", "--command SELECT 1"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("PostgreSQL health command is missing %q: %s", expected, joined)
		}
	}
	if len(env) != 1 || env[0] != "PGPASSWORD=admin-secret" {
		t.Fatalf("PostgreSQL health check did not receive the administrator password through exec environment: %v", env)
	}
}

func TestDatabaseRuntimeLogRedactionRemovesExactAndStructuredCredentials(t *testing.T) {
	logs := strings.Join([]string{
		`connection=postgresql://hf_app:p%40ss%3Aword@postgres:5432/app`,
		`POSTGRES_PASSWORD=bootstrap-secret`,
		`mongosh --password visible-command-secret`,
		`masterauth redis-replication-secret`,
		`ordinary health check passed`,
	}, "\n")
	redacted := RedactDatabaseLogs("redis", logs, []byte("p@ss:word"), []byte("bootstrap-secret"))
	for _, secret := range []string{"p@ss:word", "p%40ss%3Aword", "bootstrap-secret", "visible-command-secret", "redis-replication-secret"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("database logs retained credential %q: %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "ordinary health check passed") || strings.Count(redacted, "[REDACTED]") < 4 {
		t.Fatalf("database log redaction removed safe content or missed secrets: %s", redacted)
	}
}
