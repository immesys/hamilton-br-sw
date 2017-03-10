package main

import (
	"fmt"
	"io/ioutil"
	"path"
	"sync"
	"syscall"
	"time"

  "github.com/immesys/hamilton-br-sw/brgw/pb"
)

type dataLoggerCache struct {
	lastFlush time.Time
	messages  []*entry
	sync.Mutex
}

type entry struct {
	time    time.Time
	payload []byte
}

var DLC *dataLoggerCache

func init() {
	DLC = &dataLoggerCache{}
	go flushDLCLoop()
}
func logFrameToDatalogger(f *egressframe) {

}

func getDir(t time.Time) string {
	return t.Format("2006/01_Jan/02/15")
}
func flushDLCLoop() {
	for {
		time.Sleep(1 * time.Minute)
		flushDLC()
	}
}

func flushDLC() {

	DLC.Lock()
	if len(DLC.messages) < LengthFlushTrip &&
		time.Now().Sub(DLC.lastFlush) < TimeFlushTrip {
		DLC.Unlock()
		fmt.Printf("Skipping DLC flush, no conditions\n")
		return
	}

	msgs := DLC.messages
	DLC.messages = []*entry{}
	DLC.lastFlush = time.Now()
	DLC.Unlock()

	//Establish the marker file is there
	dat, _ := ioutil.ReadFile(path.Join(datalogdir, "enable"))
	if string(dat) != "enable_datalogger" {
		fmt.Printf("Skipping DLC flush, not enabled")
	}
	//Establish there is space
	var stat syscall.Statfs_t
	err := syscall.Statfs(datalogdir, &stat)
	if err != nil {
		fmt.Printf("Skipping DLC flush, stat error: %v\n", err)
		return
	}

	if stat.Bavail*stat.Bsize < MinAvailSpace {
		fmt.Printf("Skipping DLC flush, no space")
		return
	}

	ls := pb.LogSet{}
  ls.

}
