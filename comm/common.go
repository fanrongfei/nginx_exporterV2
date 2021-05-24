package comm

import (
	log "github.com/cihub/seelog"
	"math"
	"nginx_exporterV2/lib"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	status1 = regexp.MustCompile(`^1.*`)
	status2 = regexp.MustCompile(`^2.*`)
	status3 = regexp.MustCompile(`^3.*`)
	status4 = regexp.MustCompile(`^4.*`)
	status5 = regexp.MustCompile(`^5.*`)
	status6 = regexp.MustCompile(`^6.*`) //6开头的http响应码只有600
)

//根据响应码返回1xx/2xx/3xx/4xx/5xx
func StatusFormat(status string) string {
	switch true {
	case status1.MatchString(status):
		return "1xx"
	case status2.MatchString(status):
		return "2xx"
	case status3.MatchString(status):
		return "3xx"
	case status4.MatchString(status):
		return "4xx"
	case status5.MatchString(status):
		return "5xx"
	case status6.MatchString(status):
		return "600"
	default:
		log.Errorf("exporter_statusFormat|unknown status=%s", status)
		return "xxx"
	}
}
func statusFormatV2(status string) string {
	switch true {
	case strings.HasPrefix(status, "1"):
		return "1xx"
	case strings.HasPrefix(status, "2"):
		return "2xx"
	case strings.HasPrefix(status, "3"):
		return "3xx"
	case strings.HasPrefix(status, "4"):
		return "4xx"
	case strings.HasPrefix(status, "5"):
		return "5xx"
	case strings.HasPrefix(status, "6"):
		return "600"
	default:
		log.Errorf("exporter_statusFormat|unknown coder=%s", status)
		return "xxx"
	}
}

//59.203.160.13 - - [20/Jul/2020:03:31:12 +0800] "GET /common-api/v2/pb/getTime HTTP/1.0" 200 13 "-" "-"
func AnalysisWork(sign *lib.TaskSign, reqLine string) (Entiy lib.EntityNginx) {
	//排除第一次，启动导致的问题内存暴涨问题
	if sign.Eable {
		if !timeInRange(reqLine, sign.StartTime) {
			Entiy.Eable = false
			return
		}
	}
	Entiy = GetEntityNginx(reqLine)
	return
}
func GetEntityNginx(reqLine string) (Entiy lib.EntityNginx) {
	defer func() {
		if e:=recover();e!=nil{
			log.Errorf("GetEntityNginx error=%v",e)
			log.Errorf("问题行=%s",reqLine)
		}
	}()
	brlArr := strings.Split(string(reqLine), "\"")
	reqLineArr := brlArr[1]
	//状态码和响应时间
	statusArr := strings.Split(brlArr[2], " ")
	status := statusArr[1]
	//reqTime, _ := strconv.Atoi(statusArr[2])
	reqTime:=floatStrToInt(statusArr[2])
	reqLineList := strings.Split(reqLineArr, " ")
	if len(reqLineList) < 3 {
		Entiy = lib.EntityNginx{true, reqLineArr, reqTime, status, statusFormatV2(status)}
	} else {
		//"GET GET /cgi-bin/loadpage.cgi?user_id=1&file=../../../../../../etc/passwd HTTP/1.1" 400 166 "-" "-"
		if reqLineList[1] != "GET" && reqLineList[1] != "POST" {
			reqLineLast := strings.Split(reqLineList[1], "?")
			Entiy = lib.EntityNginx{true, reqLineLast[0], reqTime, status, statusFormatV2(status)}
		} else {
			reqLineLast := strings.Split(reqLineList[2], "?")
			Entiy = lib.EntityNginx{true, reqLineLast[0], reqTime, status, statusFormatV2(status)}
		}
	}
	return
}

//时间是否满足
func timeInRange(reqLine string, pointTime time.Time) bool {
	defer func() {
		if e:=recover();e!=nil{
			log.Errorf("timeInrange error=%v",e)
			log.Errorf("问题行:%s",reqLine)
		}
	}()
	reqLineArr := strings.Split(reqLine, " ")
	reqTime := reqLineArr[3]
	timeStr := reqTime[1:len(reqTime)]
	ParseStr := "02/Jan/2006:15:04:05"
	local, _ := time.LoadLocation("Local")
	targetTime, _ := time.ParseInLocation(ParseStr, timeStr, local)
	//端点放行
	if targetTime.Equal(pointTime) || targetTime.After(pointTime) {
		return true
	}
	return false
}
//响应时间转换（string->int）
func floatStrToInt(strFloat string)int{
	if strFloat=="-"{
		return 0
	}
	floats,err:=strconv.ParseFloat(strFloat,64)
	if err!=nil{
		log.Errorf("strFloat to int error=%v",err)
		return 0
	}else{
		return  int(math.Ceil(floats*1000))
	}
}
