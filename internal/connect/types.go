package connect

// CacheVersion is incremented when InstanceInfo (or cache format) changes.
const CacheVersion = "v2"

// CacheEnvelope is the on-disk cache format for RDS instance list.
type CacheEnvelope struct {
	Version   string         `json:"version"`
	Instances []InstanceInfo `json:"instances"`
}

// InstanceInfo describes one RDS PostgreSQL instance.
type InstanceInfo struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Size     string `json:"size"`
	Port     int32  `json:"port"`
	Version  string `json:"version"`
	SourceID string `json:"source_id"`
}

// RDSCreds holds DB username/password from Secrets Manager.
type RDSCreds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// PritunlConnection represents one Pritunl VPN connection (from pritunl-client list -j).
type PritunlConnection struct {
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}
