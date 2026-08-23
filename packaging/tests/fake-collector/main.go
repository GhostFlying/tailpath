package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	line := strings.Repeat("x", 64*1024)
	for range 420 {
		fmt.Println(line)
	}
	os.Exit(7)
}
