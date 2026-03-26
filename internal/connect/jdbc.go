package connect

import (
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

const jdbcPrefix = "jdbc:postgresql://"

// ParseJDBCURL extracts host, port, and database from a JDBC PostgreSQL URL.
// Expected format: jdbc:postgresql://host[:port][/database][?params]
// User/password in the URL are ignored (always fetched from Secrets Manager).
func ParseJDBCURL(raw string) (host string, port int, database string, err error) {
	if !strings.HasPrefix(raw, jdbcPrefix) {
		return "", 0, "", fmt.Errorf("URL must start with %s", jdbcPrefix)
	}

	trimmed := strings.TrimPrefix(raw, "jdbc:")
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid JDBC URL: %w", err)
	}

	host = u.Hostname()
	if host == "" {
		return "", 0, "", fmt.Errorf("missing host in JDBC URL")
	}

	port = 5432
	if portStr := u.Port(); portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid port %q: %w", portStr, err)
		}
		port = p
	}

	database = "postgres"
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		database = path
	}

	return host, port, database, nil
}

// ResolveCNAME follows the CNAME chain for a hostname until it stabilizes.
// Returns the final resolved hostname (e.g. the actual RDS endpoint behind
// a Route53 alias). If no CNAME exists, the original host is returned.
func ResolveCNAME(host string) (string, error) {
	resolved := host
	for i := 0; i < 10; i++ {
		cname, err := net.LookupCNAME(resolved)
		if err != nil {
			return resolved, nil
		}
		cname = strings.TrimSuffix(cname, ".")
		if cname == resolved || cname == "" {
			break
		}
		resolved = cname
	}
	return resolved, nil
}

// BuildJDBCURL constructs a JDBC PostgreSQL URL with URL-encoded credentials.
func BuildJDBCURL(host string, port int32, user, password, database string) string {
	return fmt.Sprintf("jdbc:postgresql://%s:%d/%s?user=%s&password=%s&sslmode=require",
		host, port,
		url.PathEscape(database),
		url.QueryEscape(user),
		url.QueryEscape(password),
	)
}

// copyToClipboard copies text to the system clipboard using pbcopy (macOS).
func copyToClipboard(text string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		fmt.Printf("⚠️  Could not copy to clipboard: %v\n", err)
		return
	}
	fmt.Println("📋 Copied to clipboard!")
}
