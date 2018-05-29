package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"
	"text/template"

	ps "github.com/mitchellh/go-ps"

	utils "github.com/hartfordfive/logshipper-benchmark/lib"
)

const metricbeatConfigTpl = `
---
setup:
  template:
    enabled: false

output:
  file:
    path: {{.MetricsFilePath}}
    filename: {{.MetricsFileName}}
    rotate_every_kb: 1048576
    number_of_files: 5

logging:
  level: error
  to_files: true
  json: true
  files:
    path: {{.MetricbeatLogPath}}
    name: out
    rotateeverybytes: 936870912
    keepfiles: 2
    permissions: '0644'

metricbeat.modules:
- module: system
  period: 2s
  metricsets:
    - process
  processes: [{{range $index, $ps := .MonitoredProcesses}}{{if $index}}, {{end}}'{{$ps}}'{{end}}] 
  #process.include_cpu_ticks: true

- module: system
  period: 10s
  metricsets:
    - process_summary

- module: system
  period: 5s
  metricsets:
    - cpu
    - load
    - memory

{{$shipper := index .MonitoredProcesses 0}}
{{if eq $shipper "java"}} 
#processors:
# - drop_event:
#     when:
#       regexp:
#         system.process.name:  
{{end}}

fields:
  meta:
    category: benchmark
    {{- range $name, $val := .Fields}}
    {{$name}}: {{$val}}
    {{- end}}

tags: [{{range $index, $tag := .Tags}}{{if $index}}, {{end}}'{{$tag}}'{{end}}]

`

type metricbeatConfig struct {
	MetricsFilePath    string
	MetricsFileName    string
	MetricbeatLogPath  string
	MonitoredProcesses []string
	KafkaBrokers       []string
	KafkaTopic         string
	Tags               []string
	Fields             map[string]string
}

type metricCollector struct {
}

func NewMetricCollector() *metricCollector {
	return &metricCollector{}
}

func (mc *metricCollector) CleanupFiles() {
	if _, err := os.Stat("metricbeat.yml"); err == nil {
		os.Remove("metricbeat.yml")
	}
}

func (mc *metricCollector) BuildConfig(confDestPath string, conf *metricbeatConfig) error {

	mc.CleanupFiles()

	confBaseName := path.Base(confDestPath)
	t := template.Must(template.New(fmt.Sprintf("%s.tpl", confBaseName)).Parse(metricbeatConfigTpl))

	fh, err := os.Create(confDestPath)
	if err != nil {
		fmt.Printf("[ERROR] Could not create metricbeat config at %s: %s\n", confDestPath, err)
		os.Exit(1)
	}
	defer fh.Close()

	return t.Execute(fh, conf)
}

func (mc *metricCollector) RunMetricbeat(binPath string, cmdArgs []string, workingDir string, processesToMonitor []string, fields map[string]string, tags []string, metricsLogFileName string, shutdownChan chan bool, wg *sync.WaitGroup) {

	//Generate, the config
	wd := strings.TrimRight(workingDir, "/")
	if err := mc.BuildConfig(fmt.Sprintf("%s/metricbeat.yml", wd), &metricbeatConfig{
		MetricsFilePath:    workingDir,
		MetricsFileName:    metricsLogFileName,
		MetricbeatLogPath:  fmt.Sprintf("%s/logs/", wd),
		MonitoredProcesses: processesToMonitor,
		Fields:             fields,
		Tags:               tags,
		//KafkaBrokers: []string{"kafka01:9092"},
		//KafkaTopic: "dev-logs-metrics-metricbeat",
	}); err != nil {
		fmt.Printf("[ERROR] Could not create metricbeat config: %s\n", err)
		os.Exit(1)
	}

	// Run metricbeat
	cmd := exec.Command(binPath, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = workingDir
	// var stdout, stderr bytes.Buffer
	// cmd.Stdout = &stdout
	// cmd.Stderr = &stderr
	err := cmd.Start()

	if err != nil {
		fmt.Printf("Could not run metricbeat: %s\n", err)
		os.Exit(1)
	}

	// outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	// fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)

	go mc.waitForShutdown(cmd.Process.Pid, shutdownChan)

	fmt.Printf("Metricbeat is now running (pid %d).\n", cmd.Process.Pid)
	cmd.Wait()
	wg.Done()
}

func (mc *metricCollector) waitForShutdown(metricCollectorPid int, shutdownChan <-chan bool) {

	fmt.Println("[DEBUG] Waiting for shutdown signal to terminate metric collector...")

	//for {

	//select {

	//case <-shutdownChan:
	<-shutdownChan

	pgid, err := syscall.Getpgid(metricCollectorPid)
	if err == nil {
		p, err := ps.FindProcess(pgid)

		fmt.Printf("[INFO] Shutting down %s....\n", p.Executable())
		if utils.Debug {
			fmt.Println("[DEBUG] Metric Collector Process ID: ", p.Pid())
			fmt.Println("[DEBUG] Metric Collector Parent Process ID:", p.PPid())
		}
		psName := p.Executable()

		if err != nil {
			fmt.Println("[ERROR] Could not find metric collector process by pgid: ", err)
		}
		if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil {
			fmt.Printf("[INFO] Process '%s' has been shut down.\n", psName)
		} else {
			fmt.Printf("[ERROR] Could not shut down %s: %s\n", psName, err)
		}
	} else {
		fmt.Printf("[ERROR] Could not get metric collector process pgid for shutdown: %s\n", err)
	}

	fmt.Println("[INFO] Metric collector shutdown complete.")

	//return
	//} // End select block

	//} // End for loop
}
