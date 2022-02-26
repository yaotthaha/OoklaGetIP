package main

import (
	"OoKlaGetIP/lib"
	"fmt"
)

func main() {
	a, _ := lib.GetList("119.29.29.29")
	b := lib.GetIP(&a)
	fmt.Println(b)
}
