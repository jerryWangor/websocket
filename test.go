package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)
func main(){
	http.HandleFunc("/http", httpHandler)
	http.ListenAndServe("127.0.0.1:8080", nil)
}
func httpHandler(resp http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	 fmt.Printf("参数有：%+v\n",req.Form)


	decoder:=json.NewDecoder(req.Body)
	var params map[string]interface{}
	decoder.Decode(&params)
	fmt.Println("POST json req: ",params)
	fmt.Fprintln(resp,`{"code":0,"msg":"succ"}`)

}
