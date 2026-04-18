package main

import (
	"fmt"
	"goklipper/internal/pkg/msgproto"
)

func main() {
	mp := msgproto.NewMessageParser("test")
	cmd := mp.Create_command("identify offset=0 count=40")
	fmt.Printf("CMD: %v\n", cmd)
}
