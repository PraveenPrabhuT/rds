package cmd

import (
	"testing"
)

func TestDBCommandRegistered(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"db"})
	if err != nil {
		t.Fatalf("rootCmd.Find('db'): %v", err)
	}
	if c == nil {
		t.Fatal("db command not found under root")
	}
	if c.Use != "db" {
		t.Errorf("db.Use: got %q", c.Use)
	}
}

func TestDBCreateCommandRegistered(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"db", "create"})
	if err != nil {
		t.Fatalf("rootCmd.Find('db create'): %v", err)
	}
	if c == nil {
		t.Fatal("db create command not found")
	}
	if c.Use != "create [db-name]" {
		t.Errorf("db create.Use: got %q", c.Use)
	}
}
