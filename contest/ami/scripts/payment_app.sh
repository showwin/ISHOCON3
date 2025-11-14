#!/bin/bash
set -ex

sudo su - ishocon
. /home/ishocon/.bashrc

# Build payment_app
cd /tmp/
tar -zxvf /tmp/payment_app.tar.gz
cd /tmp/payment_app
go build -x -o /home/ishocon/payment_app *.go

# Setup systemd service
sudo cp /tmp/payment_app.service /etc/systemd/system/payment_app.service
sudo systemctl daemon-reload
sudo systemctl enable payment_app
sudo systemctl start payment_app
