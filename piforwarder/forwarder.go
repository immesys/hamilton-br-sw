package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

var totaltx int

var forwardedUplink uint64
var forwardedDownlink uint64

func die() {
	os.Exit(1)
}

func ProcessUplink(rawsocket int, rethos *net.UnixConn) {
	rawdestaddr := syscall.SockaddrInet6{}
	for {
		buf := make([]byte, 16*1024)
		num, _, err := rethos.ReadFromUnix(buf)
		if err != nil {
			fmt.Printf("data socket: read: error: %v\n", err)
			die()
		}
		packet := buf[:num]

		/* Must contain at least a full IPv6 header */
		if len(packet) < 40 {
			continue
		}

		dstip := packet[24:40]
		copy(rawdestaddr.Addr[:], dstip)

		err = syscall.Sendto(rawsocket, packet, 0, &rawdestaddr)
		if err != nil {
			fmt.Printf("raw socket: sendto: error: %v\n", err)
		}
		forwardedUplink++
	}
}

// Prefix is 2001:470:4889:115/64
var RouterPrefix = []byte{0x20, 0x01, 0x04, 0x70, 0x48, 0x89, 0x01, 0x15}

func ProcessDownlink(rawsocket int, rethos *net.UnixConn) {
	packetbuffer := make([]byte, 4096)
	for {
		n, _, err := syscall.Recvfrom(rawsocket, packetbuffer, 0)
		if err != nil {
			fmt.Printf("raw socket: recvfom: error: %v\n", err)
		}

		/* Need to remove ethernet header */
		packet := packetbuffer[:n]

		/* Can't be a valid IPv6 packet if its length is shorter than an IPv6
		 * header.
		 */
		if len(packet) < 40 {
			continue
		}

		dstIPaddr := packet[24:40]
		if bytes.Equal(dstIPaddr[:len(RouterPrefix)], RouterPrefix) {
			_, err := rethos.Write(packet)
			if err != nil {
				fmt.Printf("data socket: write: error: %v\n", err)
			}
			forwardedDownlink++
		}
	}
}

func printStats() {
	for {
		time.Sleep(10 * time.Second)
		fmt.Printf("forwarded %d packets uplink, %d packets downlink\n", forwardedUplink, forwardedDownlink)
	}
}

func htons(x uint16) uint16 {
	return ((x & 0x00FF) << 8) | ((x & 0xFF00) >> 8)
}

func main() {
	go printStats()

	rethosconn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: "@rethos/7", Net: "unixpacket"})
	if err != nil {
		fmt.Printf("heartbeat socket: error: %v\n", err)
		die()
	}

	rawfd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		fmt.Printf("raw socket: error: %v\n", err)
		die()
	}

	/* Raw sockets aren't enough for receiving packets, since that ties me to
	 * only one IP protocol, whereas I want to forward all IP packets.
	 */
	packetfd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_DGRAM, int(htons(uint16(syscall.ETH_P_IPV6))))
	if err != nil {
		fmt.Printf("packet socket: error: %v\n", err)
		die()
	}

	go ProcessDownlink(packetfd, rethosconn)
	ProcessUplink(rawfd, rethosconn)
}
