package main

import (
	"time"

	"gopkg.in/immesys/bw2bind.v5"
)

var bwcl *bw2bind.BW2Client

func getClient() (*bw2bind.BW2Client, error) {
	if bwcl != nil {
		return bwcl, nil
	}
	bw, err := bw2bind.Connect("")
	if err != nil {
		return nil, err
	}
	bw.SetEntityFromEnvironOrExit()
	bw.OverrideAutoChainTo(true)
	var Maxage int64 = 6 * 60 * 60
	bw.SetBCInteractionParams(&bw2bind.BCIP{
		Maxage: &Maxage,
	})
	bwcl = bw
	return bw, nil
}

func clientIsBroken() {
	bwcl.Close()
	bwcl = nil
	time.Sleep(5 * time.Second)
}
