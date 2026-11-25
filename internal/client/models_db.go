package client

// Managed Postgres (ADR 0046). Field names follow platform/control-plane/openapi.json.

type DatabaseBackupIn struct {
	Enabled       bool   `json:"enabled"`
	RetentionDays *int64 `json:"retention_days,omitempty"`
}

type DatabaseIn struct {
	Name              string            `json:"name"`
	Region            *string           `json:"region,omitempty"`
	Zone              *string           `json:"zone,omitempty"`
	Description       string            `json:"description"`
	Tags              map[string]string `json:"tags"`
	Version           int64             `json:"version"`
	Plan              *string           `json:"plan,omitempty"`
	Mode              string            `json:"mode"`
	DiskGiB           *int64            `json:"disk_gib,omitempty"`
	VPCID             *string           `json:"vpc_id,omitempty"`
	SubnetID          *string           `json:"subnet_id,omitempty"`
	Backup            DatabaseBackupIn  `json:"backup"`
	MaintenanceWindow *string           `json:"maintenance_window,omitempty"`
}

type DatabaseBackupPatch struct {
	Enabled       *bool  `json:"enabled,omitempty"`
	RetentionDays *int64 `json:"retention_days,omitempty"`
}

type DatabasePatch struct {
	Description       *string              `json:"description,omitempty"`
	Tags              *map[string]string   `json:"tags,omitempty"`
	Plan              *string              `json:"plan,omitempty"`
	DiskGiB           *int64               `json:"disk_gib,omitempty"`
	Mode              *string              `json:"mode,omitempty"`
	Backup            *DatabaseBackupPatch `json:"backup,omitempty"`
	MaintenanceWindow *string              `json:"maintenance_window,omitempty"`
}

type DatabaseConnection struct {
	Host     string `json:"host"`
	Address  string `json:"address"`
	Port     int64  `json:"port"`
	User     string `json:"user"`
	Database string `json:"database"`
	SSLMode  string `json:"sslmode"`
}

type DatabaseBackup struct {
	Enabled       bool    `json:"enabled"`
	RetentionDays int64   `json:"retention_days"`
	Status        string  `json:"status"`
	LastSuccess   *string `json:"last_success"`
	SizeBytes     int64   `json:"size_bytes"`
}

type DatabasePublic struct {
	Available      bool     `json:"available"`
	Enabled        bool     `json:"enabled"`
	AllowAll       bool     `json:"allow_all"`
	AllowList      []string `json:"allow_list"`
	Host           *string  `json:"host"`
	Port           *int64   `json:"port"`
	FallbackEnable bool     `json:"fallback_enabled"`
	FallbackPort   *int64   `json:"fallback_port"`
}

type DatabaseExposureIn struct {
	Public    bool     `json:"public"`
	AllowList []string `json:"allow_list"`
	AllowAll  bool     `json:"allow_all"`
	Fallback  bool     `json:"fallback"`
}

type Database struct {
	Meta
	Region            string             `json:"region"`
	Zone              *string            `json:"zone"`
	Version           int64              `json:"version"`
	PostgresVersion   *string            `json:"postgres_version"`
	Plan              string             `json:"plan"`
	Mode              string             `json:"mode"`
	VCPUs             int64              `json:"vcpus"`
	CPUMillicores     int64              `json:"cpu_millicores"`
	DiskUsedBytes     *int64             `json:"disk_used_bytes"`
	MemoryMiB         int64              `json:"memory_mib"`
	DiskGiB           int64              `json:"disk_gib"`
	PowerState        string             `json:"power_state"`
	VPCID             string             `json:"vpc_id"`
	SubnetID          string             `json:"subnet_id"`
	Address           string             `json:"address"`
	Connection        DatabaseConnection `json:"connection"`
	CACertificate     string             `json:"ca_certificate"`
	Backup            DatabaseBackup     `json:"backup"`
	MaintenanceWindow string             `json:"maintenance_window"`
	Public            DatabasePublic     `json:"public"`
	KeptBackupUntil   *string            `json:"kept_backup_until"`
	// Only in the create answer.
	Password *string `json:"password"`
}

// DatabasePlan is one entry of GET /database-plans.
type DatabasePlan struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	VCPUs          int64  `json:"vcpus"`
	CPUMillicores  int64  `json:"cpu_millicores"`
	MemoryMiB      int64  `json:"memory_mib"`
	DefaultDiskGiB int64  `json:"default_disk_gib"`
	MaxDiskGiB     int64  `json:"max_disk_gib"`
	MaxConnections int64  `json:"max_connections"`
	Schedulable    bool   `json:"schedulable"`
}

// DatabaseVersions is GET /database-versions.
type DatabaseVersions struct {
	Versions                 []int64  `json:"versions"`
	Default                  int64    `json:"default"`
	MaintenanceWindows       []string `json:"maintenance_windows"`
	DefaultMaintenanceWindow string   `json:"default_maintenance_window"`
	HAEnabled                bool     `json:"ha_enabled"`
}
