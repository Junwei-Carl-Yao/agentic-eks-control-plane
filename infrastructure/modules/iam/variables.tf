variable "cluster_name" {
  description = "Cluster name — used as the prefix for role names."
  type        = string
}

variable "oidc_provider_arn" {
  description = "ARN of the IAM OIDC provider bound to the cluster. IRSA trust policies federate against this principal."
  type        = string
}

variable "oidc_provider_url" {
  description = "OIDC issuer URL from the cluster (e.g. https://oidc.eks.us-east-1.amazonaws.com/id/ABC...). Used to scope the sub condition in the IRSA trust policy."
  type        = string
}

variable "backend_namespace" {
  description = "Kubernetes namespace the backend pod runs in. Pinned in the IRSA sub condition."
  type        = string
}

variable "backend_service_account" {
  description = "Kubernetes ServiceAccount the backend pod runs as. Pinned in the IRSA sub condition."
  type        = string
}

variable "state_bucket_name" {
  description = "S3 bucket holding Terraform remote state. The IRSA role gets read-only access to it so the backend can inspect state."
  type        = string
}
