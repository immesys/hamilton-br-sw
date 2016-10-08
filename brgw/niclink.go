package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/kidoman/embd"
	_ "github.com/kidoman/embd/host/rpi"
	"gopkg.in/immesys/bw2bind.v5"
)

var OurPopID string
var BaseURI string
var totaltx int

const HeartbeatTimeout = 2 * time.Second
const BlinkInterval = 200 * time.Millisecond
const HbTypeMcuToPi = 1
const HbTypePiToMcu = 2
const PILED = 25

var LedChan chan int

const FULLOFF = 1
const FULLON = 2
const BLINKING1 = 3
const BLINKING2 = 4
const BLINKING3 = 5
const BadAge = 2 * time.Minute

var WanChan chan int

var puberror uint64
var pubsucc uint64

func die() {
	embd.CloseGPIO()
	os.Exit(1)
}
func processIncomingHeartbeats() {
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: "@rethos/4", Net: "unixpacket"})
	if err != nil {
		fmt.Printf("heartbeat socket: error: %v\n", err)
		die()
	}
	gotHeartbeat := make(chan bool, 1)
	hbokay := make(chan bool, 1)
	go func() {
		for {
			select {
			case <-time.After(HeartbeatTimeout):
				hbokay <- false
				continue
			case <-gotHeartbeat:
				hbokay <- true
				continue
			}
		}
	}()
	go func() {
		for wanstate := range WanChan {
			msg := make([]byte, 4)
			msg[0] = HbTypePiToMcu
			msg[1] = byte(wanstate)
			msg[2] = 0x55
			msg[3] = 0xAA
			_, err := conn.Write(msg)
			if err != nil {
				fmt.Printf("got wanstate error: %v\n", err)
				os.Exit(10)
			} else {
				fmt.Printf("wrote wanstate\n")
			}
		}
	}()
	go func() {
		okaycnt := 0
		for {
			select {
			case x := <-hbokay:
				if x {
					okaycnt++
					if okaycnt > 5 {
						LedChan <- FULLON
						okaycnt = 5
					}
				} else {
					LedChan <- BLINKING1
					okaycnt = 0
				}
			}
		}
	}()
	fmt.Println("hearbeat socket: connected ok")
	for {
		buf := make([]byte, 16*1024)
		num, _, err := conn.ReadFromUnix(buf)
		if err != nil {
			fmt.Printf("heartbeat socket: error: %v\n", err)
			die()
		}
		if num >= 12 && binary.LittleEndian.Uint32(buf) == HbTypeMcuToPi {
			gotHeartbeat <- true
		} else {
			hbokay <- false
		}
	}
}

const ResetInterval = 30 * time.Second

func processWANStatus(bw *bw2bind.BW2Client) {
	lasterr := puberror
	lastsucc := pubsucc
	lastReset := time.Now()
	for {
		bcip, err := bw.GetBCInteractionParams()
		if err != nil {
			fmt.Printf("Could not get BCIP: %v\n", err)
			die()
		}
		resp, err := http.Get("http://steelcode.com/hbr.check")
		hasInternet := false
		if err == nil {
			msg := make([]byte, 5)
			n, err2 := resp.Body.Read(msg)
			if err2 == nil && n == 5 && string(msg) == "HBROK" {
				hasInternet = true
			} else {
				fmt.Printf("Internet check error 2: %d %v %x\n", n, err2, msg)
			}
		} else {
			fmt.Printf("Internet check error 1: %v\n", err)
		}
		if hasInternet {
			if bcip.CurrentAge > BadAge {
				WanChan <- BLINKING1
			} else {
				if puberror > lasterr {
					WanChan <- BLINKING2
				} else if pubsucc > lastsucc {
					WanChan <- FULLON
				} else {
					//We do not give an advisory until we fail or succeed once
				}
				if time.Now().Sub(lastReset) > ResetInterval {
					lasterr = puberror
					lastsucc = pubsucc
					lastReset = time.Now()
				}
			}
		} else {
			WanChan <- FULLOFF
		}
		time.Sleep(1000 * time.Millisecond)
	}
}

/**
typedef struct {
    uint64_t serial_received;
    uint64_t domain_forwarded;

    uint64_t domain_received;
    uint64_t serial_forwarded;

    uint64_t bad_frames;
    uint64_t lost_frames;

    uint64_t drop_notconnected;
} __attribute__((packed)) global_stats_t;

typedef struct {
    uint64_t serial_received;
    uint64_t domain_forwarded;
    uint64_t drop_notconnected;

    uint64_t domain_received;
    uint64_t serial_forwarded;
} channel_stats_t;
*/
type LinkStats struct {
	BadFrames           uint64 `msgpack:"bad_frames"`
	LostFrames          uint64 `msgpack:"lost_frames"`
	DropNotConnected    uint64 `msgpack:"drop_not_connected"`
	SumSerialReceived   uint64 `msgpack:"sum_serial_received"`
	SumDomainForwarded  uint64 `msgpack:"sum_domain_forwarded"`
	SumDropNotConnected uint64 `msgpack:"drop_not_connected"`
	SumDomainReceived   uint64 `msgpack:"sum_domain_received"`
	SumSerialForwarded  uint64 `msgpack:"sum_serial_forwarded"`
}

func processStats(bw *bw2bind.BW2Client) {
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: "@rethos/0", Net: "unixpacket"})
	if err != nil {
		fmt.Printf("heartbeat socket: error: %v\n", err)
		die()
	}
	for {
		buf := make([]byte, 32*1024)
		num, _, err := conn.ReadFromUnix(buf)
		buf = buf[:num]
		ls := LinkStats{}
		idx := 4 //Skip the first four fields
		ls.BadFrames = binary.LittleEndian.Uint64(buf[idx*8:])
		idx++
		ls.LostFrames = binary.LittleEndian.Uint64(buf[idx*8:])
		idx++
		ls.DropNotConnected = binary.LittleEndian.Uint64(buf[idx*8:])
		idx++
		for i := 0; i < 255; i++ {
			serial_received := binary.LittleEndian.Uint64(buf[idx*8:])
			idx++
			domain_forwarded := binary.LittleEndian.Uint64(buf[idx*8:])
			idx++
			drop_notconnected := binary.LittleEndian.Uint64(buf[idx*8:])
			idx++
			domain_received := binary.LittleEndian.Uint64(buf[idx*8:])
			idx++
			serial_forwarded := binary.LittleEndian.Uint64(buf[idx*8:])
			idx++
			ls.SumSerialReceived += serial_received
			ls.SumDomainForwarded += domain_forwarded
			ls.SumDropNotConnected += drop_notconnected
			ls.SumDomainReceived += domain_received
			ls.SumSerialForwarded += serial_forwarded
		}
		po, _ := bw2bind.CreateMsgPackPayloadObject(bw2bind.FromDotForm("2.0.10.2"), ls)
		err = bw.Publish(&bw2bind.PublishParams{
			URI:            fmt.Sprintf("%s/%s/s.hamilton/_/i.l7g/signal/stats", BaseURI, OurPopID),
			PayloadObjects: []bw2bind.PayloadObject{po},
		})
		if err != nil {
			atomic.AddUint64(&puberror, 1)
			fmt.Printf("BW2 status publish failure: %v\n", err)
		} else {
			atomic.AddUint64(&pubsucc, 1)
		}
	}
}

func processIncomingData(bw *bw2bind.BW2Client) {
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: "@rethos/5", Net: "unixpacket"})
	if err != nil {
		fmt.Printf("heartbeat socket: error: %v\n", err)
		die()
	}
	for {
		buf := make([]byte, 16*1024)
		num, _, err := conn.ReadFromUnix(buf)
		if err != nil {
			fmt.Printf("data socket: error: %v\n", err)
			die()
		}
		frame, ok := unpack(buf[:num])
		if !ok {
			fmt.Println("bad frame")
			continue
		}
		po, _ := bw2bind.CreateMsgPackPayloadObject(bw2bind.PONumL7G1Raw, frame)
		err = bw.Publish(&bw2bind.PublishParams{
			URI:            fmt.Sprintf("%s/%s/s.hamilton/%s/i.l7g/signal/raw", BaseURI, OurPopID, frame.Srcmac),
			PayloadObjects: []bw2bind.PayloadObject{po},
		})
		totaltx++
		if err != nil {
			atomic.AddUint64(&puberror, 1)
			fmt.Printf("BW2 publish error: %v\n", err)
		} else {
			atomic.AddUint64(&pubsucc, 1)
		}
	}
}

func LedAnim(ledchan chan int) {
	state := FULLOFF
	lastval := false
	go func() {
		for x := range ledchan {
			state = x
			if state == FULLOFF {
				embd.DigitalWrite(PILED, embd.Low)
			}
			if state == FULLON {
				embd.DigitalWrite(PILED, embd.High)
			}
		}
	}()
	for {
		<-time.After(BlinkInterval)
		if state == FULLOFF {
			embd.DigitalWrite(PILED, embd.Low)
		}
		if state == FULLON {
			embd.DigitalWrite(PILED, embd.High)
		}
		if state == BLINKING1 {
			if lastval {
				embd.DigitalWrite(PILED, embd.Low)
			} else {
				embd.DigitalWrite(PILED, embd.High)
			}
			lastval = !lastval
		}
	}
}
func main() {
	embd.InitGPIO()
	defer embd.CloseGPIO()
	embd.SetDirection(PILED, embd.Out)
	LedChan = make(chan int, 1)
	WanChan = make(chan int, 1)
	go LedAnim(LedChan)
	//TODO set the Pi Led OFF before you do
	//anything that could cause exit
	OurPopID = os.Getenv("POP_ID")
	if OurPopID == "" {
		fmt.Println("Missing $POP_ID")
		die()
	}
	BaseURI = os.Getenv("POP_BASE_URI")
	if BaseURI == "" {
		fmt.Println("Missing $POP_BASE_URI")
		die()
	}
	bw := bw2bind.ConnectOrExit("")
	bw.SetEntityFromEnvironOrExit()
	bw.OverrideAutoChainTo(true)
	var Maxage int64 = 5 * 60
	bw.SetBCInteractionParams(&bw2bind.BCIP{
		Maxage: &Maxage,
	})
	go processIncomingHeartbeats()
	go processWANStatus(bw)
	processIncomingData(bw)
}

func unpack(frame []byte) (*egressmessage, bool) {
	if len(frame) < 38 {
		return nil, false
	}
	fs := egressmessage{
		//skip 0:4 - len+ cksum
		Srcmac:  fmt.Sprintf("%012x", frame[0:8]),
		Srcip:   net.IP(frame[8:24]).String(),
		Popid:   OurPopID,
		Poptime: int64(binary.LittleEndian.Uint64(frame[24:32])),
		Brtime:  time.Now().UnixNano(),
		Rssi:    int(frame[33]),
		Lqi:     int(frame[34]),
		Payload: frame[35:],
	}
	return &fs, true
}

type egressmessage struct {
	Srcmac  string `msgpack:"srcmac"`
	Srcip   string `msgpack:"srcip"`
	Popid   string `msgpack:"popid"`
	Poptime int64  `msgpack:"poptime"`
	Brtime  int64  `msgpack:"brtime"`
	Rssi    int    `msgpack:"rssi"`
	Lqi     int    `msgpack:"lqi"`
	Payload []byte `msgpack:"payload"`
}
