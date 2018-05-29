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

	utils "github.com/hartfordfive/logshipper-benchmark/lib"
)

var Debug bool = false

const supportedShipperVersionMajor = 0
const supportedShipperVersionMinor = 13
const supportedShipperVersionPatch = 1

var configTpl = `
[SERVICE]
    Flush           5
    Daemon          off
    Log_Level       debug
    HTTP_Monitoring On
    HTTP_Port       2020

{{range $index, $file := .FilesToMonitor}}
[INPUT]
    Name        tail
    Path        {{$file}}
    #Path_Key	source
    Tag         file{{$index}}
{{end}}

[OUTPUT]
    Name        kafka
    Match       *
    Brokers     {{range $index, $broker := .KafkaBrokers}}{{if $index}},{{end}}{{$broker}}{{end}}
    Topics      {{.KafkaTopic}}
`

type config struct {
	KafkaBrokers   []string
	KafkaTopic     string
	FilesToMonitor []string
}

type shipper struct{}

func (s shipper) Name() string { return "fluentbit" }

func (s shipper) CleanupFiles() {
	if _, err := os.Stat("td-agent-bit.conf"); err == nil {
		os.Remove("td-agent-bit.conf")
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
		fmt.Sprintf("%s/td-agent-bit.conf", strings.TrimRight(workingDir, "/")),
		filesToMonitor,
		fmt.Sprintf("dev-logs-shipper-benchmarks-%s", s.Name()),
		kafkBrokers,
	)

	// Now run the binary
	cmd := exec.Command(binPath, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = workingDir

	// var stdout, stderr bytes.Buffer
	// cmd.Stdout = &stdout
	// cmd.Stderr = &stderr
	err := cmd.Start()

	go utils.CollectCpuStats(cmd.Process.Pid, fmt.Sprintf("%s/metrics/%s", strings.TrimRight(workingDir, "/"), s.Name()), shutdownChan)

	if err != nil {
		fmt.Printf("[ERROR] Could not run %s: %s\n", s.Name(), err)
		os.Exit(1)
	}

	execChan <- cmd

	// outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	// fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)

	go func(shudownChan <-chan bool, cmd *exec.Cmd) {
		fmt.Printf("[INFO] Waiting for signal to shutdown %s...\n", s.Name())
		<-shutdownChan
		fmt.Printf("[INFO] Terminating shipper...\n")
		//err := cmd.Process.Signal(os.Interrupt)
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

func InitShipper() (s interface{}, err error) {
	s = shipper{}
	return
}
