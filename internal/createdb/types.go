package createdb

// Options configures a db create run.
type Options struct {
	Profile            string
	Region             string
	DBName             string
	Host               string
	Port               int
	Schema             string
	DefaultDB          string
	MigrationConnLimit int
	RWConnLimit        int
	ROConnLimit        int
	DryRun             bool
	Force              bool
	Args               []string
}

// UserCredentials holds a generated username, password, and role metadata.
type UserCredentials struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Role      string `json:"role"`
	ConnLimit int    `json:"-"`
}

// StepResult tracks the outcome of a single orchestration step.
type StepResult struct {
	Name   string
	Status string // "done", "FAILED", "SKIPPED"
	Error  error
}
