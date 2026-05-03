variable "region" {
  description = "AWS region for every resource in the root module."
  type        = string
}

variable "environment" {
  description = "Environment name (dev, staging, prod). Used in naming and tags."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,15}$", var.environment))
    error_message = "environment must be 2-16 chars, lowercase alphanumerics or hyphen, starting with a letter."
  }
}

variable "cluster_name" {
  description = "Name of the EKS cluster. Also used as a tag/name prefix for supporting resources."
  type        = string

  validation {
    condition     = can(regex("^[a-zA-Z][a-zA-Z0-9-]{0,99}$", var.cluster_name))
    error_message = "cluster_name must start with a letter and contain only letters, digits, or hyphens (max 100)."
  }
}

variable "cluster_version" {
  description = "Kubernetes version for the EKS control plane."
  type        = string
  default     = "1.35"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC. Must be a /16 or larger to fit the four /20 subnets."
  type        = string
  default     = "10.0.0.0/16"

  validation {
    condition     = can(cidrnetmask(var.vpc_cidr))
    error_message = "vpc_cidr must be a valid IPv4 CIDR block."
  }
}

variable "node_instance_types" {
  description = "EC2 instance types for the managed node group."
  type        = list(string)
  default     = ["t3.medium"]
}

variable "node_desired_size" {
  description = "Desired number of nodes in the managed node group."
  type        = number
  default     = 2
}

variable "node_min_size" {
  description = "Minimum number of nodes in the managed node group."
  type        = number
  default     = 1
}

variable "node_max_size" {
  description = "Maximum number of nodes in the managed node group."
  type        = number
  default     = 4
}

variable "state_bucket_name" {
  description = "Name of the S3 bucket holding Terraform remote state. Created by scripts/bootstrap.sh, referenced here so the IRSA policy can grant read-only access to it."
  type        = string
}

variable "backend_namespace" {
  description = "Kubernetes namespace the backend pod runs in. Used to pin the IRSA trust policy."
  type        = string
  default     = "control-plane"
}

variable "backend_service_account" {
  description = "Kubernetes ServiceAccount the backend pod runs as. Used to pin the IRSA trust policy."
  type        = string
  default     = "backend"
}

variable "tags" {
  description = "Extra tags applied to every AWS resource in addition to the module defaults."
  type        = map(string)
  default     = {}
}
