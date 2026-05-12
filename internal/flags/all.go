package flags

// List of flag names. Using constants for better management of flag names. See
// cmd/main.go for flag meanings
const (
	Password       = "password"
	User           = "user"
	PrivateKeyPath = "ssh-key-path"
	PgCertPath     = "pg-cert"
	GithubToken    = "github-token"
	NginxApiKey    = "nginx-api-key"
)
