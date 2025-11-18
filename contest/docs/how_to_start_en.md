# Create EC2 instance

Create a new EC2 instance with the AMI, `ami-04a9664e60b1a9922`.
SG needs to have 22 and 80 port open, and register your SSH key pair.

# Connect to EC2 instance

```
$ ssh -i <your-key-pair.pem> ubuntu@<your-ec2-public-ip>
$ sudo su - ishocon
$ cd
```

# Run WebApp

Start Python app

```
$ cd ~/webapp/python
$ uv run gunicorn -c gunicorn.conf.py --bind "0.0.0.0:8080"
```

Start Ruby app

```
# TODO
```

# Run benchmark

```
$ cd ~/
$ ./benchmark
```

For more detailed log output, run:

```
$ ./benchmark --log-level info
```
