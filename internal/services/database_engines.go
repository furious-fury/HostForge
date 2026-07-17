package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

type databaseContainerSpec struct {
	Env     []string
	Command []string
}

var databaseLogCredentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|requirepass)\b\s*(?:=|:)\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)(--password(?:=|\s+))[^\s]+`),
	regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^\s:/@]+:)[^\s@]+(@)`),
}

// RedactDatabaseLogs applies exact in-memory credential removal followed by
// conservative adapter-aware patterns. Callers must fail closed if credentials
// cannot be decrypted; returning unfiltered database logs is never acceptable.
func RedactDatabaseLogs(engine, logs string, secrets ...[]byte) string {
	redacted := logs
	for _, secret := range secrets {
		if len(secret) == 0 {
			continue
		}
		plaintext := string(secret)
		for _, candidate := range []string{plaintext, url.PathEscape(plaintext), url.QueryEscape(plaintext)} {
			if candidate != "" {
				redacted = strings.ReplaceAll(redacted, candidate, "[REDACTED]")
			}
		}
	}
	for _, pattern := range databaseLogCredentialPatterns {
		redacted = pattern.ReplaceAllString(redacted, `${1}[REDACTED]${2}`)
	}
	// Engine-specific configuration directives that do not use a password
	// label still receive a final narrowly scoped pass.
	if engine == "redis" || engine == "valkey" {
		redacted = regexp.MustCompile(`(?im)^(\s*masterauth\s+)[^\s]+`).ReplaceAllString(redacted, `${1}[REDACTED]`)
	}
	return redacted
}

func safeDatabaseOperationError(err error, secrets ...[]byte) error {
	if err == nil {
		return errors.New("database operation failed")
	}
	return errors.New(RedactDatabaseLogs("", err.Error(), secrets...))
}

func databaseBackupCommand(engine string, instance repository.DatabaseInstance, credential repository.DatabaseCredential, password []byte) (command, env []string, archiveFormat string, err error) {
	host, port := instance.NetworkAlias, fmt.Sprint(instance.InternalPort)
	switch engine {
	case "postgresql":
		return []string{"pg_dump", "--host", host, "--port", port, "--username", credential.Username, "--dbname", credential.DatabaseName, "--format", "custom", "--no-password"}, []string{"PGPASSWORD=" + string(password)}, "postgresql-custom+gzip+aead", nil
	case "mysql":
		return []string{"mysqldump", "--host", host, "--port", port, "--user", credential.Username, "--single-transaction", "--quick", "--routines", "--events", credential.DatabaseName}, []string{"MYSQL_PWD=" + string(password)}, "mysql-sql+gzip+aead", nil
	case "mariadb":
		return []string{"mariadb-dump", "--host", host, "--port", port, "--user", credential.Username, "--single-transaction", "--quick", "--routines", "--events", credential.DatabaseName}, []string{"MYSQL_PWD=" + string(password)}, "mariadb-sql+gzip+aead", nil
	case "mongodb":
		script := `exec mongodump --host "$HF_HOST" --port "$HF_PORT" --username "$HF_USER" --password "$MONGODB_PASSWORD" --authenticationDatabase "$HF_DATABASE" --db "$HF_DATABASE" --archive`
		return []string{"sh", "-c", script}, []string{"HOME=/tmp", "HF_HOST=" + host, "HF_PORT=" + port, "HF_USER=" + credential.Username, "HF_DATABASE=" + credential.DatabaseName, "MONGODB_PASSWORD=" + string(password)}, "mongodb-archive+gzip+aead", nil
	case "redis", "valkey":
		binary := "redis-cli"
		if engine == "valkey" {
			binary = "valkey-cli"
		}
		// Both CLIs treat '-' as stdout for --rdb. Stream the replication
		// snapshot directly into HostForge's compression/encryption pipeline so
		// backup size is not constrained by the job container's /tmp tmpfs.
		return []string{binary, "-h", host, "-p", port, "--rdb", "-"}, []string{"REDISCLI_AUTH=" + string(password), "HOME=/tmp"}, engine + "-rdb+gzip+aead", nil
	default:
		return nil, nil, "", fmt.Errorf("backup adapter %s is not implemented", engine)
	}
}

func databaseRestoreCommand(engine string, instance repository.DatabaseInstance, targetCredential repository.DatabaseCredential, password []byte, sourceDatabaseName string) (command, env []string, err error) {
	host, port := instance.NetworkAlias, fmt.Sprint(instance.InternalPort)
	switch engine {
	case "postgresql":
		// The application role owns the target database but deliberately has no
		// CREATEDB privilege. Clean and replace objects inside that database so a
		// restore never needs the HostForge administrator credential.
		script := `exec pg_restore --host "$HF_HOST" --port "$HF_PORT" --username "$HF_USER" --dbname "$HF_DATABASE" --clean --if-exists --no-owner --no-privileges --exit-on-error`
		return []string{"sh", "-c", script}, []string{"PGPASSWORD=" + string(password), "HF_HOST=" + host, "HF_PORT=" + port, "HF_USER=" + targetCredential.Username, "HF_DATABASE=" + targetCredential.DatabaseName}, nil
	case "mysql":
		script := `MYSQL_PWD="$HF_PASSWORD" mysql --host "$HF_HOST" --port "$HF_PORT" --user "$HF_USER" -e "DROP DATABASE IF EXISTS $HF_DATABASE; CREATE DATABASE $HF_DATABASE;" && MYSQL_PWD="$HF_PASSWORD" mysql --host "$HF_HOST" --port "$HF_PORT" --user "$HF_USER" "$HF_DATABASE"`
		return []string{"sh", "-c", script}, []string{"HF_HOST=" + host, "HF_PORT=" + port, "HF_DATABASE=" + targetCredential.DatabaseName, "HF_USER=" + targetCredential.Username, "HF_PASSWORD=" + string(password)}, nil
	case "mariadb":
		script := `MYSQL_PWD="$HF_PASSWORD" mariadb --host "$HF_HOST" --port "$HF_PORT" --user "$HF_USER" -e "DROP DATABASE IF EXISTS $HF_DATABASE; CREATE DATABASE $HF_DATABASE;" && MYSQL_PWD="$HF_PASSWORD" mariadb --host "$HF_HOST" --port "$HF_PORT" --user "$HF_USER" "$HF_DATABASE"`
		return []string{"sh", "-c", script}, []string{"HF_HOST=" + host, "HF_PORT=" + port, "HF_DATABASE=" + targetCredential.DatabaseName, "HF_USER=" + targetCredential.Username, "HF_PASSWORD=" + string(password)}, nil
	case "mongodb":
		script := `exec mongorestore --host "$HF_HOST" --port "$HF_PORT" --username "$HF_USER" --password "$MONGODB_PASSWORD" --authenticationDatabase "$HF_DATABASE" --archive --drop --nsFrom "$HF_SOURCE_NAMESPACE" --nsTo "$HF_TARGET_NAMESPACE"`
		return []string{"sh", "-c", script}, []string{"HOME=/tmp", "HF_HOST=" + host, "HF_PORT=" + port, "HF_USER=" + targetCredential.Username, "HF_DATABASE=" + targetCredential.DatabaseName, "HF_SOURCE_NAMESPACE=" + sourceDatabaseName + ".*", "HF_TARGET_NAMESPACE=" + targetCredential.DatabaseName + ".*", "MONGODB_PASSWORD=" + string(password)}, nil
	case "redis", "valkey":
		return []string{"sh", "-c", `rm -rf /data/appendonlydir /data/appendonly.aof /data/dump.rdb && cat > /data/dump.rdb && chmod 600 /data/dump.rdb`}, nil, nil
	default:
		return nil, nil, fmt.Errorf("restore adapter %s is not implemented", engine)
	}
}

func databaseContainerConfiguration(engine string, credential repository.DatabaseCredential, password, adminPassword []byte) (databaseContainerSpec, error) {
	switch engine {
	case "postgresql":
		return databaseContainerSpec{Env: []string{
			"POSTGRES_DB=" + credential.DatabaseName,
			"POSTGRES_USER=hostforge_admin",
			"POSTGRES_PASSWORD=" + string(adminPassword),
		}}, nil
	case "mysql":
		return databaseContainerSpec{Env: []string{
			"MYSQL_ROOT_PASSWORD=" + string(adminPassword), "MYSQL_DATABASE=" + credential.DatabaseName,
			"MYSQL_USER=" + credential.Username, "MYSQL_PASSWORD=" + string(password),
		}}, nil
	case "mariadb":
		return databaseContainerSpec{Env: []string{
			"MARIADB_ROOT_PASSWORD=" + string(adminPassword), "MARIADB_DATABASE=" + credential.DatabaseName,
			"MARIADB_USER=" + credential.Username, "MARIADB_PASSWORD=" + string(password),
		}}, nil
	case "mongodb":
		return databaseContainerSpec{Env: []string{
			"MONGO_INITDB_ROOT_USERNAME=hostforge_admin", "MONGO_INITDB_ROOT_PASSWORD=" + string(adminPassword),
		}}, nil
	case "redis":
		return databaseContainerSpec{Env: []string{"HF_DATABASE_PASSWORD=" + string(password)}, Command: []string{"sh", "-c", `if [ ! -f /data/hostforge.conf ]; then printf 'appendonly yes\nrequirepass %s\n' "$HF_DATABASE_PASSWORD" > /data/hostforge.conf && chmod 600 /data/hostforge.conf; fi; exec redis-server /data/hostforge.conf`}}, nil
	case "valkey":
		return databaseContainerSpec{Env: []string{"HF_DATABASE_PASSWORD=" + string(password)}, Command: []string{"sh", "-c", `if [ ! -f /data/hostforge.conf ]; then printf 'appendonly yes\nrequirepass %s\n' "$HF_DATABASE_PASSWORD" > /data/hostforge.conf && chmod 600 /data/hostforge.conf; fi; exec valkey-server /data/hostforge.conf`}}, nil
	default:
		return databaseContainerSpec{}, fmt.Errorf("database engine %s is not implemented", engine)
	}
}

func databaseHealthCommand(engine string, credential repository.DatabaseCredential, password, adminPassword []byte) ([]string, []string, error) {
	switch engine {
	case "postgresql":
		// The official image starts a temporary socket-only server while its
		// entrypoint initializes the data directory. Force an authenticated TCP
		// query so provisioning cannot advance until the final server is ready.
		return []string{"psql", "--host", "127.0.0.1", "--username", "hostforge_admin", "--dbname", credential.DatabaseName, "--tuples-only", "--command", "SELECT 1"}, []string{"PGPASSWORD=" + string(adminPassword)}, nil
	case "mysql":
		return []string{"mysqladmin", "ping", "-h", "127.0.0.1", "-uroot", "--silent"}, []string{"MYSQL_PWD=" + string(adminPassword)}, nil
	case "mariadb":
		return []string{"mariadb-admin", "ping", "-h", "127.0.0.1", "-uroot", "--silent"}, []string{"MYSQL_PWD=" + string(adminPassword)}, nil
	case "mongodb":
		return []string{"sh", "-c", `exec mongosh --quiet --username hostforge_admin --password "$MONGODB_ADMIN_PASSWORD" --authenticationDatabase admin --eval 'db.adminCommand({ping:1}).ok'`}, []string{"MONGODB_ADMIN_PASSWORD=" + string(adminPassword)}, nil
	case "redis", "valkey":
		binary := "redis-cli"
		if engine == "valkey" {
			binary = "valkey-cli"
		}
		return []string{binary, "ping"}, []string{"REDISCLI_AUTH=" + string(password)}, nil
	default:
		return nil, nil, fmt.Errorf("database health adapter %s is not implemented", engine)
	}
}

func databaseApplicationCredentialCommand(engine string, credential repository.DatabaseCredential, password []byte) ([]string, []string, error) {
	switch engine {
	case "postgresql":
		return []string{"psql", "--host", "127.0.0.1", "--username", credential.Username, "--dbname", credential.DatabaseName, "--tuples-only", "--command", "SELECT 1"}, []string{"PGPASSWORD=" + string(password)}, nil
	case "mysql":
		return []string{"mysql", "--host", "127.0.0.1", "--user", credential.Username, credential.DatabaseName, "--execute", "SELECT 1"}, []string{"MYSQL_PWD=" + string(password)}, nil
	case "mariadb":
		return []string{"mariadb", "--host", "127.0.0.1", "--user", credential.Username, credential.DatabaseName, "--execute", "SELECT 1"}, []string{"MYSQL_PWD=" + string(password)}, nil
	case "mongodb":
		return []string{"sh", "-c", `exec mongosh --quiet --username "$HF_USER" --password "$MONGODB_PASSWORD" --authenticationDatabase "$HF_DATABASE" "$HF_DATABASE" --eval 'db.runCommand({ping:1}).ok'`}, []string{"HF_USER=" + credential.Username, "HF_DATABASE=" + credential.DatabaseName, "MONGODB_PASSWORD=" + string(password)}, nil
	case "redis", "valkey":
		binary := "redis-cli"
		if engine == "valkey" {
			binary = "valkey-cli"
		}
		return []string{binary, "ping"}, []string{"REDISCLI_AUTH=" + string(password)}, nil
	default:
		return nil, nil, fmt.Errorf("database application credential adapter %s is not implemented", engine)
	}
}

func alterDatabasePassword(ctx context.Context, client *mobyclient.Client, containerID, engine string, credential repository.DatabaseCredential, currentPassword, newPassword, adminPassword []byte) error {
	if strings.ContainsAny(string(newPassword), "'\\\n\r\x00") {
		return errors.New("generated database password contains unsafe characters")
	}
	switch engine {
	case "postgresql":
		return alterPostgreSQLPassword(ctx, client, containerID, credential.Username, credential.DatabaseName, newPassword, adminPassword)
	case "mysql", "mariadb":
		binary := "mysql"
		if engine == "mariadb" {
			binary = "mariadb"
		}
		exitCode, err := docker.ExecExitCode(ctx, client, containerID, []string{
			binary, "-h", "127.0.0.1", "-u", credential.Username, credential.DatabaseName,
			"-e", "ALTER USER USER() IDENTIFIED BY '" + string(newPassword) + "'",
		}, []string{"MYSQL_PWD=" + string(currentPassword)})
		if err != nil {
			return err
		}
		if exitCode != 0 {
			return fmt.Errorf("%s password command exited with code %d", engine, exitCode)
		}
		return nil
	case "mongodb":
		for _, value := range []string{credential.DatabaseName, credential.Username} {
			if value == "" || strings.ContainsAny(value, "'\\\n\r\x00") {
				return errors.New("MongoDB credential contains unsafe characters")
			}
		}
		script := fmt.Sprintf(`db=db.getSiblingDB('%s'); db.updateUser('%s',{pwd:process.env.HF_NEW_PASSWORD,roles:[{role:'readWrite',db:'%s'}]})`, credential.DatabaseName, credential.Username, credential.DatabaseName)
		exitCode, err := docker.ExecExitCode(ctx, client, containerID, []string{
			"sh", "-c", `exec mongosh --quiet --username hostforge_admin --password "$MONGODB_ADMIN_PASSWORD" --authenticationDatabase admin --eval "$HF_MONGO_SCRIPT"`,
		}, []string{"MONGODB_ADMIN_PASSWORD=" + string(adminPassword), "HF_NEW_PASSWORD=" + string(newPassword), "HF_MONGO_SCRIPT=" + script})
		if err != nil {
			return err
		}
		if exitCode != 0 {
			return fmt.Errorf("MongoDB password command exited with code %d", exitCode)
		}
		return nil
	case "redis", "valkey":
		binary := "redis-cli"
		if engine == "valkey" {
			binary = "valkey-cli"
		}
		if exitCode, err := docker.ExecExitCode(ctx, client, containerID, []string{binary, "CONFIG", "SET", "requirepass", string(newPassword)}, []string{"REDISCLI_AUTH=" + string(currentPassword)}); err != nil || exitCode != 0 {
			return fmt.Errorf("%s live password update failed", engine)
		}
		if exitCode, err := docker.ExecExitCode(ctx, client, containerID, []string{binary, "CONFIG", "REWRITE"}, []string{"REDISCLI_AUTH=" + string(newPassword)}); err != nil || exitCode != 0 {
			_, _ = docker.ExecExitCode(ctx, client, containerID, []string{binary, "CONFIG", "SET", "requirepass", string(currentPassword)}, []string{"REDISCLI_AUTH=" + string(newPassword)})
			_, _ = docker.ExecExitCode(ctx, client, containerID, []string{binary, "CONFIG", "REWRITE"}, []string{"REDISCLI_AUTH=" + string(currentPassword)})
			return fmt.Errorf("%s password persistence failed", engine)
		}
		return nil
	default:
		return fmt.Errorf("credential rotation adapter %s is not implemented", engine)
	}
}

func checkDatabaseReadiness(ctx context.Context, client *mobyclient.Client, containerID, engine string, credential repository.DatabaseCredential, password, adminPassword []byte) error {
	command, env, err := databaseHealthCommand(engine, credential, password, adminPassword)
	if err != nil {
		return err
	}
	exitCode, err := docker.ExecExitCode(ctx, client, containerID, command, env)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("%s readiness command exited with code %d", engine, exitCode)
	}
	return nil
}

func checkDatabaseHealth(ctx context.Context, client *mobyclient.Client, containerID, engine string, credential repository.DatabaseCredential, password, adminPassword []byte) error {
	if err := checkDatabaseReadiness(ctx, client, containerID, engine, credential, password, adminPassword); err != nil {
		return err
	}
	return checkDatabaseApplicationCredentials(ctx, client, containerID, engine, credential, password)
}

func checkDatabaseApplicationCredentials(ctx context.Context, client *mobyclient.Client, containerID, engine string, credential repository.DatabaseCredential, password []byte) error {
	command, env, err := databaseApplicationCredentialCommand(engine, credential, password)
	if err != nil {
		return err
	}
	exitCode, err := docker.ExecExitCode(ctx, client, containerID, command, env)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("%s application credential check exited with code %d", engine, exitCode)
	}
	return nil
}

func configureDatabaseAfterStart(ctx context.Context, client *mobyclient.Client, containerID, engine string, credential repository.DatabaseCredential, password, adminPassword []byte) error {
	if engine == "postgresql" {
		for _, value := range []string{credential.DatabaseName, credential.Username} {
			if value == "" || strings.ContainsAny(value, "'\"\\\n\r\x00") {
				return errors.New("PostgreSQL credential contains unsafe characters")
			}
		}
		if strings.ContainsAny(string(password), "'\\\n\r\x00") {
			return errors.New("generated PostgreSQL password contains unsafe characters")
		}
		script := `set -eu
exists="$(PGPASSWORD="$HF_ADMIN_PASSWORD" psql --host 127.0.0.1 --username hostforge_admin --dbname "$HF_DATABASE" --tuples-only --no-align --command "SELECT 1 FROM pg_roles WHERE rolname='$HF_USER'")"
{
  if [ "$exists" = "1" ]; then
    printf 'ALTER ROLE "%s" WITH LOGIN PASSWORD '\''%s'\'';\n' "$HF_USER" "$HF_APPLICATION_PASSWORD"
  else
    printf 'CREATE ROLE "%s" WITH LOGIN PASSWORD '\''%s'\'';\n' "$HF_USER" "$HF_APPLICATION_PASSWORD"
  fi
  printf 'ALTER DATABASE "%s" OWNER TO "%s";\n' "$HF_DATABASE" "$HF_USER"
} | PGPASSWORD="$HF_ADMIN_PASSWORD" psql --host 127.0.0.1 --username hostforge_admin --dbname "$HF_DATABASE" --set ON_ERROR_STOP=1`
		exitCode, err := docker.ExecExitCode(ctx, client, containerID, []string{"sh", "-c", script}, []string{"HF_ADMIN_PASSWORD=" + string(adminPassword), "HF_APPLICATION_PASSWORD=" + string(password), "HF_USER=" + credential.Username, "HF_DATABASE=" + credential.DatabaseName})
		if err != nil {
			return err
		}
		if exitCode != 0 {
			return fmt.Errorf("PostgreSQL application-user setup exited with code %d", exitCode)
		}
		return nil
	}
	if engine != "mongodb" {
		return nil
	}
	for _, value := range []string{credential.DatabaseName, credential.Username, string(password)} {
		if value == "" || strings.ContainsAny(value, "'\\\n\r\x00") {
			return errors.New("MongoDB credential contains unsafe characters")
		}
	}
	script := fmt.Sprintf(`db=db.getSiblingDB('%s'); if(db.getUser('%s')){db.updateUser('%s',{pwd:process.env.HF_APPLICATION_PASSWORD,roles:[{role:'readWrite',db:'%s'}]})}else{db.createUser({user:'%s',pwd:process.env.HF_APPLICATION_PASSWORD,roles:[{role:'readWrite',db:'%s'}]})}`,
		credential.DatabaseName, credential.Username, credential.Username, credential.DatabaseName,
		credential.Username, credential.DatabaseName)
	exitCode, err := docker.ExecExitCode(ctx, client, containerID, []string{
		"sh", "-c", `exec mongosh --quiet --username hostforge_admin --password "$MONGODB_ADMIN_PASSWORD" --authenticationDatabase admin --eval "$HF_MONGO_SCRIPT"`,
	}, []string{"MONGODB_ADMIN_PASSWORD=" + string(adminPassword), "HF_APPLICATION_PASSWORD=" + string(password), "HF_MONGO_SCRIPT=" + script})
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("MongoDB application-user setup exited with code %d", exitCode)
	}
	return nil
}
