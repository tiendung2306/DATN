package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: dbinspect <db_path> <query>")
	}
	db, err := sql.Open("sqlite", os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(cols)
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range cols {
		valuePtrs[i] = &values[i]
	}
	for rows.Next() {
		rows.Scan(valuePtrs...)
		fmt.Println(values)
	}
}
