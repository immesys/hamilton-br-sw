package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/immesys/bw2/crypto"
)

var mastervkstring = "qheDAxY8lMJd6zXH-IyaNhiy_V8gixxqi90Ba1Z7x0c="

type state struct {
	Valid     bool
	Licensee  string
	KitId     string
	Mac       string
	StrEntity string
	Entity    []byte
}

func checklicense() (rv *state) {
	st := state{}
	// defer func() {
	// 	r := recover()
	// 	if r != nil {
	// 		fmt.Printf("license panic: %v\n", r)
	// 		gst := state{}
	// 		rv = &gst
	// 	}
	// }()
	mastervk, _ := crypto.UnFmtKey(mastervkstring)
	infile, err := os.Open("/config/license.lic")
	if err != nil {
		fmt.Printf("LICENSE OPEN ERROR\n")
		return &st
	}
	whole, err := ioutil.ReadFile("/config/license.lic")
	if err != nil {
		fmt.Printf("LICENSE  OPEN ERROR\n")
		return &st
	}
	rdr := bufio.NewReader(infile)
	for {
		str, err := rdr.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(str, "#") {
			continue
		}
		parts := strings.Split(str, ":")
		if parts[0] == "Kit ID" {
			st.KitId = strings.TrimSpace(parts[1])
		}
		if parts[0] == "Licensee" {
			st.Licensee = strings.TrimSpace(parts[1])
		}
		if parts[0] == "MAC Addr" {
			st.Mac = strings.TrimSpace(parts[1])
		}
		if parts[0] == "License key" {
			st.StrEntity = strings.TrimSpace(parts[1])
			blob, err := base64.StdEncoding.DecodeString(st.StrEntity)
			if err != nil {
				fmt.Printf("LICENSE BAD ENTITY ENCODING\n")
				return &st
			}
			st.Entity = blob
		}
	}
	sigstart := bytes.LastIndexByte(whole, ':')
	sig := bytes.TrimSpace(whole[sigstart+1:])
	sigbin, err := base64.URLEncoding.DecodeString(string(sig))
	if err != nil {
		fmt.Printf("# sig decode error\n")
		return &st
	}
	body := whole[:sigstart+1]
	siggood := crypto.VerifyBlob(mastervk, sigbin, body)
	st.Valid = siggood
	return &st
}
