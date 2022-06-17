package auth

import (
	"errors"
	//"fmt"

	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
	"websocket/util"
)
type Userinfo struct {
	Id int64	`json:"id"`//用户ID
	Name string	`json:"name"`//用户名
	Money float64	`json:"money"`	//用户钱
	Star_ids string	`json:"star_ids"`	//自选股ID
	Band_money float64 `json:"band_money"`//jwt令牌
	Token string `json:"token"`//jwt令牌
	Stocks map[string]*Stock
}
type Stock struct {
	Symbol string
	LastTime string `db:"last_time"`
	StockNum float64 `db:"stock_num"`
	BandStockNum float64 `db:"band_stock_num"`
	Cost float64 `db:"cost"`
	Cur_price float64
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
	log.Printf("userinfo 登录 %v\n", u)
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
	log.Printf("userinfo %v\n", u)
	return &u,nil
}
func Bootlist(resp http.ResponseWriter, req *http.Request){

	sqlStr := "select a.*,b.symbol,stock_num,b.band_stock_num from (select id,name,money,band_money,token from user where boot=1 ) a join stock b on a.id=b.userid"
	var MatchList []struct{
		Id int `json:"id"`
		Name string `json:"name"`
		Money float64 `json:"money"`
		Band_money float64 `json:"band_money"`
		Token string `json:"token"`
		Symbol string `json:"symbol"`
		Stock_num float64 `json:"stock_num"`
		Band_stock_num float64 `json:"band_stock_num"`
	}
	err := util.DB.Select(&MatchList, sqlStr)
	if err != nil{
		log.Printf("%+v\n",err)
		resp.Write(util.FormatReturn(util.HTTP_ERROR,"创建失败",""))
	}else{
		resp.Write(util.FormatReturn(util.HTTP_OK,"查询成功",MatchList))
	}

}
func InitBoot(resp http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	resp.Header().Set("Access-Control-Allow-Origin", "*")
	resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	resp.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	//d := fmt.Sprintf("参数有：%+v",req.Form)
	//resp.Write([]byte(d))
	////count=100,money_range=1000,2000,symbols=BTC,DOGE,symbol_num_range=200,3000
	err := CreateBoot(req.FormValue("count"),req.FormValue("money_range"),req.FormValue("symbols"),req.FormValue("symbol_num_range"))
	if err!=nil {
		resp.Write(util.FormatReturn(util.HTTP_ERROR,"创建失败",""))
		return
	}
	resp.Write(util.FormatReturn(util.HTTP_OK,"创建成功",""))
}

func CreateBoot(count1 string,money_range1 string,symbols1 string,symbol_num_range1 string) error  {
	//count=100,money_range=1000,2000,symbols=BTC,DOGE,symbol_num_range=200,3000
	log.Printf("%+v,%+v,%+v,%+v\n",count1,money_range1,symbols1,symbol_num_range1)
	if count1==""{
		count1="100"
	}
	if money_range1==""{
		money_range1="1000,2000"
	}
	if symbols1==""{
		symbols1="BTC,DOGE"
	}
	if symbol_num_range1==""{
		symbol_num_range1="200,3000"
	}
	log.Printf("end %+v,%+v,%+v,%+v\n",count1,money_range1,symbols1,symbol_num_range1)
	count,_ :=strconv.Atoi(count1)
	money_range :=strings.Split(money_range1,",")
	symbols :=strings.Split(symbols1,",")
	symbol_num_range :=strings.Split(symbol_num_range1,",")
	mmin,_:= strconv.Atoi(money_range[0])
	mmax,_:= strconv.Atoi(money_range[1])

	smin,_:= strconv.Atoi(symbol_num_range[0])
	smax,_:= strconv.Atoi(symbol_num_range[1])

	//fmt.Printf("rand is %v\n", a)
	for i:=1;i<=count;i++{
		name :="boot"+ strconv.Itoa(i)
		money := GenerateRangeNum(mmin,mmax)
		a,_ :=util.DB.Exec("INSERT INTO user (name, money, username,password,band_money,boot) VALUES (?,?,?,?,?,1)",name,money,name,name,0)
		userid,_:=a.LastInsertId()
		Token,_ :=GenerateToken(strconv.FormatInt(userid,10))
		util.DB.Exec("update user set token=? where id=?",Token,userid)
		for _,v:=range symbols{
			stock_num := GenerateRangeNum(smin,smax)
			util.DB.Exec("INSERT INTO stock (userid, symbol, stock_num,band_stock_num) VALUES (?,?,?,?)",userid,v,stock_num,0)
		}
	}
	return nil
}
func GenerateRangeNum(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	randNum := rand.Intn(max - min)
	randNum = randNum + min
	return randNum
}
