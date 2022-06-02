package main

import (
	"encoding/json"
	"log"
)

func main(){
	top100_string :=[]byte(`{"22":{"side":0,"price":"22","amount":"11"},"44":{"side":0,"price":"44","amount":"14"}}`)
	top100:= make(map[string]map[string]interface{})
	err := json.Unmarshal(top100_string,&top100)
	log.Printf("top100:%+v %+v err:%+v\n",key,top100,err)


}

