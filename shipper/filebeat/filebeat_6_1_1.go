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

	//"github.com/containerd/cgroups"
	//specs "github.com/opencontainers/runtime-spec/specs-go"

	utils "github.com/hartfordfive/logshipper-benchmark/lib"
)

var Debug bool = false

const supportedShipperVersionMajor = 6
const supportedShipperVersionMinor = 1
const supportedShipperVersionPatch = 1

const configTpl = `---
setup:
  template:
    enabled: false

output:
  kafka:
    hosts:
    {{- range .KafkaBrokers}}
    - {{.}}
    {{end}}
    topic: "{{ .KafkaTopic}}"

logging:
  level: error
  to_files: false
  json: false

filebeat.prospectors:
{{- range .FilesToMonitor}}
- enabled: true
  fields_under_root: true
  type: log
  scan_frequency: 10s
  close_eof: true
  paths:
  - "{{.}}"
{{- end}}
`

type config struct {
	KafkaBrokers   []string
	KafkaTopic     string
	FilesToMonitor []string
}

type shipper struct {
}

func (s shipper) Name() string { return "filebeat" }

func (s shipper) CleanupFiles() {
	if _, err := os.Stat("registry"); err == nil {
		os.Remove("registry")
	}
	if _, err := os.Stat("meta.json"); err == nil {
		os.Remove("meta.json")
	}
	if _, err := os.Stat("filebeat.yml"); err == nil {
		os.Remove("filebeat.yml")
	}

}

func (s shipper) BuildConfig(confDestPath string, filesToMonitor []string, kafkTopicName string, kafkaBrokersList []string) {

	s.CleanupFiles()

	confBaseName := path.Base(confDestPath)
	t := template.Must(template.New(fmt.Sprintf("%s.tpl", confBaseName)).Parse(configTpl))

	fh, err := os.Create(confDestPath)
	if err != nil {
		fmt.Println("[ERROR] Could not create config: ", err)
		os.Exit(1)
	}
	defer fh.Close()

	err = t.Execute(fh, &config{
		KafkaBrokers:   kafkaBrokersList,
		KafkaTopic:     kafkTopicName,
		FilesToMonitor: filesToMonitor,
	})

	if err != nil {
		panic(err)
	}
}

func (s shipper) Run(binPath string, cmdArgs []string, workingDir string, filesToMonitor []string, kafkBrokers []string, execChan chan *exec.Cmd, shutdownChan chan bool, wg *sync.WaitGroup) {

	// First generate, the filebeat config
	s.BuildConfig(
		fmt.Sprintf("%s/filebeat.yml", strings.TrimRight(workingDir, "/")),
		filesToMonitor,
		fmt.Sprintf("dev-logs-shipper-benchmarks-%s", s.Name()),
		kafkBrokers,
	)

	/*
	//shares := uint64(100)
        realtimeRuntime :=  int64(10000) // 10 ms
        realtimePeriod := uint64(500000) // 500 ms

	control, err := cgroups.New(cgroups.V1, cgroups.StaticPath(fmt.Sprintf("/logshipper-benchmark-%s", s.Name())), &specs.LinuxResources{
		CPU: &specs.LinuxCPU{
			//Shares:          &shares,
			RealtimeRuntime: &realtimeRuntime,  
			RealtimePeriod:  &realtimePeriod, 
		},
	})
	if err != nil {
		fmt.Println("[ERROR] Could not create cgroup: ", err)
	} else {
		defer control.Delete()
	}
	*/

	// Ensure the working directory exists, if not create it
	//workingDir :=  fmt.Sprintf("%s/benchmarks/%s/", strings.TrimRight(utils.GetCwd(), "/"), s.Name())
	if Debug {
		fmt.Println("[DEBUG] Changing to working dir: ", workingDir)
	}
	//utils.CreateDir(workingDir)

	// Now run filebeat
	cmd := exec.Command(binPath, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = workingDir
	// var stdout, stderr bytes.Buffer
	// cmd.Stdout = &stdout
	// cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("[ERROR] Could not run %s: %s\n", s.Name(), err)
		os.Exit(1)
	}

	/*
	if err := control.Add(cgroups.Process{Pid: cmd.Process.Pid}); err != nil {
		fmt.Printf("[WARN] Could not add %s (pid %d) to logshipper-benchmark cgroup: %s\n", s.Name(), cmd.Process.Pid, err)
	}
	*/	

	go utils.CollectCpuStats(cmd.Process.Pid, fmt.Sprintf("%s/metrics/%s", strings.TrimRight(workingDir, "/"), s.Name()), shutdownChan)

	execChan <- cmd

	// outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	// fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)

	go func(shudownChan <-chan bool, cmd *exec.Cmd) {
		fmt.Printf("[INFO] Waiting for signal to shutdown %s...\n", s.Name())
		<-shutdownChan
		fmt.Printf("[INFO] Terminating shipper...\n")
		// err := cmd.Process.Signal(os.Interrupt)
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err != nil {
			fmt.Printf("[ERROR] Could not shutdown shipper: %s\n", err.Error())
		} else {
			fmt.Printf("[INFO] %s has been shut down.\n", s.Name())
		}
		s.CleanupFiles()
	}(shutdownChan, cmd)

	fmt.Printf("[INFO] %s is now running.\n", s.Name())
	cmd.Wait()
}

func (s shipper) GetVersion() string {
	return fmt.Sprintf("%d.%d.%d", supportedShipperVersionMajor, supportedShipperVersionMinor, supportedShipperVersionPatch)
}

func (s shipper) EnsureVersion(binPath string) bool {
	// filebeat version [VERSION] ([ARC]), libbeat [VERSION]
	output, err := exec.Command(binPath, "version").CombinedOutput()
	if err != nil {
		os.Stderr.WriteString(err.Error())
	}
	fmt.Println(string(output))
	return false
}

func InitShipper() (s interface{}, err error) {
	s = shipper{}
	return
}
