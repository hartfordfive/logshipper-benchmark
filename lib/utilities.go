package lib

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/shirou/gopsutil/process"
)

/*
#include <unistd.h>
#include <sys/types.h>
#include <pwd.h>
#include <stdlib.h>
*/
import "C"

var Debug bool

func init() {
	Debug = false
}

func CheckErr(e error) {
	if e != nil {
		panic(e)
	}
}

func TimeTraceStart() time.Time {
	return time.Now()
}

func TimeTraceEnd(startTime time.Time) float64 {
	elapsed := time.Since(startTime)
	return elapsed.Seconds()
}

func CreateDir(dirName string) {
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		err = os.MkdirAll(dirName, 0755)
		if err != nil {
			fmt.Println("[ERROR] Could not create directory: ", err)
		}
	}
}

func GetClockTicksPerSecond() uint64 {
	var sc_clk_tck C.long
	sc_clk_tck = C.sysconf(C._SC_CLK_TCK)
	return uint64(sc_clk_tck)
}

func RoundToEven(x float64) int64 {
	t := math.Trunc(x)
	odd := math.Remainder(t, 2) != 0
	if d := math.Abs(x - t); d > 0.5 || (d == 0.5 && odd) {
		return int64(t + math.Copysign(1, x))
	}
	return int64(t)
}

func GetCwd() string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(ex)
}

func GetRandInt(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func CollectCpuStats(pid int, metricsFilePath string, shutdownChan chan bool) {

	psStats, _ := process.NewProcess(int32(pid))

	CreateDir(metricsFilePath)

	ticker := time.NewTicker(time.Millisecond * 2000)
	defer ticker.Stop()
	t := time.Now()

	filePath := fmt.Sprintf("%s/ps-%d-stats-%d%02d%02d%02d%02d%02d.csv", metricsFilePath, pid, t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0664)
	if err != nil {
		fmt.Printf("[ERROR] Could not open %s: %s\n", filePath, err)
		os.Exit(1)
	}
	defer file.Close()

	// see: https://stackoverflow.com/a/16736599
	/*
		var stat *linuxproc.ProcessStat
		var totalTime uint64
		var seconds float64
		var totalCpuUsage float64
		hertz := GetClockTicksPerSecond()

		if Debug {
			fmt.Println("[DEBUG] Current CPU clock ticks: ", hertz)
			fmt.Printf("[DEBUG] Fetching PID stats from /proc/%d/stat\n", pid)
		}

		uptime, err := linuxproc.ReadUptime("/proc/uptime")
		if err != nil {
			fmt.Println("[ERROR] Could not read system uptime")
		}
	*/

	_, err = file.Write([]byte("unix_timestamp_ms,total_cpu_pct,mem_bytes_rss,mem_bytes_vms,mem_bytes_swap\n"))
	if err != nil {
		fmt.Println(err)
	}

	var memInfoRSS, memInfoVMS, memInfoSwap uint64
	var psCPU float64

	for {
		select {

		case <-ticker.C:

			processCpuPct, err := psStats.CPUPercent()
			if err == nil {
				psCPU = processCpuPct
			}

			memInfo, err := psStats.MemoryInfo()
			if err == nil {
				memInfoRSS = memInfo.RSS
				memInfoVMS = memInfo.VMS
				memInfoSwap = memInfo.Swap
			}

			//fmt.Printf("CPU: %f, Mem Used: %d\n", psCPU, memInfoRSS)

			/*
				stat, err = linuxproc.ReadProcessStat(fmt.Sprintf("/proc/%d/stat", pid))
				if err != nil {
					fmt.Printf("[ERROR] Could not read filebeat CPU stats (PID %d)\n", pid)
					continue
				}

				totalTime = stat.Utime + stat.Stime + uint64(stat.Cutime) + uint64(stat.Cstime)
				seconds = uptime.Total - float64(stat.Starttime/hertz)
				totalCpuUsage = 100 * (float64(totalTime/hertz) / seconds)
			*/

			ts := time.Now().UnixNano() / int64(time.Millisecond)
			if Debug {
				//fmt.Printf("ts: %d, utime: %d, stime: %d, cutime: %d, cstime: %d, starttime: %d, total_cpu: %f, mem_rss: %d, mem_vms: %d, mem_swap: %d\n", ts, stat.Utime, stat.Stime, stat.Cutime, stat.Cstime, stat.Starttime, totalCpuUsage, memInfo.RSS, memInfo.VMS, memInfo.Swap)
				fmt.Printf("ts: %d, Process CPU %%: %f, Mem Used: %d\n", ts, psCPU, memInfoRSS)

			}

			/*
				_, err = file.Write([]byte(fmt.Sprintf("%d,%d,%d,%d,%d,%d,%f,%d,%d,%d\n", ts, stat.Utime, stat.Stime, stat.Cutime, stat.Cstime, stat.Starttime, totalCpuUsage, memInfo.RSS, memInfo.VMS, memInfo.Swap)))
				if err != nil {
					fmt.Println(err)
				}
			*/
			_, err = file.Write([]byte(fmt.Sprintf("%d,%f,%d,%d,%d\n", ts, psCPU, memInfoRSS, memInfoVMS, memInfoSwap)))
			if err != nil {
				fmt.Println(err)
			}

		case <-shutdownChan:
			if Debug {
				fmt.Println("[DEBUG] Terminating CPU stats collection goroutine")
			}
			file.Close()
			return
		}
	}

}

func GenerateRandomString(size int) string {

	var logEntry string
	if size <= 100 {
		logEntry = fmt.Sprintf(
			"[%s] %s %d %s (%s)",
			randomdata.FullDate(),
			randomdata.IpV4Address(),
			randomdata.Number(0, 1000),
			randomdata.SillyName(),
			randomdata.UserAgentString(),
		)
	} else {
		logEntry = fmt.Sprintf(
			"[%s] %s %d %s wrote: %s",
			randomdata.FullDate(),
			randomdata.IpV4Address(),
			randomdata.Number(0, 1000),
			randomdata.SillyName(),
			randomdata.Paragraph(),
		)
		for len(logEntry) < size {
			logEntry = fmt.Sprintf("%s %s", logEntry, randomdata.Paragraph())
		}
	}
	return logEntry[:size]
}
