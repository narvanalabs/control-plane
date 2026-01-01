package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("Worker started")
	for {
		fmt.Println("Processing jobs...")
		time.Sleep(5 * time.Second)
	}
}
