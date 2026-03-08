package connect

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/PraveenPrabhuT/rds/internal/core"
	"github.com/chzyer/readline"
	"github.com/jackc/pgx/v5"
)

func buildConnectArgs(inst core.InstanceInfo, creds core.RDSCreds) []string {
	return []string{"-h", inst.Host, "-p", fmt.Sprintf("%d", inst.Port), "-U", creds.Username, "-d", "postgres"}
}

func executeExternal(bin string, inst core.InstanceInfo, creds core.RDSCreds) {
	args := buildConnectArgs(inst, creds)
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", creds.Password))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	_ = startAndWait(cmd)
}

func runNativeConnect(host string, port int32, user, password, dbname string) {
	ctx := context.Background()
	conn, err := core.NewPgxConn(ctx, host, port, user, password, dbname)
	if err != nil {
		fmt.Printf("❌ Connection Error: %v\n", err)
		return
	}
	defer conn.Close(ctx)

	fmt.Printf("✅ Connected to %s (Native Mode)\n", host)
	rl, _ := readline.NewEx(&readline.Config{
		Prompt: dbname + "=> ",
	})
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}
		query := strings.TrimSpace(line)
		if query == "" {
			continue
		}
		if query == "exit" || query == "quit" {
			break
		}
		executeAndPrint(ctx, conn, query)
	}
}

func executeAndPrint(ctx context.Context, conn *pgx.Conn, query string) {
	rows, err := conn.Query(ctx, query)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer rows.Close()

	cols := rows.FieldDescriptions()
	colNames := make([]string, len(cols))
	for i, fd := range cols {
		colNames[i] = fd.Name
	}
	fmt.Println(strings.Join(colNames, " | "))

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			fmt.Printf("ERROR scanning row: %v\n", err)
			continue
		}
		for _, val := range values {
			fmt.Printf("%v | ", val)
		}
		fmt.Println()
	}
}
