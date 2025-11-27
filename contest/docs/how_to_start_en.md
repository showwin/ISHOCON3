# Create EC2 instance

Create a new EC2 instance with the AMI, `ami-04a9664e60b1a9922` with `c7i.xlarge`.
SG needs to have 22 and 80 port open, and register your SSH key pair.

# Connect to EC2 instance

```
$ ssh -i <your-GH-registered-key.pem> ishocon@<your-ec2-public-ip>
```
