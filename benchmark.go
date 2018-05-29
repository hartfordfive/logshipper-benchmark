package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"plugin"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	utils "github.com/hartfordfive/logshipper-benchmark/lib"
	counter "github.com/hartfordfive/logshipper-benchmark/lib/counter"
)

var GitHash string
var BuildDate string
var Version string

func catchExitSig(sigChan chan os.Signal, shutdownChan chan bool) {
	for {
		select {
		case <-sigChan:
			fmt.Printf("[INFO] Caught signal. Notifying all goroutines via hutdown channel.\n")
			close(shutdownChan)
		}
	}
}

func waitForShutdown(counter *counter.Counter, shutdownChan chan bool) {

	for {
		select {
		case <-shutdownChan:
			fmt.Println("[INFO] Total lines written: ", counter.Value())
			fmt.Println("[INFO] Shutdown complete.")
			return
		} // End select block
	} // End for loop
}

func showBuildInfoAndExit() {
	fmt.Printf("logshipper-benchmark v%s (Git: %s)\nBuild Date: %s\n\n", Version, GitHash, BuildDate)
	os.Exit(0)
}

func generateBenchmarkResults(logShipperName string, pid int, linesWritten int64, startTime time.Time, totalSeconds float64, logStr string, numActiveLogFiles int, writeWaitPeriod int, metricDataFile string) string {

	var buffer bytes.Buffer
	endTime := startTime.Add(time.Second * time.Duration(uint64(totalSeconds)))
	buffer.WriteString("\n----------------------- Test Results ---------------------\n")
	buffer.WriteString(fmt.Sprintf("Log Shipper:              %s\n", logShipperName))
	buffer.WriteString(fmt.Sprintf("PID:                      %d\n", pid))
	buffer.WriteString(fmt.Sprintf("Start Time:               %s\n", startTime.Format(time.RFC3339)))
	buffer.WriteString(fmt.Sprintf("End Time:                 %s\n", endTime.Format(time.RFC3339)))
	buffer.WriteString(fmt.Sprintf("Total Time (s):           %f\n", totalSeconds))
	buffer.WriteString(fmt.Sprintf("Sample Log Entry:         %s\n", logStr))
	buffer.WriteString(fmt.Sprintf("Write Wait Period (ms):   %d\n", writeWaitPeriod))
	buffer.WriteString(fmt.Sprintf("Total Lines Written:      %d\n", linesWritten))
	buffer.WriteString(fmt.Sprintf("Total Files Written:      %d\n", numActiveLogFiles))
	buffer.WriteString(fmt.Sprintf("Calculated lines/s:       %d\n", (linesWritten / utils.RoundToEven(totalSeconds))))
	buffer.WriteString(fmt.Sprintf("Metricbeat data file:     %s\n", metricDataFile))
	buffer.WriteString("----------------------------------------------------------\n")
	return buffer.String()
}

func SaveToFile(filePath string, data string, fileMode int) error {
	err := ioutil.WriteFile(filePath, []byte(data), os.FileMode(fileMode))
	if err != nil {
		return err
	}
	return nil
}

func main() {

	GitHash = ""
	BuildDate = ""
	Version = ""

	utils.Debug = true

	if len(os.Args) != 2 {
		fmt.Println("Usage: ./benchmark [CONFIG_FILE]")
		os.Exit(1)
	}
	args := os.Args[1:]

	if args[0] == "-v" {
		showBuildInfoAndExit()
	}

	confPath := args[0]
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		fmt.Println("[ERROR] The specified config does not exist!")
		os.Exit(1)
	}

	config := LoadConfig(confPath)

	runtime.GOMAXPROCS(config.MaxProcs)

	fileHandles := make(map[int]*os.File)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	shutdownChan := make(chan bool, 1)

	go catchExitSig(sigChan, shutdownChan)

	// Create the test files that will be written to
	var filesToMonitor []string

	os.Remove(config.LogFilesBaseDir)
	utils.CreateDir(config.LogFilesBaseDir)

	for i := 0; i < config.NumActiveLogFiles; i++ {
		fPath := config.LogFilesBaseDir + "/file" + strconv.Itoa(i) + ".log"
		f, err := os.Create(fPath)
		if err != nil {
			fmt.Println(err)
		}
		filesToMonitor = append(filesToMonitor, fPath)
		fileHandles[i] = f
		utils.CheckErr(err)
		defer f.Close()
	}

	var wg sync.WaitGroup
	wg.Add(config.NumActiveLogFiles)
	wg.Add(1) // Also add an increment for the confirmation of the log shipper being shut down

	linesWrittenCounter := counter.NewCounter()

	execAck := make(chan *exec.Cmd, 1)

	re := regexp.MustCompile("  +")
	flags := string(re.ReplaceAll(bytes.TrimSpace([]byte(config.LogShipperFlags)), []byte(" ")))
	cmdArgs := strings.Split(flags, " ")

	// *********** Now the module is loaded ***************
	modulePath := fmt.Sprintf("%s/%s.so", filepath.Dir(config.ModuleDir), config.ModuleName)
	module, err := plugin.Open(modulePath)
	if err != nil {
		fmt.Printf("[ERROR] Could not open the %s module at %s: %s\n", config.LogShipperName, modulePath, err)
		os.Exit(1)
	}

	symShipper, err := module.Lookup("InitShipper")
	if err != nil {
		fmt.Println("[ERROR] Could not load plugin: ", err)
		os.Exit(1)
	}

	shipperIface, err := symShipper.(func() (interface{}, error))()
	shipper := shipperIface.(Shipper)

	// Get the start time of the execution
	start := utils.TimeTraceStart()

	// Startup the metric collector before running the log shipper
	mc := NewMetricCollector()
	tags := []string{
		"benchmark",
	}

	fields := map[string]string{
		"line_size":       fmt.Sprintf("%v", config.LogLineSize),
		"module_name":     config.ModuleName,
		"shipper_name":    config.LogShipperName,
		"shipper_version": shipper.GetVersion(),
		"active_files":    fmt.Sprintf("%v", config.NumActiveLogFiles),
		"run_time":        fmt.Sprintf("%v", config.TotalRunTimeSeconds),
		"write_wait_ms":   fmt.Sprintf("%v", config.WriteWaitPeriodMs),
	}

	t := time.Now()
	dt := fmt.Sprintf("%d%02d%02d%02d%02d%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
	metricsFileName := fmt.Sprintf("benchmark-%s-%dbytes_%dfiles_%ds_%s.log", config.LogShipperName, config.LogLineSize, config.NumActiveLogFiles, config.TotalRunTimeSeconds, dt)
	mbWorkingDir := fmt.Sprintf("%s/%s/", strings.TrimRight(config.WorkingDir, "/"), "metricbeat")
	utils.CreateDir(mbWorkingDir)
	go mc.RunMetricbeat("/usr/share/metricbeat/bin/metricbeat", []string{"-c", "metricbeat.yml", "--path.data", "."}, mbWorkingDir, []string{config.LogShipperProcessName}, fields, tags, metricsFileName, shutdownChan, &wg)

	if config.TotalRunTimeSeconds >= 1 {
		go func(shutdownChan chan bool) {
			fmt.Printf("[INFO] Running benchark for %d seconds and then exiting.\n", config.TotalRunTimeSeconds)
			timerRunTime := time.NewTimer(time.Duration(config.TotalRunTimeSeconds) * time.Second)
			<-timerRunTime.C
			close(shutdownChan)
		}(shutdownChan)
	}

	// Start the log shipper
	workingDir := fmt.Sprintf("%s/%s/", strings.TrimRight(config.WorkingDir, "/"), config.LogShipperName)
	utils.CreateDir(workingDir)
	go shipper.Run(config.LogShipperBinPath, cmdArgs, workingDir, filesToMonitor, config.KafkaBrokerList, execAck, shutdownChan, &wg)

	// Now wait until we get a copy of the pointer to the exec.Cmd struct
	fmt.Println("Waiting for confirmation of shipper started...")
	shipperExec := <-execAck

	go waitForShutdown(linesWrittenCounter, shutdownChan)

	logStrLen := config.LogLineSize
	if config.EnableRandom {
		logStrLen = utils.GetRandInt(config.RandomLineSize[0], config.RandomLineSize[1])
	}

	logStr := utils.GenerateRandomString(logStrLen) + "\n"

	fmt.Printf("Using dummy log entry (%d bytes):\n\t%s\n", config.LogLineSize, logStr)

	// Now itterate ovear each file handle and write to the file
	for i := 0; i < config.NumActiveLogFiles; i++ {

		if utils.Debug {
			fmt.Printf("[DEBUG] Creating goroutine #%d to write to %s\n", i, fileHandles[i].Name())
		}

		go func(logStr string, filePath string, fh *os.File, counter *counter.Counter, shutdownChan <-chan bool, wg *sync.WaitGroup) {

			buffWritter := bufio.NewWriterSize(fh, 4096*8) // 32K buffer
			writeWaitPeriod := config.WriteWaitPeriodMs
			if config.EnableRandom {
				writeWaitPeriod = utils.GetRandInt(config.RandomWriteWait[0], config.RandomWriteWait[1])
			}
			ticker_write := time.NewTicker(time.Millisecond * time.Duration(writeWaitPeriod))
			ticker_flush := time.NewTicker(time.Millisecond * 2000)
			defer ticker_write.Stop()
			defer ticker_flush.Stop()

			logMsgSize := len(logStr)
			for {
				select {
				case <-ticker_write.C:
					_, err := buffWritter.WriteString(logStr)
					if err != nil {
						fmt.Println(err)
					}
					counter.Incr(1)
				case <-ticker_flush.C:
					if buffWritter.Available() < logMsgSize {
						buffWritter.Flush()
					}
				case <-shutdownChan:
					fInfo, _ := fh.Stat()
					if utils.Debug {
						fmt.Printf("[INFO] Terminating file writter for %s\n", fInfo.Name())
					}
					fh.Close()
					if err := os.Remove(filePath); err != nil {
						if utils.Debug {
							fmt.Printf("[ERROR] Coud not delete %s: %s\n", filePath, err)
						}
					}
					wg.Done()
					return
				}
			}
		}(logStr, filesToMonitor[i], fileHandles[i], linesWrittenCounter, shutdownChan, &wg)

	}

	wg.Wait()
	totalSeconds := utils.TimeTraceEnd(start)
	fmt.Println("[INFO] Generating report...")
	report := generateBenchmarkResults(
		config.LogShipperName,
		shipperExec.Process.Pid,
		linesWrittenCounter.Value(),
		start,
		totalSeconds,
		logStr,
		config.NumActiveLogFiles,
		config.WriteWaitPeriodMs,
		config.MetricsDir+"/"+metricsFileName,
	)

	fmt.Println(SaveToFile(fmt.Sprintf("%s/report-%s_%s.txt", strings.TrimRight(config.WorkingDir, "/"), config.LogShipperName, dt), report, 0644))

}
