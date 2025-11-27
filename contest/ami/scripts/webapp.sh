#!/bin/bash
set -ex

# Move files
cp /tmp/.bashrc /home/ishocon/.bashrc
cp /tmp/env.sh /home/ishocon/env.sh
cd /home/ishocon
tar -zxvf /tmp/webapp.tar.gz
chown -R ishocon:ishocon /home/ishocon/webapp

# Load .bashrc
. /home/ishocon/.bashrc

# # Install Ruby libraries
# cd /home/ishocon/webapp/ruby
# gem install bundler -v "2.7.2"
# bundle install

# # Install Python libraries
cd /home/ishocon/webapp/python
sudo apt-get install -y default-mysql-client-core
pip install uv
uv sync

# Install Go libraries
# ls /home/ishocon
# ls /home/ishocon/webapp
# cd /home/ishocon/webapp/go
# go build -o webapp *.go

# Initialize MySQL
sudo chown -R mysql:mysql /var/lib/mysql
sudo service mysql start
sudo mysql -u root -pishocon -e 'CREATE DATABASE IF NOT EXISTS ishocon3;'
sudo mysql -u root -pishocon -e "CREATE USER IF NOT EXISTS ishocon IDENTIFIED BY 'ishocon';"
sudo mysql -u root -pishocon -e 'GRANT ALL ON *.* TO ishocon;'
sudo ISHOCON_DB_OPTIONS="" sh /home/ishocon/webapp/sql/init.sh

# Nginx
sudo nginx -t
sudo service nginx start
