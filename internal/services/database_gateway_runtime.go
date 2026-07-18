package services

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/hostforge/hostforge/internal/docker"
	mobyclient "github.com/moby/moby/client"
)

type DockerPostgreSQLGatewayRuntime struct {
	Client             *mobyclient.Client
	GatewayContainerID string
}

func (runtime *DockerPostgreSQLGatewayRuntime) RunSQL(ctx context.Context, containerID, databaseName, adminPassword, script string, environment []string) (string, error) {
	if runtime == nil || runtime.Client == nil || strings.TrimSpace(containerID) == "" || !postgresIdentifier(databaseName) || strings.TrimSpace(adminPassword) == "" || strings.TrimSpace(script) == "" {
		return "", errors.New("complete PostgreSQL administrative target required")
	}
	command := []string{"sh", "-ceu", `exec psql --host 127.0.0.1 --username hostforge_admin --dbname "$HF_DATABASE" --quiet --tuples-only --no-align --set ON_ERROR_STOP=1 --command "$HF_GATEWAY_SQL"`}
	env := append([]string(nil), environment...)
	env = append(env, "PGPASSWORD="+adminPassword, "HF_DATABASE="+databaseName, "HF_GATEWAY_SQL="+script)
	return docker.ExecOutput(ctx, runtime.Client, containerID, command, env)
}

func (runtime *DockerPostgreSQLGatewayRuntime) runAdmin(ctx context.Context, command string, csvOutput bool) (string, error) {
	if runtime == nil || runtime.Client == nil || strings.TrimSpace(runtime.GatewayContainerID) == "" || strings.TrimSpace(command) == "" {
		return "", errors.New("PgBouncer administrative runtime is unavailable")
	}
	arguments := []string{"psql", "--host", "/run/pgbouncer", "--username", "hostforge_gateway_admin", "--dbname", "pgbouncer", "--set", "ON_ERROR_STOP=1"}
	if csvOutput {
		arguments = append(arguments, "--csv")
	} else {
		arguments = append(arguments, "--quiet", "--tuples-only", "--no-align")
	}
	arguments = append(arguments, "--command", command)
	return docker.ExecOutput(ctx, runtime.Client, runtime.GatewayContainerID, arguments, nil)
}

func (runtime *DockerPostgreSQLGatewayRuntime) ReloadPgBouncer(ctx context.Context) error {
	_, err := runtime.runAdmin(ctx, "RELOAD", false)
	return err
}

func (runtime *DockerPostgreSQLGatewayRuntime) ProbePostgreSQL(ctx context.Context, request GatewayProbeRequest) error {
	if !gatewayIdentifier(request.Username) || !gatewayIdentifier(request.Alias) || strings.TrimSpace(request.Password) == "" || strings.TrimSpace(request.Hostname) == "" || request.Port < 1 || request.Port > 65535 {
		return errors.New("complete PostgreSQL gateway probe required")
	}
	if runtime == nil || runtime.Client == nil || strings.TrimSpace(runtime.GatewayContainerID) == "" {
		return errors.New("PostgreSQL gateway probe runtime is unavailable")
	}
	command := []string{"psql", "--host", request.Hostname, "--port", strconv.Itoa(request.Port), "--username", request.Username, "--dbname", request.Alias, "--quiet", "--tuples-only", "--no-align", "--command", "SELECT 1"}
	output, err := docker.ExecOutput(ctx, runtime.Client, runtime.GatewayContainerID, command, postgreSQLGatewayProbeEnvironment(request.Password))
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) != "1" {
		return errors.New("PostgreSQL gateway probe returned an unexpected response")
	}
	return nil
}

func postgreSQLGatewayProbeEnvironment(password string) []string {
	return []string{
		"PGPASSWORD=" + password,
		"PGSSLMODE=verify-full",
		"PGSSLROOTCERT=system",
		"PGHOSTADDR=127.0.0.1",
	}
}

func ValidatePostgreSQLGatewayContainerImage(ctx context.Context, client *mobyclient.Client, containerID, declaredVersion string) error {
	if client == nil || strings.TrimSpace(containerID) == "" {
		return errors.New("PostgreSQL gateway container is unavailable for image validation")
	}
	pgBouncerOutput, err := docker.ExecOutput(ctx, client, containerID, []string{"pgbouncer", "--version"}, nil)
	if err != nil {
		return fmt.Errorf("PgBouncer binary preflight failed: %w", err)
	}
	psqlOutput, err := docker.ExecOutput(ctx, client, containerID, []string{"psql", "--version"}, nil)
	if err != nil {
		return fmt.Errorf("psql binary preflight failed: %w", err)
	}
	return ValidatePostgreSQLGatewayRuntimeVersions(pgBouncerOutput, psqlOutput, declaredVersion)
}

func ValidatePostgreSQLGatewayRuntimeVersions(pgBouncerOutput, psqlOutput, declaredVersion string) error {
	actualVersion := ""
	for _, line := range strings.Split(pgBouncerOutput, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && strings.EqualFold(fields[0], "pgbouncer") {
			actualVersion = strings.TrimPrefix(fields[1], "v")
			break
		}
	}
	if actualVersion == "" {
		return errors.New("PgBouncer binary did not report a version")
	}
	if err := ValidatePgBouncerImage("runtime@sha256:verified", actualVersion); err != nil {
		return err
	}
	declaredVersion = strings.TrimPrefix(strings.TrimSpace(declaredVersion), "v")
	if actualVersion != declaredVersion {
		return fmt.Errorf("PgBouncer binary version %s does not match declared version %s", actualVersion, declaredVersion)
	}
	psqlMajor := 0
	for _, line := range strings.Split(psqlOutput, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 || !strings.EqualFold(fields[0], "psql") {
			continue
		}
		version := strings.TrimPrefix(fields[2], "v")
		majorText := strings.SplitN(version, ".", 2)[0]
		major, err := strconv.Atoi(majorText)
		if err == nil {
			psqlMajor = major
			break
		}
	}
	if psqlMajor < 16 {
		return errors.New("PostgreSQL psql 16 or newer is required for the gateway TLS probe")
	}
	return nil
}

func (runtime *DockerPostgreSQLGatewayRuntime) TerminatePgBouncer(ctx context.Context, request GatewayTerminationRequest) error {
	if len(request.RoleNames) == 0 && request.RouteAlias == "" {
		return nil
	}
	roleNames := map[string]struct{}{}
	for _, roleName := range request.RoleNames {
		if !gatewayIdentifier(roleName) {
			return errors.New("unsafe role in PgBouncer termination request")
		}
		roleNames[roleName] = struct{}{}
	}
	output, err := runtime.runAdmin(ctx, "SHOW CLIENTS", true)
	if err != nil {
		return err
	}
	clientIDs, err := pgBouncerClientIDs(output, roleNames, request.RouteAlias)
	if err != nil {
		return err
	}
	for _, clientID := range clientIDs {
		if _, err := runtime.runAdmin(ctx, "KILL_CLIENT "+clientID, false); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *DockerPostgreSQLGatewayRuntime) ActivePgBouncerRoles(ctx context.Context) ([]string, error) {
	output, err := runtime.runAdmin(ctx, "SHOW CLIENTS", true)
	if err != nil {
		return nil, err
	}
	return pgBouncerActiveRoles(output)
}

func pgBouncerActiveRoles(output string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(output))
	headings, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read PgBouncer client headings: %w", err)
	}
	userColumn := -1
	for index, heading := range headings {
		if strings.EqualFold(strings.TrimSpace(heading), "user") {
			userColumn = index
			break
		}
	}
	if userColumn < 0 {
		return nil, errors.New("PgBouncer SHOW CLIENTS output lacks a user column")
	}
	roles := map[string]struct{}{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read PgBouncer client row: %w", err)
		}
		if len(record) <= userColumn {
			continue
		}
		role := strings.TrimSpace(record[userColumn])
		if strings.HasPrefix(role, "hfc_") && gatewayIdentifier(role) {
			roles[role] = struct{}{}
		}
	}
	result := make([]string, 0, len(roles))
	for role := range roles {
		result = append(result, role)
	}
	sort.Strings(result)
	return result, nil
}

func pgBouncerClientIDs(output string, roleNames map[string]struct{}, routeAlias string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(output))
	headings, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read PgBouncer client headings: %w", err)
	}
	columns := map[string]int{}
	for index, heading := range headings {
		columns[strings.ToLower(strings.TrimSpace(heading))] = index
	}
	userColumn, userFound := columns["user"]
	databaseColumn, databaseFound := columns["database"]
	clientIDColumn, clientIDFound := columns["client_id"]
	if !clientIDFound {
		clientIDColumn, clientIDFound = columns["id"]
	}
	if !userFound || !databaseFound || !clientIDFound {
		return nil, errors.New("PgBouncer SHOW CLIENTS output lacks targeted revocation columns")
	}
	clientIDs := []string{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read PgBouncer client row: %w", err)
		}
		if len(record) <= userColumn || len(record) <= databaseColumn || len(record) <= clientIDColumn {
			continue
		}
		_, roleMatch := roleNames[record[userColumn]]
		routeMatch := routeAlias != "" && record[databaseColumn] == routeAlias
		if !roleMatch && !routeMatch {
			continue
		}
		clientID := strings.TrimSpace(record[clientIDColumn])
		if !safePgBouncerClientID(clientID) {
			return nil, errors.New("PgBouncer returned an unsafe client identifier")
		}
		clientIDs = append(clientIDs, clientID)
	}
	return clientIDs, nil
}

func safePgBouncerClientID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && !strings.ContainsRune("._:-", character) {
			return false
		}
	}
	return true
}
