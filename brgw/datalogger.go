package main

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/immesys/hamilton-br-sw/brgw/pb"
)

type dataLoggerCache struct {
	lastFlush time.Time
	messages  []LogEntry
	sync.Mutex
}

type entry struct {
	payload LogEntry
}

var DLC *dataLoggerCache

func init() {
	DLC = &dataLoggerCache{}
	go flushDLCLoop()
}
func logFrameToDatalogger(f *egressframe) {

}

func getFilename(t time.Time) string {
	return t.Format("2006/01_Jan/02_Mon/15")
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

	// //Establish the marker file is there
	// dat, _ := ioutil.ReadFile(path.Join(datalogdir, "enable"))
	// if string(dat) != "enable_datalogger" {
	// 	fmt.Printf("Skipping DLC flush, not enabled")
	// }
	//Establish there is space
	var stat syscall.Statfs_t
	err := syscall.Statfs(datalogdir, &stat)
	if err != nil {
		fmt.Printf("Skipping DLC flush, stat error: %v\n", err)
		return
	}

	if stat.Bavail*stat.Bsize < MinAvailSpace {
		fmt.Printf("Skipping DLC flush, no space\n")
		return
	}

	ls := pb.LogSet{}
	//populate
	data, err := proto.Marshal(ls)
	if err != nil {
		fmt.Printf("marshaling error: \n", err)
		return
	}
	sum := md5.Sum(data)
	sz := len(data)

	fname := getFilename(ls.Time())
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	hdr := make([]byte, 8+4+16)
	copy(hdr[0:8], TOKEN)
	binary.LittleEndian.PutUint32(hdr[8:12], sz)
	copy(hdr[12:], sum)
	f.Write(hdr)
	f.Write(data)
	f.Close()
}
