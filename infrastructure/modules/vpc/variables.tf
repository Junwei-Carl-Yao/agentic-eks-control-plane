variable "cluster_name" {
  description = "Cluster name — used for Name tags and the EKS shared-subnet tag."
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC. Must be large enough for four /20 subnets."
  type        = string
}

variable "availability_zones" {
  description = "List of exactly two AZs to spread public and private subnets across."
  type        = list(string)

  validation {
    condition     = length(var.availability_zones) == 2
    error_message = "availability_zones must contain exactly two AZs."
  }
}
