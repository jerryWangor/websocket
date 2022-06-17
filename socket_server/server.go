package socket_server

import (
	"encoding/json"
	"fmt"

	//"fmt"
	"github.com/gorilla/websocket"
	redigo "github.com/gomodule/redigo/redis"
	"github.com/shopspring/decimal"
	"log"
	"net/http"
	//"os"

	//"runtime"
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
	Mux 	sync.RWMutex //并发发消息的锁
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
	//log.Println("userid:",claim.UserID)
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
		inChan:    make(chan *wsMessage, 10000),
		outChan:   make(chan *wsMessage, 10000),
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
	//go wsConn.getRedisPush()
	// 推送消息给客户端
	//go wsConn.wsWriteLoop()
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
var read_num = 0
var read_chan_num = 0
var read_chanxd_num = 0
var read_chanxdd_num = 0
var read_chanxdds_num = 0

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
			read_num++
		case <-wsConn.closeChan:
			// 获取到关闭通知,等消息处理完后关闭改携程
			//for{
			if len(wsConn.inChan)==0{
				return
			}
		}
	}
}
// 处理inchan中的信息
func (wsConn *wsConnection) reqHandle(){
	conn := util.GetRedisPool()
	defer conn.Close()

	for{
		select {
			case msg:= <-wsConn.inChan:
				read_chan_num++
				//			 msg.data = []byte(`
				//	{"msgdata":{ "Action": 0, "Symbol": "BTC","Side":0,"Amount":11,"Price":1000},"msgtype":1}//买
				//`)
				//			 msg.data = []byte(`
				//	{"msgdata":{ "Action": 0, "Symbol": "BTC","Side":1,"Amount":11,"Price":1000},"msgtype":1}//卖
				//`)
				postdata := &util.ReqData{}
				err := json.Unmarshal(msg.data,postdata)
				if err !=nil {
					wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"参数有误！",""))
					log.Println("参数解析失败", err.Error())
				}
				//log.Printf("请求来了 %+v\n",postdata)
				switch postdata.MsgType {
					case 1://下单类型
						read_chanxd_num++
						flag := false
						if postdata.MsgData.Symbol =="" {
							flag = true
						}
						if flag {
							wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"订单Symbol参数有误！",""));
							log.Println("订单参数有误")
						}else{
							switch postdata.MsgData.Action {
								case 0://挂单
									if postdata.MsgData.Amount==0 || postdata.MsgData.Price==0{
										wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"挂单价格或数量为空！",""));
										log.Println("挂单价格或数量为空")
										goto END
									}
									switch postdata.MsgData.Side {
										case 0://买
											all_amount := postdata.MsgData.Price*postdata.MsgData.Amount
											//fmt.Println("aaa",all_amount,wsConn.user.Money,all_amount>wsConn.user.Money)
											if  all_amount>wsConn.user.Money {
												wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"下单失败！余额不足！",""));
												log.Println("下单失败！余额不足！")
											}else{
												t := wsConn.order.PlaceOrder(&postdata.MsgData, int(wsConn.user.Id))

												//fmt.Printf("%+v %v\n",t,t.Msg=="success")
												read_chanxdd_num++
												if t.Msg=="success" {
													read_chanxdds_num++
													wsConn.user.Money,_ = (decimal.NewFromFloat(wsConn.user.Money).Sub(decimal.NewFromFloat(all_amount))).Float64()
													wsConn.user.Band_money,_ = (decimal.NewFromFloat(wsConn.user.Band_money).Add(decimal.NewFromFloat(all_amount))).Float64()
													//fmt.Printf("下单的钱：%+v %v\n",wsConn.user.Money,wsConn.user.Band_money)
													_,err = util.DB.Exec("update user set money=?, band_money=? where id=?", wsConn.user.Money,wsConn.user.Band_money, int(wsConn.user.Id))
													if err != nil {
														log.Printf("get failed, err:%v\n", err)
													}
												}
												//mm,_ :=json.Marshal(t)
												if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage, postdata.FormatReturn(util.HTTP_OK,"请求成功！",t)); err != nil {
													log.Println("发送消息给客户端发生错误1", err.Error())
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
												wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"下单失败！库存不足！",""));
												log.Printf("get failed, err:%v\n", err)
											}else{
												if stock_num>=postdata.MsgData.Amount {//可以卖出下单
													t := wsConn.order.PlaceOrder(&postdata.MsgData, int(wsConn.user.Id))
													//fmt.Printf("%+v %v\n",t,t.Msg=="success")
													read_chanxdd_num++
													if t.Msg=="success" {
														read_chanxdds_num++
														//锁定卖出的库存
														_,err = util.DB.Exec("update stock set stock_num=stock_num-?, band_stock_num=band_stock_num+? where userid=? and symbol=?",
															postdata.MsgData.Amount,postdata.MsgData.Amount,  int(wsConn.user.Id), postdata.MsgData.Symbol)
														log.Printf("卖出%+v 多少个%+v,价格%+v\n",postdata.MsgData.Symbol,postdata.MsgData.Amount,postdata.MsgData.Price)
														if err != nil {
															log.Printf("get failed, err:%v\n", err)
														}
													}
													//mm,_ :=json.Marshal(t)
													if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_OK,"请求成功！",t)); err != nil {
														log.Println("发送消息给客户端发生错误2", err.Error())
														// 切断服务
														wsConn.close()
													}

												}else{
													log.Printf("库存不够:%v %v\n", stock_num,postdata.MsgData.Amount)
													wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"下单失败！库存不够！",""));
												}
											}

									}
								case 1://撤单
									if postdata.MsgData.OrderId==""{
										wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"撤单订单号为空！",""));
										log.Println("撤单订单号为空")
										goto END
									}
									//查看订单状态
									sqlStr := "select status from `order` where orderid=? and userid=?"
									var status int
									err := util.DB.Get(&status, sqlStr, postdata.MsgData.OrderId,wsConn.user.Id)
									if err != nil {
											wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"无法撤单，未找到该订单！",""));
										log.Printf("get failed, err:%v\n", err)
									}else{
										if status==1 || status==3 || status==4{//1成功2部分成交0未成交3撤单
											wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"无法撤单，订单已全部成交或已申请撤单！",""));
											log.Println("无法撤单，订单已全部成交或已申请撤单")
											goto END
										}else{
											t := wsConn.order.CancelOrder(&postdata.MsgData)
											log.Printf("%+v %v\n",t,t.Msg=="success")
											if t.Msg=="success" {
												_,err = util.DB.Exec("update `order` set status=? where orderid=?", 3,postdata.MsgData.OrderId )
												if err != nil {
													log.Printf("get failed, err:%v\n", err)
												}
											}
											//mm,_ :=json.Marshal(t)
											if err := wsConn.wsSocket.WriteMessage(websocket.TextMessage, postdata.FormatReturn(util.HTTP_OK,"请求成功！",t)); err != nil {
												log.Println("发送消息给客户端发生错误3", err.Error())
												// 切断服务
												wsConn.close()
											}
										}
									}

							}
							END:
								log.Println("END!!!")
						}
					case 2://2查询自己的订单
						if postdata.MsgData2.Status==4{
							where := " where 1=1 "
							if postdata.MsgData2.Symbol!="" {
								where +=" and symbol='matching:trades:" + postdata.MsgData2.Symbol +"'"
							}
							//sqlStr := "select makerid,takerid,takerside,amount,price,deal_time from `match_log` " + where +" order by deal_time desc"
							sqlStr := "select a.*,(select name from `order` join user on `order`.userid=`user`.id where orderid=makerid) mname,(select name from `order` join user on `order`.userid=`user`.id where orderid=takerid) tname from (select makerid,takerid,takerside,0+cast(amount AS CHAR) amount,0+cast(price AS CHAR) price,DATE_FORMAT(deal_time,\"%Y-%m-%d %H:%i:%s\") as deal_time from `match_log` " + where +" order by deal_time desc) a"
							var MatchList []util.MatchLine
							err := util.DB.Select(&MatchList, sqlStr)
							//fmt.Printf("aa:%+v",MatchList)
							if err != nil{
								wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"未查询到信息！",""));
								log.Printf("err %+v\n",err)
							}else{
								wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_OK,"查询成功！",MatchList));
							}
						}else{
							where := " where userid=? "
							switch postdata.MsgData2.Status {
							case 1://委托
								where +=" and status in(0,2,3)"
							case 2://撤单
								where +=" and status=4"
							case 3://成交记录
								where +=" and status=1"
							}
							if postdata.MsgData2.Orderid!="" {
								where +=" and orderid=" + postdata.MsgData2.Orderid
							}
							if postdata.MsgData2.Symbol!="" {
								where +=" and symbol='" + postdata.MsgData2.Symbol +"'"
							}
							sqlStr := "select orderid,status,DATE_FORMAT(create_time,\"%Y-%m-%d %H:%i:%s\") as create_time,DATE_FORMAT(deal_time,\"%Y-%m-%d %H:%i:%s\") as deal_time,symbol,action,side,0+cast(amount AS CHAR) amount,0+cast(price AS CHAR) price,userid,0+cast(deal_stock AS CHAR) deal_stock,0+cast(deal_amount AS CHAR) deal_amount,0+cast(deal_money AS CHAR) deal_money from `order` " + where +"order by create_time desc"
							var OrderList []util.OrderLine
							err := util.DB.Select(&OrderList, sqlStr,wsConn.user.Id)
							if err != nil{
								wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"未查询到信息！",""));
								log.Printf("err %+v\n",err)
							}else{
								wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_OK,"查询成功！",OrderList));
							}
						}

					case 3://五档请求
						if postdata.MsgData2.Symbol !="" {
							key := "data:top5:s:symbol:"+ postdata.MsgData2.Symbol
							top100_string,err :=redigo.Bytes(conn.Do("get",key))
							if err!=nil {
								log.Printf("get top100，%+v err %+v\n",key,err)
							}else{
								top100:= make(map[string]interface{})
								 json.Unmarshal(top100_string,&top100)
								//log.Printf("err:%+v\n",err)
								wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_OK,"",top100));
							}
						}else{
							wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"标的为空！",""));
						}
					case 4://用户信息
						sqlStr := "select symbol,stock_num,last_time,band_stock_num,cost from `stock` where userid=? "
						var StockList []*auth.Stock
						_ =util.DB.Select(&StockList, sqlStr,wsConn.user.Id)
						//fmt.Printf("%+v   %+v\n",err,StockList)
						//wsConn.user.Stocks = StockList
						var tmp_map = make(map[string]*auth.Stock)
						for _,v:=range StockList  {

							key := "matching:s:price:"+ v.Symbol
							cur_price,err :=redigo.String(conn.Do("get",key))
							if err!=nil {
								//log.Printf("get top100，%+v err %+v\n",key,err)
							}else{
								v.Cur_price,_ = strconv.ParseFloat(cur_price, 64)
							}
							tmp_map[v.Symbol] = v

						}
						wsConn.user.Stocks = tmp_map
						wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_OK,"",wsConn.user));
					case 5:

				default:
					wsConn.wsSocket.WriteMessage(websocket.TextMessage,postdata.FormatReturn(util.HTTP_ERROR,"未知消息类型！",""));
					log.Printf("未知消息类型 %+v\n",postdata)

				}
			case <-wsConn.closeChan:
				// 获取到关闭通知,等消息处理完后关闭改携程
				//for{
					if len(wsConn.inChan)==0{
						return
					}
				//}
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
					//fmt.Printf("未关闭通道继续运行\n")
			}
		//conn := util.GetRedisPool()
		//r,e :=redigo.Bytes(conn.Do("get","url"))
		//if e==nil{
		//	wsConn.outChan <-&wsMessage{websocket.TextMessage,r}
		//}
		r,_ :=json.Marshal(wsConn.user)
		wsConn.outChan <-&wsMessage{websocket.TextMessage,r}
		log.Printf("%v \n", now)
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
				log.Println("发送消息给客户端发生错误4", err.Error())
				// 切断服务
				wsConn.close()
				return
			}
		case <-wsConn.closeChan:
			// 获取到关闭通知,等消息处理完后关闭改携程
			for{
				if len(wsConn.outChan)==0{
					return
				}
			}
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
	http.HandleFunc("/initboot", auth.InitBoot)
	http.HandleFunc("/bootlist", auth.Bootlist)
	for i:=0;i<30;i++ {
		//开多少个协程，每个协程对应一个消费组下的消费组
		go listenMQ(i)
	}
	http.ListenAndServe(addrPort, nil)
}
func listenMQ(i int)  {
	c := time.NewTicker(2 * time.Second)
	conn := util.GetRedisPool()
	defer conn.Close()
	for _ = range c.C {

		//获取成交结果
		trades,err :=redigo.Strings(conn.Do("keys","matching:trades*"))
		//获取撤单结果
		cancelresults,err :=redigo.Strings(conn.Do("keys","matching:cancelresults*"))
		if err!=nil{
			log.Printf("errors %v",err)
		}
		//log.Printf("%+v ",trades,)
		//消费成交结果
		for _,symbol := range trades{
			groups_name,groups_user,nil := util.GetSymbolConsumeData(conn,symbol,i)
			//查看消费者各自接收到的未ack的数据
			r,err :=conn.Do("xreadgroup","group",groups_name, groups_user,"count","20", "streams",symbol, "0")
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
									if takerId,ok := val["takerId"];ok{
										if userid := util.OrderMQHandle(takerId,val);userid>0{
											//ack消费信息
											//xack testxkey mygroup 1652948829278-0
											conn.Do("xack",symbol,groups_name,key[0])
											log.Printf("处理订单成功  %+v\n", takerId)


											//mysql 的状态改好，说明消息消费成功
											if wid, ok := wsUidToWsid[userid]; ok {
												//更新用户的货币存量
												sqlStr := "select money,band_money from `user` where id=?"
												var User_money struct{
													Money float64
													Band_money float64
												}
												err := util.DB.Get(&User_money, sqlStr,userid)
												if err!=nil {
													log.Printf("获取user 失败 %+v\n", err)
												}else{
													if wca,o:=wsConnAll[wid];o{
														wca.user.Money = User_money.Money
														wca.user.Band_money = User_money.Band_money
													}
												}
												// 该用户存在，发送订单成功的消息
												if wca,o:=wsConnAll[wid];o{
													log.Printf("debug:%+v,%+v\n",wsConnAll,wid);

													//wca.Mux.Lock()
													//wca.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"订单成功！", val))
													//wca.Mux.Unlock()
													wca.sendWsMsg(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"订单成功！", val))
												}
											}
										}else{
											log.Printf("处理订单失败takerId  %+v\n", takerId)
										}
									}
									if makerId,ok := val["makerId"];ok{
										if userid := util.OrderMQHandle(makerId,val);userid>0{
											//ack消费信息
											//xack testxkey mygroup 1652948829278-0
											conn.Do("xack",symbol,groups_name,key[0])
											log.Printf("处理订单成功  %+v\n", makerId)
											//mysql 的状态改好，说明消息消费成功
											if wid, ok := wsUidToWsid[userid]; ok {
												//更新用户的货币存量
												sqlStr := "select money,band_money from `user` where id=?"
												var User_money struct{
													Money float64
													Band_money float64
												}
												err := util.DB.Get(&User_money, sqlStr,userid)
												if err!=nil {
													log.Printf("获取user 失败 %+v\n", err)
												}else{
													if wca,o:=wsConnAll[wid];o{
														wca.user.Money = User_money.Money
														wca.user.Band_money = User_money.Band_money
													}
												}
												// 该用户存在，发送订单成功的消息
												if wca,o:=wsConnAll[wid];o {
													log.Printf("debug:%+v,%+v\n",wsConnAll,wid);

													//wca.Mux.Lock()
													//wca.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"订单成功！", val))
													//wca.Mux.Unlock()
													wca.sendWsMsg(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"订单成功！", val))
												}

											}
										}else{
											log.Printf("处理订单失败makerId  %+v\n", makerId)
										}
									}
									//存成交日志
									is,_:=strconv.ParseInt(val["timestamp"],10,64)
									tm := time.Unix(is/1000000, 0)
									dTime:= tm.Format("2006-01-02 15:04:05")
									_,err:=util.DB.Exec("insert match_log (makerId,takerid,takerside,amount,price,deal_time,symbol) VALUES(?,?,?,?,?,?,?) ",val["makerId"],val["takerId"],val["takerSide"],val["amount"],val["price"],dTime,symbol)
									log.Printf("存成交日志%v\n", err)
									log.Printf("trades :%+v %+v %T\n", key[0],val,val)
								}
							}
						}
					}
				}
			}
		}

		//消费撤单结果
		for _,symbol := range cancelresults{
			groups_name,groups_user,nil := util.GetSymbolConsumeData(conn,symbol,i)
			//查看消费者各自接收到的未ack的数据
			r,err :=conn.Do("xreadgroup","group",groups_name, groups_user,"count","20", "streams",symbol, "0")
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
									if status,err :=val["ok"];err{
										sqlStr := "select orderid,userid,amount,price,side,deal_amount,symbol from `order` where orderid=? and status <>1"
										var User struct{
											Userid int64
											Amount float64
											Price float64
											Side int
											Deal_amount float64
											Symbol string
											Orderid string
										}
										err := util.DB.Get(&User, sqlStr,val["orderId"] )
										if err!=nil {
											log.Printf("获取原订单amount 失败 %+v info :%+v\n", err,val["orderId"])
											continue
										}
										//开始一个事务，返回一个事务对象tx
										tx, err := util.DB.Beginx()
										//修改订单状态及成交数量，成交时间
										var err1,err2 error
										consume_flag := false
										switch status {
											case "1"://撤单成功，返还买单锁定的金钱，返还卖单锁定的库存，修改订单状态为1成功
												switch User.Side {
													case 0://买单
														m :=decimal.NewFromFloat(User.Price).Mul(decimal.NewFromFloat(User.Amount).Sub(decimal.NewFromFloat(User.Deal_amount)))
														_,err1 = tx.Exec("update `user` set band_money=band_money-?,money=money+? where id=?",
															m,
															m,
															User.Userid,
														)
													case 1://卖单
														m := decimal.NewFromFloat(User.Amount).Sub(decimal.NewFromFloat(User.Deal_amount))
														_,err1 = tx.Exec("update `stock` set stock_num=stock_num+?,band_stock_num=band_stock_num-? where userid=? and symbol=?",
															m,
															m,
															User.Userid,
															User.Symbol,
														)
												}
												_,err2 = tx.Exec("update `order` set status=? where orderid=?",
													4,
													val["orderId"],
												)
												if err1 != nil || err2 != nil {
													log.Printf("获取user 失败 1%+v,2%+v\n", err1,err2)
													tx.Rollback()
													consume_flag = false
												}else{
													consume_flag = true
													tx.Commit()
												}
											case "0"://撤单失败，还原订单的状态
												var st int
												if User.Deal_amount>0 && User.Deal_amount!=User.Amount  {
													st =2
												}
												if User.Deal_amount==0  {
													st =0
												}
												if User.Deal_amount==User.Amount  {
													st =1
												}
												_,err1 = util.DB.Exec("update `user` set status=? where orderid=?",
													st,
													val["orderId"],
												)
												if err1!=nil{
													consume_flag = false
												}else{
													consume_flag = true
												}
										}
										if consume_flag {
											conn.Do("xack",symbol,groups_name,key[0])
											if wid, ok := wsUidToWsid[User.Userid]; ok {
												//更新用户的货币存量
												sqlStr := "select money,band_money from `user` where id=?"
												var User_money struct{
													Money float64
													Band_money float64
												}
												err := util.DB.Get(&User_money, sqlStr,User.Userid)
												if err!=nil {
													log.Printf("获取user 失败 %+v\n", err)
												}else{
													if wca,o:=wsConnAll[wid];o {
														wca.user.Money = User_money.Money
														wca.user.Band_money = User_money.Band_money
													}
												}
												// 该用户存在，发送订单成功的消息
												//mm,_ :=json.Marshal(val)
												if status=="1" {
													if wca,o:=wsConnAll[wid];o {
														log.Printf("debug:%+v,%+v\n",wsConnAll,wid);
														//wca.Mux.Lock()
														//wca.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"撤单成功！", val))
														//wca.Mux.Unlock()
														wca.sendWsMsg(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"撤单成功！", val))
													}
												}else{
													if wca,o:=wsConnAll[wid];o {
														log.Printf("debug:%+v,%+v\n",wsConnAll,wid);
														//wca.Mux.Lock()
														//wca.wsSocket.WriteMessage(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"撤单失败！", val))
														//wca.Mux.Unlock()
														wca.sendWsMsg(websocket.TextMessage,util.FormatReturn(util.HTTP_OK,"撤单失败！", val))
													}
												}

											}
											log.Printf("消费成功cancelresults :%+v %+v %T\n", key[0],val,val)
										}else{
											log.Printf("消费失败cancelresults :%+v %+v %T\n", key[0],val,val)
										}
									}
								}
							}
						}
					}
				}
			}
		}

		//log.Printf("wsConnAll:%+v\n",wsConnAll)
		//log.Printf("wsUidToWsid:%+v\n \n",wsUidToWsid)
		//log.Printf("goroutinenum:%+v,||%+v,%+v,%+v,%+v,%+v \n",runtime.NumGoroutine(),read_num,read_chan_num,read_chanxd_num,read_chanxdd_num,read_chanxdds_num)
	}
}
func (wsConn *wsConnection) sendWsMsg(msgtype int,data []byte){
	defer func() {//避免并发时直接painc
		if err:=recover();err!=nil{
			log.Printf("recover %+v\n",err)
		}
	}()
	wsConn.Mux.Lock()
	wsConn.wsSocket.WriteMessage(msgtype,data)
	defer wsConn.Mux.Unlock()
}
func init() {
	//c, err := redis.Dial("tcp", "10.0.111.154:52311",redis.DialPassword("crimoon2015"))
	//redis_conn = c
	//if err != nil {
	//	fmt.Println("sorry,redis_conn error:",err)
	//}

}

