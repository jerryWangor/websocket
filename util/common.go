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
}
func FormatReturn(code int,msg string,data interface{}) []byte  {
	 r := &HttpResult{
		 code,msg,data,
	 }
	 res,_ := json.Marshal(r)
	return res
}
