package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rubiojr/go-edistribucion"
)

var db *sql.DB

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dbpath := flag.String("db", "", "database file")
	pause := flag.Int("sleep", 60, "pause X minutes between queries")
	flag.Parse()
	if *dbpath == "" {
		flag.Usage()
		return fmt.Errorf("required: -db")
	}

	// Open database file.
	var err error
	db, err = sql.Open("sqlite3", *dbpath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create table for storing page views.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS meters (id TEXT NOT NULL, timestamp INTEGER NOT NULL, address TEXT NOT NULL, contracted_power REAL NOT NULL, current_power REAL NOT NULL, total_power REAL NOT NULL, icp_state INT NOT NULL);`); err != nil {
		return fmt.Errorf("cannot create table: %w", err)
	}

	client := edistribucion.NewClient(os.Getenv("EDISTRIBUCION_USERNAME"), os.Getenv("EDISTRIBUCION_PASSWORD"))

	for {
		err := client.Login()
		if err != nil {
			continue
		}

		allCups, err := client.ListCups()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing cups: %s\n", err)
			os.Exit(1)
		}

		for _, cups := range allCups {
			fmt.Println("Reading data from CUPS in ", cups.ProvisioningAddress)
			err = insertData(client, &cups)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
		}
		fmt.Printf("Sleeping for %d minutes...\n", *pause)
		time.Sleep(time.Duration(*pause) * time.Minute)
	}
}

func insertData(client *edistribucion.Client, cups *edistribucion.Cups) error {
	mi, err := client.MeterInfo(cups.Id)
	if err != nil {
		return fmt.Errorf("failed listing meter info from CUPS %s: %s", cups.Name, err)
	}
	fmt.Println(cups.ProvisioningAddress)
	fmt.Println("Potencia actual: ", mi.PotenciaActual)
	fmt.Println("Potencia contratada: ", mi.PotenciaContratada)
	fmt.Println("Porcentage: ", mi.Percentage)
	fmt.Println("Estado ICP: ", mi.EstadoICP)
	fmt.Println("Totalizador: ", mi.Totalizador)
	_, err = db.Exec(`INSERT INTO meters (id, timestamp, address, contracted_power, current_power, total_power, icp_state) VALUES (?, ?, ?, ?, ?, ?, ?);`,
		cups.Name,
		time.Now().Format(time.RFC3339),
		cups.ProvisioningAddress,
		mi.PotenciaContratada,
		mi.PotenciaActual,
		mi.Totalizador,
		mi.EstadoICP,
	)

	return err
}
