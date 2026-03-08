package connect

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/PraveenPrabhuT/rds/internal/core"
	"github.com/chzyer/readline"
	_ "github.com/lib/pq"
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
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Printf("❌ Connection Error: %v\n", err)
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Printf("❌ Connection Failed: %v\n", err)
		return
	}

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
		executeAndPrint(db, query)
	}
}

func executeAndPrint(db *sql.DB, query string) {
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	fmt.Println(strings.Join(cols, " | "))
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}
		rows.Scan(columnPointers...)
		for _, val := range columns {
			fmt.Printf("%v | ", val)
		}
		fmt.Println()
	}
}
