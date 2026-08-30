package services

import (
	"testing"

	"github.com/furious-fury/HostForge/internal/databases"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
)

func TestDatabaseRuntimeReconciliationDetectsDestructiveDrift(t *testing.T) {
	engine, ok := databases.Find("postgresql")
	if !ok {
		t.Fatal("PostgreSQL catalog entry is unavailable")
	}
	instance := repository.DatabaseInstance{ID: "instance-1", ServiceID: "service-1", EnvironmentID: "env-1", NetworkAlias: "postgres-staging", ImageRef: "postgres@sha256:pinned", VolumeName: "hostforge-db-test", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024}
	valid := docker.ManagedContainerInspection{ImageRef: instance.ImageRef, NanoCPUs: 500_000_000, MemoryBytes: instance.MemoryLimitBytes, Labels: map[string]string{docker.ResourceTypeLabel: "database-container", docker.InstanceIDLabel: instance.ID, docker.ServiceIDLabel: instance.ServiceID, docker.EnvironmentIDLabel: instance.EnvironmentID}, VolumeMounts: map[string]string{instance.VolumeName: engine.VolumeTarget}, NetworkAliases: map[string][]string{docker.EnvironmentNetworkName(instance.EnvironmentID): {instance.NetworkAlias}}}
	if databaseRuntimeConfigurationDrift(valid, instance, engine, true) {
		t.Fatal("matching managed runtime was reported as drifted")
	}
	tests := map[string]func(*docker.ManagedContainerInspection){
		"image":          func(value *docker.ManagedContainerInspection) { value.ImageRef = "postgres@sha256:other" },
		"cpu":            func(value *docker.ManagedContainerInspection) { value.NanoCPUs = 1 },
		"memory":         func(value *docker.ManagedContainerInspection) { value.MemoryBytes = 1 },
		"published port": func(value *docker.ManagedContainerInspection) { value.PublishedPorts = true },
		"volume target":  func(value *docker.ManagedContainerInspection) { value.VolumeMounts[instance.VolumeName] = "/wrong" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.VolumeMounts = map[string]string{instance.VolumeName: engine.VolumeTarget}
			mutate(&candidate)
			if !databaseRuntimeConfigurationDrift(candidate, instance, engine, true) {
				t.Fatalf("%s drift was accepted", name)
			}
		})
	}
	if !databaseRuntimeConfigurationDrift(valid, instance, engine, false) {
		t.Fatal("missing private network alias was accepted")
	}
	if databaseManagedContainerDrift(valid, instance, engine) {
		t.Fatal("fully matching deterministic container could not be adopted")
	}
	invalidOwner := valid
	invalidOwner.Labels = map[string]string{docker.ResourceTypeLabel: "database-container", docker.InstanceIDLabel: "another-instance", docker.ServiceIDLabel: instance.ServiceID, docker.EnvironmentIDLabel: instance.EnvironmentID}
	if !databaseManagedContainerDrift(invalidOwner, instance, engine) {
		t.Fatal("container with mismatched ownership could be adopted")
	}
}
