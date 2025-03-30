package storage

import (
	"log"
)

func (d *DB) InitForwardingTable() {
	stmt := `CREATE TABLE IF NOT EXISTS forwarding_rules (
        source TEXT PRIMARY KEY,
        target TEXT
    );`
	_, err := d.conn.Exec(stmt)
	if err != nil {
		log.Fatal(err)
	}
}

func (d *DB) SaveForwardingRule(source, target string) {
	_, err := d.conn.Exec("REPLACE INTO forwarding_rules (source, target) VALUES (?, ?)", source, target)
	if err != nil {
		log.Println("DB SaveForwardingRule Error:", err)
	}
}

func (d *DB) GetForwardingRules() map[string]string {
	rows, err := d.conn.Query("SELECT source, target FROM forwarding_rules")
	if err != nil {
		log.Println("DB GetForwardingRules Error:", err)
		return nil
	}
	defer rows.Close()

	rules := map[string]string{}
	for rows.Next() {
		var source, target string
		rows.Scan(&source, &target)
		rules[source] = target
	}
	return rules
}

func (d *DB) GetTargetForSource(source string) string {
	row := d.conn.QueryRow("SELECT target FROM forwarding_rules WHERE source = ?", source)
	var target string
	err := row.Scan(&target)
	if err != nil {
		return ""
	}
	return target
}
