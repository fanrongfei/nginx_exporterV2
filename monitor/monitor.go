package monitor

import (
	"fmt"
	log "github.com/cihub/seelog"
	cron "github.com/robfig"
	"syscall"

	"nginx_exporterV2/config"
	"nginx_exporterV2/exporter"
	"sync/atomic"
	"time"
)

var (
	accessInode uint64
)

/*检测access.log 是否发生变化
23点59分开始中断执行
并检测inode值变化，若变化则重置句柄，开始新的循环，否则在5分钟内无法检测到inode 发生变化，则强制从头循环
todo 改进，超时不应该重新读，则会导致第一个统计结果不准，再增加一个信号量进行控制。
*/
func init() {
	accessInode = getInode()
}
func Monitor(cronString string) {
	defer func() {
		if e := recover(); e != nil {
			fmt.Printf("monitor cron 任务错误 error=%v \n", e)
			log.Errorf("monitor cron 任务错误 error=%v \n", e)
		}
	}()
	cr := cron.New()
	cr.AddFunc(cronString, func() {
		fmt.Println("句柄重置任务开始执行")
		atomic.StoreInt32(&exporter.SignalStop, 1)
		//access.log inode 变更窗口时间为5分钟,超时则放弃重置句柄
		tT := time.NewTimer(time.Minute * 5)
		for {
			select {
			case <-tT.C:
				log.Infof("accessLog 5分钟内inode未发送变动信息，放弃重置accessLog句柄！！！")
				fmt.Println("accessLog 5分钟inode内未发送变动信息，放弃重置accessLog句柄！！！")
				goto ForEnd
			default:
				if accessInode != getInode() {
					log.Infof("句柄变更")
					accessInode = getInode()
					goto ForEnd
				} else {
					//防止cpu空转
					time.Sleep(time.Second)
				}
			}
		}
	ForEnd:
		//释放中断标识
		atomic.StoreInt32(&exporter.SignalStop, 0)
		exporter.SyncCond.Signal()
		return
	})
	cr.Start()
	//阻塞
	for {
		time.Sleep(time.Second)
	}
}

//获得access.log  inode值
func getInode() uint64 {
	var statFile syscall.Stat_t
	if err := syscall.Stat(config.Instance.Common.AccessPath, &statFile); err != nil {
		log.Errorf("getInode error=%v \n", err)
		fmt.Println("getInode error" + err.Error())
		return 0
	}
	return statFile.Ino
}
