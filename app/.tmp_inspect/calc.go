package main

import (
	"fmt"
)

func main() {
    // Envelope 0 creation on Node 1
    createT := int64(1780158920416)
    
    // Attempted apply on Node 2
    applyT := int64(1780162574000)
    
    fmt.Printf("Diff in ms: %d\n", applyT - createT)
    fmt.Printf("Diff in mins: %d\n", (applyT - createT) / 1000 / 60)
}
