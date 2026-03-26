package connect

import (
	"testing"
)

func TestParseJDBCURL_Full(t *testing.T) {
	host, port, db, err := ParseJDBCURL("jdbc:postgresql://my-rds.abc.rds.amazonaws.com:5433/myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "my-rds.abc.rds.amazonaws.com" {
		t.Errorf("host: got %q", host)
	}
	if port != 5433 {
		t.Errorf("port: got %d", port)
	}
	if db != "myapp" {
		t.Errorf("database: got %q", db)
	}
}

func TestParseJDBCURL_DefaultPort(t *testing.T) {
	host, port, db, err := ParseJDBCURL("jdbc:postgresql://my-rds.example.com/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "my-rds.example.com" {
		t.Errorf("host: got %q", host)
	}
	if port != 5432 {
		t.Errorf("port: got %d, want 5432", port)
	}
	if db != "mydb" {
		t.Errorf("database: got %q", db)
	}
}

func TestParseJDBCURL_DefaultDatabase(t *testing.T) {
	host, port, db, err := ParseJDBCURL("jdbc:postgresql://my-rds.example.com:5432")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "my-rds.example.com" {
		t.Errorf("host: got %q", host)
	}
	if port != 5432 {
		t.Errorf("port: got %d", port)
	}
	if db != "postgres" {
		t.Errorf("database: got %q, want postgres", db)
	}
}

func TestParseJDBCURL_HostOnly(t *testing.T) {
	host, port, db, err := ParseJDBCURL("jdbc:postgresql://my-rds.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "my-rds.example.com" {
		t.Errorf("host: got %q", host)
	}
	if port != 5432 {
		t.Errorf("port: got %d", port)
	}
	if db != "postgres" {
		t.Errorf("database: got %q", db)
	}
}

func TestParseJDBCURL_InvalidPrefix(t *testing.T) {
	_, _, _, err := ParseJDBCURL("postgresql://host/db")
	if err == nil {
		t.Fatal("expected error for missing jdbc: prefix")
	}
}

func TestParseJDBCURL_MissingHost(t *testing.T) {
	_, _, _, err := ParseJDBCURL("jdbc:postgresql:///mydb")
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestBuildJDBCURL(t *testing.T) {
	url := BuildJDBCURL("host.example.com", 5432, "admin", "p@ss w0rd!", "mydb")
	want := "jdbc:postgresql://host.example.com:5432/mydb?user=admin&password=p%40ss+w0rd%21&sslmode=require"
	if url != want {
		t.Errorf("BuildJDBCURL:\n  got  %q\n  want %q", url, want)
	}
}

func TestBuildJDBCURL_NoSpecialChars(t *testing.T) {
	url := BuildJDBCURL("rds.example.com", 5433, "user", "pass123", "postgres")
	want := "jdbc:postgresql://rds.example.com:5433/postgres?user=user&password=pass123&sslmode=require"
	if url != want {
		t.Errorf("BuildJDBCURL:\n  got  %q\n  want %q", url, want)
	}
}
