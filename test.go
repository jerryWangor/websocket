package main

import (
	//"fmt"
	"fmt"
	"websocket/util"
)
func main(){
	fmt.Printf("%v",util.Redis_pool)
	//执行增删改
	//query里面是sql语句。
	//result, e := util.DB.Exec("insert into user(name,money) values(?,?);", "小扬", 8888)
	//if e!=nil{
	//	fmt.Println("err=",e)
	//	return
	//}
	//rowsAffected, _ := result.RowsAffected()
	//lastInsertId, _ := result.LastInsertId()
	//fmt.Println("受影响的行数=",rowsAffected)
	//fmt.Println("最后一行的ID=",lastInsertId)
}

