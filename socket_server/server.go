package socket_server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	redigo "github.com/gomodule/redigo/redis"
	"log"
	"net/http"
	"sync"
	"time"
	"websocket/util"
)

const (
	// 允许等待的写入时间
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// 最大的连接ID，每次连接都加1 处理
var maxConnId int64

// 客户端读写消息
type wsMessage struct {
	// websocket.TextMessage 消息类型
	messageType int
	data        []byte
}

// ws 的所有连接
// 用于广播
var wsConnAll map[int64]*wsConnection

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 允许所有的CORS 跨域请求，正式环境可以关闭
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
type userinfo struct {
	id int64	//用户ID
	money int64		//用户钱
	star_ids string		//自选股ID
}
// 客户端连接
type wsConnection struct {
	wsSocket *websocket.Conn // 底层websocket
	inChan   chan *wsMessage // 读队列
	outChan  chan *wsMessage // 写队列
	mutex     sync.Mutex // 避免重复关闭管道,加锁处理
	isClosed  bool
	closeChan chan byte // 关闭通知
	id        int64
	user	*userinfo
	order 	*util.Order
}
func wsHandler(resp http.ResponseWriter, req *http.Request) {
	// 应答客户端告知升级连接为websocket

	wsSocket, err := upgrader.Upgrade(resp, req, nil)
	req.ParseForm()
	if err != nil {
		log.Println("升级为websocket失败", err.Error())
		wsSocket.Close()
		return
	}
	//登录
	user,err := login(req.FormValue("user"),req.FormValue("pass"))

	if err!=nil{
		log.Printf("登录信息有误,用户（%v）,密码（%v）%v \n",req.FormValue("user"),req.FormValue("pass"), err.Error())
		wsSocket.Close()
		return
	}
	maxConnId++
	// TODO 如果要控制连接数可以计算，wsConnAll长度
	// 连接数保持一定数量，超过的部分不提供服务
	wsConn := &wsConnection{
		wsSocket:  wsSocket,
		inChan:    make(chan *wsMessage, 1000),
		outChan:   make(chan *wsMessage, 1000),
		closeChan: make(chan byte),
		isClosed:  false,
		id:        maxConnId,
		user:	user,
		order:	&util.Order{},
	}
	wsConnAll[maxConnId] = wsConn
	log.Println("当前在线人数", len(wsConnAll))

	// 读取客户端消息
	go wsConn.wsReadLoop()
	//处理客户端消息
	go wsConn.reqHandle()
	// 读取redis推送消息
	go wsConn.getRedisPush()
	// 推送消息给客户端
	go wsConn.wsWriteLoop()
}

func login(username string,password string)  (*userinfo,error) {
	if username=="root" &&  password=="root" {
		return &userinfo{
			11,8888,"1,2,3",
		},nil
	}
	return &userinfo{},errors.New("未找到该用户")
}


// 处理客户端的消息到inchan
func (wsConn *wsConnection) wsReadLoop() {
	// 设置消息的最大长度
	wsConn.wsSocket.SetReadLimit(maxMessageSize)
	//wsConn.wsSocket.SetReadDeadline(time.Now().Add(pongWait))
	for {
		// 读一个message
		msgType, data, err := wsConn.wsSocket.ReadMessage()
		if err != nil {
			websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure)
			log.Println("消息读取出现错误", err.Error())
			wsConn.close()
			return
		}
		req := &wsMessage{
			msgType,
			data,
		}
		// 放入请求队列,消息入栈
		select {
		case wsConn.inChan <- req:
		case <-wsConn.closeChan:
			return
		}
	}
}
// 处理inchan中的信息
func (wsConn *wsConnection) reqHandle(){
	for{
		select {
			case msg := <-wsConn.inChan:
				 msg.data = []byte(`
		{"Accid": 1, "Action": 0, "Symbol": "BTC","Side":1,"Amount":11,"Price":11}
	`)
				postdata := &util.PostData{}
				err := json.Unmarshal(msg.data,postdata)
				if err !=nil {
					log.Println("参数解析失败", err.Error())
				}
				res := wsConn.order.PlaceOrder(postdata)
				fmt.Printf("下单操作接口返回 %v \n 参数%+v",res,postdata)

				if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage, []byte(res)); err != nil {
					log.Println("发送消息给客户端发生错误", err.Error())
					// 切断服务
					wsConn.close()
					return
				}
			case <-wsConn.closeChan:
			// 获取到关闭通知
			return
		}
	}
}

//读取redis消息推送到outChan
func (wsConn *wsConnection) getRedisPush()  {
	c := time.NewTicker(21 * time.Second)
	for now:= range c.C {
		select {
				case v, ok := <-wsConn.closeChan:
					fmt.Printf("v is %v , ok is %v",v,ok)
					c.Stop()
					return
				default:
					fmt.Printf("未关闭通道继续运行")
			}
		conn := util.GetRedisPool()
		r,e :=redigo.Bytes(conn.Do("get","url"))
		if e==nil{
			wsConn.outChan <-&wsMessage{websocket.TextMessage,r}
		}
			//o :=&Order{}
			//o.GenerateOrderId()
			//fmt.Printf("订单号  %v ",o.OrderID)
		fmt.Printf("%v %v  %v %v\n", now,len(wsConn.outChan),e , string(r))
	}
}

// 读取outChan消息发送给客户端
func (wsConn *wsConnection) wsWriteLoop() {

	for {
		select {
		// 取一个应答
		case msg := <-wsConn.outChan:
			// 写给websocket
			if err := wsConn.wsSocket.WriteMessage(msg.messageType, msg.data); err != nil {
				log.Println("发送消息给客户端发生错误", err.Error())
				// 切断服务
				wsConn.close()
				return
			}
		case <-wsConn.closeChan:
			// 获取到关闭通知
			return

		}
	}
}

// 关闭连接
func (wsConn *wsConnection) close() {
	log.Println("关闭连接被调用了")
	wsConn.wsSocket.Close()
	wsConn.mutex.Lock()
	defer wsConn.mutex.Unlock()
	if wsConn.isClosed == false {
		wsConn.isClosed = true
		// 删除这个连接的变量
		delete(wsConnAll, wsConn.id)
		close(wsConn.closeChan)
	}
}

//广播
func guangbo(msgtype int, data []byte) {
	for _, wsConn := range wsConnAll {
		if err := wsConn.wsSocket.WriteMessage(msgtype, data); err != nil {
			log.Println("发送消息给客户端发生错误", err.Error())
			// 切断服务
			wsConn.close()
			return
		}
	}
}

// 启动程序
func StartWebsocket(addrPort string) {
	wsConnAll = make(map[int64]*wsConnection)
	http.HandleFunc("/ws", wsHandler)

	http.ListenAndServe(addrPort, nil)

}
func init() {
	//c, err := redis.Dial("tcp", "10.0.111.154:52311",redis.DialPassword("crimoon2015"))
	//redis_conn = c
	//if err != nil {
	//	fmt.Println("sorry,redis_conn error:",err)
	//}
	// logFile, err := os.OpenFile("./a.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	// if err != nil {
	// 	log.Panic("打开日志文件异常")
	// }
	// logger = log.New(nil, "socket \t", log.Ldate|log.Ltime|log.Lshortfile)
	// logger = log
}
//func main() {
//	StartWebsocket("127.0.0.1:20002")
//	defer Redis_pool.Close()
//}
