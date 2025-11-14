Use [Packer](https://www.packer.io/) to build AMI for contest environment.

# How to use

## Prerequisites
* [Session Manager plugin for the AWS CLI](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)

## Steps

```
$ cp _secret_vars.hcl secret_vars.hcl
# Update your values at `secret_vars.hcl`

$ cd ../../ && tar -zcvf contest/ami/webapp.tar.gz ./webapp && cd contest/ami
$ cd ../../ && tar -zcvf contest/ami/frontend.tar.gz ./frontend && cd contest/ami
$ cd ../../ && tar -zcvf contest/ami/benchmark.tar.gz ./benchmark && cd contest/ami
$ cd ../../ && tar -zcvf contest/ami/payment_app.tar.gz ./payment_app && cd contest/ami

$ packer init .
$ packer validate -var-file=shared_vars.hcl -var-file=secret_vars.hcl .

# This will take around 15 minutes to run
$ packer build -var-file=shared_vars.hcl -var-file=secret_vars.hcl ami.pkr.hcl

# You can see the AMI ID from the output
```
