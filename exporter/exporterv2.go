package exporter

import (
	"bufio"
	"bytes"
	"fmt"
	log "github.com/cihub/seelog"
	"io"
	"math"
	"nginx_exporterV2/comm"
	"nginx_exporterV2/config"
	"nginx_exporterV2/lib"
	"nginx_exporterV2/websend"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	/*
		存上一次报警信息 结构如下
		key:coder-->1xx/2xx/3xx/4xx/5xx
		value:map   url：错误数
		            total: 一共多少次
		            sendN：第几次发送
	*/
	HistorySendMap map[string]map[string]int
	//access.log inode 信号变更信息
	IndoeStart = make(chan struct{}, 1)
	//存今日已经告警过的的接口 2020/12/2 15点35分添加
	//Today

	TodayTime  lib.TimeSign
	WorkList   = new(lib.EntityNginxList)
	workChan   = make(chan string)
	//存档周期内，响应码出现次数
	historyMap sync.Map
	//周期响应接口响应时间统计
	responseSendTimes = 0
	//每日汇总统计
	HistoryChan = make(chan string)
	/*
	  当天接口请求统计
	   key接口
	   value：map：【key状态码；value 发生次数】
	*/
	HistoryMap map[string]map[string]int
	//记录触发 发生变更
	SignalStop int32 = 0
	//0点变更信号
	Lc       = new(sync.Mutex)
	SyncCond = sync.NewCond(Lc)
	//记录今日累计报警的path及响应码
	TodayCumulativePath chan lib.RecordPath
	//周期内统计告警信息
	CyclealarmPath chan lib.RecordPath
)

func init() {
	HistorySendMap = make(map[string]map[string]int)
	HistoryMap = make(map[string]map[string]int)
	TodayCumulativePath =make(chan lib.RecordPath,10)
	CyclealarmPath=make(chan lib.RecordPath,10)

	TodayTime.Reset()
}

//解析access.log
func TraverseAccess() {
	var errorDelay =1
Begin:
	accessFile, err := os.Open(config.Instance.Common.AccessPath)
	if err != nil {
		log.Errorf("open access.log  error=%v(please check the conf.toml)", err)
		time.Sleep(time.Second*time.Duration(errorDelay))
		errorDelay++
		if errorDelay==11{
			errorDelay=1
		}
		goto Begin
	}
	br := bufio.NewReader(accessFile)
	for {
		if atomic.LoadInt32(&SignalStop) != 0 {
			log.Infof("%s收到变更信息，执行中断", time.Now().Format("2006-01-02 15:04:05"))
			break
		}
		brl, err := br.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				//读到结束行
				time.Sleep(time.Second)
			} else {
				accessFile.Close()
				goto Begin
			}
		}
		if len(brl) != 0 && len(brl) != 1 && TodayTime.Compare(string(brl)) {
			workChan <- string(brl)
			HistoryChan <- string(brl)
		}
	}
	//释放句柄
	accessFile.Close()
	//监听inode 变更信息
	SyncCond.L.Lock()
	SyncCond.Wait()
	SyncCond.L.Unlock()
	//收到inode变更信息，重置当天日期，排除由于日志切割不准问题
	TodayTime.Reset()
	log.Infof("%s收到access.log变更信息,重新获取日志句柄，进行下轮执行……", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("%s收到access.log变更信息,重新获取日志句柄，进行下轮执行……\n", time.Now().Format("2006-01-02 15:04:05"))
	goto Begin
}

//暂时开8个协程 进行处理
func Analysis(startSign *lib.TaskSign) {
	for i := 0; i < 8; i++ {
		go func() {
			for reqLine := range workChan {
				//发送报警序列
				WorkList.Insert(comm.AnalysisWork(startSign, reqLine))
			}
		}()
	}
}
//暂时开8个协程 进行处理处理历史告警
func AnalysisTodayCumulativePath() {
	for i := 0; i < 8; i++ {
		go func() {
			for recordPath := range TodayCumulativePath {
				lib.AHPath.Add(recordPath.Path,recordPath.Code)
				//将今日告警信息纳入set性质数据结构里进行记录
			}
		}()
	}
}
//暂时开8个协程 周期内告警信息
func AnalysisCyclealarmPath() {
	for i := 0; i < 8; i++ {
		go func() {
			for recordPath := range CyclealarmPath {
				lib.AcclaPath.Add(recordPath.Path,recordPath.Code)
			}
		}()
	}
}

//周期任务
func Timetask() {
	//获取本周期内数据量
	entityArr := WorkList.GetEntityArr()
	//获取过滤正则
	statusBlacArrReg, statusWhiteArrReg, respBlackArrReg, respWhiteArrReg := getReqex()
	var wg sync.WaitGroup
	wg.Add(2)
	//接口状态码逻辑
	go interfaceAlarm(statusBlacArrReg, statusWhiteArrReg, entityArr, &wg)
	//接口响应时间逻辑
	go respTimeAlarm(respBlackArrReg, respWhiteArrReg, entityArr, &wg)
	wg.Wait()
	return
}

//接口状态码报警
func interfaceAlarm(blackArrReg, whiteArrReg []*regexp.Regexp, entityArr []lib.EntityNginx, wg *sync.WaitGroup) (tempMap map[string]map[string]int, total int) {
	defer wg.Done()
	total = len(entityArr)
	//过滤接口，存入map 4xx:number
	var temp = make(map[string]int)
	//报警序列
	var alarmMap = make(map[string]struct{})
	//tempMap[2xx]map[接口]时间
	tempMap = make(map[string]map[string]int)
	for i := 0; i < len(entityArr); i++ {
		if !ruleIn(blackArrReg, whiteArrReg, entityArr[i]) {
			//新增2020/12/4 15点47分 新增将告警path 纳入统计
			CyclealarmPath<-lib.RecordPath{entityArr[i].NginxPath,entityArr[i].NginxCode}

			temp[entityArr[i].NginxCode] = temp[entityArr[i].NginxCode] + 1
			if _, YES := tempMap[entityArr[i].NginxCode]; !YES {
				tempmap := make(map[string]int)
				tempmap[entityArr[i].NginxPath] = 1
				tempMap[entityArr[i].NginxCode] = tempmap
			} else {
				tempMap[entityArr[i].NginxCode][entityArr[i].NginxPath] = tempMap[entityArr[i].NginxCode][entityArr[i].NginxPath] + 1
			}
			//如果此时状态码已经超过阈值纳入报警序列
			if temp[entityArr[i].NginxCode] > config.Instance.RuleStatus.Threshold {
				alarmMap[entityArr[i].NginxCode] = struct{}{}
			}
		}
	}
	//去掉无报警项
	for k0, _ := range tempMap {
		sign := false
		for k1, _ := range alarmMap {
			if k0 == k1 {
				sign = true
				break
			}
		}
		if !sign {
			delete(tempMap, k0)
		}
	}
	//计算每一个接口total值
	for k, v := range tempMap {
		tempTotal := 0
		for _, tempV := range v {
			tempTotal = tempTotal + tempV
		}
		tempMap[k]["total"] = tempTotal
	}
	//获得需要报警列表
	sendAlarmArr := StatisticalThreshold(tempMap)
	//获得恢复列表
	sendRecoverArr := getRecoverArr(alarmMap)
	//发送微信告警/恢复信息
	thresholdSend(total, sendAlarmArr, sendRecoverArr, tempMap)
	return
}
func thresholdSend(total int, sendAlarmArr, sendRecoverArr []string, tempMap map[string]map[string]int) {
	//告警发送
	var wg sync.WaitGroup
	fragmentSize := config.Instance.Alarm.FragmentSize
	Threshold := config.Instance.RuleStatus.Threshold
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < len(sendAlarmArr); i++ {
			for k, v := range tempMap {
				if sendAlarmArr[i] == k {
					var rb bytes.Buffer
					rb.WriteString(fmt.Sprintf("nginx %s %d分钟内请数为%d ,超过设定预警阈值%d,周期内总请求量%d。\n", k, fragmentSize, v["total"], Threshold, total))
					rb.WriteString("[接口详情]:\n")
					var sortUr lib.PathStatusArr
					for k1, v1 := range v {
						if k1 != "total" && k1 != "sendNumber" {
							//记录当日path信息（2020/12/4 14点14分新增）
							TodayCumulativePath<-lib.RecordPath{Path:k1,Code:k}
							sortUr = append(sortUr, lib.PathStatus{k1, v1})
						}
					}
					sort.Sort(sortUr)
					var onceSend sync.Once
					SEND:=false
					for i := 0; i < len(sortUr); i++ {
						if rb.Len()>=1900{
							onceSend.Do(func() {
								websend.SendWechat(rb, k, 1,SEND)
								SEND=true
								rb.Reset()
							})
							websend.SendWechat(rb, k, 1,SEND)
							rb.Reset()
						}
						rb.WriteString(fmt.Sprintf("URL=%s, SUM=%d \n", sortUr[i].Path, sortUr[i].Num))
					}
					rb.WriteString("-----------------------------------------------")
					websend.SendWechat(rb, k, 1,SEND)
					log.Infof("Time=%s,报警信息:%s",time.Now().Format("2006-01-02 15:04:05"),rb.String())
				}
			}
		}
		return
	}()
	//报警恢复发送
	go func() {
		defer wg.Done()
		if len(sendRecoverArr)==0{
			return
		}
		for k, v := range HistorySendMap {
			sginB := false
			for i := 0; i < len(sendRecoverArr); i++ {
				if k == sendRecoverArr[i] {
					sginB = true
				}
			}
			if sginB {
				var rb bytes.Buffer
				rb.WriteString(fmt.Sprintf("nginx %s 得到恢复;error总量%d,小于预警阈值%d。\n", k, tempMap[k]["total"], Threshold))
				rb.WriteString("[接口详情]:之前告警信息如下。\n")
				var sortUr lib.PathStatusArr
				for k1, v1 := range v {
					if k1 != "total" && k1 != "sendNumber" {
						sortUr = append(sortUr, lib.PathStatus{k1, v1})
					}
				}
				sort.Sort(sortUr)
				var onceSend sync.Once
				SEND:=false
				for i := 0; i < len(sortUr); i++ {
					if rb.Len()>=1900{
						onceSend.Do(func() {
							websend.SendWechat(rb, k, 2,SEND)
							SEND=true
							rb.Reset()
						})
						websend.SendWechat(rb, k, 2,SEND)
						rb.Reset()
					}
					rb.WriteString(fmt.Sprintf("URL=%s, SUM=%d \n", sortUr[i].Path, sortUr[i].Num))
				}
				rb.WriteString("-----------------------------------------------")
				websend.SendWechat(rb, k, 2,SEND)
				delete(HistorySendMap, k)
				log.Infof("Time=%s,恢复信息:%s",time.Now().Format("2006-01-02 15:04:05"),rb.String())
			}
		}
		return
	}()
	wg.Wait()
}
//获得恢复信息
func getRecoverArr(alarmMap map[string]struct{}) (sendRecoverArr []string) {
	if len(HistorySendMap) == 0 {
		return
	}
	for k0, _ := range HistorySendMap {
		sign := false
		for k1, _ := range alarmMap {
			if k0 == k1 {
				sign = true
				break
			}
		}
		if !sign {
			sendRecoverArr = append(sendRecoverArr, k0)
		}
	}
	return
}

/*
触发报警相关规则
以状态码为统计依据
1、当相对于上次有新的错误出现-->报警
2、当该段时间内的报警数据内不包含上一次的历史数据--->恢复
3、当新的报警信息相对于上一次，无指数增加趋势,在未超过累计发送阈值之前，不报警（一旦超过累计阈值，则发送报警）
    todo  有新增url 报警
*/

func StatisticalThreshold(tempMap map[string]map[string]int) (SendAlarmArr []string) {
	//阈值
	thredMAX := config.Instance.RuleStatus.Threshold
	//发送延迟阈值
	delayTimes := config.Instance.Alarm.DelayTimes
	for k, v := range tempMap {
		//是否超过所设置的 警报阈值
		if v["total"] >= thredMAX {
			//首次发送
			if HistorySendMap[k] == nil {
				SendAlarmArr = append(SendAlarmArr, k)
				cm := make(map[string]int)
				cm = v
				cm["sendNumber"] = 1
				HistorySendMap[k] = cm
			} else {
				//达到最大“延迟等待阈值”
				if HistorySendMap[k]["sendNumber"] > delayTimes {
					fmt.Println("sendNumber>delayTimes")
					SendAlarmArr = append(SendAlarmArr, k)
					cm := make(map[string]int)
					cm = v
					cm["sendNumber"] = 1
					HistorySendMap[k] = cm
				} else {
					//小于发送阈值
					//未超发送窗口,但是错误增长过快
					if math.Round(float64(v["total"])/float64(HistorySendMap[k]["total"])) >= 2.0 {
						fmt.Printf(">=2.0[pre=%d;next=%d]\n", HistorySendMap[k]["total"], v["total"])
						SendAlarmArr = append(SendAlarmArr, k)
						cm := HistorySendMap[k]
						cm = v
						cm["sendNumber"] = 1
						HistorySendMap[k] = cm
					} else {
						//todo 暂时保留原来的历史数据
						//HistoryMap := HistorySendMap[k]
						//cm := v
						//cm["sendNumber"] = HistoryMap["sendNumber"] + 1
						//HistorySendMap[k] = cm
						HistorySendMap[k]["sendNumber"] = HistorySendMap[k]["sendNumber"] + 1
					}
				}
			}
		}
	}
	return
}

//接口响应码报警逻辑
func respTimeAlarm(blackArrReg, whiteArrReg []*regexp.Regexp, entityArr []lib.EntityNginx, wg *sync.WaitGroup) {
	defer wg.Done()
	limitRespTime := config.Instance.RuleResponse.ResponseTime
	//
	tempMap := make(map[string]int)
	//过滤接口
	for i := 0; i < len(entityArr); i++ {
		if !ruleIn(blackArrReg, whiteArrReg, entityArr[i]) {
			//查看响应值
			if entityArr[i].RespTime > limitRespTime {
				tempMap[entityArr[i].NginxPath] = tempMap[entityArr[i].NginxPath] + 1
			}
		}
	}
	responseThresholdSend(len(entityArr), tempMap)
	return
}

//提前获取黑/白正则表达式---加速计算
func getReqex() (staBlackArrReg, staWhiteArrReq, interBlackArrReg, interWhiteArrReq []*regexp.Regexp) {
	//状态码
	statusBlackArr := config.Instance.RuleStatus.BlacklistStatus
	statusWhiteArr := config.Instance.RuleStatus.WhiteListStatus
	//响应时间
	respBlackArr := config.Instance.RuleResponse.BlacklistResponse
	respWhiteArr := config.Instance.RuleResponse.WhiteListResponse
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < len(statusBlackArr); i++ {
			reC, _ := regexp.Compile(statusBlackArr[i])
			staBlackArrReg = append(staBlackArrReg, reC)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < len(statusWhiteArr); i++ {
			reC, _ := regexp.Compile(statusWhiteArr[i])
			staWhiteArrReq = append(staWhiteArrReq, reC)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < len(respBlackArr); i++ {
			reC, _ := regexp.Compile(respBlackArr[i])
			interBlackArrReg = append(interBlackArrReg, reC)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < len(respWhiteArr); i++ {
			reC, _ := regexp.Compile(respWhiteArr[i])
			interWhiteArrReq = append(interWhiteArrReq, reC)
		}
	}()
	wg.Wait()
	return
}

//白黑过滤
func ruleIn(blackArrReg, whiteArrReg []*regexp.Regexp, entityNginx lib.EntityNginx) bool {
	//先看黑名单
	for i := 0; i < len(blackArrReg); i++ {
		if blackArrReg[i].MatchString(entityNginx.NginxPath + ":" + entityNginx.NginxStatus) {
			return false
		}
	}
	//白名单过滤
	for i := 0; i < len(whiteArrReg); i++ {
		if whiteArrReg[i].MatchString(entityNginx.NginxPath + ":" + entityNginx.NginxStatus) {
			return true
		} else {
			continue
		}
	}
	//默认 不放行
	return false
}
func responseThresholdSend(total int, tempMap map[string]int) {
	if len(tempMap) == 0 {
		fmt.Println("周期内 ‘响应时间’ 这项没有告警信息")
		return
	}
	limitResponseTime := config.Instance.Alarm.DelayTimes
	if responseSendTimes == 0 || responseSendTimes > limitResponseTime {
		responseSendTimes=1
		fmt.Printf("（周期内响应时间）响应触发 %d \n",len(tempMap))
		fragmentSize := config.Instance.Alarm.FragmentSize
		var sortUr lib.PathStatusArr
		errorTotal := 0
		for k, v := range tempMap {
			sortUr = append(sortUr, lib.PathStatus{k, v})
			errorTotal = errorTotal + v
		}
		var rb bytes.Buffer
		rb.WriteString(fmt.Sprintf("nginx %d分钟内,接口响应时间超过阈值%dms,接口信息如下(涉及接口总数%d,周期内总请求量%d)。\n", fragmentSize, config.Instance.RuleResponse.ResponseTime, errorTotal, total))
		rb.WriteString("[接口详情]:\n")
		sort.Sort(sortUr)
		var onceSend sync.Once
		SEND:=false
		for i := 0; i < len(sortUr); i++ {
			if rb.Len()>=1700{
				onceSend.Do(func() {
					websend.SendWechat(rb, strconv.Itoa(config.Instance.RuleResponse.ResponseTime), 3,SEND)
					SEND=true
					rb.Reset()
				})
				websend.SendWechat(rb, strconv.Itoa(config.Instance.RuleResponse.ResponseTime), 3,SEND)
				rb.Reset()
			}
			rb.WriteString(fmt.Sprintf("URL=%s, SUM=%d \n", sortUr[i].Path, sortUr[i].Num))
		}
		rb.WriteString("-----------------------------------------------")
		websend.SendWechat(rb, strconv.Itoa(config.Instance.RuleResponse.ResponseTime), 3,SEND)
		log.Infof("Time=%s,接口响应时间超阈值%的详细接口信息:%s",time.Now().Format("2006-01-02 15:04:05"),config.Instance.RuleResponse.ResponseTime,rb.String())
	} else {
		responseSendTimes++
	}
	return
}
func AlarmPath(){
	var content bytes.Buffer
	for{
		time.Sleep(time.Minute*time.Duration(config.Instance.Alarm.FragmentSize+2))//比告警周期延迟2分钟
		result:=lib.AcclaPath.GetalarmPathMess()
		if len(result)==0{
			return
		}
		content.WriteString("接口累计告警")
		content.WriteString("\n")
		content.WriteString("-----------------------------------------------")
		content.WriteString("\n")
		content.WriteString(fmt.Sprintf("[集群]:%s", config.Instance.Wechat1.Name))
		content.WriteString("\n")
		content.WriteString(fmt.Sprintf("[IP]:%s", config.Instance.Wechat1.NginxIp))
		content.WriteString("\n")
		content.WriteString("[类型]:nginx_api_error")
		content.WriteString(fmt.Sprintf("[时间]:%s \n", time.Now().Format("2006-01-02 15:04:05")))
		content.WriteString("\n")
		content.WriteString(fmt.Sprintf("接口累积告警>=阈值%d,接口详情如下:\n",config.Instance.Wechat3.Threshold3))
		for _,v:=range result{
			content.WriteString(v)
			if content.Len()>1700{
				websend.SendAccumulateWechat(content)
				content.Reset()
			}
		}
		content.WriteString("-----------------------------------------------")
		websend.SendAccumulateWechat(content)
		content.Reset()
	}
}
