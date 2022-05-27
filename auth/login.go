package auth

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"websocket/util"
)
type Userinfo struct {
	Id int64	`json:"id"`//用户ID
	Name string	`json:"name"`//用户名
	Money float64	`json:"money"`	//用户钱
	Star_ids string	`json:"star_ids"`	//自选股ID
	Band_money float64 `json:"band_money"`//jwt令牌
	Token string `json:"token"`//jwt令牌
}
//HTTP登陆
func Login(username string,password string)  (*Userinfo,error) {
	sqlStr := "select id, name, money,star_ids,band_money from user where username=? and password=?"
	var u Userinfo
	err := util.DB.Get(&u, sqlStr, username,password)
	if err != nil {
		log.Printf("get failed, err:%v\n", err)
		return &Userinfo{},errors.New("未找到该用户")
	}
	fmt.Printf("userinfo %v\n", u)
	return &u,nil

}
func CheckUser(userid string) (*Userinfo,error) {
	id,err :=strconv.Atoi(userid)
	if err!=nil{
		return &Userinfo{},errors.New("未找到该用户")
	}
	sqlStr := "select id, name, money,star_ids,band_money from user where id=?"
	var u Userinfo
	err = util.DB.Get(&u, sqlStr, id)
	if err != nil {
		log.Printf("get failed, err:%v\n", err)
		return &Userinfo{},errors.New("未找到该用户")
	}
	fmt.Printf("userinfo %v\n", u)
	return &u,nil
}
