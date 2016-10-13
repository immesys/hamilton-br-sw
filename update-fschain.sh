#!/bin/bash

sudo systemctl stop bw2
cd rel/fschain
export EXIT_ON_FAST_COMPLETE=Y
bw2 router
rm -rf bw2bc
rsync -PHav .bw.db/bw2bc .
rm bw2bc/nodekey
rm -rf bw2bc/dapp
rm -rf bw2bc/keys
