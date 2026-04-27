provider "aws" {
  region = var.region

  default_tags {
    tags = merge(
      {
        Project     = "eks-control-plane"
        Environment = var.environment
        ManagedBy   = "terraform"
      },
      var.tags,
    )
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs = slice(data.aws_availability_zones.available.names, 0, 2)
}

module "vpc" {
  source = "./modules/vpc"

  cluster_name       = var.cluster_name
  vpc_cidr           = var.vpc_cidr
  availability_zones = local.azs
}

module "iam" {
  source = "./modules/iam"

  cluster_name            = var.cluster_name
  oidc_provider_arn       = module.eks.oidc_provider_arn
  oidc_provider_url       = module.eks.oidc_provider_url
  backend_namespace       = var.backend_namespace
  backend_service_account = var.backend_service_account
  state_bucket_name       = var.state_bucket_name
}

module "eks" {
  source = "./modules/eks"

  cluster_name        = var.cluster_name
  cluster_version     = var.cluster_version
  cluster_role_arn    = module.iam.cluster_role_arn
  node_role_arn       = module.iam.node_role_arn
  private_subnet_ids  = module.vpc.private_subnet_ids
  public_subnet_ids   = module.vpc.public_subnet_ids
  node_instance_types = var.node_instance_types
  node_desired_size   = var.node_desired_size
  node_min_size       = var.node_min_size
  node_max_size       = var.node_max_size
}
