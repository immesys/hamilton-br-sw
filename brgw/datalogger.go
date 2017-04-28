package main

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/immesys/hamilton-br-sw/brgw/pb"
)

//go:generate protoc pb/dlog.proto --go_out=.
type dataLoggerCache struct {
	lastFlush time.Time
	messages  []*pb.LogEntry
	sync.Mutex
}

const datalogparent = "/volatile"
const datalogdir = "/volatile/datalogger"

//500MB
const MinAvailSpace = 500 * 1024 * 1024

var DLC *dataLoggerCache
var TOKEN = [8]byte{0xd2, 0x14, 0x20, 0x7c, 0xc7, 0x38, 0x95, 0x2d}

func init() {
	DLC = &dataLoggerCache{}
	go flushDLCLoop()
}
func logFrameToDatalogger(f *egressmessage) {
	frame := pb.L7GFrame{
		Srcmac:  f.Srcmac,
		Srcip:   f.Srcip,
		Popid:   f.Popid,
		Poptime: f.Poptime,
		Brtime:  f.Brtime,
		Rssi:    int32(f.Rssi),
		Lqi:     int32(f.Lqi),
		Payload: f.Payload,
	}
	msg := pb.LogEntry{
		Time: int64(time.Now().UnixNano()),
		L7G:  &frame,
	}
	DLC.Lock()
	DLC.messages = append(DLC.messages, &msg)
	DLC.Unlock()
}

func getFilename(t time.Time) string {
	return path.Join(datalogdir, t.Format("2006/01_Jan/02_Mon/15/")) + "/hbr_" + t.Format("2006_01_02__15_04") + ".dat"
}
func flushDLCLoop() {
	for {
		time.Sleep(1 * time.Minute)
		flushDLC()
	}
}

const LengthFlushTrip = 10000
const TimeFlushTrip = 10 * time.Minute

func flushDLC() {

	DLC.Lock()
	if len(DLC.messages) == 0 || (len(DLC.messages) < LengthFlushTrip &&
		time.Now().Sub(DLC.lastFlush) < TimeFlushTrip) {
		fmt.Printf("We have %d messages in queue\n", len(DLC.messages))
		DLC.Unlock()
		fmt.Printf("Skipping DLC flush, no conditions\n")
		return
	}

	msgs := DLC.messages
	DLC.messages = []*pb.LogEntry{}
	DLC.lastFlush = time.Now()
	DLC.Unlock()

	fmt.Printf("flushing %d messages\n", len(msgs))
	// //Establish the marker file is there
	// dat, _ := ioutil.ReadFile(path.Join(datalogdir, "enable"))
	// if string(dat) != "enable_datalogger" {
	// 	fmt.Printf("Skipping DLC flush, not enabled")
	// }
	//Establish there is space
	var stat syscall.Statfs_t
	err := syscall.Statfs(datalogparent, &stat)
	if err != nil {
		fmt.Printf("Skipping DLC flush, stat error: %v\n", err)
		return
	}

	if stat.Bavail*uint64(stat.Bsize) < MinAvailSpace {
		fmt.Printf("Skipping DLC flush, no space\n")
		return
	}

	ls := pb.LogSet{}

	mintime := msgs[0].Time
	maxtime := msgs[0].Time
	for _, m := range msgs {
		if m.Time < mintime {
			mintime = m.Time
		}
		if m.Time > maxtime {
			maxtime = m.Time
		}
		ls.Logs = append(ls.Logs, m)
	}
	ls.StartTime = mintime
	ls.EndTime = maxtime

	//populate
	data, err := proto.Marshal(&ls)
	if err != nil {
		fmt.Printf("marshaling error: %v \n", err)
		return
	}
	sum := md5.Sum(data)
	sz := len(data)

	fname := getFilename(time.Unix(0, ls.StartTime))
	d := filepath.Dir(fname)
	err = os.MkdirAll(d, 0777)
	if err != nil {
		fmt.Printf("Could not make flush directory: %v\n", err)
		return
	}
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		fmt.Printf("Could open flush file: %v\n", err)
		return
	}
	defer f.Close()
	hdr := make([]byte, 8+4+16)
	copy(hdr[0:8], TOKEN[:])
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(sz))
	copy(hdr[12:], sum[:])
	_, err = f.Write(hdr)
	if err != nil {
		fmt.Printf("Could open write DLC file: %v\n", err)
		return
	}
	_, err = f.Write(data)
	if err != nil {
		fmt.Printf("Could open write DLC file: %v\n", err)
		return
	}
	fmt.Printf("Flushed DLC to %s\n", fname)
}
