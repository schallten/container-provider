package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type BillingDB struct {
	conn *sql.DB
}

func NewBillingDB(path string) (*BillingDB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	db := &BillingDB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *BillingDB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			balance INTEGER DEFAULT 1000,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS transactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			amount INTEGER NOT NULL,
			description TEXT DEFAULT '',
			card_last4 TEXT DEFAULT '',
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			env_id TEXT NOT NULL,
			action TEXT NOT NULL,
			credits INTEGER NOT NULL,
			created_at DATETIME NOT NULL
		)`,
	}
	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (db *BillingDB) GetOrCreateUser(id string) (int, error) {
	var balance int
	err := db.conn.QueryRow(`SELECT balance FROM users WHERE id=?`, id).Scan(&balance)
	if err == sql.ErrNoRows {
		_, err = db.conn.Exec(`INSERT INTO users (id, balance, created_at) VALUES (?, 1000, ?)`, id, time.Now())
		if err != nil {
			return 0, err
		}
		return 1000, nil
	}
	return balance, err
}

func (db *BillingDB) GetBalance(userID string) int {
	balance, _ := db.GetOrCreateUser(userID)
	return balance
}

func (db *BillingDB) AddCredits(userID string, amount int, cardLast4 string) error {
	_, err := db.conn.Exec(`UPDATE users SET balance = balance + ? WHERE id=?`, amount, userID)
	if err != nil {
		return err
	}
	_, err = db.conn.Exec(
		`INSERT INTO transactions (user_id, type, amount, description, card_last4, created_at) VALUES (?, 'topup', ?, ?, ?, ?)`,
		userID, amount, "Payment received", cardLast4, time.Now(),
	)
	return err
}

func (db *BillingDB) DeductCredits(userID string, amount int, envID, action string) error {
	var balance int
	err := db.conn.QueryRow(`SELECT balance FROM users WHERE id=?`, userID).Scan(&balance)
	if err != nil || balance < amount {
		return sql.ErrNoRows
	}
	_, err = db.conn.Exec(`UPDATE users SET balance = balance - ? WHERE id=?`, amount, userID)
	if err != nil {
		return err
	}
	_, err = db.conn.Exec(
		`INSERT INTO usage (user_id, env_id, action, credits, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, envID, action, amount, time.Now(),
	)
	if err != nil {
		return err
	}
	_, err = db.conn.Exec(
		`INSERT INTO transactions (user_id, type, amount, description, created_at) VALUES (?, 'deduction', ?, ?, ?)`,
		userID, -amount, action+" — "+envID, time.Now(),
	)
	return err
}

func (db *BillingDB) GetTransactions(userID string, limit int) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(
		`SELECT type, amount, description, card_last4, created_at FROM transactions WHERE user_id=? ORDER BY created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []map[string]interface{}
	for rows.Next() {
		var ttype, desc, card string
		var amount int
		var createdAt time.Time
		if err := rows.Scan(&ttype, &amount, &desc, &card, &createdAt); err != nil {
			return nil, err
		}
		txns = append(txns, map[string]interface{}{
			"type":        ttype,
			"amount":      amount,
			"description": desc,
			"card_last4":  card,
			"created_at":  createdAt.Format(time.RFC3339),
		})
	}
	return txns, nil
}

func (db *BillingDB) GetUsage(userID string, limit int) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(
		`SELECT env_id, action, credits, created_at FROM usage WHERE user_id=? ORDER BY created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usage []map[string]interface{}
	for rows.Next() {
		var envID, action string
		var credits int
		var createdAt time.Time
		if err := rows.Scan(&envID, &action, &credits, &createdAt); err != nil {
			return nil, err
		}
		usage = append(usage, map[string]interface{}{
			"env_id":     envID,
			"action":     action,
			"credits":    credits,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}
	return usage, nil
}

func (db *BillingDB) GetCostsByDay(userID string, days int) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(`
		SELECT date(created_at) as day, SUM(ABS(amount)) as total
		FROM transactions
		WHERE user_id=? AND amount < 0
		AND created_at >= datetime('now', ?)
		GROUP BY date(created_at)
		ORDER BY day ASC
	`, userID, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var costs []map[string]interface{}
	for rows.Next() {
		var day string
		var total int
		if err := rows.Scan(&day, &total); err != nil {
			return nil, err
		}
		costs = append(costs, map[string]interface{}{
			"date":   day,
			"credits": total,
		})
	}
	return costs, nil
}

func (db *BillingDB) GetCostsByEnv(userID string, days int) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(`
		SELECT env_id, action, SUM(credits) as total
		FROM usage
		WHERE user_id=? AND created_at >= datetime('now', ?)
		GROUP BY env_id, action
		ORDER BY total DESC
	`, userID, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var costs []map[string]interface{}
	for rows.Next() {
		var envID, action string
		var total int
		if err := rows.Scan(&envID, &action, &total); err != nil {
			return nil, err
		}
		costs = append(costs, map[string]interface{}{
			"env_id":  envID,
			"action":  action,
			"credits": total,
		})
	}
	return costs, nil
}

func (db *BillingDB) Close() {
	db.conn.Close()
}
