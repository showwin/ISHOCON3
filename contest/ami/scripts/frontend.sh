#!/bin/bash
set -ex

sudo su - ishocon
. /home/ishocon/.bashrc

# Build payment_app
cd /tmp/
tar -zxvf /tmp/frontend.tar.gz
cd /tmp/frontend
mkdir -p /home/ishocon/webapp/public
chmod 755 /home/ishocon/webapp/public
chmod 755 /home/ishocon/webapp
chmod 755 /home/ishocon
cp /tmp/frontend/* /home/ishocon/webapp/public
