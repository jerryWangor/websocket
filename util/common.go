package util

import (
	"encoding/json"
)

// 定义了各种不同的错误码
const (
	HTTP_OK int = 0
	HTTP_ERROR int = 10002
)
type HttpResult struct {
	Code int `json:"code"` // 状态码
	Msg string `json:"msg"` // 消息
	Data interface{} `json:"data"` // 返回的数据，一般来说是json对象
	Uuid string `json:"uuid"`
}
type ReqData struct {
	MsgType int //消息类型，1 订单相关 2查询自己的订单
	Uuid string
	MsgData PostData //消息内容
	MsgData2 MsgType2 //消息内容
}
func (req *ReqData) FormatReturn(code int,msg string,data interface{}) []byte  {
	 r := &HttpResult{
		 code,msg,data,req.Uuid,
	 }
	 res,_ := json.Marshal(r)
	return res
}
func  FormatReturn(code int,msg string,data interface{}) []byte  {
	r := &HttpResult{
		code,msg,data,"",
	}
	res,_ := json.Marshal(r)
	return res
}
