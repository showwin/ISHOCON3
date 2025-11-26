data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  # Specify one AZ to avoid differences caused by AZs
  first_az = slice(data.aws_availability_zones.available.names, 0, 1)
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "6.5.1"

  name = var.name
  cidr = var.vpc_cidr_block

  azs            = local.first_az
  public_subnets = [for k, v in local.first_az : cidrsubnet(var.vpc_cidr_block, 8, k + 48)]
}
