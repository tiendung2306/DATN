package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT seq, epoch, envelope FROM envelope_log WHERE msg_type=\"commit\" ORDER BY seq ASC")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq, epoch int
		var env []byte
		rows.Scan(&seq, &epoch, &env)
		var data map[string]interface{}
		json.Unmarshal(env, &data)
		fmt.Printf("Seq %d | Epoch %d\n", seq, epoch)
		payload, _ := data["payload"].(map[string]interface{})
		adds, _ := payload["add_deliveries"].([]interface{})
		for _, a := range adds {
			m := a.(map[string]interface{})
			fmt.Printf("  + Add: %v\n", m["target_peer_id"])
		}
		props, _ := payload["included_proposals"].([]interface{})
		for _, p := range props {
			m := p.(map[string]interface{})
			fmt.Printf("  * Prop: type=%v\n", m["proposal_type"])
		}
	}
}
