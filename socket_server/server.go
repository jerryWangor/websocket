package socket_server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	redigo "github.com/gomodule/redigo/redis"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
	"websocket/util"
	"websocket/auth"
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
var wsUidToWsid map[int64] int64

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 允许所有的CORS 跨域请求，正式环境可以关闭
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
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
	user	*auth.Userinfo
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
	claim,err:=auth.ParseToken(req.FormValue("token"))
	if err!=nil {
		log.Println("解析token出现错误：",err)
		wsSocket.Close()
		return
	}else if time.Now().Unix() > claim.ExpiresAt {
		log.Println("token时间超时,请重新登陆！")
		wsSocket.Close()
		return
	}
	log.Println("userid:",claim.UserID)
	user,err := auth.CheckUser(claim.UserID)
	if err!=nil {
		log.Println("未找到用户：",err)
		wsSocket.WriteMessage(websocket.TextMessage, util.FormatReturn(util.HTTP_ERROR,"未找到用户",""))
		wsSocket.Close()
		return
	}
	user.Token = req.FormValue("token")
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
	if _, ok := wsUidToWsid[user.Id]; ok {
		// 该用户存在，T出之前的客户端
		wsConnAll[wsUidToWsid[user.Id]].close()
		log.Println("该用户存在，T出之前的客户端")
	}
	wsUidToWsid[user.Id] = maxConnId
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
func httpHandler(resp http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	resp.Header().Set("Access-Control-Allow-Origin", "*")
	resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	resp.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	//d := fmt.Sprintf("参数有：%+v",req.Form)
	//resp.Write([]byte(d))
	user,err := auth.Login(req.FormValue("user"),req.FormValue("pass"))
	if err!=nil {
		resp.Write(util.FormatReturn(util.HTTP_ERROR,"未找到该用户",""))
		return
	}
	user.Token,_=auth.GenerateToken(strconv.FormatInt(user.Id,10))
	resp.Write(util.FormatReturn(util.HTTP_OK,"",user))
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
	type ReqData struct {
		MsgType int //消息类型，1 订单相关
		MsgData util.PostData //消息内容
	}

	for{
		select {
			case msg := <-wsConn.inChan:
				//			 msg.data = []byte(`
				//	{"msgdata":{ "Action": 0, "Symbol": "BTC","Side":0,"Amount":11,"Price":1000},"msgtype":1}//买
				//`)
				//			 msg.data = []byte(`
				//	{"msgdata":{ "Action": 0, "Symbol": "BTC","Side":1,"Amount":11,"Price":1000},"msgtype":1}//卖
				//`)
				postdata := &ReqData{}
				err := json.Unmarshal(msg.data,postdata)
				if err !=nil {
					wsConn.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_ERROR,"参数有误！",""));
					log.Println("参数解析失败", err.Error())
				}
				switch postdata.MsgType {
					case 1://下单类型
						flag := false
						if postdata.MsgData.Symbol =="" {
							flag = true
						}
						if flag {
							wsConn.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_ERROR,"订单Symbol参数有误！",""));
							log.Println("订单参数有误")
						}else{
							switch postdata.MsgData.Action {
								case 0://挂单
									if postdata.MsgData.Amount==0 || postdata.MsgData.Price==0{
										wsConn.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_ERROR,"挂单价格或数量为空！",""));
										log.Println("挂单价格或数量为空")
										goto END
									}
									switch postdata.MsgData.Side {
										case 0://买
											all_amount := postdata.MsgData.Price*postdata.MsgData.Amount
											//fmt.Println("aaa",all_amount,wsConn.user.Money,all_amount>wsConn.user.Money)
											if  all_amount>wsConn.user.Money {
												wsConn.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_ERROR,"下单失败！余额不足！",""));
												log.Println("下单失败！余额不足！")
											}else{
												t := wsConn.order.PlaceOrder(&postdata.MsgData, int(wsConn.user.Id))

												fmt.Printf("%+v %v\n",t,t.Msg=="success")
												if t.Msg=="success" {
													wsConn.user.Money = wsConn.user.Money - all_amount
													wsConn.user.Band_money = wsConn.user.Band_money + all_amount
													_,err = util.DB.Exec("update user set money=?, band_money=? where id=?", wsConn.user.Money,wsConn.user.Band_money, int(wsConn.user.Id))
													if err != nil {
														log.Printf("get failed, err:%v\n", err)
													}
												}
												mm,_ :=json.Marshal(t)
												if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage, mm); err != nil {
													log.Println("发送消息给客户端发生错误", err.Error())
													// 切断服务
													wsConn.close()
												}
											}
										case 1://卖
											//看用户是否有库存
											sqlStr := "select stock_num from stock where userid=? and symbol=?"
											var stock_num float64
											err := util.DB.Get(&stock_num, sqlStr, int(wsConn.user.Id), postdata.MsgData.Symbol)
											if err != nil {
												log.Printf("get failed, err:%v\n", err)
											}else{
												if stock_num>=postdata.MsgData.Amount {//可以卖出下单
													t := wsConn.order.PlaceOrder(&postdata.MsgData, int(wsConn.user.Id))
													fmt.Printf("%+v %v\n",t,t.Msg=="success")
													if t.Msg=="success" {
														//锁定卖出的库存
														_,err = util.DB.Exec("update stock set stock_num=stock_num-?, band_stock_num=+? where userid=? and symbol=?",
															postdata.MsgData.Amount,postdata.MsgData.Amount,  int(wsConn.user.Id), postdata.MsgData.Symbol)
														if err != nil {
															log.Printf("get failed, err:%v\n", err)
														}
													}
													mm,_ :=json.Marshal(t)
													if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage,mm); err != nil {
														log.Println("发送消息给客户端发生错误", err.Error())
														// 切断服务
														wsConn.close()
													}

												}else{
													log.Printf("库存不够:%v %v\n", stock_num,postdata.MsgData.Amount)
													wsConn.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_ERROR,"下单失败！库存不够！",""));
												}
											}

									}
								case 1://撤单
									if postdata.MsgData.OrderId==""{
										wsConn.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_ERROR,"撤单订单号为空！",""));
										log.Println("撤单订单号为空")
										goto END
									}
									//查看订单状态
									sqlStr := "select status from `order` where orderid=?"
									var status int
									err := util.DB.Get(&status, sqlStr, postdata.MsgData.OrderId)
									if err != nil {
										log.Printf("get failed, err:%v\n", err)
									}else{
										if status==1 || status==3{//1成功2部分成交0未成交3撤单
											wsConn.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_ERROR,"无法撤单，订单已全部成交或已申请撤单！",""));
											log.Println("无法撤单，订单已全部成交或已申请撤单")
											goto END
										}else{
											t := wsConn.order.CancelOrder(&postdata.MsgData)
											fmt.Printf("%+v %v\n",t,t.Msg=="success")
											if t.Msg=="success" {
												_,err = util.DB.Exec("update `order` set status=? where orderid=?", 3,postdata.MsgData.OrderId )
												if err != nil {
													log.Printf("get failed, err:%v\n", err)
												}
											}
											mm,_ :=json.Marshal(t)
											if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage, mm); err != nil {
												log.Println("发送消息给客户端发生错误", err.Error())
												// 切断服务
												wsConn.close()
											}
										}
									}

							}
							END:
								fmt.Println("END!!!")
						}
				default:
						log.Printf("未知消息类型 %+v\n",postdata)

				}
				//log.Printf("%+v\n", postdata)
				//postdata := &util.PostData{}
				//err := json.Unmarshal(msg.data,postdata)
				//if err !=nil {
				//	log.Println("参数解析失败", err.Error())
				//}
				//res := wsConn.order.PlaceOrder(postdata)

				//if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage,res ); err != nil {
				//	log.Println("发送消息给客户端发生错误", err.Error())
				//	// 切断服务
				//	wsConn.close()
				//	return
				//}
			case <-wsConn.closeChan:
			// 获取到关闭通知
			return
		}
	}
}

//读取redis消息推送到outChan
func (wsConn *wsConnection) getRedisPush()  {
	c := time.NewTicker(555 * time.Second)
	for now:= range c.C {
		select {
				case v, ok := <-wsConn.closeChan:
					fmt.Printf("v is %v , ok is %v",v,ok)
					c.Stop()
					return
				default:
					fmt.Printf("未关闭通道继续运行\n")
			}
		//conn := util.GetRedisPool()
		//r,e :=redigo.Bytes(conn.Do("get","url"))
		//if e==nil{
		//	wsConn.outChan <-&wsMessage{websocket.TextMessage,r}
		//}
		r,_ :=json.Marshal(wsConn.user)
		wsConn.outChan <-&wsMessage{websocket.TextMessage,r}
		fmt.Printf("%v \n", now)
	}
}

// 读取outChan消息发送给客户端
func (wsConn *wsConnection) wsWriteLoop() {

	for {
		select {
		// 取一个应答
		case msg := <-wsConn.outChan:
			// 写给websocket
			data := &auth.Userinfo{}
			json.Unmarshal(msg.data,data)
			if err := wsConn.wsSocket.WriteMessage(msg.messageType,util.FormatReturn(util.HTTP_OK,"",data)); err != nil {
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
		delete(wsUidToWsid, wsConn.user.Id)
		close(wsConn.closeChan)
	}
}

//广播
func guangbo(msgtype int, data []byte) {
	for _, wsConn := range wsConnAll {
		if err := wsConn.wsSocket.WriteMessage(msgtype, util.FormatReturn(util.HTTP_OK,"",data)); err != nil {
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
	wsUidToWsid = make(map[int64]int64)
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/http", httpHandler)
	for i:=0;i<3;i++ {
		//开多少个协程，每个协程对应一个消费组下的消费组
		go listenMQ(i)
	}
	http.ListenAndServe(addrPort, nil)
}
func listenMQ(i int)  {
	c := time.NewTicker(5 * time.Second)
	conn := util.GetRedisPool()
	defer conn.Close()
	for _ = range c.C {

		//获取成交结果
		trades,err :=redigo.Strings(conn.Do("keys","matching:trades*"))
		//获取撤单结果
		//cancelresults,err :=redigo.Strings(conn.Do("keys","matching:cancelresults*"))
		if err!=nil{
			log.Printf("errors %v",err)
		}
		//log.Printf("%+v ",trades,)
		//消费成交结果
		for _,symbol := range trades{
			groups,err := conn.Do("xinfo","groups",symbol)
			groups_name := symbol+"_groups"
			if len(groups.([]interface{}))==0{
				//创建消费组
				conn.Do("xgroup","create",symbol,groups_name,0)
				//log.Printf("创建消费组 %v",groups_name)
			}else{
				var P1 struct{
					Name string `redis:"name"`
					Consumers int `redis:"consumers"`
					Pending int `redis:"pending"`
					Ldi string `redis:"last-delivered-id"`
				}
				flag := false
				for _,v := range groups.([]interface{}){
					vv, err := redigo.Values(v,nil)
					if err != nil {
						log.Println(err)
					}
					if err := redigo.ScanStruct(vv, &P1); err != nil {
						log.Println(err)
					}
					if P1.Name==groups_name{
						flag = true
					}
				}
				if !flag{
					//创建消费组
					conn.Do("xgroup","create",symbol,groups_name,0)
				}
			}
			if err!=nil {
				log.Printf("errors1 %v",err)
				continue
			}
			//拉取信息到groups_user用户消费
			groups_user := "u" + strconv.Itoa(i)
			conn.Do("xreadgroup","group",groups_name, groups_user,"count","2", "streams",symbol, ">")

			//查看消费者各自接收到的未ack的数据
			r,err :=conn.Do("xreadgroup","group",groups_name, groups_user,"count","5", "streams",symbol, "0")
			if err!=nil {
				log.Printf("errors2 %v",err)
				continue
			}
			for _,v := range r.([]interface{}){
				for kk,vv := range v.([]interface{}){
					if kk==1{
						for _,vvv := range vv.([]interface{}) {
							key,_:= redigo.Strings(vvv,nil)
							for kkkk,vvvv := range vvv.([]interface{}) {
								if kkkk==1 {
									val,_:= redigo.StringMap(vvvv,nil)
									log.Printf("info :%+v %+v %T\n", key[0],val,val)
									//利用取到的key,val 来发送给客户端消费
									if _,ok := val["takerId"];ok{

										sqlStr := "select userid,amount,price,side,deal_amount,symbol from `order` where orderid=? and status <>1"
										var User struct{
											Userid int64
											Amount float64
											Price float64
											Side int
											Deal_amount float64
											Symbol string
										}
										err := util.DB.Get(&User, sqlStr,val["takerId"] )
										if err!=nil {
											log.Printf("获取原订单amount 失败 %+v\n", err)
											continue
										}
										var status int
										mq_amount,_ := strconv.ParseFloat(val["amount"],64)
										mq_price,_ := strconv.ParseFloat(val["price"],64)
										if User.Amount-User.Deal_amount == mq_amount{
											//全部成交
											status = 1
										}else {
											//部分成交
											status = 2
										}
										//修改订单状态
										//开始一个事务，返回一个事务对象tx
										tx, err := util.DB.Beginx()
										//修改订单状态及成交数量，成交时间
										var err1,err2,err3 error
										switch User.Side {
											case 0://买单
												now_time :=time.Now().Format("2006-01-02 15:04:05")
												_,err1 = tx.Exec("update `order` set status=?, deal_time=?,deal_amount=deal_amount+? where orderid=?",
													status,
													now_time,
													mq_amount,
													val["takerId"],
													)
												//修改用户band_money扣减
												//如果成交价格低于挂单价格，则还要返还冻结里面钱到账户里
												cj := mq_amount*(User.Price-mq_price)
												dj := User.Price * mq_amount
												_,err2 = tx.Exec("update user set money=money+ band_money=band_money-?  where id=?",cj, dj,User.Userid)
												//更新用户对应标的存量
												_,err3 = tx.Exec("insert User (userid,symbol,stock_num,last_time) VALUES(?,?,?,?) " +
													"on DUPLICATE KEY UPDATE userid=VALUES(userid),symbol=VALUES(symbol),last_time=VALUES(last_time),stock_num=VALUES(stock_num)+stock_num",User.Userid,User.Symbol,mq_amount,now_time)

											case 1: //卖单
												now_time :=time.Now().Format("2006-01-02 15:04:05")
												_,err1 = tx.Exec("update `order` set status=?, deal_time=?,deal_amount=deal_amount+? where orderid=?",
													status,
													now_time,
													mq_amount,
													val["takerId"],
												)
												//如果成交价格高于挂单价格，则还要按实际价格加钱
												cj := mq_amount * mq_price
												_,err2 = tx.Exec("update user set money=money+  where id=?",cj, User.Userid)
												//更新用户对应标的存量
												_,err3 = tx.Exec("insert User (userid,symbol,stock_num,last_time) VALUES(?,?,?,?) " +
													"on DUPLICATE KEY UPDATE userid=VALUES(userid),symbol=VALUES(symbol),last_time=VALUES(last_time),stock_num=VALUES(stock_num)-stock_num",User.Userid,User.Symbol,mq_amount,now_time)

										}


										if err1 != nil || err2 != nil || err3 != nil{
											tx.Rollback()
											log.Printf("消费订单%v失败，数据库没修改成功 0:%+v 1:%+v 2:%+v 3:%+v\n", val["takerId"],err,err1,err2,err3)
										}else{
											tx.Commit()
											//ack消费信息
											//xack testxkey mygroup 1652948829278-0
											conn.Do("xack",symbol,groups_name,key[0])

											//mysql 的状态改好，说明消息消费成功
											if _, ok := wsUidToWsid[User.Userid]; ok {
												//更新用户的货币存量
												sqlStr := "select money,band_money from `user` where id=?"
												var User_money struct{
													Money float64
													Band_money float64
												}
												err := util.DB.Get(&User_money, sqlStr,User.Userid)
												if err!=nil {
													log.Printf("获取原订单amount 失败 %+v\n", err)
												}else{
													wsConnAll[wsUidToWsid[User.Userid]].user.Money = User_money.Money
													wsConnAll[wsUidToWsid[User.Userid]].user.Band_money = User_money.Band_money
												}
												// 该用户存在，发送订单成功的消息
												switch status {
													case 1:
														wsConnAll[wsUidToWsid[User.Userid]].wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"订单成功！", val))
													case 2:
														wsConnAll[wsUidToWsid[User.Userid]].wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"部分订单成功！", val));
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		//消费撤单结果
		//for _,symbol := range cancelresults{
		//
		//}

		log.Printf("wsConnAll:%+v\n",wsConnAll)
		log.Printf("wsUidToWsid:%+v\n \n",wsUidToWsid)
	}
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

