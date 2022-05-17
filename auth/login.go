package auth

import (
	"errors"
	"strconv"
)
type Userinfo struct {
	Id int64	//用户ID
	Money int64		//用户钱
	Star_ids string		//自选股ID
	Token string //jwt令牌
}
//HTTP登陆
func Login(username string,password string)  (*Userinfo,error) {
	if username=="root" &&  password=="root" {
		return &Userinfo{
			11,8888,"1,2,3","",
		},nil
	}else if username=="typ" &&  password=="type"{
		return &Userinfo{
			22,1111,"3,4,5","",
		},nil
	}
	return &Userinfo{},errors.New("未找到该用户")
}
func CheckUser(userid string) (*Userinfo,error) {
	id,err :=strconv.Atoi(userid)
	if err!=nil{
		return &Userinfo{},errors.New("未找到该用户")
	}
	if id==11{
		return &Userinfo{
			11,8888,"1,2,3","",
		},nil
	}else if id==22 {
		return &Userinfo{
			22,1111,"3,4,5","",
		},nil
	}
	return &Userinfo{},errors.New("未找到该用户")
}
