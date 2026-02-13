package cmd

import (
	"testing"
)

func TestConnectCommandRegistered(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"connect"})
	if err != nil {
		t.Fatalf("rootCmd.Find: %v", err)
	}
	if c == nil {
		t.Fatal("connect command not found under root")
	}
	if c.Use != "connect [rds-identifier]" {
		t.Errorf("connect.Use: got %q", c.Use)
	}
	if c.Short == "" {
		t.Error("connect.Short should be set")
	}
}
