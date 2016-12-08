package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/immesys/wd"
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
const BadAge = 5 * time.Minute

var WanChan chan int

var puberror uint64
var pubsucc uint64

var BRName string

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
			_ = wanstate
			msg := make([]byte, 4)
			msg[0] = HbTypePiToMcu
			msg[1] = byte(wanstate)
			msg[2] = 0x55
			msg[3] = 0xAA
			_, err := conn.Write(msg)
			if err != nil {
				fmt.Printf("got wanstate error: %v\n", err)
				os.Exit(10)
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
			go wd.RLKick(5*time.Second, "410.br."+BRName+".mcu", 30)
		} else {
			hbokay <- false
		}
	}
}

const ResetInterval = 30 * time.Second

var hasInternet bool

func checkInternet() {
	for {
		err := wd.Kick("410.br."+BRName+".wan", 30)
		if err != nil {
			hasInternet = false
		} else {
			hasInternet = true
		}
		// resp, err := http.Get("http://steelcode.com/hbr.check")
		// if err == nil {
		// 	msg := make([]byte, 5)
		// 	n, err2 := resp.Body.Read(msg)
		// 	if err2 == nil && n == 5 && string(msg) == "HBROK" {
		// 		hasInternet = true
		// 	} else {
		// 		hasInternet = false
		// 		fmt.Printf("Internet check error 2: %d %v %x\n", n, err2, msg)
		// 	}
		// 	resp.Body.Close()
		// } else {
		// 	hasInternet = false
		// 	fmt.Printf("Internet check error 1: %v\n", err)
		// }
		time.Sleep(10 * time.Second)
	}
}
func processWANStatus(bw *bw2bind.BW2Client) {
	lasterr := puberror
	lastsucc := pubsucc
	lastReset := time.Now()
	lastAdvisory := BLINKING1
	for {
		bcip, err := bw.GetBCInteractionParams()
		if err != nil {
			fmt.Printf("Could not get BCIP: %v\n", err)
			die()
		}
		if hasInternet {
			if bcip.CurrentAge > BadAge {
				lastAdvisory = BLINKING1
				WanChan <- BLINKING1
				go wd.RLFault(5*time.Second, "410.br."+BRName+".chain", fmt.Sprintf("Chain is %s old", bcip.CurrentAge))
			} else {
				go wd.RLKick(5*time.Second, "410.br."+BRName+".chain", 25)
				if puberror > lasterr {
					lastAdvisory = BLINKING2
					go wd.RLFault(5*time.Second, "410.br."+BRName+".priv", "publish errors")
					WanChan <- BLINKING2
				} else if pubsucc > lastsucc {
					go wd.RLKick(5*time.Second, "410.br."+BRName+".priv", 25)
					lastAdvisory = FULLON
					WanChan <- FULLON
				} else {
					//We do not give a new advisory until we fail or succeed once
					WanChan <- lastAdvisory
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
		time.Sleep(500 * time.Millisecond)
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
	BRGW_PubOK          uint64 `msgpack:"br_pub_ok"`
	BRGW_PubERR         uint64 `msgpack:"br_pub_err"`
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
		ls.BRGW_PubERR = puberror
		ls.BRGW_PubOK = pubsucc
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
func printStats() {
	for {
		time.Sleep(10 * time.Second)
		fmt.Printf("published %d ok, %d err\n", pubsucc, puberror)
	}
}
func main() {
	embd.InitGPIO()
	defer embd.CloseGPIO()
	embd.SetDirection(PILED, embd.Out)
	LedChan = make(chan int, 1)
	WanChan = make(chan int, 1)
	go LedAnim(LedChan)
	go checkInternet()
	go printStats()
	//TODO set the Pi Led OFF before you do
	//anything that could cause exit
	OurPopID = os.Getenv("POP_ID")
	if OurPopID == "" {
		fmt.Println("Missing $POP_ID")
		die()
	}
	BRName = strings.Replace(OurPopID, "-", "_", -1)
	BRName = strings.ToLower(OurPopID)
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
	go processStats(bw)
	processIncomingData(bw)
}

func unpack(frame []byte) (*egressmessage, bool) {
	if len(frame) < 38 {
		return nil, false
	}
	fs := egressmessage{
		//skip 0:4 - len+ cksum
		Srcmac:  fmt.Sprintf("%012x", frame[2:10]),
		Srcip:   net.IP(frame[10:26]).String(),
		Popid:   OurPopID,
		Poptime: int64(binary.LittleEndian.Uint64(frame[26:34])),
		Brtime:  time.Now().UnixNano(),
		Rssi:    int(frame[34]),
		Lqi:     int(frame[35]),
		Payload: frame[36:],
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
