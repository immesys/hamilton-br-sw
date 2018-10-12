package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"periph.io/x/periph/host"
	"periph.io/x/periph/host/rpi"

	"github.com/golang/protobuf/proto"
	"github.com/immesys/hamilton-br-sw/brgw2/pb"
	"github.com/immesys/wavemq/mqpb"
	"github.com/immesys/wd"
)

var OurPopID string
var BaseURI string
var totaltx int

const HeartbeatTimeout = 2 * time.Second
const BlinkInterval = 200 * time.Millisecond
const HbTypeMcuToPi = 1
const HbTypePiToMcu = 2

var PILED = rpi.P1_22

var LedChan chan int

const FULLOFF = 1
const FULLON = 2
const BLINKING1 = 3
const BLINKING2 = 4
const BLINKING3 = 5
const BadAge = 2 * time.Hour
const MaxBadAgeTrigger = 1 * time.Hour

var WanChan chan int

var puberror uint64
var pubsucc uint64

var BRName string

var client mqpb.WAVEMQClient
var perspective *mqpb.Perspective
var namespace []byte

func writeMessage(conn net.Conn, message []byte) error {
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr[:], uint32(len(message)))
	_, err := conn.Write(hdr) //binary.Write(conn, binary.BigEndian, len(message))
	if err != nil {
		return err
	}
	_, err = conn.Write(message)
	return err
}

func readMessage(conn net.Conn) ([]byte, error) {
	hdr := make([]byte, 4)
	_, err := io.ReadFull(conn, hdr)
	if err != nil {
		return nil, err
	}
	msgsize := binary.BigEndian.Uint32(hdr[0:])
	buf := make([]byte, msgsize, msgsize)
	_, err = io.ReadFull(conn, buf)
	return buf, err
}

func die() {
	os.Exit(1)
}
func processIncomingHeartbeats() {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: "@rethos/4", Net: "unix"})
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
			err := writeMessage(conn, msg)
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
		buf, err := readMessage(conn)
		if err != nil {
			fmt.Printf("heartbeat socket: error: %v\n", err)
			die()
		}
		num := len(buf)
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
		time.Sleep(10 * time.Second)
	}
}

var timeEnteredBadness time.Time

func processWANStatus() {
	lasterr := puberror
	lastsucc := pubsucc
	lastReset := time.Now()
	lastAdvisory := BLINKING1
	for {
		resp, err := client.ConnectionStatus(context.Background(), &mqpb.ConnectionStatusParams{})
		if err != nil {
			fmt.Printf("got WAVEMQ error: %v\n", err)
			die()
		}
		hasInternet := false
		if resp.ConnectedPeers == resp.TotalPeers {
			hasInternet = true
		}
		if hasInternet {
			go wd.RLKick(5*time.Second, "410.br."+BRName+".peered", 25)
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
// type LinkStats struct {
// 	BadFrames           uint64 `msgpack:"bad_frames"`
// 	LostFrames          uint64 `msgpack:"lost_frames"`
// 	DropNotConnected    uint64 `msgpack:"drop_not_connected"`
// 	SumSerialReceived   uint64 `msgpack:"sum_serial_received"`
// 	SumDomainForwarded  uint64 `msgpack:"sum_domain_forwarded"`
// 	SumDropNotConnected uint64 `msgpack:"drop_not_connected"`
// 	SumDomainReceived   uint64 `msgpack:"sum_domain_received"`
// 	SumSerialForwarded  uint64 `msgpack:"sum_serial_forwarded"`
// 	BRGW_PubOK          uint64 `msgpack:"br_pub_ok"`
// 	BRGW_PubERR         uint64 `msgpack:"br_pub_err"`
// }

func processStats() {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: "@rethos/0", Net: "unix"})
	if err != nil {
		fmt.Printf("heartbeat socket: error: %v\n", err)
		die()
	}
	for {
		buf, err := readMessage(conn)
		if err != nil {
			fmt.Printf("Unix socket error: %v\n", err)
			os.Exit(1)
		}
		num := len(buf)
		if num < 10256 {
			fmt.Printf("Abort malformed stats frame, length %d\n", num)
			os.Exit(1)
		}
		ls := pb.HamiltonBRLinkStats{}
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
		ls.PublishError = puberror
		ls.PublishOkay = pubsucc

		ser, err := proto.Marshal(&pb.XBOS{HamiltonBRLinkStats: &ls})
		if err != nil {
			panic(err)
		}

		resp, err := client.Publish(context.Background(), &mqpb.PublishParams{
			Perspective: perspective,
			Namespace:   namespace,
			Uri:         fmt.Sprintf("%s/%s/s.hamilton/_/i.l7g/signal/stats", BaseURI, OurPopID),
			Content: []*mqpb.PayloadObject{&mqpb.PayloadObject{
				Schema:  "xbosproto/XBOS",
				Content: ser,
			}},
		})
		if err != nil {
			panic(err)
		}
		if resp.Error != nil {
			atomic.AddUint64(&puberror, 1)
			fmt.Printf("WAVEMQ publish failure: %v\n", resp.Error.Message)
		} else {
			atomic.AddUint64(&pubsucc, 1)
		}
	}
}

func processIncomingData() {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: "@rethos/5", Net: "unix"})
	if err != nil {
		fmt.Printf("heartbeat socket: error: %v\n", err)
		die()
	}
	for {
		buf, err := readMessage(conn)
		if err != nil {
			fmt.Printf("data socket: error: %v\n", err)
			die()
		}
		num := len(buf)
		frame, ok := unpack(buf[:num])
		if !ok {
			fmt.Println("bad frame")
			continue
		}

		ser, err := proto.Marshal(&pb.XBOS{HamiltonBRMessage: frame})
		if err != nil {
			panic(err)
		}

		resp, err := client.Publish(context.Background(), &mqpb.PublishParams{
			Perspective: perspective,
			Namespace:   namespace,
			Uri:         fmt.Sprintf("%s/%s/s.hamilton/%s/i.l7g/signal/raw", BaseURI, OurPopID, frame.SrcMAC),
			Content: []*mqpb.PayloadObject{&mqpb.PayloadObject{
				Schema:  "xbosproto/XBOS",
				Content: ser,
			}},
		})
		totaltx++
		if err != nil {
			panic(err)
		}
		if resp.Error != nil {
			atomic.AddUint64(&puberror, 1)
			fmt.Printf("WAVEMQ publish failure: %v\n", resp.Error.Message)
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
				PILED.Out(false)
			}
			if state == FULLON {
				PILED.Out(true)
			}
		}
	}()
	for {
		<-time.After(BlinkInterval)
		if state == FULLOFF {
			PILED.Out(false)
		}
		if state == FULLON {
			PILED.Out(true)
		}
		if state == BLINKING1 {
			if lastval {
				PILED.Out(false)
			} else {
				PILED.Out(true)
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
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	Entity := os.Getenv("ENTITY")
	if Entity == "" {
		fmt.Println("Missing $ENTITY")
		die()
	}
	contents, err := ioutil.ReadFile(Entity)
	if err != nil {
		panic(err)
	}
	perspective = &mqpb.Perspective{
		EntitySecret: &mqpb.EntitySecret{
			DER: contents,
		},
	}
	NamespaceStr := os.Getenv("NAMESPACE")
	if NamespaceStr == "" {
		fmt.Println("Missing $NAMESPACE")
		die()
	}
	namespace, err = base64.URLEncoding.DecodeString(NamespaceStr)
	if err != nil {
		fmt.Printf("Bad namespace: %v\n", err)
		die()
	}

	fmt.Printf("doing dial\n")
	conn, err := grpc.Dial("corbusier.cs.berkeley.edu:4516", grpc.WithInsecure(), grpc.FailOnNonTempDialError(true), grpc.WithBlock())
	if err != nil {
		fmt.Printf("could not connect to the site router: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("dial done\n")

	// Create the WAVEMQ client
	client = mqpb.NewWAVEMQClient(conn)

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
	if strings.HasSuffix(BaseURI, "/") {
		BaseURI = BaseURI[:len(BaseURI)-1]
	}

	go processIncomingHeartbeats()
	go processWANStatus()
	go processStats()
	processIncomingData()
}

func unpack(frame []byte) (*pb.HamiltonBRMessage, bool) {
	if len(frame) < 38 {
		return nil, false
	}
	fs := pb.HamiltonBRMessage{
		//skip 0:4 - len+ cksum
		SrcMAC:  fmt.Sprintf("%012x", frame[2:10]),
		SrcIP:   net.IP(frame[10:26]).String(),
		PopID:   OurPopID,
		PopTime: int64(binary.LittleEndian.Uint64(frame[26:34])),
		BRTime:  time.Now().UnixNano(),
		RSSI:    int32(frame[34]),
		LQI:     int32(frame[35]),
		Payload: frame[36:],
	}
	return &fs, true
}
