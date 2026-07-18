package repository

import (
	"context"
	"testing"
	"time"
)

func TestRollbackDatabaseExternalCredentialRotationRestoresWorkingGeneration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	instance := createGatewayTestInstance(t, store)
	if _, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2"); err != nil {
		t.Fatal(err)
	}
	connection, _, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{
		Name: "Rotating client", PermissionProfile: "read_write", CIDRs: []string{"198.51.100.8/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	previous, err := store.CreateDatabaseExternalCredential(ctx, connection.ID, []byte("previous-password"), []byte("previous-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetDatabaseExternalConnectionStatus(ctx, connection.ID, "active", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueueDatabaseExternalConnectionAction(ctx, connection.ID, "rotate", 24, "operator"); err != nil {
		t.Fatal(err)
	}
	failed, err := store.CreateDatabaseExternalCredential(ctx, connection.ID, []byte("failed-password"), []byte("failed-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RollbackDatabaseExternalCredentialRotation(ctx, connection.ID, failed.ID, previous.Generation); err != nil {
		t.Fatal(err)
	}
	restored, err := store.GetDatabaseExternalConnection(ctx, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != "active" || restored.CurrentGeneration != previous.Generation {
		t.Fatalf("restored connection status=%q generation=%d", restored.Status, restored.CurrentGeneration)
	}
	previousSealed, err := store.GetDatabaseExternalCredentialSealed(ctx, previous.ID)
	if err != nil {
		t.Fatal(err)
	}
	failedSealed, err := store.GetDatabaseExternalCredentialSealed(ctx, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previousSealed.State != "active" || len(previousSealed.PasswordCT) == 0 || len(previousSealed.SCRAMVerifierCT) == 0 {
		t.Fatalf("previous credential was not preserved: %+v", previousSealed)
	}
	if failedSealed.State != "revoked" || len(failedSealed.PasswordCT) != 0 || len(failedSealed.SCRAMVerifierCT) != 0 || failedSealed.RevokedAt.IsZero() {
		t.Fatalf("failed credential was not precisely erased: %+v", failedSealed)
	}
}

func TestTouchDatabaseExternalCredentialUsageUpdatesCredentialAndConnection(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	instance := createGatewayTestInstance(t, store)
	if _, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2"); err != nil {
		t.Fatal(err)
	}
	connection, _, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{
		Name: "Usage client", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.8/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := store.CreateDatabaseExternalCredential(ctx, connection.ID, []byte("sealed-password"), []byte("sealed-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	if err := store.TouchDatabaseExternalCredentialUsage(ctx, []string{credential.RoleName, credential.RoleName, "hfc_missing"}, at); err != nil {
		t.Fatal(err)
	}
	usedCredential, err := store.GetDatabaseExternalCredentialSealed(ctx, credential.ID)
	if err != nil {
		t.Fatal(err)
	}
	usedConnection, err := store.GetDatabaseExternalConnection(ctx, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !usedCredential.LastUsedAt.Equal(at) {
		t.Fatalf("credential last use=%s want=%s", usedCredential.LastUsedAt, at)
	}
	if !usedConnection.LastUsedAt.Equal(at) {
		t.Fatalf("connection last use=%s want=%s", usedConnection.LastUsedAt, at)
	}
}
