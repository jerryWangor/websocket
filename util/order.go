package util

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
	"websocket/config"
	"github.com/shopspring/decimal"
)

type orderFace interface {
	PlaceOrder()	//下单
	CancelOrder() //撤单
	GenerateOrderId()	//生成订单号
}
type Order struct {
	OrderID string
}
var OrderIdWork  *OrderWorker

type PostData struct {
	Time int64 `json:"time"` //时间戳
	Sign string `json:"sign"` //加密串 md5(time + a4fdc2af6949b3945b8e556d9fbce343)
	//Accid int  //账号ID *传
	Action int8 `json:"action"`//操作类型 0 挂单 1 撤单	*传
	Symbol string  `json:"symbol"`//交易标 BTC	*传
	OrderId string `json:"orderId"`//订单号
	Side int8 `json:"side"`//0 买 1 卖 */传
	Type int8 `json:"type"`//0 普通买卖
	Amount float64 `json:"amount"` //数量	*/传
	Price float64 `json:"price"` //价格	*传
	//code=0 是成功 其他失败
}
type MsgType2 struct {
	Status int `json:"status"`
	Orderid string `json:"orderid"`
	Symbol string `json:"symbol"`
	Createtime string `json:"createtime"`
	Dealtime string `json:"dealtime"`
}
type OrderLine struct {
	Orderid string
	Status int
	Create_time string `mysql:"create_time"`
	Deal_time string `mysql:"deal_time"`
	Symbol string
	Action int
	Side int
	Amount float64
	Price float64
	Userid int
	Deal_amount float64 `mysql:"deal_amount"`
	Deal_stock float64
	Deal_money float64
}
type MatchLine struct {
	MName string
	TName string
	Makerid string `json:"makerid"`
	Takerid string `json:"takerid"`
	Takerside string `json:"takerside"`
	Amount string `json:"amount"`
	Price string `json:"price"`
	Deal_time string `json:"deal_time"`
}
func init()  {
	OrderIdWork = NewOrderWorker()
}
//func main()  {
//	o :=&Order{}
//	o.GenerateOrderId()
//	fmt.Printf("%v ",o.OrderID)
//}
//生成订单号
func (o *Order) GenerateOrderId() string{
	//fmt.Printf("\n aaaaaaaa %v %v\n",OrderIdWork,&OrderIdWork)
	o.OrderID = OrderIdWork.GetId(time.Now())
	return o.OrderID
}
type Api_struct struct {
	Code int `json:"code"`
	Msg string `json:"msg"`
}
func (o *Order) PlaceOrder(data *PostData,userid int) *Api_struct{
	data.Time = time.Now().Unix()
	md5str := fmt.Sprintf("%x", md5.Sum([]byte(strconv.FormatInt(data.Time, 10) + "a4fdc2af6949b3945b8e556d9fbce343")))
	data.Sign = md5str
	data.OrderId = o.GenerateOrderId()
	data.Type = 0
	res := o.post(data)

	_, err := DB.Exec("insert into `order`(userid,orderid,status,symbol,action,side,type,amount,price,api_res) values(?,?,?,?,?,?,?,?,?,?);",
		userid,data.OrderId,0,data.Symbol,
		data.Action,data.Side,
		data.Type,data.Amount,
		data.Price,
			string(res))
	if err != nil {
		log.Printf("get failed, err:%v\n", err)
	}

	t := &Api_struct{}
	json.Unmarshal(res,t)
	//userinfo.Money = userinfo.Money-(data.Amount*data.Price)
	return t
}
func (o *Order) CancelOrder(data *PostData) *Api_struct{
	data.Time = time.Now().Unix()
	md5str := fmt.Sprintf("%x", md5.Sum([]byte(strconv.FormatInt(data.Time, 10) + "a4fdc2af6949b3945b8e556d9fbce343")))
	data.Sign = md5str
	//data.OrderId = strconv.FormatInt(o.GenerateOrderId(), 10)
	data.Type = 0
	res := o.post(data)
	t := &Api_struct{}
	json.Unmarshal(res,t)
	return t
}

func (o *Order) post(postData *PostData) []byte {

	postBody, _ := json.Marshal(postData)
	responseBody := bytes.NewBuffer(postBody)

	resp, err := http.Post(config.GlobalConfig.Api_addr, "application/json", responseBody)
	if err != nil {
		log.Println("post handleOrder errors", err)
		return FormatReturn(HTTP_ERROR,"撮合系统无响应，请稍后再试","")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	sb := string(body)
	log.Printf("参数 %+v \n 下单操作接口返回 %v \n",string(postBody),sb)
	return body
}

func OrderMQHandle(orderid string,mq map[string]string) int64{
	//log.Printf("OrderMQHandle :%+v %+v %T\n", orderid,mq)
	aa,_:=strconv.Atoi(orderid)
	if aa==0{
		return 0
	}
	sqlStr := "select userid,amount,price,side,deal_amount,symbol from `order` where orderid=? and status <>1"
	var User struct{
		Userid int64
		Amount float64
		Price float64
		Side int
		Deal_amount float64
		Symbol string
	}
	err := DB.Get(&User, sqlStr,orderid )
	if err!=nil {
		log.Printf("获取原订单amount 失败 %+v info :%+v\n", err,orderid)
		return 0
	}
	StockSqlStr := "select stock_num,cost from `stock` where userid=? and symbol=?"
	var Stock struct{
		Stock_num float64
		Cost float64
	}
	err = DB.Get(&Stock, StockSqlStr,User.Userid,User.Symbol)
	if err!=nil {
		log.Printf("获取存量失败 %+v info :%+v\n", err,orderid)
		Stock.Stock_num = 0
		Stock.Cost = 0
	}
	var status int
	mq_amount,_ := strconv.ParseFloat(mq["amount"],64)
	mq_price,_ := strconv.ParseFloat(mq["price"],64)
	if User.Amount-User.Deal_amount == mq_amount{
		//全部成交
		status = 1
	}else {
		//部分成交
		status = 2
	}
		//修改订单状态
		//开始一个事务，返回一个事务对象tx
		tx, err := DB.Beginx()
		//修改订单状态及成交数量，成交时间
		var err1,err2,err3 error
	switch User.Side {
	case 0://买单
		now_time :=time.Now().Format("2006-01-02 15:04:05")
		_,err1 = tx.Exec("update `order` set status=?, deal_time=?,deal_amount=deal_amount+? where orderid=?",
			status,
			now_time,
			mq_amount,
			orderid,
		)
		//修改用户band_money扣减
		//如果成交价格低于挂单价格，则还要返还冻结里面钱到账户里
		cj := decimal.NewFromFloat(mq_amount).Mul(decimal.NewFromFloat(User.Price).Sub(decimal.NewFromFloat(mq_price)))
		dj := decimal.NewFromFloat(User.Price).Mul(decimal.NewFromFloat(mq_amount))
		//fmt.Printf("买单成交价格：%+v，%+v,%+v,%+v \n",User.Price,mq_amount,dj,cj)
		_,err2 = tx.Exec("update user set money=money+?, band_money=band_money-?  where id=?",cj, dj, User.Userid)
		//更新用户对应标的存量
		//算成本价 ：最新 ((cost*stock_num)+(mq_amount*mq_price))/(stock_num+mq_amount)
		tmp_cb :=decimal.NewFromFloat(Stock.Cost).Mul(decimal.NewFromFloat(Stock.Stock_num))
		tmp_cur :=decimal.NewFromFloat(mq_amount).Mul(decimal.NewFromFloat(mq_price))
		tmp_allnum :=decimal.NewFromFloat(Stock.Stock_num).Add(decimal.NewFromFloat(mq_amount))
		cost :=tmp_cb.Add(tmp_cur).Div(tmp_allnum)
		log.Printf("订单:%+v,买单，成本公式((%+v*%+v)+(%+v*%+v))/(%+v+%+v)",orderid,decimal.NewFromFloat(Stock.Cost),decimal.NewFromFloat(Stock.Stock_num),
			decimal.NewFromFloat(mq_amount),decimal.NewFromFloat(mq_price),decimal.NewFromFloat(Stock.Stock_num),decimal.NewFromFloat(mq_amount),
		)
		_,err3 = tx.Exec("insert stock (userid,symbol,stock_num,last_time,cost) VALUES(?,?,?,?,?) " +
			"on DUPLICATE KEY UPDATE userid=VALUES(userid),symbol=VALUES(symbol),last_time=VALUES(last_time),stock_num=VALUES(stock_num)+stock_num,cost=VALUES(cost)",User.Userid,User.Symbol,mq_amount,now_time,cost)

	case 1: //卖单
		now_time :=time.Now().Format("2006-01-02 15:04:05")
		_,err1 = tx.Exec("update `order` set status=?, deal_time=?,deal_amount=deal_amount+? where orderid=?",
			status,
			now_time,
			mq_amount,
			orderid,
		)
		//如果成交价格高于挂单价格，则还要按实际价格加钱
		cj := decimal.NewFromFloat(mq_amount).Mul(decimal.NewFromFloat(mq_price))
		_,err2 = tx.Exec("update user set money=money+?  where id=?",cj, User.Userid)
		//更新用户对应标的存量
		//算成本价 ：最新 ((cost*stock_num)-(mq_amount*mq_price))/(stock_num-mq_amount)
		tmp_cb :=decimal.NewFromFloat(Stock.Cost).Mul(decimal.NewFromFloat(Stock.Stock_num))
		tmp_cur :=decimal.NewFromFloat(mq_amount).Mul(decimal.NewFromFloat(mq_price))
		tmp_allnum :=decimal.NewFromFloat(Stock.Stock_num).Sub(decimal.NewFromFloat(mq_amount))
		var cost decimal.Decimal
		if tmp_allnum.IsZero(){
			cost = decimal.Zero
		}else{
			cost =tmp_cb.Sub(tmp_cur).Div(tmp_allnum)
		}
		log.Printf("订单:%+v,卖单，成本公式((%+v*%+v)-(%+v*%+v))/(%+v-%+v)",orderid,decimal.NewFromFloat(Stock.Cost),decimal.NewFromFloat(Stock.Stock_num),
			decimal.NewFromFloat(mq_amount),decimal.NewFromFloat(mq_price),decimal.NewFromFloat(Stock.Stock_num),decimal.NewFromFloat(mq_amount),
		)
		_,err3 = tx.Exec("insert stock (userid,symbol,band_stock_num,last_time,cost) VALUES(?,?,?,?,?) " +
			"on DUPLICATE KEY UPDATE userid=VALUES(userid),symbol=VALUES(symbol),last_time=VALUES(last_time),band_stock_num=band_stock_num-VALUES(band_stock_num),cost=VALUES(cost)",User.Userid,User.Symbol,mq_amount,now_time,cost)


	}
	if err1 != nil || err2 != nil || err3 != nil{
		tx.Rollback()
		log.Printf("消费订单%v失败，数据库没修改成功 0:%+v 1:%+v 2:%+v 3:%+v\n", orderid,err,err1,err2,err3)
		return 0
	}else{
		tx.Commit()

		//更新订单记录成交时的存量，对账
		sqlStr := "select a.stock_num,(b.money+b.band_money) as money from(select userid,(stock_num+band_stock_num) as stock_num from `stock` where userid=? and symbol=? ) a join  user b on a.userid=b.id"
		var User_money struct{
			Stock_num float64
			Money float64
		}
		err := DB.Get(&User_money, sqlStr,User.Userid,User.Symbol)
		if err==nil {
			_,er1:=DB.Exec("update `order` set deal_stock=?,deal_money=? where orderid=?", User_money.Stock_num, User_money.Money,orderid)
			log.Printf("更新订单记录成交时的存量%v\n", er1)
		}

		return User.Userid
	}
}