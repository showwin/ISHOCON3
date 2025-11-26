Use [Packer](https://www.packer.io/) to build AMI for contest environment.

# How to use

## Prerequisites
* [Session Manager plugin for the AWS CLI](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)

## Steps

```
$ cp _secret_vars.hcl secret_vars.hcl
# Update your values at `secret_vars.hcl`

$ task all

# You can see the AMI ID from the output
```
