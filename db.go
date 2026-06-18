package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func NewDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS envs (
		id TEXT PRIMARY KEY,
		container_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		last_ping DATETIME NOT NULL,
		tunnel_url TEXT DEFAULT '',
		tunnel_pid INTEGER DEFAULT 0,
		status TEXT DEFAULT 'active',
		no_idle_timeout INTEGER DEFAULT 0,
		no_max_lifetime INTEGER DEFAULT 0,
		tags TEXT DEFAULT '{}'
	);`

	if _, err := db.conn.Exec(query); err != nil {
		return err
	}

	// Migrations
	db.conn.Exec(`ALTER TABLE envs ADD COLUMN no_idle_timeout INTEGER DEFAULT 0`)
	db.conn.Exec(`ALTER TABLE envs ADD COLUMN no_max_lifetime INTEGER DEFAULT 0`)
	db.conn.Exec(`ALTER TABLE envs ADD COLUMN tags TEXT DEFAULT '{}'`)

	// Logs table
	db.conn.Exec(`CREATE TABLE IF NOT EXISTS logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		level TEXT NOT NULL,
		event TEXT NOT NULL,
		env_id TEXT DEFAULT '',
		detail TEXT DEFAULT '',
		created_at DATETIME NOT NULL
	)`)

	// Location cache table
	db.conn.Exec(`CREATE TABLE IF NOT EXISTS cache (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		expires_at DATETIME NOT NULL
	)`)

	return nil
}

func (db *DB) InsertEnv(env *Env) error {
	tagsJSON, _ := json.Marshal(env.Tags)
	_, err := db.conn.Exec(
		`INSERT INTO envs (id, container_id, created_at, last_ping, tunnel_url, tunnel_pid, status, no_idle_timeout, no_max_lifetime, tags)
		 VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)`,
		env.ID, env.Container, env.CreatedAt, env.LastPing, env.TunnelURL, env.TunnelPID, env.NoIdleTimeout, env.NoMaxLifetime, string(tagsJSON),
	)
	return err
}

func (db *DB) UpdateEnv(env *Env) error {
	_, err := db.conn.Exec(
		`UPDATE envs SET last_ping=?, tunnel_url=?, tunnel_pid=? WHERE id=?`,
		env.LastPing, env.TunnelURL, env.TunnelPID, env.ID,
	)
	return err
}

func (db *DB) UpdatePing(id string, t time.Time) error {
	_, err := db.conn.Exec(`UPDATE envs SET last_ping=? WHERE id=?`, t, id)
	return err
}

func (db *DB) SetStatus(id, status string) error {
	_, err := db.conn.Exec(`UPDATE envs SET status=? WHERE id=?`, status, id)
	return err
}

func (db *DB) DeleteEnv(id string) error {
	_, err := db.conn.Exec(`DELETE FROM envs WHERE id=?`, id)
	return err
}

func (db *DB) SetNoIdleTimeout(id string, val bool) error {
	v := 0
	if val {
		v = 1
	}
	_, err := db.conn.Exec(`UPDATE envs SET no_idle_timeout=? WHERE id=?`, v, id)
	return err
}

func (db *DB) SetNoMaxLifetime(id string, val bool) error {
	v := 0
	if val {
		v = 1
	}
	_, err := db.conn.Exec(`UPDATE envs SET no_max_lifetime=? WHERE id=?`, v, id)
	return err
}

func (db *DB) CacheSet(key, value string, ttl time.Duration) {
	db.conn.Exec(`INSERT OR REPLACE INTO cache (key, value, expires_at) VALUES (?, ?, ?)`,
		key, value, time.Now().Add(ttl))
}

func (db *DB) CacheGet(key string) (string, bool) {
	var value string
	var expiresAt time.Time
	err := db.conn.QueryRow(`SELECT value, expires_at FROM cache WHERE key=?`, key).Scan(&value, &expiresAt)
	if err != nil || time.Now().After(expiresAt) {
		return "", false
	}
	return value, true
}

func (db *DB) SetTags(id string, tags map[string]string) error {
	tagsJSON, _ := json.Marshal(tags)
	_, err := db.conn.Exec(`UPDATE envs SET tags=? WHERE id=?`, string(tagsJSON), id)
	return err
}

func (db *DB) LogEvent(level, event, envID, detail string) {
	db.conn.Exec(
		`INSERT INTO logs (level, event, env_id, detail, created_at) VALUES (?, ?, ?, ?, ?)`,
		level, event, envID, detail, time.Now(),
	)
}

func (db *DB) GetLogs(limit int, filter string) ([]map[string]interface{}, error) {
	query := `SELECT id, level, event, env_id, detail, created_at FROM logs`
	var args []interface{}
	if filter != "" {
		query += ` WHERE event LIKE ? OR env_id LIKE ? OR detail LIKE ?`
		f := "%" + filter + "%"
		args = append(args, f, f, f)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id int
		var level, event, envID, detail string
		var createdAt time.Time
		if err := rows.Scan(&id, &level, &event, &envID, &detail, &createdAt); err != nil {
			return nil, err
		}
		logs = append(logs, map[string]interface{}{
			"id":         id,
			"level":      level,
			"event":      event,
			"env_id":     envID,
			"detail":     detail,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}
	return logs, nil
}

func (db *DB) GetActiveEnvs() ([]*Env, error) {
	rows, err := db.conn.Query(`SELECT id, container_id, created_at, last_ping, tunnel_url, tunnel_pid, no_idle_timeout, no_max_lifetime, tags FROM envs WHERE status='active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []*Env
	for rows.Next() {
		env := &Env{}
		var tagsJSON string
		if err := rows.Scan(&env.ID, &env.Container, &env.CreatedAt, &env.LastPing, &env.TunnelURL, &env.TunnelPID, &env.NoIdleTimeout, &env.NoMaxLifetime, &tagsJSON); err != nil {
			return nil, err
		}
		env.Tags = make(map[string]string)
		json.Unmarshal([]byte(tagsJSON), &env.Tags)
		envs = append(envs, env)
	}
	return envs, nil
}

func (db *DB) GetEnv(id string) (*Env, error) {
	env := &Env{}
	var tagsJSON string
	err := db.conn.QueryRow(
		`SELECT id, container_id, created_at, last_ping, tunnel_url, tunnel_pid, no_idle_timeout, no_max_lifetime, tags FROM envs WHERE id=? AND status='active'`, id,
	).Scan(&env.ID, &env.Container, &env.CreatedAt, &env.LastPing, &env.TunnelURL, &env.TunnelPID, &env.NoIdleTimeout, &env.NoMaxLifetime, &tagsJSON)
	if err != nil {
		return nil, err
	}
	env.Tags = make(map[string]string)
	json.Unmarshal([]byte(tagsJSON), &env.Tags)
	return env, nil
}

func (db *DB) CleanupOrphans() int {
	envs, err := db.GetActiveEnvs()
	if err != nil {
		log.Printf("DB: failed to load envs for cleanup: %v", err)
		return 0
	}

	removed := 0
	for _, env := range envs {
		// Check if container is actually running
		cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", env.Container)
		output, err := cmd.Output()
		if err != nil || string(output) != "true\n" {
			log.Printf("DB: orphan detected env=%s container=%s — cleaning up", env.ID, env.Container[:12])
			exec.Command("docker", "rm", "-f", env.Container).Run()
			if env.TunnelPID > 0 {
				exec.Command("kill", "-9", fmt.Sprintf("%d", env.TunnelPID)).Run()
			}
			db.DeleteEnv(env.ID)
			removed++
		}
	}

	return removed
}

func (db *DB) Close() {
	db.conn.Close()
}
