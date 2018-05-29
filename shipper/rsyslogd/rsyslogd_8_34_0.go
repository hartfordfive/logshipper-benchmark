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

const supportedShipperVersionMajor = 8
const supportedShipperVersionMinor = 34
const supportedShipperVersionPatch = 0

var configTpl = `module(load="imfile")    # Input module from files
module(load="omkafka")   # Output module to kafka

{{range $index, $file := .FilesToMonitor}}
input(type="imfile"
  File="{{$file}}"
  Tag="file{{$index}}"
)
{{end}}

template(name="json" type="list" option.json="on") {
        constant(value="{")
        constant(value="\"@timestamp\":\"")
        property(name="timegenerated" dateFormat="rfc3339")
        constant(value="\",\"message\":\"")
        property(name="msg")
        constant(value="\",")
        constant(value="\"host\":\"")
        property(name="hostname")
        constant(value="\"}")
}

main_queue(
  queue.workerthreads="1"      # threads to work on the queue
  queue.dequeueBatchSize="100" # max number of messages to process at once
  queue.size="10000"           # max queue size
)

# Global (confParam) and topic level (topicConfParam) configs can be found here: https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md

action(
  broker=[{{range $index, $broker := .KafkaBrokers}}{{if $index}},{{end}}"{{$broker}}"{{end}}]
  type="omkafka"
  topic="{{ .KafkaTopic}}"
  #confParam=[ "compression.codec=snappy",
  #            "socket.timeout.ms=1000",
  #            "socket.keepalive.enable=true"]
  topicConfParam=[ "request.required.acks=1" ]
  template="json"
)
`

type config struct {
	KafkaBrokers   []string
	KafkaTopic     string
	FilesToMonitor []string
}

type shipper struct{}

func (s shipper) Name() string { return "rsyslogd" }

func (s shipper) CleanupFiles() {
	if _, err := os.Stat("rsyslog.conf"); err == nil {
		os.Remove("rsyslog.conf")
	}
	if _, err := os.Stat("rsyslog.pid"); err == nil {
		os.Remove("rsyslog.pid")
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
		fmt.Sprintf("%s/rsyslog.conf", strings.TrimRight(workingDir, "/")),
		filesToMonitor,
		fmt.Sprintf("dev-logs-shipper-benchmarks-%s", s.Name()),
		kafkBrokers,
	)

	if Debug {
		fmt.Println("[DEBUG] Changing to working dir: ", workingDir)
	}

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
