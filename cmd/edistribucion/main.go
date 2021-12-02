package main

import (
	"fmt"
	"os"

	"github.com/rubiojr/go-edistribucion"
)

func main() {
	client := edistribucion.NewClient(os.Getenv("EDISTRIBUCION_USERNAME"), os.Getenv("EDISTRIBUCION_PASSWORD"))
	err := client.Login()
	if err != nil {
		panic(err)
	}

	allCups, err := client.ListCups()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing cups: %s\n", err)
		os.Exit(1)
	}

	for _, cups := range allCups {
		mi, err := client.MeterInfo(cups.Id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing meter info from CUPS %s: %s\n", cups.Name, err)
			continue
		}
		fmt.Println(cups.ProvisioningAddress)
		fmt.Println("Potencia actual: ", mi.PotenciaActual)
		fmt.Println("Potencia contratada: ", mi.PotenciaContratada)
		fmt.Println("Porcentage: ", mi.Percentage)
		fmt.Println("Estado ICP: ", mi.EstadoICP)
		fmt.Println("Totalizador: ", mi.Totalizador)
	}

}
