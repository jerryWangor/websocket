package main

import (
	"fmt"
	"time"
	"websocket/auth"
)
func main(){
		// 生成一个token
		token,_:=auth.GenerateToken("12")
		fmt.Println("生成的token:",token)
		// 验证并解密token
		claim,err:=auth.ParseToken(token)
		if err!=nil {
			fmt.Println("解析token出现错误：",err)
		}else if time.Now().Unix() > claim.ExpiresAt {
			fmt.Println("时间超时")
		}else {
			fmt.Println("userid:",claim.UserID)
		}
}
