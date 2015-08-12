package main

import (
	"encoding/hex"
	"fmt"
)

func dataDump(bytes []byte) {
	fmt.Println("\n******DUMP********\nData:\n", string(bytes), "\nHex:")
	fmt.Println(hex.Dump(bytes), "\n*******************")
}
