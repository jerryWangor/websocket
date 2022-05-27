package main

import (
	"fmt"
	"strconv"
)
func main(){
	var a float64
	a = 1111112.333332
	fmt.Printf("%+v,%T,     %+v,%T",a,a,strconv.FormatFloat(a,'g',1,64),strconv.FormatFloat(a,'g',1,64))
}

