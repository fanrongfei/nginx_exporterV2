package websend

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/cihub/seelog"
	"io/ioutil"
	"net/http"
	"net/url"
	"nginx_exporterV2/config"
	"nginx_exporterV2/lib"
	"time"
)

var (
	//暂时缓存一下企业微信token
	tokenWechat = ""
	//性能方面
	tokenWechat1=""
	//
	tokenWechat2=""
)

/*
发送企业微信告警
msg:    告警/恢复 信息
symbol: nginx错误码，例如1xx,2xx,3xx,4xx,5xx
sign:   true:告警，false:恢复
*/
func SendWechat(msg bytes.Buffer, symbol string, sign int,flag bool) {
	var content bytes.Buffer
	if flag{
		goto SEND
	}
	switch sign {
	case 1:
		content.WriteString("告警")
	case 2:
		content.WriteString("恢复")
	case 3:
		content.WriteString("warning")
	}
	content.WriteString("\n")
	content.WriteString("-----------------------------------------------")
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("[集群]:%s", config.Instance.Wechat1.Name))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("[IP]:%s", config.Instance.Wechat1.NginxIp))
	content.WriteString("\n")
	content.WriteString("[类型]:nginx_api_error")
	content.WriteString("\n")
	if sign==3{
		content.WriteString(fmt.Sprintf("[级别]:nginx_接口响应超过阈值%sms", symbol))
	}else{
		content.WriteString(fmt.Sprintf("[级别]:nginx_%s_count", symbol))
	}
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("[时间]:%s", time.Now().Format("2006-01-02 15:04:05")))
	content.WriteString("\n")
	content.WriteString("[问题]:")
SEND:
	content.WriteString(msg.String())
	if sign==3{
		sendMsg1(creatMessages1(content.String()))
		return
	}
	sendMsg(creatMessages(content.String()))
}


// 发送企业微信信息
func sendMsg(msg []byte) {
	send_url:=fmt.Sprintf("%s/cgi-bin/message/send?access_token=%s",config.Instance.Wechat1.Wechatserver,getAccessToken())
	client := &http.Client{}
	req,err:= http.NewRequest("POST", send_url, bytes.NewReader(msg))
	if err!=nil{
		log.Errorf("newRequest error=%v",err)
		return
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("sendMsg(%s) error=%v",send_url,err.Error())
		return
	}
	resp.Body.Close()
}

// 获取企业微信token
func getAccessToken() string {
	var v =url.Values{}
	v.Add("corpid",config.Instance.Wechat1.Corpid1)
	v.Add("corpsecret",config.Instance.Wechat1.Secret1)
	token_url:=fmt.Sprintf("%s/cgi-bin/gettoken?%s",config.Instance.Wechat1.Wechatserver,v.Encode())
	req, err := http.Get(token_url)
	if err != nil {
		log.Errorf("wechat_Get_AccessToken|error=", err)
		return tokenWechat
	}
	defer req.Body.Close()
	reqD,err:= ioutil.ReadAll(req.Body)
	if err!=nil{
		log.Errorf("getAccessToken|ioutil.ReadAll error=%v",err)
		return tokenWechat
	}
	var resultTo lib.TokenC
	json.Unmarshal(reqD, &resultTo)
	tokenWechat = resultTo.Access_token
	return tokenWechat
}

// 定义信息格式
func creatMessages(content string) []byte {
	msg := lib.MESSAGES{
		Touser:  "@all",
		Toparty: "2", //部门id
		Msgtype: "text",
		Agentid: config.Instance.Wechat1.Agentid1,
		Safe:    0,
		Text: struct {
			Content string `json:"content"`
		}{Content: content},
	}
	msgB, _ := json.Marshal(msg)
	return msgB
}

//gateway 发送
func SendDataToGateWay(msg []byte){
	client := &http.Client{}
	req, _ := http.NewRequest("POST", config.Instance.Gateway.ServerURL, bytes.NewReader(msg))
	if resp,err:=client.Do(req);err!=nil{
		log.Errorf("wechat_SendDataToGateWay|error=%v",err)
	}else {
		defer resp.Body.Close()
		respD,err:=ioutil.ReadAll(resp.Body)
		if err!=nil{
			log.Errorf("SendDataToGateWay|ioutil.ReadAll error=%v",err)
		}else{
			log.Infof("wechat_SendDataToGateWay|succes infor=%v",string(respD))
		}
	}
	return
}
/*-------------------------------------------------性能警告------------------------------------------------------------*/
//专门发送0点发送统计信息及发送
func SendSummaryWechat(msg bytes.Buffer ,sign int,extra interface{},flag bool) {
	var content bytes.Buffer
	if flag{
		goto SEND
	}
	switch sign {
	case 1:
		content.WriteString("今日接口5xx >=200 接口统计统计")
	case 2:
		content.WriteString(fmt.Sprintf("今日接口响应时间>阈值%v ms统计",extra))
	}
	content.WriteString("\n")
	content.WriteString("-----------------------------------------------")
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("[集群]:%s", config.Instance.Wechat1.Name))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("[IP]:%s", config.Instance.Wechat1.NginxIp))
	content.WriteString("\n")
	content.WriteString("[类型]:nginx_api_error")
	content.WriteString("\n")
	switch sign {
	case 1:
		content.WriteString("今日接口报5xx错误码接口总数>=200个统计")
	case 2:
		content.WriteString(fmt.Sprintf("[级别]:nginx接口响应时间>=阈值%v ms统计",extra))
	}
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("[时间]:%s", time.Now().Format("2006-01-02 15:04:05")))
	content.WriteString("\n")
	content.WriteString("[接口列表]:\n")
SEND:	content.Write(msg.Bytes())
	sendMsg1(creatMessages1(content.String()))
	content.Reset()
}


// 发送企业微信信息
func sendMsg1(msg []byte) {
	send_url:=fmt.Sprintf("%s/cgi-bin/message/send?access_token=%s",config.Instance.Wechat1.Wechatserver,getAccessToken1())
	client := &http.Client{}
	req, err:= http.NewRequest("POST", send_url, bytes.NewReader(msg))
	if err!=nil{
		log.Errorf("newRequest error=%v",err)
		return
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("sendMsg(%s) error=%v",send_url,err.Error())
		return
	}
	resp.Body.Close()
}

// 获取企业微信token
func getAccessToken1() string {
	var v =url.Values{}
	v.Add("corpid",config.Instance.Wechat2.Corpid2)
	v.Add("corpsecret",config.Instance.Wechat2.Secret2)
	token_url:=fmt.Sprintf("%s/cgi-bin/gettoken?%s",config.Instance.Wechat1.Wechatserver,v.Encode())
	req, err := http.Get(token_url)
	if err != nil {
		log.Errorf("wechat_Get_AccessToken1|error=", err)
		return tokenWechat1
	}
	defer req.Body.Close()
	reqD,err:= ioutil.ReadAll(req.Body)
	if err!=nil{
		log.Errorf("getAccessToken|ioutil.ReadAll error=%v",err)
		return tokenWechat1
	}
	var resultTo lib.TokenC
	json.Unmarshal(reqD, &resultTo)
	tokenWechat1 = resultTo.Access_token
	return tokenWechat1
}

// 定义信息格式
func creatMessages1(content string) []byte {
	msg := lib.MESSAGES{
		Touser:  "@all",
		Toparty: "2", //部门id
		Msgtype: "text",
		Agentid: config.Instance.Wechat2.Agentid2,
		Safe:    0,
		Text: struct {
			Content string `json:"content"`
		}{Content: content},
	}
	msgB, _ := json.Marshal(msg)
	return msgB
}

// 获取企业微信token
func getAccessToken2() string {
	var v =url.Values{}
	v.Add("corpid",config.Instance.Wechat3.Corpid3)
	v.Add("corpsecret",config.Instance.Wechat3.Secret3)
	token_url:=fmt.Sprintf("%s/cgi-bin/gettoken?%s",config.Instance.Wechat1.Wechatserver,v.Encode())
	req, err := http.Get(token_url)
	if err != nil {
		log.Errorf("wechat_Get_AccessToken1|error=", err)
		return tokenWechat2
	}
	defer req.Body.Close()
	reqD,err:= ioutil.ReadAll(req.Body)
	if err!=nil{
		log.Errorf("getAccessToken|ioutil.ReadAll error=%v",err)
		return tokenWechat2
	}
	var resultTo lib.TokenC
	json.Unmarshal(reqD, &resultTo)
	tokenWechat2 = resultTo.Access_token
	return tokenWechat2
}

// 接口累计发送
func creatMessages2(content string) []byte {
	msg := lib.MESSAGES{
		Touser:  "@all",
		Toparty: "2", //部门id
		Msgtype: "text",
		Agentid: config.Instance.Wechat3.Agentid3,
		Safe:    0,
		Text: struct {
			Content string `json:"content"`
		}{Content: content},
	}
	msgB, _ := json.Marshal(msg)
	return msgB
}
// 发送企业微信信息
func sendMsg2(msg []byte) {
	send_url:=fmt.Sprintf("%s/cgi-bin/message/send?access_token=%s",config.Instance.Wechat1.Wechatserver,getAccessToken2())
	client := &http.Client{}
	req, err:= http.NewRequest("POST", send_url, bytes.NewReader(msg))
	if err!=nil{
		log.Errorf("newRequest error=%v",err)
		return
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("sendMsg(%s) error=%v",send_url,err.Error())
		return
	}
	resp.Body.Close()
}
/*-------------------------------------------------性能警告2------------------------------------------------------------*/
//发送 接口积累告警数据
func SendAccumulateWechat(content bytes.Buffer) {
	sendMsg2(creatMessages2(content.String()))
}
