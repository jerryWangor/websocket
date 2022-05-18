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
)

type orderFace interface {
	PlaceOrder()	//下单
	GenerateOrderId()	//生成订单号
}
type Order struct {
	OrderID int64
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
func init()  {
	OrderIdWork = NewOrderWorker()
}
//func main()  {
//	o :=&Order{}
//	o.GenerateOrderId()
//	fmt.Printf("%v ",o.OrderID)
//}
//生成订单号
func (o *Order) GenerateOrderId() int64{
	//fmt.Printf("\n aaaaaaaa %v %v\n",OrderIdWork,&OrderIdWork)
	o.OrderID = OrderIdWork.GetId()
	return o.OrderID
}
func (o *Order) PlaceOrder(data *PostData) []byte{
	data.Time = time.Now().Unix()
	md5str := fmt.Sprintf("%x", md5.Sum([]byte(strconv.FormatInt(data.Time, 10) + "a4fdc2af6949b3945b8e556d9fbce343")))
	data.Sign = md5str
	data.OrderId = strconv.FormatInt(o.GenerateOrderId(), 10)
	data.Type = 0
	res := o.post(data)
	return res
}

func (o *Order) post(postData *PostData) []byte {

	postBody, _ := json.Marshal(postData)
	responseBody := bytes.NewBuffer(postBody)

	resp, err := http.Post(config.GlobalConfig.Api_addr, "application/json", responseBody)
	if err != nil {
		log.Fatalln("post handleOrder errors", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	sb := string(body)
	fmt.Printf("参数 %+v \n 下单操作接口返回 %v \n",responseBody,sb)
	return body
}