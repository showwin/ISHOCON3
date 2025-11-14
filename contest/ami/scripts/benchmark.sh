#!/bin/bash
set -ex
sudo su - ishocon
. /home/ishocon/.bashrc

cd /tmp/
tar -zxvf /tmp/benchmark.tar.gz
cd /tmp/benchmark
go build -x -o /home/ishocon/benchmark *.go
