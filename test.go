package main

import (
	"fmt"
	"time"
)
func main() {
	fmt.Println("bg")
	//a:=sync.WaitGroup{}
	//a.Add(1)
	//go func() {
	//	defer a.Done()
	//	time.Sleep(time.Second)
	//}()
	//a.Wait()
	select {
	}
	go func() {
		fmt.Println("goroutine")
		time.Sleep(time.Second)
	}()


	fmt.Println("end")


}




