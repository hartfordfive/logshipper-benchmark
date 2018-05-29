package main

import (
	"os/exec"
	"sync"
)

type Shipper interface {
	Name() string
	Run(binPath string, cmdArgs []string, workingDir string, filesToMonitor []string, kafkaBrokers []string, filebeatExec chan *exec.Cmd, terminate chan bool, wg *sync.WaitGroup)
	CleanupFiles()
	BuildConfig(confDestPath string, filesToMonitor []string, kafkTopicName string, kafkaBrokersList []string)
	GetVersion() string
}
