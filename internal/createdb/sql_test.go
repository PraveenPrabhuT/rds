package createdb

import (
	"strings"
	"testing"
)

func TestBuildSteps_StepCount(t *testing.T) {
	users := []UserCredentials{
		{Username: "testdb", Password: "pw1", Role: "migration", ConnLimit: 10},
		{Username: "testdb_ro_v1", Password: "pw2", Role: "read-only", ConnLimit: 10},
		{Username: "testdb_ro_v2", Password: "pw3", Role: "read-only", ConnLimit: 10},
		{Username: "testdb_rw_v1", Password: "pw4", Role: "read-write", ConnLimit: 10},
		{Username: "testdb_rw_v2", Password: "pw5", Role: "read-write", ConnLimit: 10},
	}

	steps := BuildSteps("testdb", "public", users)
	if len(steps) != 8 {
		t.Errorf("BuildSteps: got %d steps, want 8", len(steps))
	}
}

func TestBuildSteps_CreateDatabaseStep(t *testing.T) {
	users := []UserCredentials{
		{Username: "myapp", Password: "pw1", Role: "migration", ConnLimit: 10},
	}

	steps := BuildSteps("myapp", "public", users)
	if len(steps) == 0 {
		t.Fatal("BuildSteps: returned no steps")
	}

	step := steps[0]
	if step.ConnectAs != "superuser" {
		t.Errorf("step 1 ConnectAs: got %q", step.ConnectAs)
	}
	if step.ConnectDB != "default" {
		t.Errorf("step 1 ConnectDB: got %q", step.ConnectDB)
	}
	if len(step.Statements) != 1 || !strings.Contains(step.Statements[0], "CREATE DATABASE") {
		t.Errorf("step 1 statement: %v", step.Statements)
	}
}

func TestBuildSteps_CreateUsersStep(t *testing.T) {
	users := []UserCredentials{
		{Username: "app", Password: "pw1", Role: "migration", ConnLimit: 5},
		{Username: "app_ro_v1", Password: "pw2", Role: "read-only", ConnLimit: 3},
	}

	steps := BuildSteps("app", "public", users)
	if len(steps) < 2 {
		t.Fatal("BuildSteps: not enough steps")
	}

	userStep := steps[1]
	if len(userStep.Statements) != 2 {
		t.Errorf("create users step: got %d statements, want 2", len(userStep.Statements))
	}
	for _, stmt := range userStep.Statements {
		if !strings.Contains(stmt, "CREATE USER") {
			t.Errorf("expected CREATE USER: %s", stmt)
		}
		if !strings.Contains(stmt, "CONNECTION LIMIT") {
			t.Errorf("expected CONNECTION LIMIT: %s", stmt)
		}
	}
}

func TestBuildSteps_IAMUserStep(t *testing.T) {
	users := []UserCredentials{
		{Username: "svc", Password: "pw1", Role: "migration", ConnLimit: 10},
	}

	steps := BuildSteps("svc", "public", users)
	lastStep := steps[len(steps)-1]

	if !strings.Contains(lastStep.Name, "svc_iam") {
		t.Errorf("last step name should mention IAM user: %s", lastStep.Name)
	}
	if lastStep.ConnectAs != "superuser" {
		t.Errorf("IAM step ConnectAs: got %q", lastStep.ConnectAs)
	}

	foundRdsIam := false
	for _, stmt := range lastStep.Statements {
		if strings.Contains(stmt, "rds_iam") {
			foundRdsIam = true
		}
	}
	if !foundRdsIam {
		t.Error("IAM step should GRANT rds_iam")
	}
}

func TestBuildSteps_CustomSchema(t *testing.T) {
	users := []UserCredentials{
		{Username: "app", Password: "pw1", Role: "migration", ConnLimit: 10},
	}

	steps := BuildSteps("app", "myschema", users)
	found := false
	for _, step := range steps {
		for _, stmt := range step.Statements {
			if strings.Contains(stmt, "myschema") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("custom schema 'myschema' not found in any statement")
	}
}
