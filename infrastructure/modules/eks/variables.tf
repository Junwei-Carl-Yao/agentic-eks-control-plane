variable "cluster_name" {
  description = "Name of the EKS cluster."
  type        = string
}

variable "cluster_version" {
  description = "Kubernetes version for the EKS control plane."
  type        = string
}

variable "cluster_role_arn" {
  description = "ARN of the IAM role the EKS control plane assumes."
  type        = string
}

variable "node_role_arn" {
  description = "ARN of the IAM role attached to managed node group instances."
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnet IDs where managed node group instances are placed."
  type        = list(string)
}

variable "public_subnet_ids" {
  description = "Public subnet IDs. Attached to the cluster so internet-facing ingress is possible; nodes are not placed here."
  type        = list(string)
}

variable "node_instance_types" {
  description = "EC2 instance types for the managed node group."
  type        = list(string)
}

variable "node_desired_size" {
  description = "Desired number of nodes in the managed node group."
  type        = number
}

variable "node_min_size" {
  description = "Minimum number of nodes in the managed node group."
  type        = number
}

variable "node_max_size" {
  description = "Maximum number of nodes in the managed node group."
  type        = number
}
