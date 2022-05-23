package auth

import (
	jwt "github.com/dgrijalva/jwt-go"
	"time"
)
// 设置jwt密钥secret
var jwtSecret=[]byte("matching_websocket")

type  Claims struct {
	UserID 	 string `json:"userid"`
	jwt.StandardClaims
}

// GenerateToken 生成token的函数
func GenerateToken(userid string)(string,error){
	nowTime :=time.Now()
	expireTime:=nowTime.Add(24*time.Hour)

	claims:=Claims{
		userid,  // 自行添加的信息
		jwt.StandardClaims{
			ExpiresAt: expireTime.Unix(), // 设置token过期时间
			Issuer: "crimoon",  // 设置jwt签发者
		},
	}
	// 生成token
	tokenClaims:=jwt.NewWithClaims(jwt.SigningMethodHS256,claims)
	token,err:=tokenClaims.SignedString(jwtSecret)
	return token,err
}

// ParseToken 验证token的函数
func ParseToken(token string)(*Claims,error){
	// 对token的密钥进行验证
	tokenClaims,err:=jwt.ParseWithClaims(token,&Claims{},func(token *jwt.Token)(interface{},error){
		return jwtSecret,nil
	})

	// 判断token是否过期
	if tokenClaims!=nil{
		if claims,ok:=tokenClaims.Claims.(*Claims);ok && tokenClaims.Valid{
			return claims,nil
		}
	}
	return nil,err
}

//func main() {
//	// 生成一个token
//	token,_:=GenerateToken("12")
//	fmt.Println("生成的token:",token)
//	// 验证并解密token
//	claim,err:=ParseToken(token)
//	if err!=nil {
//		fmt.Println("解析token出现错误：",err)
//	}else if time.Now().Unix() > claim.ExpiresAt {
//		fmt.Println("时间超时")
//	}else {
//		fmt.Println("userid:",claim.UserID)
//	}
//}
