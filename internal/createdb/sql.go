package createdb

import "fmt"

// Step represents a named group of SQL statements and the connection context
// they require (which user and database to connect as/to).
type Step struct {
	Name       string
	ConnectAs  string // "superuser" or "migration"
	ConnectDB  string // "default" or "newdb"
	Statements []string
}

// BuildSteps returns the ordered list of SQL steps matching the Ansible playbook.
func BuildSteps(dbName, schema string, users []UserCredentials) []Step {
	migrationUser := dbName
	roV1 := dbName + "_ro_v1"
	rwV1 := dbName + "_rw_v1"
	roV2 := dbName + "_ro_v2"
	rwV2 := dbName + "_rw_v2"
	iamUser := dbName + "_iam"

	var steps []Step

	// Step 1: Create database (superuser -> default DB)
	steps = append(steps, Step{
		Name:      fmt.Sprintf("Create database %q", dbName),
		ConnectAs: "superuser",
		ConnectDB: "default",
		Statements: []string{
			fmt.Sprintf(`CREATE DATABASE "%s"`, dbName),
		},
	})

	// Step 2: Create users (superuser -> default DB)
	var createUserStmts []string
	for _, u := range users {
		createUserStmts = append(createUserStmts,
			fmt.Sprintf(`CREATE USER "%s" WITH ENCRYPTED PASSWORD '%s' CONNECTION LIMIT %d`,
				u.Username, u.Password, u.ConnLimit),
		)
	}
	steps = append(steps, Step{
		Name:       "Create users",
		ConnectAs:  "superuser",
		ConnectDB:  "default",
		Statements: createUserStmts,
	})

	// Step 3: Grant role memberships (superuser -> default DB)
	steps = append(steps, Step{
		Name:      "Grant role memberships",
		ConnectAs: "superuser",
		ConnectDB: "default",
		Statements: []string{
			fmt.Sprintf(`GRANT "%s" TO "%s"`, roV1, roV2),
			fmt.Sprintf(`GRANT "%s" TO "%s"`, rwV1, rwV2),
		},
	})

	// Step 4: Grant ALL on database to migration user (superuser -> default DB)
	steps = append(steps, Step{
		Name:      fmt.Sprintf("Grant ALL on database to %q", migrationUser),
		ConnectAs: "superuser",
		ConnectDB: "default",
		Statements: []string{
			fmt.Sprintf(`GRANT ALL ON DATABASE "%s" TO "%s"`, dbName, migrationUser),
		},
	})

	// Step 5: Schema permissions (superuser -> new DB)
	steps = append(steps, Step{
		Name:      fmt.Sprintf("Configure schema %q permissions", schema),
		ConnectAs: "superuser",
		ConnectDB: "newdb",
		Statements: []string{
			fmt.Sprintf(`REVOKE CREATE ON SCHEMA %s FROM PUBLIC`, schema),
			fmt.Sprintf(`GRANT CREATE ON SCHEMA %s TO "%s"`, schema, migrationUser),
		},
	})

	// Step 6: RO privileges (migration user -> new DB)
	steps = append(steps, Step{
		Name:      fmt.Sprintf("Grant read-only privileges to %q", roV1),
		ConnectAs: "migration",
		ConnectDB: "newdb",
		Statements: []string{
			fmt.Sprintf(`GRANT SELECT ON ALL TABLES IN SCHEMA %s TO "%s"`, schema, roV1),
			fmt.Sprintf(`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA %s TO "%s"`, schema, roV1),
			fmt.Sprintf(`ALTER DEFAULT PRIVILEGES FOR ROLE "%s" IN SCHEMA %s GRANT SELECT ON TABLES TO "%s"`,
				migrationUser, schema, roV1),
			fmt.Sprintf(`ALTER DEFAULT PRIVILEGES FOR ROLE "%s" IN SCHEMA %s GRANT USAGE, SELECT ON SEQUENCES TO "%s"`,
				migrationUser, schema, roV1),
		},
	})

	// Step 7: RW privileges (migration user -> new DB)
	steps = append(steps, Step{
		Name:      fmt.Sprintf("Grant read-write privileges to %q", rwV1),
		ConnectAs: "migration",
		ConnectDB: "newdb",
		Statements: []string{
			fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %s TO "%s"`, schema, rwV1),
			fmt.Sprintf(`GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA %s TO "%s"`, schema, rwV1),
			fmt.Sprintf(`ALTER DEFAULT PRIVILEGES FOR ROLE "%s" IN SCHEMA %s GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "%s"`,
				migrationUser, schema, rwV1),
			fmt.Sprintf(`ALTER DEFAULT PRIVILEGES FOR ROLE "%s" IN SCHEMA %s GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO "%s"`,
				migrationUser, schema, rwV1),
		},
	})

	// Step 8: IAM user (superuser -> new DB)
	steps = append(steps, Step{
		Name:      fmt.Sprintf("Create IAM user %q", iamUser),
		ConnectAs: "superuser",
		ConnectDB: "newdb",
		Statements: []string{
			fmt.Sprintf(`CREATE USER "%s" WITH LOGIN`, iamUser),
			fmt.Sprintf(`GRANT rds_iam TO "%s"`, iamUser),
			fmt.Sprintf(`GRANT CONNECT ON DATABASE "%s" TO "%s"`, dbName, iamUser),
			fmt.Sprintf(`GRANT USAGE, CREATE ON SCHEMA %s TO "%s"`, schema, iamUser),
			fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %s TO "%s"`, schema, iamUser),
			fmt.Sprintf(`ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "%s"`,
				schema, iamUser),
		},
	})

	return steps
}
