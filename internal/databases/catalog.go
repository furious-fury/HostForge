// Package databases defines the server-owned catalog for persistent database
// engines. Runtime image digests are added by engine adapters; clients must not
// be allowed to submit arbitrary images, ports, or volume paths.
package databases

type Version struct {
	Version               string `json:"version"`
	Default               bool   `json:"default"`
	ProvisioningAvailable bool   `json:"provisioning_available"`
	ImageRef              string `json:"-"`
}

type Engine struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Description           string    `json:"description"`
	Category              string    `json:"category"`
	Versions              []Version `json:"versions"`
	InternalPort          int       `json:"internal_port"`
	ConnectionVariable    string    `json:"connection_variable"`
	VolumeTarget          string    `json:"-"`
	MinimumMemoryBytes    int64     `json:"minimum_memory_bytes"`
	StopTimeoutSeconds    int       `json:"-"`
	PublicAccessAvailable bool      `json:"public_access_available"`
}

type ResourcePreset struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	CPULimitMillis   int    `json:"cpu_limit_millis"`
	MemoryLimitBytes int64  `json:"memory_limit_bytes"`
}

var catalog = []Engine{
	{
		ID: "postgresql", Name: "PostgreSQL", Category: "Relational",
		Description: "General-purpose relational database with strong SQL and extension support.",
		Versions: []Version{
			{Version: "18", Default: true, ProvisioningAvailable: true, ImageRef: "postgres@sha256:c2d42a104eb6b37b286a2d9c5cf83f349de4d6516d513d00a2bd9610e2c2e5e4"},
			{Version: "17", ProvisioningAvailable: true, ImageRef: "postgres@sha256:39fb82e41109483c81ac15422a302500b4a753777b47f8431038703536bc6c52"},
			{Version: "16", ProvisioningAvailable: true, ImageRef: "postgres@sha256:17e67d7b9890c99b055ba1e0d5c5be4ec27c9d3a72bda32db24a5e5d8a85af0c"},
		},
		InternalPort: 5432, ConnectionVariable: "DATABASE_URL",
		// Mount the parent so PostgreSQL 18's versioned PGDATA layout and the
		// older /var/lib/postgresql/data layout are both retained safely.
		VolumeTarget: "/var/lib/postgresql", MinimumMemoryBytes: 512 * 1024 * 1024, StopTimeoutSeconds: 30,
	},
	{
		ID: "mysql", Name: "MySQL", Category: "Relational",
		Description:  "Widely supported relational database using the MySQL protocol.",
		Versions:     []Version{{Version: "8.4", Default: true, ProvisioningAvailable: true, ImageRef: "mysql@sha256:c831a0f11348d402b43d77453e17d770be2eef356615a2823fe0f5a0d6c8b9af"}},
		InternalPort: 3306, ConnectionVariable: "DATABASE_URL",
		VolumeTarget: "/var/lib/mysql", MinimumMemoryBytes: 1024 * 1024 * 1024, StopTimeoutSeconds: 60,
	},
	{
		ID: "mariadb", Name: "MariaDB", Category: "Relational",
		Description:  "Open-source MySQL-compatible relational database.",
		Versions:     []Version{{Version: "11.4", Default: true, ProvisioningAvailable: true, ImageRef: "mariadb@sha256:a794d9eb009e20de605858a11f32f63b4075cbd197c650436f0e3b457e4caed7"}},
		InternalPort: 3306, ConnectionVariable: "DATABASE_URL",
		VolumeTarget: "/var/lib/mysql", MinimumMemoryBytes: 1024 * 1024 * 1024, StopTimeoutSeconds: 60,
	},
	{
		ID: "mongodb", Name: "MongoDB", Category: "Document",
		Description:  "Document database for flexible JSON-like application data.",
		Versions:     []Version{{Version: "8.0", Default: true, ProvisioningAvailable: true, ImageRef: "mongo@sha256:3ce3de7f40e914034b03b7dec654005ab54f7dc8306937e44ec6760d9e9409a1"}},
		InternalPort: 27017, ConnectionVariable: "MONGODB_URL",
		VolumeTarget: "/data/db", MinimumMemoryBytes: 1024 * 1024 * 1024, StopTimeoutSeconds: 60,
	},
	{
		ID: "redis", Name: "Redis", Category: "Key-value",
		Description: "In-memory data store for caching, queues, sessions, and realtime workloads.",
		Versions: []Version{
			{Version: "8.8", Default: true, ProvisioningAvailable: true, ImageRef: "redis@sha256:234c902a2db49461a129e2d4aeff85b28cf20187ed274a67f6e50995fa713c7b"},
			{Version: "8.4", ProvisioningAvailable: true, ImageRef: "redis@sha256:c44528447fa07ed62bdb0c1944cba54f8cad6a4e4a49ada9d4843b5b07d03227"},
		},
		InternalPort: 6379, ConnectionVariable: "REDIS_URL",
		VolumeTarget: "/data", MinimumMemoryBytes: 512 * 1024 * 1024, StopTimeoutSeconds: 30,
	},
	{
		ID: "valkey", Name: "Valkey", Category: "Key-value",
		Description: "Open-source key-value data store compatible with the Redis protocol.",
		Versions: []Version{
			{Version: "9.0", Default: true, ProvisioningAvailable: true, ImageRef: "valkey/valkey@sha256:bdf93f670fdb026eba9e2cf852c3fa8062f92e850fa181626c3b056e83ef04cf"},
			{Version: "8.1", ProvisioningAvailable: true, ImageRef: "valkey/valkey@sha256:3e31dd49b6b742e614975e8ab7b1b19809d00ecac7657c6b34bff23582a433cd"},
		},
		InternalPort: 6379, ConnectionVariable: "VALKEY_URL",
		VolumeTarget: "/data", MinimumMemoryBytes: 512 * 1024 * 1024, StopTimeoutSeconds: 30,
	},
}

var resourcePresets = []ResourcePreset{
	{ID: "development", Name: "Development", Description: "Staging, prototypes, and low traffic.", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024},
	{ID: "standard", Name: "Standard", Description: "Small production workloads.", CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024},
	{ID: "performance", Name: "Performance", Description: "Heavier production workloads.", CPULimitMillis: 2000, MemoryLimitBytes: 4 * 1024 * 1024 * 1024},
	{ID: "custom", Name: "Custom", Description: "Operator-selected CPU and memory limits."},
}

func Catalog() []Engine {
	out := make([]Engine, len(catalog))
	for index, engine := range catalog {
		out[index] = engine
		out[index].Versions = append([]Version(nil), engine.Versions...)
	}
	return out
}

func ResourcePresets() []ResourcePreset {
	return append([]ResourcePreset(nil), resourcePresets...)
}

func FindResourcePreset(presetID string) (ResourcePreset, bool) {
	for _, preset := range resourcePresets {
		if preset.ID == presetID {
			return preset, true
		}
	}
	return ResourcePreset{}, false
}

func Find(engineID string) (Engine, bool) {
	for _, engine := range catalog {
		if engine.ID == engineID {
			engine.Versions = append([]Version(nil), engine.Versions...)
			return engine, true
		}
	}
	return Engine{}, false
}

func FindVersion(engineID, version string) (Engine, Version, bool) {
	engine, ok := Find(engineID)
	if !ok {
		return Engine{}, Version{}, false
	}
	for _, candidate := range engine.Versions {
		if candidate.Version == version {
			return engine, candidate, true
		}
	}
	return Engine{}, Version{}, false
}
