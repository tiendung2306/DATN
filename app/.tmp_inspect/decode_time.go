package main

import (
	"encoding/binary"
	"fmt"
)

func main() {
    b1 := []byte{0, 0, 1, 158, 121, 188, 167, 48}
    b2 := []byte{0, 0, 1, 158, 121, 189, 49, 114}
    b3 := []byte{0, 0, 1, 158, 121, 189, 224, 10}
    
    fmt.Printf("Node 1: %d\n", binary.BigEndian.Uint64(b1))
    fmt.Printf("Node 2: %d\n", binary.BigEndian.Uint64(b2))
    fmt.Printf("Node 3: %d\n", binary.BigEndian.Uint64(b3))
}
