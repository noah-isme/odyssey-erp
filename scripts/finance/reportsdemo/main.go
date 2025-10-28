package main

import "fmt"

func main() {
	fmt.Println("Reports demo placeholder – execute curl commands against running server:")
	fmt.Println(" curl -s -o /tmp/tb.pdf http://localhost:8080/finance/reports/trial-balance/pdf")
	fmt.Println(" curl -s -o /tmp/pl.pdf http://localhost:8080/finance/reports/pl/pdf")
	fmt.Println(" curl -s -o /tmp/bs.pdf http://localhost:8080/finance/reports/bs/pdf")
}
